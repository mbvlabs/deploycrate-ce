package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	Backup                  models.BackupEntity
	PolicyRetention         json.RawMessage
	PolicyVerification      json.RawMessage
	PolicySettings          json.RawMessage
	DestinationProvider     string
	DestinationEndpoint     string
	DestinationRegion       string
	DestinationBucket       string
	DestinationPrefix       string
	DestinationPathStyle    bool
	DestinationArchived     bool
	CredentialProvider      string
	CredentialPayload       []byte
	DestinationCredentialID uuid.UUID
	CredentialArchived      bool
	CredentialVerified      bool
	ResourceInstallationID  *uuid.UUID
	InstallationContainer   string
	InstallationServerID    *uuid.UUID
	InstallationServerIPv4  string
	InstallationArchived    bool
	ResourceEngine          string
	ResourceSystemManaged   bool
	ResourceArchived        bool
	ResourceID              *uuid.UUID
	DatabaseName            string
	AdministratorUsername   string
	AdministratorPayload    []byte
	AdministratorCount      int
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
	Run(
		context.Context,
		BackupScope,
		PostgreSQLBackupTarget,
		BackupCredentialPayload,
		objectstorage.Store,
	) (BackupArtifact, error)
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
		return service.recordPreflightFailure(
			ctx,
			scope.Backup,
			fmt.Errorf("validate backup scope: %w", err),
		)
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
		return service.recordPreflightFailure(
			ctx,
			scope.Backup,
			errors.New("decode backup credential"),
		)
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
			_ = markBackupLifecycleFailed(
				persistCtx,
				service.db.Executor(),
				scope.Backup,
				returnErr,
			)
			_ = failDatabaseRestoreSafetyBackup(persistCtx, service.db, backupID, returnErr)
		}
	}()
	if err := markBackupExecutionStarted(ctx, service.db.Executor(), scope.Backup); err != nil {
		return fmt.Errorf("start backup lifecycle: %w", err)
	}

	var artifact BackupArtifact
	if scope.Backup.TargetType == "server" {
		artifact, err = service.server.Run(ctx, scope, credential)
	} else {
		target, targetErr := service.postgreSQLTarget(scope)
		if targetErr != nil {
			return targetErr
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
		artifact, err = service.database.Run(ctx, scope, target, credential, store)
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
	if _, err := tx.NewUpdate().Table("credentials").
		Set("last_used_at = ?", time.Now().UTC()).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", scope.DestinationCredentialID).
		Where("archived_at IS NULL").
		Exec(ctx); err != nil {
		return fmt.Errorf("record backup destination use: %w", err)
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
		failDatabaseRestoreSafetyBackup(persistCtx, service.db, backup.ID, operationErr),
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
	if scope.DestinationArchived || scope.CredentialArchived || !scope.CredentialVerified {
		return errors.New("backup destination and credential must be active and verified")
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
	if scope.Backup.ResourceID == nil || scope.ResourceID == nil || *scope.ResourceID != *scope.Backup.ResourceID ||
		scope.InstallationServerID == nil || scope.Backup.ResourceInstallationID == nil ||
		scope.ResourceInstallationID == nil || *scope.ResourceInstallationID != *scope.Backup.ResourceInstallationID ||
		scope.InstallationArchived || scope.ResourceArchived || scope.ResourceEngine != "postgresql" ||
		strings.TrimSpace(scope.DatabaseName) == "" || strings.TrimSpace(scope.InstallationContainer) == "" ||
		scope.AdministratorCount != 1 ||
		strings.TrimSpace(scope.AdministratorUsername) == "" ||
		len(scope.AdministratorPayload) < 2 {
		return errors.New("database backup target is not the local PostgreSQL resource")
	}
	if scope.Backup.Strategy != "logical" || scope.Backup.Driver != "postgresql" ||
		scope.Backup.Format != "tar.age" {
		return errors.New("database backup driver scope is incompatible")
	}
	return nil
}

func (service *BackupExecutor) postgreSQLTarget(scope BackupScope) (PostgreSQLBackupTarget, error) {
	if err := validateBackupScope(scope); err != nil {
		return PostgreSQLBackupTarget{}, err
	}
	plaintext, err := secretcrypto.DecryptForPurpose(
		scope.AdministratorPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return PostgreSQLBackupTarget{}, errors.New(
			"decrypt Database Cluster administrator credential",
		)
	}
	defer clear(plaintext)
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 ||
		strings.TrimSpace(payload.Values["password"]) == "" {
		return PostgreSQLBackupTarget{}, errors.New(
			"Resource administrator credential is incomplete",
		)
	}
	return PostgreSQLBackupTarget{
		ResourceID: *scope.Backup.ResourceID, InstallationID: *scope.Backup.ResourceInstallationID,
		ServerID:      *scope.InstallationServerID,
		ContainerName: scope.InstallationContainer, DatabaseName: scope.DatabaseName,
		Username: scope.AdministratorUsername, Password: payload.Values["password"],
		ExcludeRiverTableData: scope.ResourceSystemManaged,
	}, nil
}

func (service *BackupExecutor) postgreSQLTargetForDatabase(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
) (PostgreSQLBackupTarget, error) {
	var target struct {
		ResourceID           uuid.UUID `bun:"resource_id"`
		InstallationID       uuid.UUID `bun:"installation_id"`
		ServerID             uuid.UUID `bun:"server_id"`
		ContainerName        string    `bun:"container_name"`
		Username             string    `bun:"username"`
		AdministratorPayload []byte    `bun:"administrator_payload"`
		SystemManaged        bool      `bun:"system_managed"`
	}
	err := service.db.Executor().NewSelect().TableExpr("resources AS resource").
		ColumnExpr("resource.id AS resource_id, installation.id AS installation_id, installation.server_id, installation.container_name").
		ColumnExpr("resource.system_managed").
		ColumnExpr("administrator.username, administrator.enc_payload AS administrator_payload").
		Join("JOIN resource_installations AS installation ON installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Join("JOIN resource_credentials AS administrator ON administrator.resource_id = resource.id AND administrator.metadata ->> 'purpose' = 'administrator' AND administrator.archived_at IS NULL").
		Where("resource.id = ?", resourceID).
		Where("resource.configuration ->> 'engine' = 'postgresql'").
		Where("resource.archived_at IS NULL").Scan(ctx, &target)
	if err != nil {
		return PostgreSQLBackupTarget{}, err
	}
	plaintext, err := secretcrypto.DecryptForPurpose(
		target.AdministratorPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return PostgreSQLBackupTarget{}, errors.New("decrypt Resource administrator credential")
	}
	defer clear(plaintext)
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 ||
		strings.TrimSpace(payload.Values["password"]) == "" {
		return PostgreSQLBackupTarget{}, errors.New(
			"Resource administrator credential is incomplete",
		)
	}
	return PostgreSQLBackupTarget{
		ResourceID:            target.ResourceID,
		InstallationID:        target.InstallationID,
		ServerID:              target.ServerID,
		ContainerName:         target.ContainerName,
		DatabaseName:          databaseName,
		Username:              target.Username,
		Password:              payload.Values["password"],
		ExcludeRiverTableData: target.SystemManaged,
	}, nil
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

func (service *BackupExecutor) loadScope(
	ctx context.Context,
	backupID uuid.UUID,
) (BackupScope, error) {
	row, err := models.Backup.FindExecutionScope(ctx, service.db.Executor(), backupID)
	if err != nil {
		return BackupScope{}, fmt.Errorf("load backup scope: %w", err)
	}
	return BackupScope{
		Backup:                  row.Backup,
		PolicyRetention:         row.PolicyRetention,
		PolicyVerification:      row.PolicyVerification,
		PolicySettings:          row.PolicySettings,
		DestinationProvider:     row.DestinationProvider,
		DestinationEndpoint:     row.DestinationEndpoint,
		DestinationRegion:       row.DestinationRegion,
		DestinationBucket:       row.DestinationBucket,
		DestinationPrefix:       row.DestinationPrefix,
		DestinationPathStyle:    row.DestinationPathStyle,
		DestinationArchived:     row.DestinationArchived,
		CredentialProvider:      row.CredentialProvider,
		CredentialPayload:       row.CredentialPayload,
		DestinationCredentialID: row.DestinationCredentialID,
		CredentialArchived:      row.CredentialArchived,
		CredentialVerified:      row.CredentialVerified,
		ResourceInstallationID:  row.ResourceInstallationID,
		InstallationContainer:   row.InstallationContainer,
		InstallationServerID:    row.InstallationServerID,
		InstallationServerIPv4:  row.InstallationServerIPv4,
		InstallationArchived:    row.InstallationArchived,
		ResourceEngine:          row.ResourceEngine,
		ResourceSystemManaged:   row.ResourceSystemManaged,
		ResourceArchived:        row.ResourceArchived,
		ResourceID:              row.ResourceID,
		DatabaseName:            row.DatabaseName,
		AdministratorUsername:   row.AdministratorUsername,
		AdministratorPayload:    row.AdministratorPayload,
		AdministratorCount:      row.AdministratorCount,
	}, nil
}
