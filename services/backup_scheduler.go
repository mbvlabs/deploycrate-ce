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
	"github.com/robfig/cron/v3"
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

type scheduledBackupPolicy struct {
	ID                     uuid.UUID  `bun:"id"`
	Schedule               string     `bun:"schedule"`
	Strategy               string     `bun:"strategy"`
	Driver                 string     `bun:"driver"`
	Format                 string     `bun:"format"`
	TargetType             string     `bun:"target_type"`
	ServerID               *uuid.UUID `bun:"server_id"`
	ResourceID             *uuid.UUID `bun:"resource_id"`
	EnvironmentResourceID  *uuid.UUID `bun:"environment_resource_id"`
	ResourceVolumeID       *uuid.UUID `bun:"resource_volume_id"`
	ResourceInstallationID *uuid.UUID `bun:"resource_installation_id"`
	BackupDestinationID    uuid.UUID  `bun:"backup_destination_id"`
	NextRunAt              time.Time  `bun:"next_run_at"`
	EnvironmentID          uuid.UUID  `bun:"environment_id"`
}

func (service *BackupScheduler) ScheduleDue(ctx context.Context) error {
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var policies []scheduledBackupPolicy
	if err := tx.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id, policy.schedule, policy.strategy, policy.driver, policy.format").
		ColumnExpr("policy.target_type, policy.server_id, policy.resource_id").
		ColumnExpr("policy.environment_resource_id, policy.resource_volume_id").
		ColumnExpr("policy.backup_destination_id, policy.next_run_at").
		ColumnExpr("endpoint.resource_installation_id AS resource_installation_id").
		ColumnExpr("COALESCE(server_target.environment_id, resource_binding.environment_id) AS environment_id").
		Join("LEFT JOIN LATERAL (SELECT environment_id FROM environment_targets WHERE server_id = policy.server_id AND detached_at IS NULL ORDER BY attached_at DESC LIMIT 1) AS server_target ON TRUE").
		Join("LEFT JOIN environment_resources AS resource_binding ON resource_binding.id = policy.environment_resource_id AND resource_binding.archived_at IS NULL").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = resource_binding.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Where("policy.archived_at IS NULL").
		Where("policy.next_run_at <= ?", now).
		OrderExpr("policy.next_run_at ASC").
		Limit(100).
		For("UPDATE OF policy SKIP LOCKED").
		Scan(ctx, &policies); err != nil {
		return fmt.Errorf("lock due backup policies: %w", err)
	}

	scheduled := make([]struct {
		backupID uuid.UUID
		policy   scheduledBackupPolicy
	}, 0, len(policies))
	for _, policy := range policies {
		backupID, err := service.enqueue(ctx, tx, policy, "schedule", policy.NextRunAt)
		if err != nil {
			return err
		}
		scheduled = append(scheduled, struct {
			backupID uuid.UUID
			policy   scheduledBackupPolicy
		}{backupID: backupID, policy: policy})
		nextRun, err := nextBackupRun(policy.Schedule, policy.NextRunAt)
		if err != nil {
			return fmt.Errorf("advance backup policy %s: %w", policy.ID, err)
		}
		if _, err := tx.NewUpdate().TableExpr("backup_policies").
			Set("last_scheduled_at = ?", policy.NextRunAt).
			Set("next_run_at = ?", nextRun).
			Set("updated_at = ?", now).
			Where("id = ?", policy.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("advance backup policy %s: %w", policy.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backup schedule tick: %w", err)
	}
	for _, item := range scheduled {
		slog.InfoContext(
			ctx,
			"backup scheduled",
			"backup_id", item.backupID,
			"backup_policy_id", item.policy.ID,
			"target_type", item.policy.TargetType,
			"driver", item.policy.Driver,
			"trigger_type", "schedule",
			"lifecycle_status", models.BackupStatusPending,
		)
	}
	return nil
}

func (service *BackupScheduler) EnqueueInitial(
	ctx context.Context,
	policyIDs []uuid.UUID,
) ([]uuid.UUID, error) {
	if len(policyIDs) == 0 {
		return nil, nil
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var policies []scheduledBackupPolicy
	if err := tx.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id, policy.schedule, policy.strategy, policy.driver, policy.format").
		ColumnExpr("policy.target_type, policy.server_id, policy.resource_id").
		ColumnExpr("policy.environment_resource_id, policy.resource_volume_id").
		ColumnExpr("policy.backup_destination_id, policy.next_run_at").
		ColumnExpr("endpoint.resource_installation_id AS resource_installation_id").
		ColumnExpr("COALESCE(server_target.environment_id, resource_binding.environment_id) AS environment_id").
		Join("LEFT JOIN LATERAL (SELECT environment_id FROM environment_targets WHERE server_id = policy.server_id AND detached_at IS NULL ORDER BY attached_at DESC LIMIT 1) AS server_target ON TRUE").
		Join("LEFT JOIN environment_resources AS resource_binding ON resource_binding.id = policy.environment_resource_id AND resource_binding.archived_at IS NULL").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = resource_binding.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Where("policy.id IN (?)", bun.In(policyIDs)).
		Where("policy.archived_at IS NULL").
		For("UPDATE OF policy").
		Scan(ctx, &policies); err != nil {
		return nil, fmt.Errorf("load initial backup policies: %w", err)
	}

	slot := time.Now().UTC()
	backupIDs := make([]uuid.UUID, 0, len(policies))
	for _, policy := range policies {
		backupID, err := service.enqueue(ctx, tx, policy, "installer", slot)
		if err != nil {
			return nil, err
		}
		backupIDs = append(backupIDs, backupID)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit initial backup jobs: %w", err)
	}
	return backupIDs, nil
}

func (service *BackupScheduler) enqueue(
	ctx context.Context,
	tx bun.Tx,
	policy scheduledBackupPolicy,
	trigger string,
	slot time.Time,
) (uuid.UUID, error) {
	sequence, err := models.Change.NextSequence(ctx, tx, policy.EnvironmentID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("allocate backup change sequence: %w", err)
	}
	now := time.Now().UTC()
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "backup", TriggerType: trigger, ActorType: "system",
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
		IdempotencyKey: "backup:" + policy.ID.String() + ":" + slot.UTC().Format(time.RFC3339),
		Input:          json.RawMessage(`{}`), Status: "pending", AvailableAt: now,
		ChangeID: change.ID, ServerID: policy.ServerID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create backup change task: %w", err)
	}
	backupID := uuid.New()
	if _, err := models.Backup.Create(ctx, tx, models.CreateBackupData{
		ID: backupID, TargetType: policy.TargetType, TriggerType: trigger, ScheduledAt: slot,
		Strategy: policy.Strategy, Driver: policy.Driver, Format: policy.Format,
		FormatVersion: "1", ProviderMetadata: json.RawMessage(`{}`),
		Status: models.BackupStatusPending, RequestedAt: now,
		ProducerVersion: string(service.version), ChangeID: change.ID, ChangeTaskID: task.ID,
		BackupPolicyID: policy.ID, ServerID: policy.ServerID, ResourceID: policy.ResourceID,
		EnvironmentResourceID:  policy.EnvironmentResourceID,
		ResourceInstallationID: policy.ResourceInstallationID,
		ResourceVolumeID:       policy.ResourceVolumeID,
		BackupDestinationID:    policy.BackupDestinationID,
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

func nextBackupRun(expression string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(after), nil
}
