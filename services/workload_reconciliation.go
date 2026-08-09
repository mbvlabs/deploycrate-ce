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
	workloads *WorkloadExecution
	releases  *ReleaseDeployment
}

func NewWorkloadReconciliation(
	db storage.Pool,
	queue storage.InsertQueue,
	caddy CaddyRouteService,
	workloads *WorkloadExecution,
	releases *ReleaseDeployment,
) *WorkloadReconciliation {
	return &WorkloadReconciliation{
		db:        db,
		queue:     queue,
		caddy:     caddy,
		workloads: workloads,
		releases:  releases,
	}
}

func (service *WorkloadReconciliation) Reconcile(ctx context.Context) error {
	if err := service.reconcileReleaseCommands(ctx); err != nil {
		return err
	}
	unresolved, err := models.Deployment.UnresolvedWorkloads(ctx, service.db.Executor())
	if err != nil {
		return fmt.Errorf("load unresolved workload Deployments: %w", err)
	}
	for _, deployment := range unresolved {
		instances, err := models.Instance.ActiveForDeployment(
			ctx, service.db.Executor(), deployment.ID,
		)
		if err != nil {
			slog.Error(
				"workload startup reconciliation could not load candidate",
				"deployment_id",
				deployment.ID,
				"error",
				err,
			)
			continue
		}
		if len(instances) == 0 {
			continue
		}
		target, targetErr := models.EnvironmentTarget.Find(
			ctx,
			service.db.Executor(),
			instances[0].EnvironmentTargetID,
		)
		if targetErr != nil {
			slog.Error(
				"workload startup reconciliation could not load target",
				"deployment_id",
				deployment.ID,
				"error",
				targetErr,
			)
			continue
		}
		for _, instance := range instances {
			state, inspectErr := service.workloads.Find(
				ctx,
				target.ServerID,
				deployment.ID,
				instance.ID,
			)
			if inspectErr != nil {
				slog.Error(
					"workload startup reconciliation could not inspect candidate",
					"deployment_id",
					deployment.ID,
					"instance_id",
					instance.ID,
					"error",
					inspectErr,
				)
				continue
			}
			if state.Exists {
				ports := json.RawMessage(`{}`)
				if instance.ProcessKind == models.EnvironmentProcessWeb {
					encodedPorts, _ := json.Marshal(
						map[string]any{"host": state.HostAddress, "http": state.HostPort},
					)
					ports = encodedPorts
				}
				now := time.Now().UTC()
				observedState := map[bool]string{true: "running", false: "failed"}[state.Running]
				_ = models.Instance.ObserveRuntime(
					ctx, service.db.Executor(), instance.ID, state.ID, &observedState, ports, now,
				)
			}
		}
		if _, insertErr := service.queue.Insert(
			ctx,
			jobs.DeployReleaseArgs{DeploymentID: deployment.ID},
			jobs.DeployReleaseInsertOpts(deployment.ID),
		); insertErr != nil {
			slog.Error(
				"workload startup reconciliation could not requeue Deployment",
				"deployment_id",
				deployment.ID,
				"error",
				insertErr,
			)
		}
	}
	if err := service.reconcileServingInstances(ctx); err != nil {
		return err
	}

	routes, err := models.CaddyRoute.ActiveWorkloadRoutes(ctx, service.db.Executor())
	if err != nil {
		return fmt.Errorf("load active workload Caddy routes: %w", err)
	}
	for _, route := range routes {
		if _, err := service.caddy.Reconcile(ctx, route.ID); err != nil {
			slog.Error(
				"workload startup reconciliation could not apply Caddy route",
				"route_id",
				route.ID,
				"error",
				err,
			)
		}
	}
	if err := service.cleanupOldBackends(ctx); err != nil {
		return err
	}
	return service.cleanupUnroutedInstances(ctx)
}

func (service *WorkloadReconciliation) reconcileReleaseCommands(ctx context.Context) error {
	executions, err := models.ReleaseCommandExecution.Running(ctx, service.db.Executor())
	if err != nil {
		return fmt.Errorf("load interrupted release commands: %w", err)
	}
	for _, execution := range executions {
		release, err := models.Release.Find(ctx, service.db.Executor(), execution.ReleaseID)
		if err != nil {
			continue
		}
		target, err := models.EnvironmentTarget.Find(
			ctx,
			service.db.Executor(),
			execution.EnvironmentTargetID,
		)
		if err != nil {
			continue
		}
		states, inspectErr := service.workloads.FindEnvironment(
			ctx,
			target.ServerID,
			release.EnvironmentID,
		)
		if inspectErr != nil {
			slog.Error(
				"workload startup reconciliation could not inspect release command",
				"execution_id",
				execution.ID,
				"error",
				inspectErr,
			)
			continue
		}
		containerFound := false
		for _, state := range states {
			if state.Labels[containerclient.WorkloadLabelReleaseCommand] == execution.ID.String() {
				containerFound = true
				break
			}
		}
		message := "release command was interrupted before its outcome was recorded"
		if !containerFound && !execution.ExternalID.Valid {
			message = "release command worker was interrupted before container creation was recorded"
		}
		operationErr := errors.New(message)
		status := "ambiguous"
		if !containerFound && !execution.ExternalID.Valid {
			status = "failed"
		}
		now := time.Now().UTC()
		if err := models.ReleaseCommandExecution.MarkFailed(
			ctx,
			service.db.Executor(),
			execution.ID,
			status,
			nil,
			operationErr,
			now,
		); err != nil {
			return err
		}
		_ = models.Change.MarkFailed(
			ctx,
			service.db.Executor(),
			execution.ChangeID,
			operationErr,
			now,
		)
	}
	return nil
}

func (service *WorkloadReconciliation) reconcileServingInstances(ctx context.Context) error {
	instances, err := models.Instance.ServingForReconciliation(ctx, service.db.Executor())
	if err != nil {
		return err
	}
	for _, instance := range instances {
		target, err := models.EnvironmentTarget.Find(
			ctx,
			service.db.Executor(),
			instance.EnvironmentTargetID,
		)
		if err != nil {
			slog.Error(
				"workload startup reconciliation could not load serving target",
				"instance_id",
				instance.ID,
				"error",
				err,
			)
			continue
		}
		state, err := service.workloads.Find(
			ctx,
			target.ServerID,
			instance.DeploymentID,
			instance.ID,
		)
		if err != nil {
			slog.Error(
				"workload startup reconciliation could not inspect serving Instance",
				"instance_id",
				instance.ID,
				"error",
				err,
			)
			continue
		}
		if state.Exists && !state.Running {
			state, err = service.workloads.Start(
				ctx,
				target.ServerID,
				instance.DeploymentID,
				instance.ID,
			)
		}
		if err != nil {
			slog.Error(
				"workload startup reconciliation could not restart serving Instance",
				"instance_id",
				instance.ID,
				"error",
				err,
			)
			continue
		}
		if state.Exists {
			ports := json.RawMessage(`{}`)
			if instance.ProcessKind == models.EnvironmentProcessWeb {
				encodedPorts, _ := json.Marshal(
					map[string]any{"host": state.HostAddress, "http": state.HostPort},
				)
				ports = encodedPorts
			}
			now := time.Now().UTC()
			_ = models.Instance.ObserveRuntime(
				ctx, service.db.Executor(), instance.ID, state.ID, nil, ports, now,
			)
			continue
		}
		if err := service.queueMissingServingRecovery(ctx, instance); err != nil {
			slog.Error(
				"workload startup reconciliation could not queue missing Instance recovery",
				"instance_id",
				instance.ID,
				"error",
				err,
			)
		}
	}
	return nil
}

func (service *WorkloadReconciliation) queueMissingServingRecovery(
	ctx context.Context,
	instance models.InstanceEntity,
) error {
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
	unresolved, err := models.Deployment.ActiveCountForTarget(
		ctx, tx, instance.EnvironmentTargetID,
	)
	if err != nil || unresolved != 0 {
		return err
	}
	previous, err := models.Deployment.Find(ctx, tx, instance.DeploymentID)
	if err != nil {
		return err
	}
	revision, err := models.EnvironmentStateRevision.FindResultForChange(ctx, tx, previous.ChangeID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, release.EnvironmentID)
	if err != nil {
		return err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "deployment_recovery",
		TriggerType: "system",
		ActorType:   "system",
		CauseSystem: sql.NullString{
			String: "startup_reconciliation",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: instance.ID.String(), Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           "Recover missing serving workload",
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     release.EnvironmentID,
	})
	if err != nil {
		return err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return err
	}
	var processes []models.EnvironmentProcessState
	if json.Unmarshal(previous.ProcessConfiguration, &processes) != nil {
		return errors.New("recovery Deployment process snapshot is invalid")
	}
	if _, err := service.releases.QueueTargetTx(
		ctx,
		tx,
		change.ID,
		release.ID,
		instance.EnvironmentTargetID,
		previous.Attempt+1,
		previous.Strategy,
		processes,
		now,
	); err != nil {
		return err
	}
	if err := models.Instance.MarkState(ctx, tx, instance.ID, "failed", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *WorkloadReconciliation) cleanupOldBackends(ctx context.Context) error {
	rows, err := models.CaddyRouteBackend.RetiredWorkloads(ctx, service.db.Executor())
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := service.workloads.Remove(
			ctx,
			row.ServerID,
			row.DeploymentID,
			row.InstanceID,
		); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			continue
		}
		if err := service.caddy.RemoveBackend(ctx, row.RouteID, row.InstanceID); err != nil {
			continue
		}
		now := time.Now().UTC()
		_ = models.Instance.MarkRemoved(ctx, service.db.Executor(), row.InstanceID, now)
	}
	return nil
}

func (service *WorkloadReconciliation) cleanupUnroutedInstances(ctx context.Context) error {
	return cleanupUnroutedWorkloadInstances(ctx, service.db, service.workloads, uuid.Nil)
}

func cleanupUnroutedWorkloadInstances(
	ctx context.Context,
	db storage.Pool,
	workloads *WorkloadExecution,
	environmentTargetID uuid.UUID,
) error {
	rows, err := models.Instance.UnroutedWorkloads(ctx, db.Executor(), environmentTargetID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := workloads.Remove(
			ctx,
			row.ServerID,
			row.DeploymentID,
			row.InstanceID,
		); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			continue
		}
		now := time.Now().UTC()
		_ = models.Instance.MarkRemoved(ctx, db.Executor(), row.InstanceID, now)
	}
	return nil
}
