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
	Backup                 models.BackupEntity
	PolicyRetention        json.RawMessage
	PolicyVerification     json.RawMessage
	PolicySettings         json.RawMessage
	DestinationProvider    string
	DestinationEndpoint    string
	DestinationRegion      string
	DestinationBucket      string
	DestinationPrefix      string
	DestinationPathStyle   bool
	CredentialProvider     string
	CredentialPayload      []byte
	BindingResourceID      *uuid.UUID
	EndpointResourceID     *uuid.UUID
	EndpointInstallationID *uuid.UUID
	InstallationResourceID *uuid.UUID
	ResourceKind           string
	DatabaseExternal       bool
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
	if err := models.ChangeTask.MarkCompleted(
		ctx, tx, scope.Backup.ChangeTaskID, time.Now().UTC(),
	); err != nil {
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
	if scope.CredentialProvider != "backup_"+scope.DestinationProvider {
		return errors.New("backup credential provider does not match its destination")
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
	if scope.Backup.ResourceID == nil || scope.Backup.EnvironmentResourceID == nil ||
		scope.Backup.ResourceInstallationID == nil || scope.BindingResourceID == nil ||
		scope.EndpointResourceID == nil || scope.EndpointInstallationID == nil ||
		scope.InstallationResourceID == nil ||
		*scope.BindingResourceID != *scope.Backup.ResourceID ||
		*scope.EndpointResourceID != *scope.Backup.ResourceID ||
		*scope.InstallationResourceID != *scope.Backup.ResourceID ||
		*scope.EndpointInstallationID != *scope.Backup.ResourceInstallationID ||
		scope.ResourceKind != "postgresql" {
		return errors.New("database backup target is not the local PostgreSQL resource")
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
	if err := models.Change.MarkRunning(ctx, db, backup.ChangeID, now); err != nil {
		return err
	}
	return models.ChangeTask.MarkRunning(ctx, db, backup.ChangeTaskID, now)
}

func markBackupLifecycleFailed(
	ctx context.Context,
	db storage.Executor,
	backup models.BackupEntity,
	operationErr error,
) error {
	now := time.Now().UTC()
	if err := models.ChangeTask.MarkFailed(ctx, db, backup.ChangeTaskID, now); err != nil {
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
	return models.Change.MarkFailed(
		ctx, db, backup.ChangeID, operationErr, time.Now().UTC(),
	)
}

func (service *BackupExecutor) loadScope(ctx context.Context, backupID uuid.UUID) (BackupScope, error) {
	row, err := models.Backup.FindExecutionScope(ctx, service.db.Executor(), backupID)
	if err != nil {
		return BackupScope{}, fmt.Errorf("load backup scope: %w", err)
	}
	return BackupScope{
		Backup: row.Backup, PolicyRetention: row.PolicyRetention,
		PolicyVerification: row.PolicyVerification, PolicySettings: row.PolicySettings,
		DestinationProvider: row.DestinationProvider, DestinationEndpoint: row.DestinationEndpoint,
		DestinationRegion: row.DestinationRegion, DestinationBucket: row.DestinationBucket,
		DestinationPrefix: row.DestinationPrefix, DestinationPathStyle: row.DestinationPathStyle,
		CredentialProvider: row.CredentialProvider, CredentialPayload: row.CredentialPayload,
		BindingResourceID: row.BindingResourceID, EndpointResourceID: row.EndpointResourceID,
		EndpointInstallationID: row.EndpointInstallationID,
		InstallationResourceID: row.InstallationResourceID, ResourceKind: row.ResourceKind,
		DatabaseExternal: row.DatabaseExternal,
	}, nil
}
