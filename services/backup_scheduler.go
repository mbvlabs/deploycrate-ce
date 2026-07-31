package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type BackupScheduler struct {
	db      storage.Pool
	queue   storage.InsertQueue
	version CurrentVersion
}

func NewBackupScheduler(
	db storage.Pool,
	queue storage.InsertQueue,
	version CurrentVersion,
) *BackupScheduler {
	return &BackupScheduler{db: db, queue: queue, version: version}
}

func (service *BackupScheduler) Schedule(
	ctx context.Context,
	policyID uuid.UUID,
	scheduledAt time.Time,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	policyState, err := models.BackupPolicy.FindForUpdate(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("lock scheduled backup policy %s: %w", policyID, err)
	}
	slot := scheduledAt.UTC()
	if !policyState.Schedulable() || !policyState.NextRunAt.Equal(slot) {
		return nil
	}

	policy, err := models.BackupPolicy.FindScheduled(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("load scheduled backup policy %s: %w", policyID, err)
	}
	backupID, err := service.enqueue(ctx, tx, policy, slot, "schedule")
	if err != nil {
		return err
	}
	nextRunAt, err := models.NextBackupRun(policyState.Schedule, slot)
	if err != nil {
		return fmt.Errorf("advance backup policy %s schedule: %w", policyID, err)
	}
	now := time.Now().UTC()
	if err := models.BackupPolicy.AdvanceSchedule(
		ctx,
		tx,
		policyID,
		slot,
		nextRunAt,
		now,
	); err != nil {
		return fmt.Errorf("advance backup policy %s: %w", policyID, err)
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupScheduleArgs{BackupPolicyID: policyID, ScheduledAt: nextRunAt},
		jobs.BackupScheduleInsertOpts(nextRunAt),
	); err != nil {
		return fmt.Errorf("insert next backup schedule job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scheduled backup: %w", err)
	}

	slog.InfoContext(
		ctx,
		"backup scheduled",
		"backup_id", backupID,
		"backup_policy_id", policy.ID,
		"target_type", policy.TargetType,
		"driver", policy.Driver,
		"trigger_type", "schedule",
		"lifecycle_status", models.BackupStatusPending,
		"next_run_at", nextRunAt,
	)
	return nil
}

func (service *BackupScheduler) ReplaceSchedule(
	ctx context.Context,
	policyID uuid.UUID,
	expression string,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	policy, err := models.BackupPolicy.FindForUpdate(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("lock backup policy %s: %w", policyID, err)
	}
	now := time.Now().UTC()
	nextRunAt, err := models.NextBackupRun(expression, now)
	if err != nil {
		return fmt.Errorf("parse backup policy schedule: %w", err)
	}
	policy.Schedule = expression
	policy.NextRunAt = nextRunAt
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("validate replacement backup policy schedule: %w", err)
	}
	if err := models.BackupPolicy.ReplaceSchedule(
		ctx,
		tx,
		policyID,
		expression,
		nextRunAt,
		now,
	); err != nil {
		return fmt.Errorf("replace backup policy schedule: %w", err)
	}
	if policy.Schedulable() {
		if _, err := service.queue.InsertTx(
			ctx,
			tx.Tx,
			jobs.BackupScheduleArgs{BackupPolicyID: policyID, ScheduledAt: nextRunAt},
			jobs.BackupScheduleInsertOpts(nextRunAt),
		); err != nil {
			return fmt.Errorf("insert replacement backup schedule job: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replacement backup schedule: %w", err)
	}
	return nil
}

func (service *BackupScheduler) enqueue(
	ctx context.Context,
	tx bun.Tx,
	policy models.ScheduledBackupPolicy,
	slot time.Time,
	triggerType string,
) (uuid.UUID, error) {
	sequence, err := models.Change.NextSequence(ctx, tx, policy.EnvironmentID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("allocate backup change sequence: %w", err)
	}
	now := time.Now().UTC()
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "backup", TriggerType: triggerType, ActorType: "system",
		CauseSystem:    sql.NullString{String: "deploycrate-ce", Valid: true},
		CauseReference: sql.NullString{String: policy.ID.String(), Valid: true},
		CorrelationID:  uuid.New(), CorrectionContext: json.RawMessage(`{}`),
		Summary: "Create " + policy.TargetType + " backup", Status: "pending",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true},
		EnvironmentID: policy.EnvironmentID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create backup change: %w", err)
	}
	targetID := policy.ServerID
	taskServerID := policy.ServerID
	if policy.TargetType == "database" {
		targetID = policy.DatabaseID
		taskServerID = policy.ExecutionServerID
	}
	if targetID == nil {
		return uuid.Nil, errors.New("backup policy target is missing")
	}
	task, err := models.ChangeTask.Create(ctx, tx, models.CreateChangeTaskData{
		Kind: "backup_execute", SubjectType: policy.TargetType, SubjectID: *targetID,
		IdempotencyKey: "backup:" + triggerType + ":" + policy.ID.String() + ":" + slot.Format(time.RFC3339Nano),
		Input:          json.RawMessage(`{}`), Status: "pending", AvailableAt: now,
		ChangeID: change.ID, ServerID: taskServerID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create backup change task: %w", err)
	}
	backupID := uuid.New()
	if _, err := models.Backup.Create(ctx, tx, models.CreateBackupData{
		ID: backupID, TargetType: policy.TargetType, TriggerType: triggerType, ScheduledAt: slot,
		Strategy: policy.Strategy, Driver: policy.Driver, Format: policy.Format,
		FormatVersion: "1", ProviderMetadata: json.RawMessage(`{}`),
		Status: models.BackupStatusPending, RequestedAt: now,
		ProducerVersion: string(service.version), ChangeID: change.ID, ChangeTaskID: task.ID,
		BackupPolicyID: policy.ID, ServerID: policy.ServerID, DatabaseID: policy.DatabaseID,
		DatabaseClusterID: policy.DatabaseClusterID, DatabaseClusterNodeID: policy.DatabaseClusterNodeID,
		DatabaseNodeInstallationID: policy.DatabaseNodeInstallationID,
		BackupDestinationID:        policy.BackupDestinationID,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("create pending backup: %w", err)
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupExecuteArgs{BackupID: backupID},
		nil,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert backup execution job: %w", err)
	}
	return backupID, nil
}

func (service *BackupScheduler) InsertScheduleTx(
	ctx context.Context,
	tx bun.Tx,
	policyID uuid.UUID,
	scheduledAt time.Time,
) error {
	_, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupScheduleArgs{BackupPolicyID: policyID, ScheduledAt: scheduledAt},
		jobs.BackupScheduleInsertOpts(scheduledAt),
	)
	return err
}

func (service *BackupScheduler) Manual(ctx context.Context, policyID uuid.UUID) (uuid.UUID, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()
	state, err := models.BackupPolicy.FindForUpdate(ctx, tx, policyID)
	if err != nil {
		return uuid.Nil, err
	}
	if !state.Schedulable() {
		return uuid.Nil, errors.New("backup policy is paused or archived")
	}
	policy, err := models.BackupPolicy.FindScheduled(ctx, tx, policyID)
	if err != nil {
		return uuid.Nil, err
	}
	backupID, err := service.enqueue(ctx, tx, policy, time.Now().UTC(), "manual")
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, err
	}
	return backupID, nil
}

func (service *BackupScheduler) EnsureInitial(
	ctx context.Context,
	policyID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	policyState, err := models.BackupPolicy.FindForUpdate(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("lock initial backup policy %s: %w", policyID, err)
	}
	if !policyState.Schedulable() {
		return nil
	}
	exists, err := models.Backup.ExistsForPolicy(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("inspect initial backup policy %s: %w", policyID, err)
	}
	if exists {
		return nil
	}
	policy, err := models.BackupPolicy.FindScheduled(ctx, tx, policyID)
	if err != nil {
		return fmt.Errorf("load initial backup policy %s: %w", policyID, err)
	}
	slot := time.Now().UTC()
	backupID, err := service.enqueue(ctx, tx, policy, slot, "installer")
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit initial backup: %w", err)
	}
	slog.InfoContext(
		ctx,
		"initial backup scheduled",
		"backup_id", backupID,
		"backup_policy_id", policy.ID,
		"target_type", policy.TargetType,
		"driver", policy.Driver,
		"trigger_type", "installer",
		"lifecycle_status", models.BackupStatusPending,
	)
	return nil
}
