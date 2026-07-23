package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type systemApplicationTopology struct {
	ApplicationID        uuid.UUID `bun:"application_id"`
	EnvironmentID        uuid.UUID `bun:"environment_id"`
	EnvironmentTargetID  uuid.UUID `bun:"environment_target_id"`
	CaddyRouteID         uuid.UUID `bun:"caddy_route_id"`
	CaddyRouteExternalID string    `bun:"caddy_route_external_id"`
	ActiveInstanceID     uuid.UUID `bun:"active_instance_id"`
	ActiveInstanceSlot   string    `bun:"active_instance_slot"`
	ActiveBackendID      int32     `bun:"active_backend_id"`
	ActiveReleaseID      uuid.UUID `bun:"active_release_id"`
}

type selfUpdateDeployment struct {
	Topology      systemApplicationTopology
	ChangeID      uuid.UUID
	ReleaseID     uuid.UUID
	DeploymentID  uuid.UUID
	InstanceID    uuid.UUID
	BackendID     int32
	EventSequence int64
	InactiveSlot  string
}

func (s *SelfUpdate) loadSystemTopology(ctx context.Context) (systemApplicationTopology, error) {
	if _, err := models.Application.FindSystem(ctx, s.db.Executor()); err != nil {
		return systemApplicationTopology{}, fmt.Errorf("find DeployCrate CE system application: %w", err)
	}

	var topology systemApplicationTopology
	if err := s.db.Executor().NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.id AS application_id").
		ColumnExpr("environment.id AS environment_id").
		ColumnExpr("target.id AS environment_target_id").
		ColumnExpr("route.id AS caddy_route_id").
		ColumnExpr("route.external_id AS caddy_route_external_id").
		ColumnExpr("instance.id AS active_instance_id").
		ColumnExpr("instance.slot AS active_instance_slot").
		ColumnExpr("backend.id AS active_backend_id").
		ColumnExpr("instance.release_id AS active_release_id").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN caddy_routes AS route ON route.environment_target_id = target.id AND route.removed_at IS NULL").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL AND backend.weight = 100").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Where("application.slug = ?", models.SystemApplicationSlug).
		OrderExpr("route.created_at DESC").
		Limit(1).
		Scan(ctx, &topology); err != nil {
		return systemApplicationTopology{}, fmt.Errorf("load DeployCrate CE system topology: %w", err)
	}
	if topology.ActiveInstanceSlot != blueInstance && topology.ActiveInstanceSlot != greenInstance {
		return systemApplicationTopology{}, fmt.Errorf("system topology has invalid active slot %q", topology.ActiveInstanceSlot)
	}
	return topology, nil
}

func (s *SelfUpdate) createDeploymentRecords(
	ctx context.Context,
	actorID uuid.UUID,
	release updateRelease,
	topology systemApplicationTopology,
) (*selfUpdateDeployment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin self-update deployment transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var sequence int64
	if err := tx.NewSelect().
		TableExpr("changes").
		ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("environment_id = ?", topology.EnvironmentID).
		Scan(ctx, &sequence); err != nil {
		return nil, fmt.Errorf("allocate self-update change sequence: %w", err)
	}

	now := time.Now().UTC()
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:          sequence,
		Kind:              "system_update",
		TriggerType:       "manual",
		ActorType:         "user",
		ActorID:           &actorID,
		CauseSystem:       sql.NullString{String: "deploycrate-ce", Valid: true},
		CauseReference:    sql.NullString{String: release.Version, Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           fmt.Sprintf("Update DeployCrate CE to v%s", release.Version),
		Status:            "queued",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     topology.EnvironmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update change: %w", err)
	}

	releasePath := s.releaseBinaryPath(release.Version)
	releaseEntity, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version:           sql.NullString{String: release.Version, Valid: true},
		ArtifactReference: releasePath,
		ArtifactDigest:    []byte{},
		EnvironmentID:     topology.EnvironmentID,
		CreatedByChangeID: change.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update release: %w", err)
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{
		ChangeID: change.ID, ReleaseID: releaseEntity.ID,
	}); err != nil {
		return nil, fmt.Errorf("associate self-update release: %w", err)
	}

	inactiveSlot := otherInstance(topology.ActiveInstanceSlot)
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: 1,
		Strategy: json.RawMessage(fmt.Sprintf(
			`{"type":"blue_green","slots":{"blue":%d,"green":%d}}`,
			bluePort,
			greenPort,
		)),
		RuntimeConfiguration: json.RawMessage(fmt.Sprintf(
			`{"service_template":"deploycrate-ce@.service","active_slot":%q,"target_slot":%q}`,
			topology.ActiveInstanceSlot,
			inactiveSlot,
		)),
		Status:              "queued",
		CurrentStep:         sql.NullString{String: "queued", Valid: true},
		ChangeID:            change.ID,
		ReleaseID:           releaseEntity.ID,
		EnvironmentTargetID: topology.EnvironmentTargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update deployment: %w", err)
	}
	instance, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID:          serviceForInstance(inactiveSlot),
		Slot:                inactiveSlot,
		ReplicaKey:          "primary",
		State:               "queued",
		Ports:               json.RawMessage(fmt.Sprintf(`{"http":%d}`, portForInstance(inactiveSlot))),
		ObservedAt:          now,
		DeploymentID:        deployment.ID,
		ReleaseID:           releaseEntity.ID,
		EnvironmentTargetID: topology.EnvironmentTargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update instance: %w", err)
	}
	backend, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{
		Weight: 0, CaddyRouteID: topology.CaddyRouteID, InstanceID: instance.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update Caddy backend: %w", err)
	}
	if _, err := models.DeploymentEvent.Create(ctx, tx, models.CreateDeploymentEventData{
		Sequence:     1,
		EventType:    "deployment_status",
		Status:       sql.NullString{String: "queued", Valid: true},
		Step:         sql.NullString{String: "queued", Valid: true},
		Message:      "Update queued",
		Metadata:     json.RawMessage(`{}`),
		OccurredAt:   now,
		DeploymentID: deployment.ID,
	}); err != nil {
		return nil, fmt.Errorf("create queued deployment event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit self-update deployment transaction: %w", err)
	}
	committed = true
	return &selfUpdateDeployment{
		Topology: topology, ChangeID: change.ID, ReleaseID: releaseEntity.ID,
		DeploymentID: deployment.ID, InstanceID: instance.ID, BackendID: backend.ID,
		EventSequence: 1, InactiveSlot: inactiveSlot,
	}, nil
}

func (s *SelfUpdate) recordTransition(status SelfUpdateState, step, message string) {
	s.mu.Lock()
	record := s.currentDeployment
	if record != nil {
		record.EventSequence++
	}
	s.mu.Unlock()
	if record == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	dbStatus := string(status)
	if status == SelfUpdateInProgress {
		dbStatus = "in_progress"
	}
	if _, err := s.db.Executor().NewUpdate().
		TableExpr("deployments").
		Set("status = ?", dbStatus).
		Set("current_step = ?", step).
		Set("started_at = COALESCE(started_at, ?)", now).
		Set("updated_at = ?", now).
		Where("id = ?", record.DeploymentID).
		Exec(ctx); err != nil {
		slogDatabaseTrackingError("update deployment progress", err)
		return
	}
	if _, err := s.db.Executor().NewUpdate().
		TableExpr("changes").
		Set("status = ?", dbStatus).
		Set("started_at = COALESCE(started_at, ?)", now).
		Set("updated_at = ?", now).
		Where("id = ?", record.ChangeID).
		Exec(ctx); err != nil {
		slogDatabaseTrackingError("update change progress", err)
	}
	if _, err := models.DeploymentEvent.Create(ctx, s.db.Executor(), models.CreateDeploymentEventData{
		Sequence: record.EventSequence, EventType: "deployment_status",
		Status: sql.NullString{String: dbStatus, Valid: true},
		Step:   sql.NullString{String: step, Valid: true}, Message: message,
		Metadata: json.RawMessage(`{}`), OccurredAt: now, DeploymentID: record.DeploymentID,
	}); err != nil {
		slogDatabaseTrackingError("create deployment event", err)
	}

	instanceState := ""
	switch step {
	case "start_inactive_instance", "verify_inactive_instance":
		instanceState = "starting"
	case "switch_traffic", "verify_public_path", "update_service_boot_state", "stop_previous_instance":
		instanceState = "serving"
	}
	if instanceState != "" {
		if _, err := s.db.Executor().NewUpdate().TableExpr("instances").
			Set("state = ?", instanceState).
			Set("observed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", record.InstanceID).
			Exec(ctx); err != nil {
			slogDatabaseTrackingError("update instance progress", err)
		}
	}
}

func (s *SelfUpdate) recordArtifact(digest []byte) error {
	s.mu.RLock()
	record := s.currentDeployment
	s.mu.RUnlock()
	if record == nil {
		return nil
	}
	_, err := s.db.Executor().NewUpdate().TableExpr("releases").
		Set("artifact_digest = ?", digest).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", record.ReleaseID).
		Exec(context.Background())
	return err
}

func (s *SelfUpdate) finishDeployment(succeeded bool, failure error) {
	s.mu.Lock()
	record := s.currentDeployment
	if record != nil {
		record.EventSequence++
	}
	s.mu.Unlock()
	if record == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slogDatabaseTrackingError("begin deployment completion transaction", err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	deploymentStatus := "succeeded"
	changeStatus := "completed"
	step := "completed"
	errorValue := sql.NullString{}
	message := "Update completed"
	if !succeeded {
		deploymentStatus = "failed"
		changeStatus = "failed"
		step = "failed"
		errorValue = sql.NullString{String: failure.Error(), Valid: true}
		message = "Update failed: " + failure.Error()
	}
	if _, err := tx.NewUpdate().TableExpr("deployments").
		Set("status = ?", deploymentStatus).Set("current_step = ?", step).
		Set("finished_at = ?", now).Set("error = ?", errorValue).
		Set("updated_at = ?", now).Where("id = ?", record.DeploymentID).Exec(ctx); err != nil {
		slogDatabaseTrackingError("finish deployment", err)
		return
	}
	if _, err := tx.NewUpdate().TableExpr("changes").
		Set("status = ?", changeStatus).Set("finished_at = ?", now).
		Set("error = ?", errorValue).Set("updated_at = ?", now).
		Where("id = ?", record.ChangeID).Exec(ctx); err != nil {
		slogDatabaseTrackingError("finish change", err)
		return
	}
	if _, err := models.DeploymentEvent.Create(ctx, tx, models.CreateDeploymentEventData{
		Sequence: record.EventSequence, EventType: "deployment_status",
		Status: sql.NullString{String: deploymentStatus, Valid: true},
		Step:   sql.NullString{String: step, Valid: true}, Message: message,
		Metadata: json.RawMessage(`{}`), Error: errorValue, OccurredAt: now,
		DeploymentID: record.DeploymentID,
	}); err != nil {
		slogDatabaseTrackingError("create final deployment event", err)
		return
	}

	if succeeded {
		if _, err := tx.NewUpdate().TableExpr("instances").
			Set("state = 'serving'").Set("observed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.InstanceID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("mark updated instance serving", err)
			return
		}
		if _, err := tx.NewUpdate().TableExpr("caddy_route_backends").
			Set("weight = 100").Set("updated_at = ?", now).
			Where("id = ?", record.BackendID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("activate updated Caddy backend", err)
			return
		}
		if _, err := tx.NewUpdate().TableExpr("instances").
			Set("state = 'stopped'").Set("removed_at = ?", now).Set("observed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.Topology.ActiveInstanceID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("retire previous instance", err)
			return
		}
		if _, err := tx.NewUpdate().TableExpr("caddy_route_backends").
			Set("weight = 0").Set("removed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.Topology.ActiveBackendID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("retire previous Caddy backend", err)
			return
		}
		if _, err := tx.NewUpdate().TableExpr("caddy_routes").
			Set("release_id = ?", record.ReleaseID).Set("observed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.Topology.CaddyRouteID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("update Caddy route release", err)
			return
		}
	} else {
		if _, err := tx.NewUpdate().TableExpr("instances").
			Set("state = 'failed'").Set("removed_at = ?", now).Set("observed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.InstanceID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("mark failed instance", err)
			return
		}
		if _, err := tx.NewUpdate().TableExpr("caddy_route_backends").
			Set("weight = 0").Set("removed_at = ?", now).Set("updated_at = ?", now).
			Where("id = ?", record.BackendID).Exec(ctx); err != nil {
			slogDatabaseTrackingError("remove failed Caddy backend", err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		slogDatabaseTrackingError("commit deployment completion", err)
		return
	}
	committed = true
}

func slogDatabaseTrackingError(operation string, err error) {
	slog.Error("self-update database tracking failed", "operation", operation, "error", err)
}
