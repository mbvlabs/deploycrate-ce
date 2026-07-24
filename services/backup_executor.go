package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
)

type BackupCredentialPayload struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ResticPassword  string `json:"restic_password"`
	AgeIdentity     string `json:"age_identity"`
}

type BackupScope struct {
	Backup               models.BackupEntity
	PolicyRetention      json.RawMessage
	PolicyVerification   json.RawMessage
	PolicySettings       json.RawMessage
	DestinationProvider  string
	DestinationEndpoint  string
	DestinationRegion    string
	DestinationBucket    string
	DestinationPrefix    string
	DestinationPathStyle bool
	CredentialPayload    []byte
	DatabaseExternal     bool
}

func (scope BackupScope) ObjectStorageConfig() objectstorage.Config {
	return objectstorage.Config{
		Provider: scope.DestinationProvider, Endpoint: scope.DestinationEndpoint,
		Region: scope.DestinationRegion, Bucket: scope.DestinationBucket,
		Prefix: scope.DestinationPrefix, ForcePathStyle: scope.DestinationPathStyle,
	}
}

type BackupArtifact struct {
	Reference string
	Metadata  json.RawMessage
	Size      int64
	Digest    []byte
}

type ServerBackupRunner interface {
	Run(context.Context, BackupScope, BackupCredentialPayload) (BackupArtifact, error)
}

type DatabaseBackupRunner interface {
	Run(context.Context, BackupScope, BackupCredentialPayload, objectstorage.Store) (BackupArtifact, error)
}

type BackupExecutor struct {
	db       storage.Pool
	queue    storage.InsertQueue
	config   config.Config
	server   ServerBackupRunner
	database DatabaseBackupRunner
}

func NewBackupExecutor(
	db storage.Pool,
	queue storage.InsertQueue,
	configuration config.Config,
	server *ServerBackup,
	database *DatabaseBackup,
) *BackupExecutor {
	return &BackupExecutor{
		db: db, queue: queue, config: configuration, server: server, database: database,
	}
}

func (service *BackupExecutor) Execute(ctx context.Context, backupID uuid.UUID) (returnErr error) {
	scope, err := service.loadScope(ctx, backupID)
	if err != nil {
		return err
	}
	if scope.Backup.Status == models.BackupStatusUploaded ||
		scope.Backup.Status == models.BackupStatusVerified ||
		scope.Backup.Status == models.BackupStatusVerificationFailed ||
		scope.Backup.Status == models.BackupStatusPruned {
		return nil
	}
	if err := validateBackupScope(scope); err != nil {
		return service.recordPreflightFailure(ctx, scope.Backup, fmt.Errorf("validate backup scope: %w", err))
	}
	plaintext, err := secretcrypto.Decrypt(
		scope.CredentialPayload,
		service.config.App.SessionEncryptionKey,
	)
	if err != nil {
		return service.recordPreflightFailure(
			ctx,
			scope.Backup,
			fmt.Errorf("decrypt backup credential: %w", err),
		)
	}
	defer clear(plaintext)
	var credential BackupCredentialPayload
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		return service.recordPreflightFailure(ctx, scope.Backup, errors.New("decode backup credential"))
	}
	if credential.AccessKeyID == "" || credential.SecretAccessKey == "" ||
		credential.ResticPassword == "" || credential.AgeIdentity == "" {
		return service.recordPreflightFailure(
			ctx,
			scope.Backup,
			errors.New("backup credential payload is incomplete"),
		)
	}
	claimed, err := models.Backup.Claim(ctx, service.db.Executor(), backupID)
	if err != nil {
		return fmt.Errorf("claim backup %s: %w", backupID, err)
	}
	if !claimed {
		return nil
	}
	defer func() {
		if returnErr != nil {
			persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_ = models.Backup.MarkFailed(persistCtx, service.db.Executor(), backupID, returnErr)
			_ = markBackupLifecycleFailed(persistCtx, service.db.Executor(), scope.Backup, returnErr)
		}
	}()
	if err := markBackupExecutionStarted(ctx, service.db.Executor(), scope.Backup); err != nil {
		return fmt.Errorf("start backup lifecycle: %w", err)
	}

	var artifact BackupArtifact
	if scope.Backup.TargetType == "server" {
		artifact, err = service.server.Run(ctx, scope, credential)
	} else {
		if scope.DatabaseExternal {
			return errors.New("externally managed databases cannot be backed up")
		}
		store, storeErr := objectstorage.New(
			ctx,
			scope.ObjectStorageConfig(),
			objectstorage.Credentials{
				AccessKeyID: credential.AccessKeyID, SecretAccessKey: credential.SecretAccessKey,
			},
		)
		if storeErr != nil {
			return storeErr
		}
		artifact, err = service.database.Run(ctx, scope, credential, store)
	}
	if err != nil {
		return err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.Backup.MarkUploaded(ctx, tx, backupID, models.UploadedBackupData{
		ArtifactReference: artifact.Reference,
		ProviderMetadata:  artifact.Metadata,
		SizeBytes:         artifact.Size,
		Digest:            artifact.Digest,
	}); err != nil {
		return fmt.Errorf("record uploaded backup: %w", err)
	}
	if _, err := tx.NewUpdate().TableExpr("change_tasks").
		Set("status = ?", "completed").
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", scope.Backup.ChangeTaskID).
		Exec(ctx); err != nil {
		return fmt.Errorf("complete backup execution task: %w", err)
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupVerifyArgs{BackupID: backupID},
		nil,
	); err != nil {
		return fmt.Errorf("insert backup verification job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit uploaded backup: %w", err)
	}
	slog.InfoContext(
		ctx,
		"backup artifact uploaded",
		"backup_id", backupID,
		"backup_policy_id", scope.Backup.BackupPolicyID,
		"target_type", scope.Backup.TargetType,
		"target_id", backupTargetID(scope.Backup),
		"driver", scope.Backup.Driver,
		"trigger_type", scope.Backup.TriggerType,
		"lifecycle_status", models.BackupStatusUploaded,
		"bytes_uploaded", artifact.Size,
	)
	return nil
}

func backupTargetID(backup models.BackupEntity) string {
	if backup.ServerID != nil {
		return backup.ServerID.String()
	}
	if backup.ResourceID != nil {
		return backup.ResourceID.String()
	}
	return ""
}

func (service *BackupExecutor) recordPreflightFailure(
	ctx context.Context,
	backup models.BackupEntity,
	operationErr error,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return errors.Join(
		operationErr,
		models.Backup.MarkFailed(persistCtx, service.db.Executor(), backup.ID, operationErr),
		markBackupLifecycleFailed(persistCtx, service.db.Executor(), backup, operationErr),
	)
}

func validateBackupScope(scope BackupScope) error {
	if err := scope.Backup.Validate(); err != nil {
		return err
	}
	if _, err := objectstorage.Normalize(scope.ObjectStorageConfig()); err != nil {
		return err
	}
	if !json.Valid(scope.PolicyRetention) || !json.Valid(scope.PolicyVerification) ||
		!json.Valid(scope.PolicySettings) || len(scope.CredentialPayload) < 2 {
		return errors.New("backup policy or credential scope is incomplete")
	}
	if scope.Backup.TargetType == "server" {
		if scope.Backup.Strategy != "filesystem" || scope.Backup.Driver != "restic" ||
			scope.Backup.Format != "restic" {
			return errors.New("server backup driver scope is incompatible")
		}
		return nil
	}
	if scope.DatabaseExternal {
		return errors.New("externally managed databases cannot be backed up")
	}
	if scope.Backup.Strategy != "logical" || scope.Backup.Driver != "postgresql" ||
		scope.Backup.Format != "tar.age" {
		return errors.New("database backup driver scope is incompatible")
	}
	return nil
}

func markBackupExecutionStarted(
	ctx context.Context,
	db storage.Executor,
	backup models.BackupEntity,
) error {
	now := time.Now().UTC()
	if _, err := db.NewUpdate().TableExpr("changes").
		Set("status = ?", "running").
		Set("started_at = COALESCE(started_at, ?)", now).
		Set("finished_at = NULL").
		Set("error = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", backup.ChangeID).
		Exec(ctx); err != nil {
		return err
	}
	_, err := db.NewUpdate().TableExpr("change_tasks").
		Set("status = ?", "running").
		Set("attempt_count = attempt_count + 1").
		Set("updated_at = ?", now).
		Where("id = ?", backup.ChangeTaskID).
		Exec(ctx)
	return err
}

func markBackupLifecycleFailed(
	ctx context.Context,
	db storage.Executor,
	backup models.BackupEntity,
	operationErr error,
) error {
	now := time.Now().UTC()
	if _, err := db.NewUpdate().TableExpr("change_tasks").
		Set("status = ?", "failed").
		Set("updated_at = ?", now).
		Where("id = ?", backup.ChangeTaskID).
		Exec(ctx); err != nil {
		return err
	}
	return markBackupChangeFailed(ctx, db, backup, operationErr)
}

func markBackupChangeFailed(
	ctx context.Context,
	db storage.Executor,
	backup models.BackupEntity,
	operationErr error,
) error {
	now := time.Now().UTC()
	_, err := db.NewUpdate().TableExpr("changes").
		Set("status = ?", "failed").
		Set("finished_at = ?", now).
		Set("error = ?", operationErr.Error()).
		Set("updated_at = ?", now).
		Where("id = ?", backup.ChangeID).
		Exec(ctx)
	return err
}

func (service *BackupExecutor) loadScope(ctx context.Context, backupID uuid.UUID) (BackupScope, error) {
	type scopeRow struct {
		models.BackupEntity  `bun:"embed:backup_"`
		PolicyRetention      json.RawMessage `bun:"policy_retention"`
		PolicyVerification   json.RawMessage `bun:"policy_verification"`
		PolicySettings       json.RawMessage `bun:"policy_settings"`
		DestinationProvider  string          `bun:"destination_provider"`
		DestinationEndpoint  string          `bun:"destination_endpoint"`
		DestinationRegion    string          `bun:"destination_region"`
		DestinationBucket    string          `bun:"destination_bucket"`
		DestinationPrefix    string          `bun:"destination_prefix"`
		DestinationPathStyle bool            `bun:"destination_path_style"`
		CredentialPayload    []byte          `bun:"credential_payload"`
		DatabaseExternal     bool            `bun:"database_external"`
	}
	var row scopeRow
	if err := service.db.Executor().NewSelect().
		TableExpr("backups AS backup").
		ColumnExpr("backup.id AS backup_id, backup.created_at AS backup_created_at, backup.updated_at AS backup_updated_at").
		ColumnExpr("backup.target_type AS backup_target_type, backup.trigger_type AS backup_trigger_type, backup.scheduled_at AS backup_scheduled_at").
		ColumnExpr("backup.strategy AS backup_strategy, backup.driver AS backup_driver, backup.format AS backup_format, backup.format_version AS backup_format_version").
		ColumnExpr("backup.artifact_reference AS backup_artifact_reference, backup.provider_metadata AS backup_provider_metadata, backup.status AS backup_status").
		ColumnExpr("backup.requested_at AS backup_requested_at, backup.started_at AS backup_started_at, backup.uploaded_at AS backup_uploaded_at").
		ColumnExpr("backup.finished_at AS backup_finished_at, backup.verified_at AS backup_verified_at, backup.pruned_at AS backup_pruned_at").
		ColumnExpr("backup.size_bytes AS backup_size_bytes, backup.digest AS backup_digest, backup.producer_version AS backup_producer_version, backup.error AS backup_error").
		ColumnExpr("backup.change_id AS backup_change_id, backup.change_task_id AS backup_change_task_id, backup.backup_policy_id AS backup_backup_policy_id").
		ColumnExpr("backup.server_id AS backup_server_id, backup.resource_id AS backup_resource_id, backup.environment_resource_id AS backup_environment_resource_id").
		ColumnExpr("backup.resource_installation_id AS backup_resource_installation_id, backup.resource_volume_id AS backup_resource_volume_id").
		ColumnExpr("backup.backup_destination_id AS backup_backup_destination_id").
		ColumnExpr("policy.retention AS policy_retention, policy.verification AS policy_verification, policy.settings AS policy_settings").
		ColumnExpr("destination.provider AS destination_provider, COALESCE(destination.endpoint, '') AS destination_endpoint").
		ColumnExpr("COALESCE(destination.region, '') AS destination_region, destination.bucket AS destination_bucket").
		ColumnExpr("COALESCE(destination.prefix, '') AS destination_prefix, destination.force_path_style AS destination_path_style").
		ColumnExpr("credential.enc_payload AS credential_payload").
		ColumnExpr("COALESCE((endpoint.settings ->> 'external')::boolean, FALSE) AS database_external").
		Join("JOIN backup_policies AS policy ON policy.id = backup.backup_policy_id AND policy.archived_at IS NULL").
		Join("JOIN backup_destinations AS destination ON destination.id = backup.backup_destination_id AND destination.archived_at IS NULL").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id AND credential.archived_at IS NULL").
		Join("LEFT JOIN environment_resources AS binding ON binding.id = backup.environment_resource_id").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = binding.resource_endpoint_id").
		Where("backup.id = ?", backupID).
		Scan(ctx, &row); err != nil {
		return BackupScope{}, fmt.Errorf("load backup scope: %w", err)
	}
	return BackupScope{
		Backup: row.BackupEntity, PolicyRetention: row.PolicyRetention,
		PolicyVerification: row.PolicyVerification, PolicySettings: row.PolicySettings,
		DestinationProvider: row.DestinationProvider, DestinationEndpoint: row.DestinationEndpoint,
		DestinationRegion: row.DestinationRegion, DestinationBucket: row.DestinationBucket,
		DestinationPrefix: row.DestinationPrefix, DestinationPathStyle: row.DestinationPathStyle,
		CredentialPayload: row.CredentialPayload, DatabaseExternal: row.DatabaseExternal,
	}, nil
}
