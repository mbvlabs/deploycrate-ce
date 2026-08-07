package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DatabaseRestoreWorkflow struct {
	db        storage.Pool
	queue     storage.InsertQueue
	scheduler *BackupScheduler
	executor  *BackupExecutor
	engine    *DatabaseRestoreEngine
}

func NewDatabaseRestoreWorkflow(
	db storage.Pool,
	queue storage.InsertQueue,
	scheduler *BackupScheduler,
	executor *BackupExecutor,
	engine *DatabaseRestoreEngine,
) *DatabaseRestoreWorkflow {
	return &DatabaseRestoreWorkflow{
		db:        db,
		queue:     queue,
		scheduler: scheduler,
		executor:  executor,
		engine:    engine,
	}
}

type DatabaseRestoreInput struct {
	BackupID     uuid.UUID
	Confirmation string
	ActorID      uuid.UUID
}

func (service *DatabaseRestoreWorkflow) RequestForResource(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	input DatabaseRestoreInput,
) (models.ResourceRestoreEntity, error) {
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	if strings.TrimSpace(input.Confirmation) != resource.Name {
		return models.ResourceRestoreEntity{}, domainError(
			"confirmation",
			"mismatch",
			"Enter the Resource name exactly to confirm the restore",
		)
	}
	if resource.SystemManaged {
		return models.ResourceRestoreEntity{}, domainError(
			"backupId",
			"system_managed",
			"The running control-plane Database cannot be restored from the application",
		)
	}
	backup, err := models.Backup.Find(ctx, service.db.Executor(), input.BackupID)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	if backup.ResourceID == nil || *backup.ResourceID != resourceID ||
		resourceCredentialMetadataDatabase(backup.Target) != databaseName ||
		!resourceHasDatabase(resource, databaseName) {
		return models.ResourceRestoreEntity{}, domainError(
			"backupId",
			"ineligible",
			"Choose a Database backup from this Resource",
		)
	}
	return service.Request(ctx, resourceID, databaseName, input)
}

func (service *DatabaseRestoreWorkflow) Request(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	input DatabaseRestoreInput,
) (models.ResourceRestoreEntity, error) {
	backup, err := models.Backup.Find(ctx, service.db.Executor(), input.BackupID)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	if backup.Status != models.BackupStatusVerified || backup.ResourceID == nil ||
		*backup.ResourceID != resourceID ||
		resourceCredentialMetadataDatabase(backup.Target) != databaseName {
		return models.ResourceRestoreEntity{}, domainError(
			"backupId",
			"ineligible",
			"Choose a verified backup from this Database",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	defer tx.Rollback()
	policy, err := activeDatabaseBackupPolicy(ctx, tx, resourceID, databaseName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ResourceRestoreEntity{}, domainError(
				"backupId",
				"safety_backup_unavailable",
				"An active backup policy is required to create the mandatory safety backup",
			)
		}
		return models.ResourceRestoreEntity{}, err
	}
	sequence, err := models.Change.NextSequence(ctx, tx, policy.EnvironmentID)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	now := time.Now().UTC()
	change, err := models.Change.Create(
		ctx,
		tx,
		models.CreateChangeData{
			Sequence:          sequence,
			Kind:              "database_restore",
			TriggerType:       "manual",
			ActorType:         "user",
			ActorID:           &input.ActorID,
			CorrelationID:     uuid.New(),
			CorrectionContext: json.RawMessage(`{}`),
			Summary:           "Restore Database from verified backup",
			Status:            "pending",
			RequestedAt:       now,
			CommittedAt:       sql.NullTime{Time: now, Valid: true},
			EnvironmentID:     policy.EnvironmentID,
		},
	)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	task, err := models.ChangeTask.Create(
		ctx,
		tx,
		models.CreateChangeTaskData{
			Kind:           "resource_restore",
			SubjectType:    "resource",
			SubjectID:      resourceID,
			IdempotencyKey: "resource-restore:" + resourceID.String() + ":" + databaseName + ":" + input.BackupID.String(),
			Input:          json.RawMessage(`{}`),
			Status:         "pending",
			AvailableAt:    now,
			ChangeID:       change.ID,
			ServerID:       policy.ExecutionServerID,
		},
	)
	if err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	target, _ := json.Marshal(map[string]string{"database": databaseName})
	restore, err := models.ResourceRestore.Create(
		ctx,
		tx,
		models.CreateResourceRestoreData{
			Status:       models.ResourceRestoreStatusPending,
			RequestedAt:  now,
			Target:       target,
			ChangeID:     change.ID,
			ChangeTaskID: task.ID,
			BackupID:     input.BackupID,
			ResourceID:   resourceID,
		},
	)
	if err != nil {
		return models.ResourceRestoreEntity{}, mapRestoreConflict(err)
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.DatabaseRestorePrepareArgs{DatabaseRestoreID: restore.ID},
		nil,
	); err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	return restore, nil
}

func (service *DatabaseRestoreWorkflow) Prepare(ctx context.Context, restoreID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	restore, err := models.ResourceRestore.FindForUpdate(ctx, tx, restoreID)
	if err != nil {
		return err
	}
	if restore.Status != models.ResourceRestoreStatusPending {
		return nil
	}
	policy, err := activeDatabaseBackupPolicy(
		ctx,
		tx,
		restore.ResourceID,
		resourceCredentialMetadataDatabase(restore.Target),
	)
	if err != nil {
		operationErr := fmt.Errorf("load safety backup policy: %w", err)
		return errors.Join(operationErr, service.failTx(ctx, tx, restore, operationErr, false, nil))
	}
	backupID, err := service.scheduler.enqueue(ctx, tx, policy, time.Now().UTC(), "pre_restore")
	if err != nil {
		operationErr := fmt.Errorf("create safety backup: %w", err)
		return errors.Join(operationErr, service.failTx(ctx, tx, restore, operationErr, false, nil))
	}
	if err := models.ResourceRestore.MarkSafetyBackup(
		ctx,
		tx,
		restore.ID,
		backupID,
		time.Now().UTC(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *DatabaseRestoreWorkflow) Apply(ctx context.Context, restoreID uuid.UUID) error {
	restore, err := models.ResourceRestore.Find(ctx, service.db.Executor(), restoreID)
	if err != nil {
		return err
	}
	if restore.Status == models.ResourceRestoreStatusCompleted ||
		restore.Status == models.ResourceRestoreStatusRolledBack ||
		restore.Status == models.ResourceRestoreStatusFailed {
		return nil
	}
	if restore.Status != models.ResourceRestoreStatusRestoring {
		return fmt.Errorf("Database restore %s is not ready to apply", restoreID)
	}
	if err := models.Change.MarkRunning(
		ctx,
		service.db.Executor(),
		restore.ChangeID,
		time.Now().UTC(),
	); err != nil {
		return err
	}
	if err := models.ChangeTask.MarkRunning(
		ctx,
		service.db.Executor(),
		restore.ChangeTaskID,
		time.Now().UTC(),
	); err != nil {
		return err
	}
	scope, err := service.executor.loadScope(ctx, restore.BackupID)
	if err != nil {
		return service.fail(ctx, restore, err, DatabaseRestoreResult{})
	}
	if scope.Backup.Status != models.BackupStatusVerified || scope.Backup.ResourceID == nil ||
		*scope.Backup.ResourceID != restore.ResourceID ||
		resourceCredentialMetadataDatabase(
			scope.Backup.Target,
		) != resourceCredentialMetadataDatabase(
			restore.Target,
		) {
		return service.fail(
			ctx,
			restore,
			errors.New("restore source is no longer a verified backup for the Database"),
			DatabaseRestoreResult{},
		)
	}
	target, err := service.executor.postgreSQLTargetForDatabase(
		ctx,
		restore.ResourceID,
		resourceCredentialMetadataDatabase(restore.Target),
	)
	if err != nil {
		return service.fail(ctx, restore, err, DatabaseRestoreResult{})
	}
	credential, err := service.backupCredential(scope)
	if err != nil {
		return service.fail(ctx, restore, err, DatabaseRestoreResult{})
	}
	store, err := objectstorage.New(
		ctx,
		scope.ObjectStorageConfig(),
		objectstorage.Credentials{
			AccessKeyID:     credential.AccessKeyID,
			SecretAccessKey: credential.SecretAccessKey,
		},
	)
	if err != nil {
		return service.fail(ctx, restore, err, DatabaseRestoreResult{})
	}
	result, err := service.engine.Run(ctx, scope, target, credential, store, restore.ID.String())
	if err != nil {
		return service.fail(ctx, restore, err, result)
	}
	if result.CutoverAt == nil {
		return service.fail(
			ctx,
			restore,
			errors.New("Database restore completed without a cutover timestamp"),
			result,
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if err := models.ResourceRestore.MarkCompleted(
		ctx,
		tx,
		restore.ID,
		*result.CutoverAt,
		now,
	); err != nil {
		return err
	}
	if err := models.ChangeTask.MarkCompleted(ctx, tx, restore.ChangeTaskID, now); err != nil {
		return err
	}
	if err := models.Change.MarkCompleted(ctx, tx, restore.ChangeID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.InfoContext(
		ctx,
		"Resource database restore completed",
		"resource_restore_id",
		restore.ID,
		"resource_id",
		restore.ResourceID,
		"database",
		resourceCredentialMetadataDatabase(restore.Target),
		"backup_id",
		restore.BackupID,
	)
	return nil
}

func (service *DatabaseRestoreWorkflow) backupCredential(
	scope BackupScope,
) (BackupCredentialPayload, error) {
	plaintext, err := secretcrypto.Decrypt(
		scope.CredentialPayload,
		service.executor.config.App.SessionEncryptionKey,
	)
	if err != nil {
		return BackupCredentialPayload{}, errors.New("decrypt backup credential for restore")
	}
	defer clear(plaintext)
	var credential BackupCredentialPayload
	if json.Unmarshal(plaintext, &credential) != nil || credential.AccessKeyID == "" ||
		credential.SecretAccessKey == "" ||
		credential.AgeIdentity == "" {
		return BackupCredentialPayload{}, errors.New("backup credential is incomplete for restore")
	}
	return credential, nil
}

func (service *DatabaseRestoreWorkflow) fail(
	ctx context.Context,
	restore models.ResourceRestoreEntity,
	operationErr error,
	result DatabaseRestoreResult,
) error {
	persistContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	tx, err := service.db.BeginTx(persistContext, nil)
	if err != nil {
		return errors.Join(operationErr, err)
	}
	defer tx.Rollback()
	if err := service.failTx(
		persistContext,
		tx,
		restore,
		operationErr,
		result.RolledBack,
		result.CutoverAt,
	); err != nil {
		return errors.Join(operationErr, err)
	}
	return operationErr
}

func (service *DatabaseRestoreWorkflow) failTx(
	ctx context.Context,
	tx bun.Tx,
	restore models.ResourceRestoreEntity,
	operationErr error,
	rolledBack bool,
	cutoverAt *time.Time,
) error {
	now := time.Now().UTC()
	if err := models.ResourceRestore.MarkFailed(
		ctx,
		tx,
		restore.ID,
		operationErr,
		rolledBack,
		cutoverAt,
		now,
	); err != nil {
		return err
	}
	if err := models.ChangeTask.MarkFailed(ctx, tx, restore.ChangeTaskID, now); err != nil {
		return err
	}
	if err := models.Change.MarkFailed(ctx, tx, restore.ChangeID, operationErr, now); err != nil {
		return err
	}
	return tx.Commit()
}

func activeDatabaseBackupPolicy(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	databaseName string,
) (models.ScheduledBackupPolicy, error) {
	var policyID uuid.UUID
	if err := db.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id").
		Where("policy.target_type = 'resource'").
		Where("policy.resource_id = ?", resourceID).
		Where("policy.target ->> 'database' = ?", databaseName).
		Where("policy.archived_at IS NULL AND policy.activated_at IS NOT NULL").
		Limit(1).
		Scan(ctx, &policyID); err != nil {
		return models.ScheduledBackupPolicy{}, err
	}
	return models.BackupPolicy.FindScheduled(ctx, db, policyID)
}

func advanceDatabaseRestoreAfterSafetyBackup(
	ctx context.Context,
	tx bun.Tx,
	queue storage.InsertQueue,
	backupID uuid.UUID,
) error {
	restoreID, err := models.ResourceRestore.MarkRestoringBySafetyBackup(
		ctx,
		tx,
		backupID,
		time.Now().UTC(),
	)
	if err != nil || restoreID == nil {
		return err
	}
	_, err = queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.DatabaseRestoreApplyArgs{DatabaseRestoreID: *restoreID},
		nil,
	)
	return err
}

func failDatabaseRestoreSafetyBackup(
	ctx context.Context,
	db storage.Pool,
	backupID uuid.UUID,
	operationErr error,
) error {
	var restore models.ResourceRestoreEntity
	if err := db.Executor().
		NewSelect().
		Model(&restore).
		Where("resource_restore.safety_backup_id = ?", backupID).
		Where("resource_restore.status = ?", models.ResourceRestoreStatusSafetyBackup).
		Scan(ctx); errors.Is(
		err,
		sql.ErrNoRows,
	) {
		return nil
	} else if err != nil {
		return err
	}
	service := DatabaseRestoreWorkflow{db: db}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return service.failTx(
		ctx,
		tx,
		restore,
		fmt.Errorf("safety backup failed: %w", operationErr),
		false,
		nil,
	)
}
func mapRestoreConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "resource-active-restore") {
		return domainError(
			"backupId",
			"restore_active",
			"A restore is already active for this Database",
		)
	}
	return err
}
