package services

import (
	"context"
	"database/sql"
	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type WorkloadReconciliation struct {
	db        storage.Pool
	queue     storage.InsertQueue
	caddy     CaddyRouteService
	container containerclient.WorkloadClient
}

func NewWorkloadReconciliation(db storage.Pool, queue storage.InsertQueue, caddy CaddyRouteService) *WorkloadReconciliation {
	return &WorkloadReconciliation{db: db, queue: queue, caddy: caddy, container: containerclient.NewWorkload()}
}

func (service *WorkloadReconciliation) Reconcile(ctx context.Context) error {
	var unresolved []models.DeploymentEntity
	if err := service.db.Executor().NewSelect().Model(&unresolved).
		Join("JOIN releases AS release ON release.id = deployments.release_id AND release.build_id IS NOT NULL").
		Where("deployments.status IN ('queued', 'running')").OrderExpr("deployments.created_at").Scan(ctx); err != nil {
		return fmt.Errorf("load unresolved workload Deployments: %w", err)
	}
	for _, deployment := range unresolved {
		var instance models.InstanceEntity
		err := service.db.Executor().NewSelect().Model(&instance).Where("deployment_id = ?", deployment.ID).Where("removed_at IS NULL").Limit(1).Scan(ctx)
		if err != nil {
			slog.Error("workload startup reconciliation could not load candidate", "deployment_id", deployment.ID, "error", err)
			continue
		}
		state, inspectErr := service.container.Find(ctx, deployment.ID, instance.ID)
		if inspectErr != nil {
			slog.Error("workload startup reconciliation could not inspect candidate", "deployment_id", deployment.ID, "error", inspectErr)
			continue
		}
		if state.Exists {
			encodedPorts, _ := json.Marshal(map[string]int32{"http": state.HostPort})
			ports := json.RawMessage(encodedPorts)
			now := time.Now().UTC()
			_, _ = service.db.Executor().NewUpdate().TableExpr("instances").Set("external_id = ?", state.ID).
				Set("state = ?", map[bool]string{true: "running", false: "failed"}[state.Running]).Set("ports = ?", ports).
				Set("observed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", instance.ID).Exec(ctx)
		}
		if _, insertErr := service.queue.Insert(ctx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); insertErr != nil {
			slog.Error("workload startup reconciliation could not requeue Deployment", "deployment_id", deployment.ID, "error", insertErr)
		}
	}
	if err := service.reconcileServingInstances(ctx); err != nil {
		return err
	}

	var routes []models.CaddyRouteEntity
	if err := service.db.Executor().NewSelect().Model(&routes).
		Join("JOIN releases AS release ON release.id = caddy_routes.release_id AND release.build_id IS NOT NULL").
		Where("caddy_routes.removed_at IS NULL").Where("caddy_routes.state IN ('pending', 'applied')").Scan(ctx); err != nil {
		return fmt.Errorf("load active workload Caddy routes: %w", err)
	}
	for _, route := range routes {
		if _, err := service.caddy.Reconcile(ctx, route.ID); err != nil {
			slog.Error("workload startup reconciliation could not apply Caddy route", "route_id", route.ID, "error", err)
		}
	}
	if err := service.cleanupOldBackends(ctx); err != nil {
		return err
	}
	return service.cleanupUnroutedInstances(ctx)
}

func (service *WorkloadReconciliation) reconcileServingInstances(ctx context.Context) error {
	var instances []models.InstanceEntity
	if err := service.db.Executor().NewSelect().Model(&instances).
		Join("JOIN deployments AS deployment ON deployment.id = instances.deployment_id AND deployment.status = 'succeeded'").
		Join("JOIN releases AS release ON release.id = instances.release_id AND release.build_id IS NOT NULL").
		Where("EXISTS (SELECT 1 FROM caddy_routes route JOIN caddy_route_backends backend ON backend.caddy_route_id = route.id AND backend.instance_id = instances.id AND backend.removed_at IS NULL AND backend.weight = 100 WHERE route.environment_target_id = instances.environment_target_id AND route.release_id = instances.release_id AND route.removed_at IS NULL)").
		Where("instances.state = 'serving'").Where("instances.removed_at IS NULL").OrderExpr("instances.created_at").Scan(ctx); err != nil {
		return err
	}
	for _, instance := range instances {
		state, err := service.container.Find(ctx, instance.DeploymentID, instance.ID)
		if err != nil {
			slog.Error("workload startup reconciliation could not inspect serving Instance", "instance_id", instance.ID, "error", err)
			continue
		}
		if state.Exists && !state.Running {
			state, err = service.container.Start(ctx, instance.DeploymentID, instance.ID)
		}
		if err != nil {
			slog.Error("workload startup reconciliation could not restart serving Instance", "instance_id", instance.ID, "error", err)
			continue
		}
		if state.Exists {
			encodedPorts, _ := json.Marshal(map[string]int32{"http": state.HostPort})
			ports := json.RawMessage(encodedPorts)
			now := time.Now().UTC()
			_, _ = service.db.Executor().NewUpdate().TableExpr("instances").Set("external_id = ?", state.ID).
				Set("ports = ?", ports).Set("observed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", instance.ID).Exec(ctx)
			continue
		}
		if err := service.queueMissingServingRecovery(ctx, instance); err != nil {
			slog.Error("workload startup reconciliation could not queue missing Instance recovery", "instance_id", instance.ID, "error", err)
		}
	}
	return nil
}

func (service *WorkloadReconciliation) queueMissingServingRecovery(ctx context.Context, instance models.InstanceEntity) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	release, err := models.Release.Find(ctx, tx, instance.ReleaseID)
	if err != nil {
		return err
	}
	if _, err := models.Environment.Lock(ctx, tx, release.EnvironmentID); err != nil {
		return err
	}
	unresolved, err := tx.NewSelect().Model((*models.DeploymentEntity)(nil)).Where("environment_target_id = ?", instance.EnvironmentTargetID).
		Where("status IN ('queued', 'running')").Count(ctx)
	if err != nil || unresolved != 0 {
		return err
	}
	previous, err := models.Deployment.Find(ctx, tx, instance.DeploymentID)
	if err != nil {
		return err
	}
	var revision models.EnvironmentStateRevisionEntity
	if err := tx.NewSelect().Model(&revision).
		Join("JOIN change_state_revisions AS association ON association.environment_state_revision_id = environment_state_revisions.id").
		Where("association.change_id = ?", previous.ChangeID).Where("association.role = 'result'").Limit(1).Scan(ctx); err != nil {
		return err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, release.EnvironmentID)
	if err != nil {
		return err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "deployment_recovery", TriggerType: "system", ActorType: "system",
		CauseSystem: sql.NullString{String: "startup_reconciliation", Valid: true}, CauseReference: sql.NullString{String: instance.ID.String(), Valid: true},
		CorrelationID: uuid.New(), CorrectionContext: json.RawMessage(`{}`), Summary: "Recover missing serving workload",
		Status: "committed", RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: release.EnvironmentID,
	})
	if err != nil {
		return err
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID}); err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return err
	}
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: previous.Attempt + 1, Strategy: previous.Strategy, RuntimeConfiguration: previous.RuntimeConfiguration,
		Status: "queued", CurrentStep: sql.NullString{String: "queued", Valid: true}, ChangeID: change.ID,
		ReleaseID: release.ID, EnvironmentTargetID: instance.EnvironmentTargetID,
	})
	if err != nil {
		return err
	}
	if _, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID: "pending:" + deployment.ID.String(), Slot: "candidate", ReplicaKey: "primary", State: "candidate",
		Ports: json.RawMessage(`{}`), ObservedAt: now, DeploymentID: deployment.ID, ReleaseID: release.ID, EnvironmentTargetID: instance.EnvironmentTargetID,
	}); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("instances").Set("state = 'failed'").Set("updated_at = ?", now).Where("id = ?", instance.ID).Exec(ctx); err != nil {
		return err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *WorkloadReconciliation) cleanupOldBackends(ctx context.Context) error {
	var rows []struct {
		RouteID      uuid.UUID `bun:"route_id"`
		InstanceID   uuid.UUID `bun:"instance_id"`
		DeploymentID uuid.UUID `bun:"deployment_id"`
	}
	err := service.db.Executor().NewSelect().TableExpr("caddy_route_backends AS backend").
		ColumnExpr("backend.caddy_route_id AS route_id, instance.id AS instance_id, instance.deployment_id AS deployment_id").
		Join("JOIN caddy_routes AS route ON route.id = backend.caddy_route_id AND route.removed_at IS NULL").
		Join("JOIN releases AS release ON release.id = route.release_id AND release.build_id IS NOT NULL").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id AND deployment.status NOT IN ('queued', 'running')").
		Where("backend.removed_at IS NULL").Where("backend.weight = 0").Scan(ctx, &rows)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := service.container.Remove(ctx, row.DeploymentID, row.InstanceID); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			continue
		}
		if err := service.caddy.RemoveBackend(ctx, row.RouteID, row.InstanceID); err != nil {
			continue
		}
		now := time.Now().UTC()
		_, _ = service.db.Executor().NewUpdate().TableExpr("instances").Set("state = 'removed'").Set("removed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", row.InstanceID).Exec(ctx)
	}
	return nil
}

func (service *WorkloadReconciliation) cleanupUnroutedInstances(ctx context.Context) error {
	return cleanupUnroutedWorkloadInstances(ctx, service.db, service.container, uuid.Nil)
}

func cleanupUnroutedWorkloadInstances(
	ctx context.Context,
	db storage.Pool,
	container containerclient.WorkloadClient,
	environmentTargetID uuid.UUID,
) error {
	var rows []struct {
		InstanceID   uuid.UUID `bun:"instance_id"`
		DeploymentID uuid.UUID `bun:"deployment_id"`
	}
	query := db.Executor().NewSelect().TableExpr("instances AS instance").
		ColumnExpr("instance.id AS instance_id, instance.deployment_id AS deployment_id").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id AND deployment.status NOT IN ('queued', 'running')").
		Join("JOIN releases AS release ON release.id = instance.release_id AND release.build_id IS NOT NULL").
		Where("instance.removed_at IS NULL").
		Where(`NOT EXISTS (
			SELECT 1 FROM caddy_route_backends AS own_backend
			JOIN caddy_routes AS own_route ON own_route.id = own_backend.caddy_route_id AND own_route.removed_at IS NULL
			WHERE own_backend.instance_id = instance.id AND own_backend.removed_at IS NULL
		)`).
		Where(`(deployment.status = 'failed' OR EXISTS (
			SELECT 1 FROM caddy_routes AS active_route
			JOIN caddy_route_backends AS active_backend ON active_backend.caddy_route_id = active_route.id
			JOIN instances AS active_instance ON active_instance.id = active_backend.instance_id AND active_instance.removed_at IS NULL
			WHERE active_route.environment_target_id = instance.environment_target_id AND active_route.removed_at IS NULL
			AND active_backend.removed_at IS NULL AND active_backend.weight = 100 AND active_instance.id <> instance.id
		))`)
	if environmentTargetID != uuid.Nil {
		query = query.Where("instance.environment_target_id = ?", environmentTargetID)
	}
	err := query.Scan(ctx, &rows)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := container.Remove(ctx, row.DeploymentID, row.InstanceID); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			continue
		}
		now := time.Now().UTC()
		_, _ = db.Executor().NewUpdate().TableExpr("instances").Set("state = 'removed'").Set("removed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", row.InstanceID).Exec(ctx)
	}
	return nil
}
