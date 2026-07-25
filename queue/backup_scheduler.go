package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"
)

type CurrentVersion string

type BackupScheduleWorker struct {
	river.WorkerDefaults[jobs.BackupScheduleArgs]
	db               storage.Pool
	queue            InsertOnly
	version          CurrentVersion
	installationID   string
	serverSchedule   string
	databaseSchedule string
}

func NewBackupScheduleWorker(
	db storage.Pool,
	queue InsertOnly,
	version CurrentVersion,
	configuration config.Config,
) *BackupScheduleWorker {
	return &BackupScheduleWorker{
		db:               db,
		queue:            queue,
		version:          version,
		installationID:   configuration.App.InstallationID,
		serverSchedule:   configuration.Backups.ServerSchedule,
		databaseSchedule: configuration.Backups.DatabaseSchedule,
	}
}

func (worker *BackupScheduleWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupScheduleWorker) Timeout(*river.Job[jobs.BackupScheduleArgs]) time.Duration {
	return time.Minute
}

func (worker *BackupScheduleWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupScheduleArgs],
) error {
	return runBackupWorker(
		ctx,
		"schedule",
		job.ID,
		job.Attempt,
		[]any{"target_type", job.Args.TargetType, "scheduled_at", job.Args.ScheduledAt},
		func() error { return worker.schedule(ctx, job.Args) },
	)
}

func (worker *BackupScheduleWorker) schedule(
	ctx context.Context,
	args jobs.BackupScheduleArgs,
) error {
	schedule, legacyPolicyName, err := worker.scheduleFor(args.TargetType)
	if err != nil {
		return river.JobCancel(err)
	}
	if worker.installationID == "" {
		return river.JobCancel(errors.New("backup installation ID is not configured"))
	}

	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	policy, err := models.BackupPolicy.FindForInstallationTargetUpdate(
		ctx,
		tx,
		worker.installationID,
		args.TargetType,
		legacyPolicyName,
	)
	if err != nil {
		return fmt.Errorf("load scheduled %s backup policy: %w", args.TargetType, err)
	}
	slot := args.ScheduledAt.UTC()
	backupID, err := worker.enqueue(ctx, tx, policy, slot)
	if err != nil {
		return err
	}
	nextRun, err := models.NextBackupRun(schedule, slot)
	if err != nil {
		return river.JobCancel(fmt.Errorf("advance %s backup schedule: %w", args.TargetType, err))
	}
	if err := models.BackupPolicy.AdvanceSchedule(
		ctx,
		tx,
		policy.ID,
		slot,
		nextRun,
		time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("advance backup policy %s: %w", policy.ID, err)
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
	)
	return nil
}

func (worker *BackupScheduleWorker) scheduleFor(targetType string) (string, string, error) {
	switch targetType {
	case "server":
		return worker.serverSchedule, "Control-plane server state", nil
	case "resource":
		return worker.databaseSchedule, "Control-plane PostgreSQL", nil
	default:
		return "", "", fmt.Errorf("unsupported backup target type %q", targetType)
	}
}

func (worker *BackupScheduleWorker) enqueue(
	ctx context.Context,
	tx bun.Tx,
	policy models.ScheduledBackupPolicy,
	slot time.Time,
) (uuid.UUID, error) {
	sequence, err := models.Change.NextSequence(ctx, tx, policy.EnvironmentID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("allocate backup change sequence: %w", err)
	}
	now := time.Now().UTC()
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "backup", TriggerType: "schedule", ActorType: "system",
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
	if policy.TargetType == "resource" {
		targetID = policy.ResourceID
	}
	if targetID == nil {
		return uuid.Nil, errors.New("backup policy target is missing")
	}
	task, err := models.ChangeTask.Create(ctx, tx, models.CreateChangeTaskData{
		Kind: "backup_execute", SubjectType: policy.TargetType, SubjectID: *targetID,
		IdempotencyKey: "backup:" + policy.ID.String() + ":" + slot.Format(time.RFC3339),
		Input:          json.RawMessage(`{}`), Status: "pending", AvailableAt: now,
		ChangeID: change.ID, ServerID: policy.ServerID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create backup change task: %w", err)
	}
	backupID := uuid.New()
	if _, err := models.Backup.Create(ctx, tx, models.CreateBackupData{
		ID: backupID, TargetType: policy.TargetType, TriggerType: "schedule", ScheduledAt: slot,
		Strategy: policy.Strategy, Driver: policy.Driver, Format: policy.Format,
		FormatVersion: "1", ProviderMetadata: json.RawMessage(`{}`),
		Status: models.BackupStatusPending, RequestedAt: now,
		ProducerVersion: string(worker.version), ChangeID: change.ID, ChangeTaskID: task.ID,
		BackupPolicyID: policy.ID, ServerID: policy.ServerID, ResourceID: policy.ResourceID,
		EnvironmentResourceID:  policy.EnvironmentResourceID,
		ResourceInstallationID: policy.ResourceInstallationID,
		ResourceVolumeID:       policy.ResourceVolumeID,
		BackupDestinationID:    policy.BackupDestinationID,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("create pending backup: %w", err)
	}
	if _, err := worker.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupExecuteArgs{BackupID: backupID},
		nil,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert backup execution job: %w", err)
	}
	return backupID, nil
}
