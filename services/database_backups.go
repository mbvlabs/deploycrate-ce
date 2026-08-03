package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type DatabaseBackups struct {
	db        storage.Pool
	scheduler *BackupScheduler
}

func NewDatabaseBackups(db storage.Pool, scheduler *BackupScheduler) *DatabaseBackups {
	return &DatabaseBackups{db: db, scheduler: scheduler}
}

type DatabaseBackupPolicyInput struct {
	Schedule                                     string
	KeepLast, KeepDaily, KeepWeekly, KeepMonthly int
	BackupDestinationID                          uuid.UUID
}

func (service *DatabaseBackups) DetailsForResource(ctx context.Context, resourceID uuid.UUID) (models.ResourceBackupCatalog, error) {
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
		eligibility, eligibilityErr := service.eligibility(ctx, service.db.Executor(), resource, database.Name)
		if eligibilityErr != nil {
			return models.ResourceBackupCatalog{}, eligibilityErr
		}
		var policy models.BackupPolicyEntity
		policyErr := service.db.Executor().NewSelect().Model(&policy).
			Where("resource_id = ?", resourceID).Where("target_type = 'resource'").
			Where("target ->> 'database' = ?", database.Name).Where("archived_at IS NULL").Limit(1).Scan(ctx)
		var activePolicy *models.BackupPolicyEntity
		if policyErr == nil {
			activePolicy = &policy
		} else if !errors.Is(policyErr, sql.ErrNoRows) {
			return models.ResourceBackupCatalog{}, policyErr
		}
		history, historyErr := models.Backup.RecentForResourceDatabase(ctx, service.db.Executor(), resourceID, database.Name, 10)
		if historyErr != nil {
			return models.ResourceBackupCatalog{}, historyErr
		}
		restores, restoreErr := models.ResourceRestore.RecentForResourceDatabase(ctx, service.db.Executor(), resourceID, database.Name, 10)
		if restoreErr != nil {
			return models.ResourceBackupCatalog{}, restoreErr
		}
		details = append(details, models.ResourceBackupDetails{DatabaseName: database.Name, Eligibility: eligibility, Policy: activePolicy, History: history, Restores: restores})
	}
	return models.ResourceBackupCatalog{Destinations: destinations, Databases: details}, nil
}

func (service *DatabaseBackups) Destinations(ctx context.Context) ([]models.BackupDestinationSummary, error) {
	return models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
}

func (service *DatabaseBackups) CreateForResource(ctx context.Context, resourceID uuid.UUID, databaseName string, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
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
		return models.BackupPolicyEntity{}, domainError("schedule", "invalid", "Schedule must be a five-field cron expression")
	}
	retention, err := databaseBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	target, _ := json.Marshal(map[string]string{"database": databaseName})
	policy, err := models.BackupPolicy.Create(ctx, tx, models.CreateBackupPolicyData{
		Name: databaseName + " PostgreSQL", Schedule: input.Schedule, Strategy: "logical", Driver: "postgresql", Format: "tar.age",
		Retention: retention, Verification: json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`), Settings: json.RawMessage(`{}`),
		ActivatedAt: sql.NullTime{Time: now, Valid: true}, TargetType: "resource", Target: target, ResourceID: &resourceID,
		NextRunAt: nextRunAt, BackupDestinationID: input.BackupDestinationID,
	})
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if err := service.scheduler.InsertScheduleTx(ctx, tx, policy.ID, nextRunAt); err != nil {
		return models.BackupPolicyEntity{}, fmt.Errorf("insert first Resource backup schedule: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return policy, nil
}

func (service *DatabaseBackups) UpdateForResource(ctx context.Context, resourceID uuid.UUID, databaseName string, policyID uuid.UUID, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
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
		return models.BackupPolicyEntity{}, domainError("schedule", "invalid", "Schedule must be a five-field cron expression")
	}
	retention, err := databaseBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{
		ID: policy.ID, Name: policy.Name, Schedule: input.Schedule, Strategy: policy.Strategy, Driver: policy.Driver,
		Retention: retention, Format: policy.Format, Verification: policy.Verification, Settings: policy.Settings,
		ArchivedAt: policy.ArchivedAt, ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, Target: policy.Target,
		ResourceID: policy.ResourceID, NextRunAt: nextRunAt, LastScheduledAt: policy.LastScheduledAt,
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

func (service *DatabaseBackups) SetStateForResource(ctx context.Context, resourceID uuid.UUID, databaseName string, policyID uuid.UUID, action string) error {
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
		ID: policy.ID, Name: policy.Name, Schedule: policy.Schedule, Strategy: policy.Strategy, Driver: policy.Driver,
		Retention: policy.Retention, Format: policy.Format, Verification: policy.Verification, Settings: policy.Settings,
		ArchivedAt: policy.ArchivedAt, ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, Target: policy.Target,
		ResourceID: policy.ResourceID, NextRunAt: policy.NextRunAt, LastScheduledAt: policy.LastScheduledAt,
		BackupDestinationID: policy.BackupDestinationID,
	})
	if err != nil {
		return err
	}
	if action == "resume" {
		if err := service.scheduler.InsertScheduleTx(ctx, tx, updated.ID, updated.NextRunAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (service *DatabaseBackups) ManualForResource(ctx context.Context, resourceID uuid.UUID, databaseName string, policyID uuid.UUID) (uuid.UUID, error) {
	policy, err := service.loadPolicy(ctx, service.db.Executor(), resourceID, databaseName, policyID)
	if err != nil {
		return uuid.Nil, err
	}
	return service.scheduler.Manual(ctx, policy.ID)
}

func (service *DatabaseBackups) loadPolicy(ctx context.Context, db storage.Executor, resourceID uuid.UUID, databaseName string, policyID uuid.UUID) (models.BackupPolicyEntity, error) {
	policy, err := models.BackupPolicy.FindForUpdate(ctx, db, policyID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (policy.ResourceID == nil || *policy.ResourceID != resourceID || policy.TargetType != "resource" || resourceCredentialMetadataDatabase(policy.Target) != databaseName || policy.ArchivedAt.Valid) {
		return models.BackupPolicyEntity{}, models.ErrNotFound
	}
	return policy, err
}

func (service *DatabaseBackups) validateDestination(ctx context.Context, db storage.Executor, destinationID uuid.UUID) error {
	count, err := db.NewSelect().TableExpr("backup_destinations AS destination").Join("JOIN credentials AS credential ON credential.id = destination.credential_id").Where("destination.id = ?", destinationID).Where("destination.archived_at IS NULL").Where("credential.archived_at IS NULL").Where("credential.verified_at IS NOT NULL").Where("credential.provider = 'backup_' || destination.provider").Count(ctx)
	if err != nil {
		return err
	}
	return requireChild(count, "backupDestinationId", "Choose an active, verified Object Storage destination")
}

func (service *DatabaseBackups) eligibility(ctx context.Context, db storage.Executor, resource models.ResourceEntity, databaseName string) (models.ResourceBackupEligibility, error) {
	ineligible := func(reason string) (models.ResourceBackupEligibility, error) {
		return models.ResourceBackupEligibility{Reason: reason}, nil
	}
	if resource.Engine() != "postgresql" || !resourceHasDatabase(resource, databaseName) {
		return ineligible("Logical backups currently support configured PostgreSQL databases only.")
	}
	var installation models.ResourceInstallationEntity
	if err := db.NewSelect().Model(&installation).Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ineligible("This Resource has no active Docker installation.")
		}
		return models.ResourceBackupEligibility{}, err
	}
	if _, err := models.RequireServerCapability(ctx, db, installation.ServerID, models.ServerCapabilityResource); err != nil {
		return ineligible("The Resource installation is not on an available Resource-capable Server.")
	}
	administrators, err := db.NewSelect().TableExpr("resource_credentials").Where("resource_id = ?", resource.ID).Where("metadata ->> 'purpose' = 'administrator'").Where("archived_at IS NULL").Count(ctx)
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
	retention := models.BackupRetentionPolicy{KeepLast: input.KeepLast, KeepDaily: input.KeepDaily, KeepWeekly: input.KeepWeekly, KeepMonthly: input.KeepMonthly}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 || retention.KeepWeekly < 0 || retention.KeepMonthly < 0 || retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly < 1 {
		return nil, domainError("retention", "invalid", "Retention must preserve at least one recovery point")
	}
	return json.Marshal(retention)
}
