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
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
	if err != nil {
		return models.ResourceBackupCatalog{}, err
	}
	databases := make([]models.DatabaseEntity, 0)
	if err := service.db.Executor().NewSelect().Model(&databases).
		Join("JOIN database_resources AS backing ON backing.database_cluster_id = database.database_cluster_id").
		Where("backing.resource_id = ?", resourceID).Where("database.archived_at IS NULL").OrderExpr("database.name").Scan(ctx); err != nil {
		return models.ResourceBackupCatalog{}, err
	}
	details := make([]models.ResourceBackupDetails, 0, len(databases))
	for _, database := range databases {
		eligibility, err := service.eligibility(ctx, service.db.Executor(), database.ID)
		if err != nil {
			return models.ResourceBackupCatalog{}, err
		}
		var policy models.BackupPolicyEntity
		policyErr := service.db.Executor().NewSelect().Model(&policy).Where("database_id = ?", database.ID).Where("target_type = 'database'").Where("archived_at IS NULL").Limit(1).Scan(ctx)
		var policyPointer *models.BackupPolicyEntity
		if policyErr == nil {
			policyPointer = &policy
		} else if !errors.Is(policyErr, sql.ErrNoRows) {
			return models.ResourceBackupCatalog{}, policyErr
		}
		history, err := models.Backup.RecentForDatabase(ctx, service.db.Executor(), database.ID, 10)
		if err != nil {
			return models.ResourceBackupCatalog{}, err
		}
		restores, err := models.DatabaseRestore.RecentForDatabase(ctx, service.db.Executor(), database.ID, 10)
		if err != nil {
			return models.ResourceBackupCatalog{}, err
		}
		details = append(details, models.ResourceBackupDetails{
			DatabaseID: database.ID, DatabaseName: database.Name, Eligibility: eligibility,
			Policy: policyPointer, History: history, Restores: restores,
		})
	}
	return models.ResourceBackupCatalog{Destinations: destinations, Databases: details}, nil
}

func (service *DatabaseBackups) Destinations(ctx context.Context) ([]models.BackupDestinationSummary, error) {
	return models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
}

func (service *DatabaseBackups) CreateForResource(ctx context.Context, resourceID, databaseID uuid.UUID, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
	database, err := service.databaseForResource(ctx, service.db.Executor(), resourceID, databaseID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return service.Create(ctx, database.ID, input)
}

func (service *DatabaseBackups) Create(ctx context.Context, databaseID uuid.UUID, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	eligibility, err := service.eligibility(ctx, tx, databaseID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if !eligibility.Eligible {
		return models.BackupPolicyEntity{}, domainError("backup", "ineligible", eligibility.Reason)
	}
	if err := service.validateDestination(ctx, tx, input.BackupDestinationID); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	database, err := models.Database.Find(ctx, tx, databaseID)
	if err != nil {
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
	policy, err := models.BackupPolicy.Create(ctx, tx, models.CreateBackupPolicyData{Name: database.Name + " PostgreSQL", Schedule: input.Schedule, Strategy: "logical", Driver: "postgresql", Format: "tar.age", Retention: retention, Verification: json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`), Settings: json.RawMessage(`{}`), ActivatedAt: sql.NullTime{Time: now, Valid: true}, TargetType: "database", DatabaseID: &databaseID, NextRunAt: nextRunAt, BackupDestinationID: input.BackupDestinationID})
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if err := service.scheduler.InsertScheduleTx(ctx, tx, policy.ID, nextRunAt); err != nil {
		return models.BackupPolicyEntity{}, fmt.Errorf("insert first Database backup schedule: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return policy, nil
}

func (service *DatabaseBackups) UpdateForResource(ctx context.Context, resourceID, databaseID, policyID uuid.UUID, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
	databaseID, err := service.policyDatabaseForResource(ctx, resourceID, databaseID, policyID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	return service.Update(ctx, databaseID, policyID, input)
}

func (service *DatabaseBackups) Update(ctx context.Context, databaseID, policyID uuid.UUID, input DatabaseBackupPolicyInput) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, databaseID, policyID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	eligibility, err := service.eligibility(ctx, tx, databaseID)
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
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{ID: policy.ID, Name: policy.Name, Schedule: input.Schedule, Strategy: policy.Strategy, Driver: policy.Driver, Retention: retention, Format: policy.Format, Verification: policy.Verification, Settings: policy.Settings, ArchivedAt: policy.ArchivedAt, ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, DatabaseID: policy.DatabaseID, NextRunAt: nextRunAt, LastScheduledAt: policy.LastScheduledAt, BackupDestinationID: input.BackupDestinationID})
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

func (service *DatabaseBackups) SetStateForResource(ctx context.Context, resourceID, databaseID, policyID uuid.UUID, action string) error {
	databaseID, err := service.policyDatabaseForResource(ctx, resourceID, databaseID, policyID)
	if err != nil {
		return err
	}
	return service.setState(ctx, databaseID, policyID, action)
}
func (service *DatabaseBackups) ManualForResource(ctx context.Context, resourceID, databaseID, policyID uuid.UUID) (uuid.UUID, error) {
	databaseID, err := service.policyDatabaseForResource(ctx, resourceID, databaseID, policyID)
	if err != nil {
		return uuid.Nil, err
	}
	policy, err := models.BackupPolicy.Find(ctx, service.db.Executor(), policyID)
	if err != nil || policy.DatabaseID == nil || *policy.DatabaseID != databaseID || policy.ArchivedAt.Valid {
		return uuid.Nil, models.ErrNotFound
	}
	return service.scheduler.Manual(ctx, policyID)
}

func (service *DatabaseBackups) databaseForResource(ctx context.Context, db storage.Executor, resourceID, databaseID uuid.UUID) (models.DatabaseEntity, error) {
	var database models.DatabaseEntity
	err := db.NewSelect().Model(&database).
		Join("JOIN database_resources AS backing ON backing.database_cluster_id = database.database_cluster_id").
		Where("database.id = ?", databaseID).Where("backing.resource_id = ?", resourceID).Where("database.archived_at IS NULL").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.DatabaseEntity{}, models.ErrNotFound
	}
	return database, err
}

func (service *DatabaseBackups) policyDatabaseForResource(ctx context.Context, resourceID, databaseID, policyID uuid.UUID) (uuid.UUID, error) {
	var scopedDatabaseID uuid.UUID
	err := service.db.Executor().NewSelect().TableExpr("backup_policies AS policy").ColumnExpr("database.id").
		Join("JOIN databases AS database ON database.id = policy.database_id AND database.archived_at IS NULL").
		Join("JOIN database_resources AS backing ON backing.database_cluster_id = database.database_cluster_id AND backing.resource_id = ?", resourceID).
		Where("database.id = ?", databaseID).Where("policy.id = ?", policyID).Where("policy.archived_at IS NULL").Scan(ctx, &scopedDatabaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, models.ErrNotFound
	}
	return scopedDatabaseID, err
}

func (service *DatabaseBackups) setState(ctx context.Context, databaseID, policyID uuid.UUID, action string) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, databaseID, policyID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	switch action {
	case "pause":
		policy.ActivatedAt = sql.NullTime{}
	case "resume":
		eligibility, eligibilityErr := service.eligibility(ctx, tx, databaseID)
		if eligibilityErr != nil {
			return eligibilityErr
		}
		if !eligibility.Eligible {
			return domainError("backup", "ineligible", eligibility.Reason)
		}
		if err := service.validateDestination(ctx, tx, policy.BackupDestinationID); err != nil {
			return err
		}
		policy.ActivatedAt = sql.NullTime{Time: now, Valid: true}
		policy.NextRunAt, err = models.NextBackupRun(policy.Schedule, now)
		if err != nil {
			return err
		}
	case "archive":
		policy.ActivatedAt = sql.NullTime{}
		policy.ArchivedAt = sql.NullTime{Time: now, Valid: true}
	default:
		return errors.New("unsupported Database backup policy state change")
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{ID: policy.ID, Name: policy.Name, Schedule: policy.Schedule, Strategy: policy.Strategy, Driver: policy.Driver, Retention: policy.Retention, Format: policy.Format, Verification: policy.Verification, Settings: policy.Settings, ArchivedAt: policy.ArchivedAt, ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, DatabaseID: policy.DatabaseID, NextRunAt: policy.NextRunAt, LastScheduledAt: policy.LastScheduledAt, BackupDestinationID: policy.BackupDestinationID})
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

func (service *DatabaseBackups) loadPolicy(ctx context.Context, db storage.Executor, databaseID, policyID uuid.UUID) (models.BackupPolicyEntity, error) {
	policy, err := models.BackupPolicy.FindForUpdate(ctx, db, policyID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (policy.DatabaseID == nil || *policy.DatabaseID != databaseID || policy.TargetType != "database" || policy.ArchivedAt.Valid) {
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

func (service *DatabaseBackups) eligibility(ctx context.Context, db storage.Executor, databaseID uuid.UUID) (models.ResourceBackupEligibility, error) {
	var target struct {
		DatabaseID                                 uuid.UUID
		Engine, ManagementMode, InstallationMethod string
		NodeID, InstallationID                     uuid.UUID
		ServerID                                   uuid.UUID
		AdministratorCount                         int
	}
	err := db.NewSelect().TableExpr("databases AS database").ColumnExpr("database.id AS database_id, cluster.engine, cluster.management_mode, COALESCE(installation.installation_method, '') AS installation_method").ColumnExpr("installation.server_id, node.id AS node_id, installation.id AS installation_id").ColumnExpr("(SELECT count(*) FROM database_cluster_credentials credential WHERE credential.database_cluster_id = cluster.id AND credential.role = 'administrator' AND credential.archived_at IS NULL) AS administrator_count").Join("JOIN database_clusters AS cluster ON cluster.id = database.database_cluster_id AND cluster.archived_at IS NULL").Join("LEFT JOIN database_cluster_nodes AS node ON node.database_cluster_id = cluster.id AND node.role = 'primary' AND node.archived_at IS NULL").Join("LEFT JOIN database_node_installations AS installation ON installation.database_cluster_node_id = node.id AND installation.archived_at IS NULL").Where("database.id = ?", databaseID).Where("database.archived_at IS NULL").Scan(ctx, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceBackupEligibility{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	ineligible := func(reason string) (models.ResourceBackupEligibility, error) {
		return models.ResourceBackupEligibility{Reason: reason}, nil
	}
	if target.Engine != "postgresql" {
		return ineligible("Logical backups currently support PostgreSQL Databases only.")
	}
	if target.ManagementMode != "managed" || target.InstallationMethod != "docker" {
		return ineligible("This Database does not currently have an executable managed Docker primary Node.")
	}
	if _, err := models.RequireServerCapability(ctx, db, target.ServerID, models.ServerCapabilityDatabase); err != nil {
		return ineligible("The active primary Node is not an available Database-capable Server.")
	}
	if target.AdministratorCount != 1 {
		return ineligible("Exactly one active Database Cluster administrator credential is required.")
	}
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, db)
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if len(destinations) == 0 {
		return ineligible("No active, verified Object Storage destination is available.")
	}
	return models.ResourceBackupEligibility{Eligible: true, InstallationID: &target.InstallationID}, nil
}

func databaseBackupRetention(input DatabaseBackupPolicyInput) (json.RawMessage, error) {
	retention := models.BackupRetentionPolicy{KeepLast: input.KeepLast, KeepDaily: input.KeepDaily, KeepWeekly: input.KeepWeekly, KeepMonthly: input.KeepMonthly}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 || retention.KeepWeekly < 0 || retention.KeepMonthly < 0 || retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly < 1 {
		return nil, domainError("retention", "invalid", "Retention must preserve at least one recovery point")
	}
	return json.Marshal(retention)
}
