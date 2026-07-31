package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	BackupStatusPending            = "pending"
	BackupStatusRunning            = "running"
	BackupStatusUploaded           = "uploaded"
	BackupStatusVerified           = "verified"
	BackupStatusVerificationFailed = "verification_failed"
	BackupStatusFailed             = "failed"
	BackupStatusPruned             = "pruned"
)

type BackupEntity struct {
	bun.BaseModel              `bun:"table:backups,alias:backups"`
	ID                         uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt                  time.Time       `bun:"created_at"`
	UpdatedAt                  time.Time       `bun:"updated_at"`
	TargetType                 string          `bun:"target_type"`
	TriggerType                string          `bun:"trigger_type"`
	ScheduledAt                time.Time       `bun:"scheduled_at"`
	Strategy                   string          `bun:"strategy"`
	Driver                     string          `bun:"driver"`
	Format                     string          `bun:"format"`
	FormatVersion              string          `bun:"format_version"`
	ArtifactReference          string          `bun:"artifact_reference"`
	ProviderMetadata           json.RawMessage `bun:"provider_metadata,type:jsonb"`
	Status                     string          `bun:"status"`
	RequestedAt                time.Time       `bun:"requested_at"`
	StartedAt                  sql.NullTime    `bun:"started_at"`
	UploadedAt                 sql.NullTime    `bun:"uploaded_at"`
	FinishedAt                 sql.NullTime    `bun:"finished_at"`
	VerifiedAt                 sql.NullTime    `bun:"verified_at"`
	PrunedAt                   sql.NullTime    `bun:"pruned_at"`
	SizeBytes                  sql.NullInt64   `bun:"size_bytes"`
	Digest                     []byte          `bun:"digest"`
	ProducerVersion            string          `bun:"producer_version"`
	Error                      sql.NullString  `bun:"error"`
	ChangeID                   uuid.UUID       `bun:"change_id,type:uuid"`
	ChangeTaskID               uuid.UUID       `bun:"change_task_id,type:uuid"`
	BackupPolicyID             uuid.UUID       `bun:"backup_policy_id,type:uuid"`
	ServerID                   *uuid.UUID      `bun:"server_id,type:uuid"`
	DatabaseID                 *uuid.UUID      `bun:"database_id,type:uuid"`
	DatabaseClusterID          *uuid.UUID      `bun:"database_cluster_id,type:uuid"`
	DatabaseClusterNodeID      *uuid.UUID      `bun:"database_cluster_node_id,type:uuid"`
	DatabaseNodeInstallationID *uuid.UUID      `bun:"database_node_installation_id,type:uuid"`
	BackupDestinationID        uuid.UUID       `bun:"backup_destination_id,type:uuid"`
}

func (entity *BackupEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.ID == uuid.Nil {
		builder.Add("id", "required", "backup ID is required")
	}
	if entity.TargetType == "server" {
		if entity.ServerID == nil || *entity.ServerID == uuid.Nil || entity.DatabaseID != nil || entity.DatabaseClusterID != nil || entity.DatabaseClusterNodeID != nil || entity.DatabaseNodeInstallationID != nil {
			builder.Add("target_type", "incoherent", "server backup scope is invalid")
		}
	} else if entity.TargetType == "database" {
		if entity.ServerID != nil || entity.DatabaseID == nil || *entity.DatabaseID == uuid.Nil || entity.DatabaseClusterID == nil || *entity.DatabaseClusterID == uuid.Nil || (entity.DatabaseClusterNodeID == nil) != (entity.DatabaseNodeInstallationID == nil) {
			builder.Add("target_type", "incoherent", "Database backup scope is invalid")
		}
	} else {
		builder.Add("target_type", "unsupported", "backup target must be Server or Database")
	}
	if entity.TargetType == "server" &&
		(entity.Strategy != "filesystem" || entity.Driver != "restic" || entity.Format != "restic") {
		builder.Add("driver", "incompatible", "server backups require filesystem, restic, and restic")
	}
	if entity.TargetType == "database" &&
		(entity.Strategy != "logical" || entity.Driver != "postgresql" || entity.Format != "tar.age") {
		builder.Add("driver", "incompatible", "database backups require logical, postgresql, and tar.age")
	}
	if entity.TriggerType != "installer" && entity.TriggerType != "schedule" &&
		entity.TriggerType != "manual" && entity.TriggerType != "pre_restore" {
		builder.Add("trigger_type", "unsupported", "backup trigger is unsupported")
	}
	if entity.ScheduledAt.IsZero() || entity.RequestedAt.IsZero() {
		builder.Add("scheduled_at", "required", "backup schedule and request times are required")
	}
	if strings.TrimSpace(entity.Strategy) == "" || strings.TrimSpace(entity.Driver) == "" ||
		strings.TrimSpace(entity.Format) == "" || strings.TrimSpace(entity.FormatVersion) == "" {
		builder.Add("format", "required", "backup strategy, driver, format, and version are required")
	}
	if !validJSONObject(entity.ProviderMetadata) {
		builder.Add("provider_metadata", "invalid", "backup provider metadata must be a JSON object")
	}
	if entity.Status == "" {
		builder.Add("status", "required", "backup status is required")
	}
	if entity.ChangeID == uuid.Nil || entity.ChangeTaskID == uuid.Nil ||
		entity.BackupPolicyID == uuid.Nil || entity.BackupDestinationID == uuid.Nil {
		builder.Add("backup_policy_id", "required", "backup lifecycle references are required")
	}
	return builder.Err()
}

type CreateBackupData struct {
	ID                         uuid.UUID
	TargetType                 string
	TriggerType                string
	ScheduledAt                time.Time
	Strategy                   string
	Driver                     string
	Format                     string
	FormatVersion              string
	ArtifactReference          string
	ProviderMetadata           json.RawMessage
	Status                     string
	RequestedAt                time.Time
	ProducerVersion            string
	ChangeID                   uuid.UUID
	ChangeTaskID               uuid.UUID
	BackupPolicyID             uuid.UUID
	ServerID                   *uuid.UUID
	DatabaseID                 *uuid.UUID
	DatabaseClusterID          *uuid.UUID
	DatabaseClusterNodeID      *uuid.UUID
	DatabaseNodeInstallationID *uuid.UUID
	BackupDestinationID        uuid.UUID
}

func (backup) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupData,
) (BackupEntity, error) {
	now := time.Now().UTC()
	if data.ID == uuid.Nil {
		data.ID = uuid.New()
	}
	entity := BackupEntity{
		ID: data.ID, CreatedAt: now, UpdatedAt: now, TargetType: data.TargetType,
		TriggerType: data.TriggerType, ScheduledAt: data.ScheduledAt, Strategy: data.Strategy,
		Driver: data.Driver, Format: data.Format, FormatVersion: data.FormatVersion,
		ArtifactReference: data.ArtifactReference, ProviderMetadata: data.ProviderMetadata,
		Status: data.Status, RequestedAt: data.RequestedAt, ProducerVersion: data.ProducerVersion,
		ChangeID: data.ChangeID, ChangeTaskID: data.ChangeTaskID,
		BackupPolicyID: data.BackupPolicyID, ServerID: data.ServerID, DatabaseID: data.DatabaseID,
		DatabaseClusterID: data.DatabaseClusterID, DatabaseClusterNodeID: data.DatabaseClusterNodeID,
		DatabaseNodeInstallationID: data.DatabaseNodeInstallationID,
		BackupDestinationID:        data.BackupDestinationID,
	}
	if err := validation.Validate(&entity); err != nil {
		return BackupEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(ctx, db, "backup-scheduled-slot:"+entity.BackupPolicyID.String()+":"+entity.ScheduledAt.UTC().Format(time.RFC3339Nano), db.NewSelect().Model((*BackupEntity)(nil)).Where("backup_policy_id = ?", entity.BackupPolicyID).Where("scheduled_at = ?", entity.ScheduledAt), "scheduledAt", "the backup policy already has a backup in this scheduled slot"); err != nil {
		return BackupEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BackupEntity{}, err
	}
	return entity, nil
}

func (backup) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (BackupEntity, error) {
	var entity BackupEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return BackupEntity{}, err
	}
	return entity, nil
}

type BackupExecutionScopeRecord struct {
	Backup                     BackupEntity    `bun:"embed:backup_"`
	PolicyRetention            json.RawMessage `bun:"policy_retention"`
	PolicyVerification         json.RawMessage `bun:"policy_verification"`
	PolicySettings             json.RawMessage `bun:"policy_settings"`
	DestinationProvider        string          `bun:"destination_provider"`
	DestinationEndpoint        string          `bun:"destination_endpoint"`
	DestinationRegion          string          `bun:"destination_region"`
	DestinationBucket          string          `bun:"destination_bucket"`
	DestinationPrefix          string          `bun:"destination_prefix"`
	DestinationPathStyle       bool            `bun:"destination_path_style"`
	DestinationArchived        bool            `bun:"destination_archived"`
	CredentialProvider         string          `bun:"credential_provider"`
	CredentialPayload          []byte          `bun:"credential_payload"`
	DestinationCredentialID    uuid.UUID       `bun:"destination_credential_id"`
	CredentialArchived         bool            `bun:"credential_archived"`
	CredentialVerified         bool            `bun:"credential_verified"`
	DatabaseClusterID          *uuid.UUID      `bun:"database_cluster_id"`
	DatabaseClusterNodeID      *uuid.UUID      `bun:"database_cluster_node_id"`
	DatabaseNodeInstallationID *uuid.UUID      `bun:"database_node_installation_id"`
	InstallationMethod         string          `bun:"installation_method"`
	InstallationContainer      string          `bun:"installation_container"`
	InstallationServerID       *uuid.UUID      `bun:"installation_server_id"`
	InstallationServerIPv4     string          `bun:"installation_server_ipv4"`
	InstallationArchived       bool            `bun:"installation_archived"`
	DatabaseEngine             string          `bun:"database_engine"`
	ClusterManagementMode      string          `bun:"cluster_management_mode"`
	ResourceSystemManaged      bool            `bun:"resource_system_managed"`
	DatabaseArchived           bool            `bun:"database_archived"`
	ClusterArchived            bool            `bun:"cluster_archived"`
	ResourceID                 *uuid.UUID      `bun:"resource_id"`
	DatabaseName               string          `bun:"database_name"`
	AdministratorUsername      string          `bun:"administrator_username"`
	AdministratorPayload       []byte          `bun:"administrator_payload"`
	AdministratorCount         int             `bun:"administrator_count"`
}

func (backup) FindExecutionScope(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (BackupExecutionScopeRecord, error) {
	var row BackupExecutionScopeRecord
	if err := db.NewSelect().
		TableExpr("backups AS backup").
		ColumnExpr("backup.id AS backup_id, backup.created_at AS backup_created_at, backup.updated_at AS backup_updated_at").
		ColumnExpr("backup.target_type AS backup_target_type, backup.trigger_type AS backup_trigger_type, backup.scheduled_at AS backup_scheduled_at").
		ColumnExpr("backup.strategy AS backup_strategy, backup.driver AS backup_driver, backup.format AS backup_format, backup.format_version AS backup_format_version").
		ColumnExpr("backup.artifact_reference AS backup_artifact_reference, backup.provider_metadata AS backup_provider_metadata, backup.status AS backup_status").
		ColumnExpr("backup.requested_at AS backup_requested_at, backup.started_at AS backup_started_at, backup.uploaded_at AS backup_uploaded_at").
		ColumnExpr("backup.finished_at AS backup_finished_at, backup.verified_at AS backup_verified_at, backup.pruned_at AS backup_pruned_at").
		ColumnExpr("backup.size_bytes AS backup_size_bytes, backup.digest AS backup_digest, backup.producer_version AS backup_producer_version, backup.error AS backup_error").
		ColumnExpr("backup.change_id AS backup_change_id, backup.change_task_id AS backup_change_task_id, backup.backup_policy_id AS backup_backup_policy_id").
		ColumnExpr("backup.server_id AS backup_server_id, backup.database_id AS backup_database_id, backup.database_cluster_id AS backup_database_cluster_id").
		ColumnExpr("backup.database_cluster_node_id AS backup_database_cluster_node_id, backup.database_node_installation_id AS backup_database_node_installation_id").
		ColumnExpr("backup.backup_destination_id AS backup_backup_destination_id").
		ColumnExpr("policy.retention AS policy_retention, policy.verification AS policy_verification, policy.settings AS policy_settings").
		ColumnExpr("destination.provider AS destination_provider, COALESCE(destination.endpoint, '') AS destination_endpoint").
		ColumnExpr("COALESCE(destination.region, '') AS destination_region, destination.bucket AS destination_bucket").
		ColumnExpr("COALESCE(destination.prefix, '') AS destination_prefix, destination.force_path_style AS destination_path_style").
		ColumnExpr("destination.archived_at IS NOT NULL AS destination_archived").
		ColumnExpr("credential.id AS destination_credential_id, credential.provider AS credential_provider, credential.enc_payload AS credential_payload").
		ColumnExpr("credential.archived_at IS NOT NULL AS credential_archived, credential.verified_at IS NOT NULL AS credential_verified").
		ColumnExpr("cluster.id AS database_cluster_id, node.id AS database_cluster_node_id, installation.id AS database_node_installation_id").
		ColumnExpr("COALESCE(installation.installation_method, '') AS installation_method, COALESCE(docker_installation.container_name, '') AS installation_container").
		ColumnExpr("installation.server_id AS installation_server_id, COALESCE(server.ipv4_address, '') AS installation_server_ipv4, installation.archived_at IS NOT NULL AS installation_archived").
		ColumnExpr("COALESCE(cluster.engine, '') AS database_engine, COALESCE(cluster.management_mode, '') AS cluster_management_mode").
		ColumnExpr("COALESCE(resource.system_managed, FALSE) AS resource_system_managed, database.archived_at IS NOT NULL AS database_archived, cluster.archived_at IS NOT NULL AS cluster_archived").
		ColumnExpr("resource.id AS resource_id, COALESCE(database.name, '') AS database_name").
		ColumnExpr("COALESCE(administrator.username, '') AS administrator_username, administrator.enc_payload AS administrator_payload").
		ColumnExpr("(SELECT count(*) FROM database_cluster_credentials AS candidate WHERE candidate.database_cluster_id = cluster.id AND candidate.role = 'administrator' AND candidate.archived_at IS NULL) AS administrator_count").
		Join("JOIN backup_policies AS policy ON policy.id = backup.backup_policy_id").
		Join("JOIN backup_destinations AS destination ON destination.id = backup.backup_destination_id").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id").
		Join("LEFT JOIN databases AS database ON database.id = backup.database_id").
		Join("LEFT JOIN database_clusters AS cluster ON cluster.id = backup.database_cluster_id AND cluster.id = database.database_cluster_id").
		Join("LEFT JOIN database_cluster_nodes AS node ON node.id = backup.database_cluster_node_id AND node.database_cluster_id = cluster.id").
		Join("LEFT JOIN database_node_installations AS installation ON installation.id = backup.database_node_installation_id AND installation.database_cluster_node_id = node.id").
		Join("LEFT JOIN docker_database_node_installations AS docker_installation ON docker_installation.database_node_installation_id = installation.id").
		Join("LEFT JOIN servers AS server ON server.id = installation.server_id AND server.archived_at IS NULL").
		Join("LEFT JOIN database_resources AS database_backing ON database_backing.database_id = database.id").
		Join("LEFT JOIN resources AS resource ON resource.id = database_backing.resource_id").
		Join("LEFT JOIN LATERAL (SELECT cluster_credential.username, cluster_credential.enc_payload FROM database_cluster_credentials AS cluster_credential WHERE cluster_credential.database_cluster_id = cluster.id AND cluster_credential.role = 'administrator' AND cluster_credential.archived_at IS NULL ORDER BY cluster_credential.created_at LIMIT 1) AS administrator ON TRUE").
		Where("backup.id = ?", id).
		Scan(ctx, &row); err != nil {
		return BackupExecutionScopeRecord{}, err
	}
	return row, nil
}

type DatabaseBackupHistory struct {
	ID                uuid.UUID  `bun:"id"`
	Status            string     `bun:"status"`
	TriggerType       string     `bun:"trigger_type"`
	ScheduledAt       time.Time  `bun:"scheduled_at"`
	FinishedAt        *time.Time `bun:"finished_at"`
	VerifiedAt        *time.Time `bun:"verified_at"`
	SizeBytes         *int64     `bun:"size_bytes"`
	Error             string     `bun:"error"`
	ArtifactReference string     `bun:"artifact_reference"`
}

func (backup) RecentForDatabase(
	ctx context.Context,
	db storage.Executor,
	databaseID uuid.UUID,
	limit int,
) ([]DatabaseBackupHistory, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	items := make([]DatabaseBackupHistory, 0, limit)
	err := db.NewSelect().TableExpr("backups").
		ColumnExpr("id, status, trigger_type, scheduled_at, finished_at, verified_at, size_bytes").
		ColumnExpr("LEFT(COALESCE(error, ''), 800) AS error, artifact_reference").
		Where("database_id = ?", databaseID).
		OrderExpr("scheduled_at DESC").
		Limit(limit).
		Scan(ctx, &items)
	return items, err
}

func (backup) FindVerifiedByPolicy(
	ctx context.Context,
	db storage.Executor,
	policyID uuid.UUID,
) ([]BackupEntity, error) {
	var backups []BackupEntity
	if err := db.NewSelect().
		Model(&backups).
		Where("backups.backup_policy_id = ?", policyID).
		Where("backups.status = ?", BackupStatusVerified).
		Where("NOT EXISTS (SELECT 1 FROM database_restores AS restore WHERE (restore.backup_id = backups.id OR restore.safety_backup_id = backups.id) AND restore.status IN ('pending', 'safety_backup', 'restoring'))").
		OrderExpr("backups.scheduled_at DESC").
		Scan(ctx, &backups); err != nil {
		return nil, err
	}
	return backups, nil
}

func (backup) ExistsForPolicy(
	ctx context.Context,
	db storage.Executor,
	policyID uuid.UUID,
) (bool, error) {
	count, err := db.NewSelect().
		Model((*BackupEntity)(nil)).
		Where("backup_policy_id = ?", policyID).
		Count(ctx)
	return count > 0, err
}

func SelectBackupsToPrune(
	backups []BackupEntity,
	retention BackupRetentionPolicy,
) []BackupEntity {
	if len(backups) <= 1 {
		return nil
	}
	keep := map[uuid.UUID]bool{backups[0].ID: true}
	for index := 0; index < min(retention.KeepLast, len(backups)); index++ {
		keep[backups[index].ID] = true
	}
	daily := map[string]bool{}
	weekly := map[string]bool{}
	monthly := map[string]bool{}
	for _, candidate := range backups {
		date := candidate.ScheduledAt.UTC()
		dayKey := date.Format("2006-01-02")
		year, week := date.ISOWeek()
		weekKey := fmt.Sprintf("%04d-%02d", year, week)
		monthKey := date.Format("2006-01")
		if len(daily) < retention.KeepDaily && !daily[dayKey] {
			daily[dayKey] = true
			keep[candidate.ID] = true
		}
		if len(weekly) < retention.KeepWeekly && !weekly[weekKey] {
			weekly[weekKey] = true
			keep[candidate.ID] = true
		}
		if len(monthly) < retention.KeepMonthly && !monthly[monthKey] {
			monthly[monthKey] = true
			keep[candidate.ID] = true
		}
	}
	prune := make([]BackupEntity, 0, len(backups))
	for _, candidate := range backups[1:] {
		if !keep[candidate.ID] {
			prune = append(prune, candidate)
		}
	}
	return prune
}

func (backup) Claim(ctx context.Context, db storage.Executor, id uuid.UUID) (bool, error) {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("status = ?", BackupStatusRunning).
		Set("started_at = COALESCE(started_at, ?)", now).
		Set("updated_at = ?", now).
		Set("error = NULL").
		Where("id = ?", id).
		Where("status IN (?, ?, ?)", BackupStatusPending, BackupStatusRunning, BackupStatusFailed).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (backup) MarkFailed(ctx context.Context, db storage.Executor, id uuid.UUID, operationErr error) error {
	message := boundedBackupError(operationErr)
	_, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("status = ?", BackupStatusFailed).
		Set("error = ?", message).
		Set("finished_at = ?", time.Now().UTC()).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("status IN (?, ?, ?)", BackupStatusPending, BackupStatusRunning, BackupStatusFailed).
		Exec(ctx)
	return err
}

type UploadedBackupData struct {
	ArtifactReference string
	ProviderMetadata  json.RawMessage
	SizeBytes         int64
	Digest            []byte
}

func (backup) MarkUploaded(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	data UploadedBackupData,
) error {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("artifact_reference = ?", data.ArtifactReference).
		Set("provider_metadata = ?", data.ProviderMetadata).
		Set("size_bytes = ?", data.SizeBytes).
		Set("digest = ?", data.Digest).
		Set("status = ?", BackupStatusUploaded).
		Set("uploaded_at = ?", now).
		Set("finished_at = ?", now).
		Set("updated_at = ?", now).
		Set("error = NULL").
		Where("id = ?", id).
		Where("status = ?", BackupStatusRunning).
		Where("artifact_reference = ''").
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("backup artifact identity is already immutable or lifecycle changed")
	}
	return nil
}

func (backup) MarkVerified(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("status = ?", BackupStatusVerified).
		Set("verified_at = ?", now).
		Set("updated_at = ?", now).
		Set("error = NULL").
		Where("id = ?", id).
		Where("status IN (?, ?)", BackupStatusUploaded, BackupStatusVerificationFailed).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("backup lifecycle changed before verification completed")
	}
	return nil
}

func (backup) MarkVerificationFailed(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	verificationErr error,
) error {
	message := boundedBackupError(verificationErr)
	result, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("status = ?", BackupStatusVerificationFailed).
		Set("error = ?", message).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("status IN (?, ?)", BackupStatusUploaded, BackupStatusVerificationFailed).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("backup lifecycle changed before verification failure was recorded")
	}
	return nil
}

func boundedBackupError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 800 {
		return message[:800]
	}
	return message
}

func (backup) MarkPruned(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*BackupEntity)(nil)).
		Set("status = ?", BackupStatusPruned).
		Set("pruned_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("status = ?", BackupStatusVerified).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("backup lifecycle changed before pruning was recorded")
	}
	return nil
}

type BackupHealthPolicy struct {
	PolicyID         uuid.UUID  `json:"policyId" bun:"policy_id"`
	TargetType       string     `json:"targetType" bun:"target_type"`
	Schedule         string     `json:"schedule" bun:"schedule"`
	Provider         string     `json:"provider" bun:"provider"`
	Bucket           string     `json:"bucket" bun:"bucket"`
	Prefix           string     `json:"prefix" bun:"prefix"`
	LastStatus       string     `json:"lastStatus" bun:"last_status"`
	LastError        string     `json:"lastError" bun:"last_error"`
	LastSuccessfulAt *time.Time `json:"lastSuccessfulAt" bun:"last_successful_at"`
	LastVerifiedAt   *time.Time `json:"lastVerifiedAt" bun:"last_verified_at"`
	LastSizeBytes    int64      `json:"lastSizeBytes" bun:"last_size_bytes"`
	ActiveOrRetrying bool       `json:"activeOrRetrying" bun:"active_or_retrying"`
}

func (backup) FindSystemHealth(
	ctx context.Context,
	db storage.Executor,
) ([]BackupHealthPolicy, error) {
	var policies []BackupHealthPolicy
	if err := db.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id AS policy_id, policy.target_type, policy.schedule").
		ColumnExpr("destination.provider, destination.bucket, COALESCE(destination.prefix, '') AS prefix").
		ColumnExpr("COALESCE(latest.status, '') AS last_status, COALESCE(latest.error, '') AS last_error").
		ColumnExpr("verified.finished_at AS last_successful_at, verified.verified_at AS last_verified_at").
		ColumnExpr("COALESCE(verified.size_bytes, 0) AS last_size_bytes").
		ColumnExpr("EXISTS (SELECT 1 FROM backups active WHERE active.backup_policy_id = policy.id AND active.status IN ('pending', 'running', 'failed', 'uploaded', 'verification_failed')) AS active_or_retrying").
		Join("JOIN backup_destinations AS destination ON destination.id = policy.backup_destination_id AND destination.archived_at IS NULL").
		Join("LEFT JOIN LATERAL (SELECT status, error FROM backups WHERE backup_policy_id = policy.id ORDER BY scheduled_at DESC LIMIT 1) AS latest ON TRUE").
		Join("LEFT JOIN LATERAL (SELECT finished_at, verified_at, size_bytes FROM backups WHERE backup_policy_id = policy.id AND status = 'verified' ORDER BY verified_at DESC LIMIT 1) AS verified ON TRUE").
		Where("policy.archived_at IS NULL").
		OrderExpr("policy.target_type DESC").
		Scan(ctx, &policies); err != nil {
		return nil, err
	}
	return policies, nil
}
