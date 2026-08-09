package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"filippo.io/age"
	"github.com/google/uuid"
)

type DatabaseBackups struct {
	db        storage.Pool
	scheduler *BackupScheduler
	config    config.Config
	identity  Identity
}

func NewDatabaseBackups(
	db storage.Pool,
	scheduler *BackupScheduler,
	configuration config.Config,
	identity Identity,
) *DatabaseBackups {
	return &DatabaseBackups{db: db, scheduler: scheduler, config: configuration, identity: identity}
}

type ObjectStorageDestinationInput struct {
	Name            string
	Provider        string
	Endpoint        string
	Region          string
	Bucket          string
	Prefix          string
	ForcePathStyle  bool
	AccessKeyID     string
	SecretAccessKey string
}

type ObjectStorageRecoveryMaterial struct {
	ResticPassword string `json:"resticPassword"`
	AgeIdentity    string `json:"ageIdentity"`
}

type DatabaseBackupPolicyInput struct {
	Schedule                                     string
	KeepLast, KeepDaily, KeepWeekly, KeepMonthly int
	BackupDestinationID                          uuid.UUID
}

func (service *DatabaseBackups) DetailsForResource(
	ctx context.Context,
	resourceID uuid.UUID,
) (models.ResourceBackupCatalog, error) {
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return models.ResourceBackupCatalog{}, err
	}
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
	if err != nil {
		return models.ResourceBackupCatalog{}, err
	}
	details := make([]models.ResourceBackupDetails, 0, len(resource.Databases()))
	for _, database := range resource.Databases() {
		eligibility, eligibilityErr := service.eligibility(
			ctx,
			service.db.Executor(),
			resource,
			database.Name,
		)
		if eligibilityErr != nil {
			return models.ResourceBackupCatalog{}, eligibilityErr
		}
		policy, policyErr := models.BackupPolicy.FindForResourceDatabase(
			ctx,
			service.db.Executor(),
			resourceID,
			database.Name,
		)
		var activePolicy *models.BackupPolicyEntity
		if policyErr == nil {
			activePolicy = &policy
		} else if !errors.Is(policyErr, sql.ErrNoRows) {
			return models.ResourceBackupCatalog{}, policyErr
		}
		history, historyErr := models.Backup.RecentForResourceDatabase(
			ctx,
			service.db.Executor(),
			resourceID,
			database.Name,
			10,
		)
		if historyErr != nil {
			return models.ResourceBackupCatalog{}, historyErr
		}
		restores, restoreErr := models.ResourceRestore.RecentForResourceDatabase(
			ctx,
			service.db.Executor(),
			resourceID,
			database.Name,
			10,
		)
		if restoreErr != nil {
			return models.ResourceBackupCatalog{}, restoreErr
		}
		details = append(
			details,
			models.ResourceBackupDetails{
				DatabaseName: database.Name,
				Eligibility:  eligibility,
				Policy:       activePolicy,
				History:      history,
				Restores:     restores,
			},
		)
	}
	return models.ResourceBackupCatalog{Destinations: destinations, Databases: details}, nil
}

func (service *DatabaseBackups) Destinations(
	ctx context.Context,
) ([]models.BackupDestinationSummary, error) {
	return models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
}

func (service *DatabaseBackups) Destination(
	ctx context.Context,
	id uuid.UUID,
) (models.BackupDestinationSummary, error) {
	return models.BackupDestination.ActiveSummary(ctx, service.db.Executor(), id)
}

func (service *DatabaseBackups) CreateDestination(
	ctx context.Context,
	input ObjectStorageDestinationInput,
) (models.BackupDestinationEntity, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.AccessKeyID = strings.TrimSpace(input.AccessKeyID)
	input.SecretAccessKey = strings.TrimSpace(input.SecretAccessKey)

	builder := validation.NewBuilder()
	builder.Required("name", input.Name)
	builder.Required("provider", input.Provider)
	builder.Required("bucket", input.Bucket)
	builder.Required("accessKeyId", input.AccessKeyID)
	builder.Required("secretAccessKey", input.SecretAccessKey)
	if len(input.Name) > 120 {
		builder.Add("name", "too_long", "Destination name must be 120 characters or fewer")
	}
	if input.Provider != objectstorage.ProviderS3 && input.Provider != objectstorage.ProviderR2 {
		builder.Add("provider", "unsupported", "Choose Amazon S3 or Cloudflare R2")
	}
	if input.Provider == objectstorage.ProviderS3 && strings.TrimSpace(input.Region) == "" {
		builder.Add("region", "required", "S3 region is required")
	}
	if input.Provider == objectstorage.ProviderR2 && strings.TrimSpace(input.Endpoint) == "" {
		builder.Add("endpoint", "required", "Cloudflare R2 endpoint is required")
	}
	if len(input.AccessKeyID) > 4096 {
		builder.Add("accessKeyId", "too_long", "Access key ID is too large")
	}
	if len(input.SecretAccessKey) > 16384 {
		builder.Add("secretAccessKey", "too_long", "Secret access key is too large")
	}
	if err := builder.Err(); err != nil {
		return models.BackupDestinationEntity{}, errors.Join(models.ErrDomainValidation, err)
	}

	normalized, err := objectstorage.Normalize(objectstorage.Config{
		Provider: input.Provider, Endpoint: input.Endpoint, Region: input.Region,
		Bucket: input.Bucket, Prefix: input.Prefix, ForcePathStyle: input.ForcePathStyle,
	})
	if err != nil {
		field := "endpoint"
		switch {
		case strings.Contains(err.Error(), "provider"):
			field = "provider"
		case strings.Contains(err.Error(), "bucket"):
			field = "bucket"
		case strings.Contains(err.Error(), "region"):
			field = "region"
		case strings.Contains(err.Error(), "prefix"):
			field = "prefix"
		}
		return models.BackupDestinationEntity{}, domainError(field, "invalid", err.Error())
	}

	store, err := objectstorage.New(ctx, normalized, objectstorage.Credentials{
		AccessKeyID: input.AccessKeyID, SecretAccessKey: input.SecretAccessKey,
	})
	if err != nil {
		return models.BackupDestinationEntity{}, domainError(
			"secretAccessKey",
			"unverified",
			"Object Storage credentials could not be configured",
		)
	}
	if err := store.Probe(ctx, service.config.App.InstanceID); err != nil {
		return models.BackupDestinationEntity{}, domainError(
			"secretAccessKey",
			"unverified",
			"Credentials could not read and write the configured bucket",
		)
	}

	resticPassword, err := backupRandomHex(32)
	if err != nil {
		return models.BackupDestinationEntity{}, fmt.Errorf(
			"generate Object Storage recovery password: %w",
			err,
		)
	}
	ageIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		return models.BackupDestinationEntity{}, fmt.Errorf(
			"generate Object Storage recovery identity: %w",
			err,
		)
	}
	payload, err := json.Marshal(BackupCredentialPayload{
		AccessKeyID: input.AccessKeyID, SecretAccessKey: input.SecretAccessKey,
		ResticPassword: resticPassword, AgeIdentity: ageIdentity.String(),
	})
	if err != nil {
		return models.BackupDestinationEntity{}, err
	}
	defer clear(payload)
	encrypted, err := secretcrypto.Encrypt(payload, service.config.App.SessionEncryptionKey)
	if err != nil {
		return models.BackupDestinationEntity{}, fmt.Errorf(
			"encrypt Object Storage credential: %w",
			err,
		)
	}
	metadata, err := json.Marshal(map[string]any{
		"schema_version": 1, "credential_kind": "object_storage_backup_access",
		"instance_id": service.config.App.InstanceID, "provider": normalized.Provider,
		"endpoint": normalized.Endpoint, "region": normalized.Region, "bucket": normalized.Bucket,
		"prefix": normalized.Prefix, "force_path_style": normalized.ForcePathStyle,
	})
	if err != nil {
		return models.BackupDestinationEntity{}, err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupDestinationEntity{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	credential, err := models.Credential.Create(ctx, tx, models.CreateCredentialData{
		Name: input.Name, Provider: "backup_" + normalized.Provider, Metadata: metadata,
		EncPayload: encrypted, VerifiedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return models.BackupDestinationEntity{}, err
	}
	destination, err := models.BackupDestination.Create(ctx, tx, models.CreateBackupDestinationData{
		Name:     input.Name,
		Provider: normalized.Provider,
		Endpoint: sql.NullString{
			String: normalized.Endpoint,
			Valid:  normalized.Endpoint != "",
		},
		Region:         sql.NullString{String: normalized.Region, Valid: normalized.Region != ""},
		Bucket:         normalized.Bucket,
		Prefix:         sql.NullString{String: normalized.Prefix, Valid: normalized.Prefix != ""},
		ForcePathStyle: normalized.ForcePathStyle,
		CredentialID:   credential.ID,
	})
	if err != nil {
		return models.BackupDestinationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.BackupDestinationEntity{}, err
	}
	return destination, nil
}

func (service *DatabaseBackups) RecoveryMaterial(
	ctx context.Context,
	destinationID, userID uuid.UUID,
	password string,
) (ObjectStorageRecoveryMaterial, error) {
	if err := service.identity.VerifyUserPassword(ctx, userID, password); err != nil {
		return ObjectStorageRecoveryMaterial{}, err
	}
	encPayload, err := models.BackupDestination.ActiveVerifiedCredentialPayload(
		ctx,
		service.db.Executor(),
		destinationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ObjectStorageRecoveryMaterial{}, models.ErrNotFound
	}
	if err != nil {
		return ObjectStorageRecoveryMaterial{}, err
	}
	plaintext, err := secretcrypto.Decrypt(encPayload, service.config.App.SessionEncryptionKey)
	if err != nil {
		return ObjectStorageRecoveryMaterial{}, errors.New(
			"Object Storage recovery material could not be decrypted",
		)
	}
	defer clear(plaintext)
	var credential BackupCredentialPayload
	if json.Unmarshal(plaintext, &credential) != nil || credential.ResticPassword == "" ||
		credential.AgeIdentity == "" {
		return ObjectStorageRecoveryMaterial{}, errors.New(
			"Object Storage recovery material is invalid",
		)
	}
	return ObjectStorageRecoveryMaterial{
		ResticPassword: credential.ResticPassword,
		AgeIdentity:    credential.AgeIdentity,
	}, nil
}

func backupRandomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (service *DatabaseBackups) CreateForResource(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	input DatabaseBackupPolicyInput,
) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	resource, err := models.Resource.Find(ctx, tx, resourceID)
	if err != nil || resource.ArchivedAt.Valid || !resourceHasDatabase(resource, databaseName) {
		return models.BackupPolicyEntity{}, models.ErrNotFound
	}
	eligibility, err := service.eligibility(ctx, tx, resource, databaseName)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if !eligibility.Eligible {
		return models.BackupPolicyEntity{}, domainError("backup", "ineligible", eligibility.Reason)
	}
	if err := service.validateDestination(ctx, tx, input.BackupDestinationID); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	now := time.Now().UTC()
	nextRunAt, err := models.NextBackupRun(input.Schedule, now)
	if err != nil {
		return models.BackupPolicyEntity{}, domainError(
			"schedule",
			"invalid",
			"Schedule must be a five-field cron expression",
		)
	}
	retention, err := databaseBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	target, _ := json.Marshal(map[string]string{"database": databaseName})
	policy, err := models.BackupPolicy.Create(ctx, tx, models.CreateBackupPolicyData{
		Name:         databaseName + " PostgreSQL",
		Schedule:     input.Schedule,
		Strategy:     "logical",
		Driver:       "postgresql",
		Format:       "tar.age",
		Retention:    retention,
		Verification: json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`),
		Settings:     json.RawMessage(`{}`),
		ActivatedAt: sql.NullTime{
			Time:  now,
			Valid: true,
		},
		TargetType:          "resource",
		Target:              target,
		ResourceID:          &resourceID,
		NextRunAt:           nextRunAt,
		BackupDestinationID: input.BackupDestinationID,
	})
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if err := service.scheduler.InsertScheduleTx(ctx, tx, policy.ID, nextRunAt); err != nil {
		return models.BackupPolicyEntity{}, fmt.Errorf(
			"insert first Resource backup schedule: %w",
			err,
		)
	}
	if err := tx.Commit(); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return policy, nil
}

func (service *DatabaseBackups) UpdateForResource(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	policyID uuid.UUID,
	input DatabaseBackupPolicyInput,
) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, resourceID, databaseName, policyID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	resource, err := models.Resource.Find(ctx, tx, resourceID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	eligibility, err := service.eligibility(ctx, tx, resource, databaseName)
	if err != nil || !eligibility.Eligible {
		if err != nil {
			return models.BackupPolicyEntity{}, err
		}
		return models.BackupPolicyEntity{}, domainError("backup", "ineligible", eligibility.Reason)
	}
	if err := service.validateDestination(ctx, tx, input.BackupDestinationID); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	nextRunAt, err := models.NextBackupRun(input.Schedule, time.Now().UTC())
	if err != nil {
		return models.BackupPolicyEntity{}, domainError(
			"schedule",
			"invalid",
			"Schedule must be a five-field cron expression",
		)
	}
	retention, err := databaseBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{
		ID:                  policy.ID,
		Name:                policy.Name,
		Schedule:            input.Schedule,
		Strategy:            policy.Strategy,
		Driver:              policy.Driver,
		Retention:           retention,
		Format:              policy.Format,
		Verification:        policy.Verification,
		Settings:            policy.Settings,
		ArchivedAt:          policy.ArchivedAt,
		ActivatedAt:         policy.ActivatedAt,
		TargetType:          policy.TargetType,
		Target:              policy.Target,
		ResourceID:          policy.ResourceID,
		NextRunAt:           nextRunAt,
		LastScheduledAt:     policy.LastScheduledAt,
		BackupDestinationID: input.BackupDestinationID,
	})
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if updated.Schedulable() {
		if err := service.scheduler.InsertScheduleTx(ctx, tx, updated.ID, nextRunAt); err != nil {
			return models.BackupPolicyEntity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return updated, nil
}

func (service *DatabaseBackups) SetStateForResource(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	policyID uuid.UUID,
	action string,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, resourceID, databaseName, policyID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	switch action {
	case "pause":
		policy.ActivatedAt = sql.NullTime{}
	case "resume":
		policy.ActivatedAt = sql.NullTime{Time: now, Valid: true}
		policy.NextRunAt, err = models.NextBackupRun(policy.Schedule, now)
	case "archive":
		policy.ActivatedAt = sql.NullTime{}
		policy.ArchivedAt = sql.NullTime{Time: now, Valid: true}
	default:
		return errors.New("unsupported Resource backup policy state change")
	}
	if err != nil {
		return err
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{
		ID:                  policy.ID,
		Name:                policy.Name,
		Schedule:            policy.Schedule,
		Strategy:            policy.Strategy,
		Driver:              policy.Driver,
		Retention:           policy.Retention,
		Format:              policy.Format,
		Verification:        policy.Verification,
		Settings:            policy.Settings,
		ArchivedAt:          policy.ArchivedAt,
		ActivatedAt:         policy.ActivatedAt,
		TargetType:          policy.TargetType,
		Target:              policy.Target,
		ResourceID:          policy.ResourceID,
		NextRunAt:           policy.NextRunAt,
		LastScheduledAt:     policy.LastScheduledAt,
		BackupDestinationID: policy.BackupDestinationID,
	})
	if err != nil {
		return err
	}
	if action == "resume" {
		if err := service.scheduler.InsertScheduleTx(
			ctx,
			tx,
			updated.ID,
			updated.NextRunAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (service *DatabaseBackups) ManualForResource(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
	policyID uuid.UUID,
) (uuid.UUID, error) {
	policy, err := service.loadPolicy(
		ctx,
		service.db.Executor(),
		resourceID,
		databaseName,
		policyID,
	)
	if err != nil {
		return uuid.Nil, err
	}
	return service.scheduler.Manual(ctx, policy.ID)
}

func (service *DatabaseBackups) loadPolicy(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	databaseName string,
	policyID uuid.UUID,
) (models.BackupPolicyEntity, error) {
	policy, err := models.BackupPolicy.FindForUpdate(ctx, db, policyID)
	if errors.Is(err, sql.ErrNoRows) ||
		err == nil &&
			(policy.ResourceID == nil || *policy.ResourceID != resourceID || policy.TargetType != "resource" || resourceCredentialMetadataDatabase(policy.Target) != databaseName || policy.ArchivedAt.Valid) {
		return models.BackupPolicyEntity{}, models.ErrNotFound
	}
	return policy, err
}

func (service *DatabaseBackups) validateDestination(
	ctx context.Context,
	db storage.Executor,
	destinationID uuid.UUID,
) error {
	active, err := models.BackupDestination.IsActiveVerified(ctx, db, destinationID)
	if err != nil {
		return err
	}
	count := 0
	if active {
		count = 1
	}
	return requireChild(
		count,
		"backupDestinationId",
		"Choose an active, verified Object Storage destination",
	)
}

func (service *DatabaseBackups) eligibility(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	databaseName string,
) (models.ResourceBackupEligibility, error) {
	ineligible := func(reason string) (models.ResourceBackupEligibility, error) {
		return models.ResourceBackupEligibility{Reason: reason}, nil
	}
	if resource.Engine() != "postgresql" || !resourceHasDatabase(resource, databaseName) {
		return ineligible("Logical backups currently support configured PostgreSQL databases only.")
	}
	installation, err := models.ResourceInstallation.FindActiveForResource(ctx, db, resource.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ineligible("This Resource has no active Docker installation.")
		}
		return models.ResourceBackupEligibility{}, err
	}
	if _, err := models.RequireServerCapability(
		ctx,
		db,
		installation.ServerID,
		models.ServerCapabilityResource,
	); err != nil {
		return ineligible(
			"The Resource installation is not on an available Resource-capable Server.",
		)
	}
	administrators, err := models.ResourceCredential.ActiveAdministratorCount(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if administrators != 1 {
		return ineligible("Exactly one active Resource administrator credential is required.")
	}
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, db)
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if len(destinations) == 0 {
		return ineligible("No active, verified Object Storage destination is available.")
	}
	return models.ResourceBackupEligibility{Eligible: true, InstallationID: &installation.ID}, nil
}

func databaseBackupRetention(input DatabaseBackupPolicyInput) (json.RawMessage, error) {
	retention := models.BackupRetentionPolicy{
		KeepLast:    input.KeepLast,
		KeepDaily:   input.KeepDaily,
		KeepWeekly:  input.KeepWeekly,
		KeepMonthly: input.KeepMonthly,
	}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 || retention.KeepWeekly < 0 ||
		retention.KeepMonthly < 0 ||
		retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly < 1 {
		return nil, domainError(
			"retention",
			"invalid",
			"Retention must preserve at least one recovery point",
		)
	}
	return json.Marshal(retention)
}
