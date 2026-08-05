package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type systemApplicationState = models.SystemApplicationState

type selfUpdateDeployment struct {
	SystemState   systemApplicationState
	ChangeID      uuid.UUID
	ReleaseID     uuid.UUID
	Version       string
	ReleasePath   string
	DeploymentID  uuid.UUID
	InstanceID    uuid.UUID
	BackendID     int32
	EventSequence int64
	InactiveSlot  string
	Checkpoint    models.SystemUpdateCheckpoint
}

func (s *SelfUpdate) loadSystemState(ctx context.Context) (systemApplicationState, error) {
	state, err := models.Application.FindSystemState(ctx, s.db.Executor())
	if err != nil {
		return systemApplicationState{}, fmt.Errorf("load DeployCrate CE system state: %w", err)
	}
	if state.ActiveInstanceSlot != blueInstance && state.ActiveInstanceSlot != greenInstance {
		return systemApplicationState{}, fmt.Errorf(
			"system state has invalid active slot %q",
			state.ActiveInstanceSlot,
		)
	}
	return state, nil
}

func (s *SelfUpdate) createDeploymentRecords(
	ctx context.Context,
	actorID uuid.UUID,
	release updateRelease,
	systemState systemApplicationState,
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

	sequence, err := models.Change.NextSequence(ctx, tx, systemState.EnvironmentID)
	if err != nil {
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
		EnvironmentID:     systemState.EnvironmentID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update change: %w", err)
	}

	releasePath := s.releaseBinaryPath(release.Version)
	releaseEntity, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version:           sql.NullString{String: release.Version, Valid: true},
		ArtifactReference: releasePath,
		ArtifactDigest:    []byte{},
		EnvironmentID:     systemState.EnvironmentID,
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

	inactiveSlot := otherInstance(systemState.ActiveInstanceSlot)
	checkpoint := models.SystemUpdateCheckpoint{
		ServiceTemplate: "deploycrate-ce@.service",
		ActiveSlot:      systemState.ActiveInstanceSlot,
		TargetSlot:      inactiveSlot,
		Phase:           "queued",
	}
	runtimeConfiguration, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("encode self-update runtime configuration: %w", err)
	}
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: 1,
		Strategy: json.RawMessage(fmt.Sprintf(
			`{"type":"blue_green","slots":{"blue":%d,"green":%d}}`,
			bluePort,
			greenPort,
		)),
		ProcessConfiguration: runtimeConfiguration,
		Status:               "queued",
		CurrentStep:          sql.NullString{String: "queued", Valid: true},
		ChangeID:             change.ID,
		ReleaseID:            releaseEntity.ID,
		EnvironmentTargetID:  systemState.EnvironmentTargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update deployment: %w", err)
	}
	instance, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID:  serviceForInstance(inactiveSlot),
		Slot:        inactiveSlot,
		ProcessName: models.EnvironmentProcessWeb,
		ProcessKind: models.EnvironmentProcessWeb,
		ReplicaKey:  "primary",
		State:       "queued",
		Ports: json.RawMessage(
			fmt.Sprintf(`{"http":%d}`, portForInstance(inactiveSlot)),
		),
		ObservedAt:          now,
		DeploymentID:        deployment.ID,
		ReleaseID:           releaseEntity.ID,
		EnvironmentTargetID: systemState.EnvironmentTargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("create self-update instance: %w", err)
	}
	backend, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{
		Weight: 0, CaddyRouteID: systemState.CaddyRouteID, InstanceID: instance.ID,
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
		SystemState: systemState, ChangeID: change.ID, ReleaseID: releaseEntity.ID,
		Version: release.Version, ReleasePath: releasePath,
		DeploymentID: deployment.ID, InstanceID: instance.ID, BackendID: backend.ID,
		EventSequence: 1, InactiveSlot: inactiveSlot, Checkpoint: checkpoint,
	}, nil
}

func (s *SelfUpdate) persistCheckpoint(ctx context.Context, record *selfUpdateDeployment) error {
	if record == nil {
		return errors.New("self-update deployment record is missing")
	}
	checkpointCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := models.Deployment.SaveSystemUpdateCheckpoint(
		checkpointCtx,
		s.db.Executor(),
		record.DeploymentID,
		record.Checkpoint,
	); err != nil {
		return fmt.Errorf("persist self-update checkpoint: %w", err)
	}
	return nil
}

func (s *SelfUpdate) loadUnresolvedDeployment(ctx context.Context) (*selfUpdateDeployment, error) {
	persisted, err := models.Deployment.FindUnresolvedSystemUpdate(ctx, s.db.Executor())
	if err != nil {
		return nil, fmt.Errorf("load unresolved self-update deployment: %w", err)
	}
	if persisted == nil {
		return nil, nil
	}

	var checkpoint models.SystemUpdateCheckpoint
	if err := json.Unmarshal(persisted.ProcessConfiguration, &checkpoint); err != nil {
		return nil, fmt.Errorf("decode self-update checkpoint: %w", err)
	}
	systemState := systemApplicationState{
		EnvironmentID:        persisted.EnvironmentID,
		EnvironmentTargetID:  persisted.EnvironmentTargetID,
		CaddyRouteID:         persisted.CaddyRouteID,
		CaddyRouteExternalID: persisted.CaddyRouteExternalID,
		ActiveInstanceID:     persisted.PreviousInstanceID,
		ActiveInstanceSlot:   persisted.PreviousInstanceSlot,
		ActiveBackendID:      persisted.PreviousBackendID,
		ActiveReleaseID:      persisted.PreviousReleaseID,
	}
	if checkpoint.ActiveSlot != persisted.PreviousInstanceSlot ||
		checkpoint.TargetSlot != persisted.InactiveSlot {
		return nil, errors.New(
			"unresolved self-update checkpoint does not match the persisted system state",
		)
	}
	return &selfUpdateDeployment{
		SystemState: systemState, ChangeID: persisted.ChangeID, ReleaseID: persisted.ReleaseID,
		Version: persisted.Version, ReleasePath: persisted.ReleasePath,
		DeploymentID: persisted.DeploymentID, InstanceID: persisted.InstanceID,
		BackendID: persisted.BackendID, EventSequence: persisted.EventSequence,
		InactiveSlot: persisted.InactiveSlot, Checkpoint: checkpoint,
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
	if err := models.Deployment.RecordProgress(
		ctx,
		s.db.Executor(),
		record.DeploymentID,
		dbStatus,
		step,
		now,
	); err != nil {
		slogDatabaseTrackingError("update deployment progress", err)
		return
	}
	if err := models.Change.RecordProgress(
		ctx,
		s.db.Executor(),
		record.ChangeID,
		dbStatus,
		now,
	); err != nil {
		slogDatabaseTrackingError("update change progress", err)
	}
	if _, err := models.DeploymentEvent.Create(
		ctx,
		s.db.Executor(),
		models.CreateDeploymentEventData{
			Sequence: record.EventSequence, EventType: "deployment_status",
			Status: sql.NullString{String: dbStatus, Valid: true},
			Step:   sql.NullString{String: step, Valid: true}, Message: message,
			Metadata: json.RawMessage(`{}`), OccurredAt: now, DeploymentID: record.DeploymentID,
		},
	); err != nil {
		slogDatabaseTrackingError("create deployment event", err)
	}

	instanceState := ""
	switch step {
	case "start_inactive_instance", "verify_inactive_instance":
		instanceState = "starting"
	case "switch_traffic",
		"verify_public_path",
		"update_service_boot_state",
		"stop_previous_instance":
		instanceState = "serving"
	}
	if instanceState != "" {
		if err := models.Instance.ObserveState(
			ctx,
			s.db.Executor(),
			record.InstanceID,
			instanceState,
			now,
		); err != nil {
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
	return models.Release.RecordArtifactDigest(
		context.Background(),
		s.db.Executor(),
		record.ReleaseID,
		digest,
	)
}

func (s *SelfUpdate) finishDeployment(succeeded bool, failure error) error {
	s.mu.Lock()
	record := s.currentDeployment
	if record != nil {
		record.EventSequence++
	}
	s.mu.Unlock()
	if record == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deployment completion transaction: %w", err)
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
	checkpoint := record.Checkpoint
	checkpoint.Phase = "completed"
	if !succeeded {
		deploymentStatus = "failed"
		changeStatus = "failed"
		step = "failed"
		checkpoint.Phase = "rolled_back"
		if failure != nil {
			errorValue = sql.NullString{String: failure.Error(), Valid: true}
			message = "Update failed: " + failure.Error()
		}
	}
	if err := models.Deployment.FinishSystemUpdate(
		ctx, tx, record.DeploymentID, deploymentStatus, step, errorValue, checkpoint, now,
	); err != nil {
		return fmt.Errorf("finish deployment: %w", err)
	}
	if err := models.Change.FinishSystemUpdate(
		ctx,
		tx,
		record.ChangeID,
		changeStatus,
		errorValue,
		now,
	); err != nil {
		return fmt.Errorf("finish change: %w", err)
	}
	if _, err := models.DeploymentEvent.Create(ctx, tx, models.CreateDeploymentEventData{
		Sequence: record.EventSequence, EventType: "deployment_status",
		Status: sql.NullString{String: deploymentStatus, Valid: true},
		Step:   sql.NullString{String: step, Valid: true}, Message: message,
		Metadata: json.RawMessage(`{}`), Error: errorValue, OccurredAt: now,
		DeploymentID: record.DeploymentID,
	}); err != nil {
		return fmt.Errorf("create final deployment event: %w", err)
	}

	if succeeded {
		if err := models.Instance.FinishSystemUpdate(
			ctx,
			tx,
			record.InstanceID,
			"serving",
			false,
			now,
		); err != nil {
			return fmt.Errorf("mark updated instance serving: %w", err)
		}
		if err := models.CaddyRouteBackend.FinishSystemUpdate(
			ctx,
			tx,
			record.BackendID,
			100,
			false,
			now,
		); err != nil {
			return fmt.Errorf("activate updated Caddy backend: %w", err)
		}
		if err := models.Instance.FinishSystemUpdate(
			ctx,
			tx,
			record.SystemState.ActiveInstanceID,
			"stopped",
			true,
			now,
		); err != nil {
			return fmt.Errorf("retire previous instance: %w", err)
		}
		if err := models.CaddyRouteBackend.FinishSystemUpdate(
			ctx,
			tx,
			record.SystemState.ActiveBackendID,
			0,
			true,
			now,
		); err != nil {
			return fmt.Errorf("retire previous Caddy backend: %w", err)
		}
		if err := models.CaddyRoute.ActivateRelease(
			ctx,
			tx,
			record.SystemState.CaddyRouteID,
			record.ReleaseID,
			now,
		); err != nil {
			return fmt.Errorf("update Caddy route release: %w", err)
		}
	} else {
		if err := models.Instance.FinishSystemUpdate(
			ctx,
			tx,
			record.InstanceID,
			"failed",
			true,
			now,
		); err != nil {
			return fmt.Errorf("mark failed instance: %w", err)
		}
		if err := models.CaddyRouteBackend.FinishSystemUpdate(
			ctx,
			tx,
			record.BackendID,
			0,
			true,
			now,
		); err != nil {
			return fmt.Errorf("remove failed Caddy backend: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deployment completion: %w", err)
	}
	committed = true
	record.Checkpoint = checkpoint
	return nil
}

func slogDatabaseTrackingError(operation string, err error) {
	slog.Error("self-update database tracking failed", "operation", operation, "error", err)
}
