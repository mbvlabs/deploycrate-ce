package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type ResourceBackups struct {
	db        storage.Pool
	scheduler *BackupScheduler
}

func NewResourceBackups(db storage.Pool, scheduler *BackupScheduler) *ResourceBackups {
	return &ResourceBackups{db: db, scheduler: scheduler}
}

type ResourceBackupPolicyInput struct {
	Schedule            string
	KeepLast            int
	KeepDaily           int
	KeepWeekly          int
	KeepMonthly         int
	BackupDestinationID uuid.UUID
}

func (service *ResourceBackups) Details(ctx context.Context, resourceID uuid.UUID) (models.ResourceBackupDetails, error) {
	eligibility, err := service.eligibility(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return models.ResourceBackupDetails{}, err
	}
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
	if err != nil {
		return models.ResourceBackupDetails{}, err
	}
	var policy models.BackupPolicyEntity
	policyErr := service.db.Executor().NewSelect().Model(&policy).
		Where("resource_id = ?", resourceID).
		Where("target_type = 'resource'").
		Where("archived_at IS NULL").
		Limit(1).
		Scan(ctx)
	var policyPointer *models.BackupPolicyEntity
	if policyErr == nil {
		policyPointer = &policy
	} else if !errors.Is(policyErr, sql.ErrNoRows) {
		return models.ResourceBackupDetails{}, policyErr
	}
	history, err := models.Backup.RecentForResource(ctx, service.db.Executor(), resourceID, 10)
	if err != nil {
		return models.ResourceBackupDetails{}, err
	}
	return models.ResourceBackupDetails{
		Eligibility: eligibility, Policy: policyPointer, Destinations: destinations, History: history,
	}, nil
}

func (service *ResourceBackups) Destinations(ctx context.Context) ([]models.BackupDestinationSummary, error) {
	return models.BackupDestination.ActiveSummaries(ctx, service.db.Executor())
}

func (service *ResourceBackups) Create(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceBackupPolicyInput,
) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	eligibility, err := service.eligibility(ctx, tx, resourceID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if !eligibility.Eligible || eligibility.InstallationID == nil {
		return models.BackupPolicyEntity{}, domainError("backup", "ineligible", eligibility.Reason)
	}
	if err := service.validateDestination(ctx, tx, input.BackupDestinationID); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	resource, err := models.Resource.Find(ctx, tx, resourceID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	now := time.Now().UTC()
	nextRunAt, err := models.NextBackupRun(input.Schedule, now)
	if err != nil {
		return models.BackupPolicyEntity{}, domainError("schedule", "invalid", "Schedule must be a five-field cron expression")
	}
	retention, err := resourceBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	policy, err := models.BackupPolicy.Create(ctx, tx, models.CreateBackupPolicyData{
		Name: resource.Name + " PostgreSQL", Schedule: input.Schedule,
		Strategy: "logical", Driver: "postgresql", Format: "tar.age",
		Retention: retention, Verification: json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`),
		Settings: json.RawMessage(`{}`), ActivatedAt: sql.NullTime{Time: now, Valid: true},
		TargetType: "resource", ResourceID: &resourceID, ResourceInstallationID: eligibility.InstallationID,
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

func (service *ResourceBackups) Update(ctx context.Context, resourceID, policyID uuid.UUID, input ResourceBackupPolicyInput) (models.BackupPolicyEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, resourceID, policyID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	eligibility, err := service.eligibility(ctx, tx, resourceID)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	if !eligibility.Eligible || eligibility.InstallationID == nil || policy.ResourceInstallationID == nil ||
		*eligibility.InstallationID != *policy.ResourceInstallationID {
		reason := eligibility.Reason
		if reason == "" {
			reason = "The backup policy no longer targets the active Resource installation."
		}
		return models.BackupPolicyEntity{}, domainError("backup", "ineligible", reason)
	}
	if err := service.validateDestination(ctx, tx, input.BackupDestinationID); err != nil {
		return models.BackupPolicyEntity{}, err
	}
	now := time.Now().UTC()
	nextRunAt, err := models.NextBackupRun(input.Schedule, now)
	if err != nil {
		return models.BackupPolicyEntity{}, domainError("schedule", "invalid", "Schedule must be a five-field cron expression")
	}
	retention, err := resourceBackupRetention(input)
	if err != nil {
		return models.BackupPolicyEntity{}, err
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{
		ID: policy.ID, Name: policy.Name, Schedule: input.Schedule, Strategy: policy.Strategy,
		Driver: policy.Driver, Retention: retention, Format: policy.Format,
		Verification: policy.Verification, Settings: policy.Settings, ArchivedAt: policy.ArchivedAt,
		ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, ResourceID: policy.ResourceID,
		ResourceInstallationID: policy.ResourceInstallationID, NextRunAt: nextRunAt,
		LastScheduledAt: policy.LastScheduledAt, BackupDestinationID: input.BackupDestinationID,
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

func (service *ResourceBackups) Pause(ctx context.Context, resourceID, policyID uuid.UUID) error {
	return service.setState(ctx, resourceID, policyID, "pause")
}

func (service *ResourceBackups) Resume(ctx context.Context, resourceID, policyID uuid.UUID) error {
	return service.setState(ctx, resourceID, policyID, "resume")
}

func (service *ResourceBackups) Archive(ctx context.Context, resourceID, policyID uuid.UUID) error {
	return service.setState(ctx, resourceID, policyID, "archive")
}

func (service *ResourceBackups) Manual(ctx context.Context, resourceID, policyID uuid.UUID) (uuid.UUID, error) {
	policy, err := models.BackupPolicy.Find(ctx, service.db.Executor(), policyID)
	if err != nil || policy.ResourceID == nil || *policy.ResourceID != resourceID || policy.ArchivedAt.Valid {
		return uuid.Nil, models.ErrNotFound
	}
	return service.scheduler.Manual(ctx, policyID)
}

func (service *ResourceBackups) setState(ctx context.Context, resourceID, policyID uuid.UUID, action string) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	policy, err := service.loadPolicy(ctx, tx, resourceID, policyID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if action == "pause" {
		policy.ActivatedAt = sql.NullTime{}
	} else if action == "resume" {
		eligibility, eligibilityErr := service.eligibility(ctx, tx, resourceID)
		if eligibilityErr != nil {
			return eligibilityErr
		}
		if !eligibility.Eligible || eligibility.InstallationID == nil || policy.ResourceInstallationID == nil ||
			*eligibility.InstallationID != *policy.ResourceInstallationID {
			reason := eligibility.Reason
			if reason == "" {
				reason = "The backup policy no longer targets the active Resource installation."
			}
			return domainError("backup", "ineligible", reason)
		}
		if destinationErr := service.validateDestination(ctx, tx, policy.BackupDestinationID); destinationErr != nil {
			return destinationErr
		}
		policy.ActivatedAt = sql.NullTime{Time: now, Valid: true}
		policy.NextRunAt, err = models.NextBackupRun(policy.Schedule, now)
		if err != nil {
			return err
		}
	} else if action == "archive" {
		policy.ActivatedAt = sql.NullTime{}
		policy.ArchivedAt = sql.NullTime{Time: now, Valid: true}
	} else {
		return errors.New("unsupported Resource backup policy state change")
	}
	updated, err := models.BackupPolicy.Update(ctx, tx, models.UpdateBackupPolicyData{
		ID: policy.ID, Name: policy.Name, Schedule: policy.Schedule, Strategy: policy.Strategy,
		Driver: policy.Driver, Retention: policy.Retention, Format: policy.Format,
		Verification: policy.Verification, Settings: policy.Settings, ArchivedAt: policy.ArchivedAt,
		ActivatedAt: policy.ActivatedAt, TargetType: policy.TargetType, ResourceID: policy.ResourceID,
		ResourceInstallationID: policy.ResourceInstallationID, NextRunAt: policy.NextRunAt,
		LastScheduledAt: policy.LastScheduledAt, BackupDestinationID: policy.BackupDestinationID,
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

func (service *ResourceBackups) loadPolicy(ctx context.Context, db storage.Executor, resourceID, policyID uuid.UUID) (models.BackupPolicyEntity, error) {
	policy, err := models.BackupPolicy.FindForUpdate(ctx, db, policyID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (policy.ResourceID == nil || *policy.ResourceID != resourceID || policy.TargetType != "resource" || policy.ArchivedAt.Valid) {
		return models.BackupPolicyEntity{}, models.ErrNotFound
	}
	return policy, err
}

func (service *ResourceBackups) validateDestination(ctx context.Context, db storage.Executor, destinationID uuid.UUID) error {
	count, err := db.NewSelect().TableExpr("backup_destinations AS destination").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id").
		Where("destination.id = ?", destinationID).
		Where("destination.archived_at IS NULL").
		Where("credential.archived_at IS NULL").
		Where("credential.verified_at IS NOT NULL").
		Where("credential.provider = 'backup_' || destination.provider").
		Count(ctx)
	if err != nil {
		return err
	}
	return requireChild(count, "backupDestinationId", "Choose an active, verified Object Storage destination")
}

func (service *ResourceBackups) eligibility(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (models.ResourceBackupEligibility, error) {
	resource, err := models.Resource.Find(ctx, db, resourceID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && resource.ArchivedAt.Valid {
		return models.ResourceBackupEligibility{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	ineligible := func(reason string) (models.ResourceBackupEligibility, error) {
		return models.ResourceBackupEligibility{Reason: reason}, nil
	}
	if resource.SystemManaged {
		return ineligible("System Resources use the control-plane backup policy.")
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return ineligible("Only managed Resources can use local container backups.")
	}
	if resource.Category != "database" || resource.Kind != "postgresql" {
		return ineligible("Resource backups currently support PostgreSQL databases only.")
	}
	type installationRow struct {
		ID          uuid.UUID `bun:"id"`
		IPv4Address string    `bun:"ipv4_address"`
	}
	installations := make([]installationRow, 0, 2)
	if err := db.NewSelect().TableExpr("resource_installations AS installation").
		ColumnExpr("installation.id, server.ipv4_address").
		Join("JOIN servers AS server ON server.id = installation.server_id AND server.archived_at IS NULL").
		Where("installation.resource_id = ?", resourceID).
		Where("installation.archived_at IS NULL").
		Limit(2).
		Scan(ctx, &installations); err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if len(installations) != 1 {
		return ineligible("Exactly one active local Docker installation is required.")
	}
	address, parseErr := netip.ParseAddr(strings.TrimSpace(installations[0].IPv4Address))
	if parseErr != nil || !address.IsLoopback() {
		return ineligible("The active installation is not on the local DeployCrate CE host.")
	}
	credentials, err := db.NewSelect().TableExpr("resource_credentials").
		Where("resource_id = ?", resourceID).
		Where("resource_installation_id = ?", installations[0].ID).
		Where("archived_at IS NULL").
		Where("username IS NOT NULL AND length(btrim(username)) > 0").
		Where("octet_length(enc_payload) > 1").
		Count(ctx)
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if credentials != 1 {
		return ineligible("Exactly one encrypted installation administrator credential is required.")
	}
	destinations, err := models.BackupDestination.ActiveSummaries(ctx, db)
	if err != nil {
		return models.ResourceBackupEligibility{}, err
	}
	if len(destinations) == 0 {
		return ineligible("No active, verified Object Storage destination is available.")
	}
	return models.ResourceBackupEligibility{Eligible: true, InstallationID: &installations[0].ID}, nil
}

func resourceBackupRetention(input ResourceBackupPolicyInput) (json.RawMessage, error) {
	retention := models.BackupRetentionPolicy{
		KeepLast: input.KeepLast, KeepDaily: input.KeepDaily,
		KeepWeekly: input.KeepWeekly, KeepMonthly: input.KeepMonthly,
	}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 || retention.KeepWeekly < 0 || retention.KeepMonthly < 0 ||
		retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly < 1 {
		return nil, domainError("retention", "invalid", "Retention must preserve at least one recovery point")
	}
	value, err := json.Marshal(retention)
	return value, err
}
