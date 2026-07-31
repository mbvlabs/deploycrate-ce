package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"
)

type BackupPolicyNullTime = sql.NullTime

type BackupPolicyEntity struct {
	bun.BaseModel       `bun:"table:backup_policies,alias:backup_policies"`
	ID                  uuid.UUID            `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time            `bun:"created_at"`
	UpdatedAt           time.Time            `bun:"updated_at"`
	Name                string               `bun:"name"`
	Schedule            string               `bun:"schedule"`
	Strategy            string               `bun:"strategy"`
	Driver              string               `bun:"driver"`
	Retention           json.RawMessage      `bun:"retention,type:jsonb"`
	Format              string               `bun:"format"`
	Verification        json.RawMessage      `bun:"verification,type:jsonb"`
	Settings            json.RawMessage      `bun:"settings,type:jsonb"`
	ArchivedAt          BackupPolicyNullTime `bun:"archived_at"`
	ActivatedAt         BackupPolicyNullTime `bun:"activated_at"`
	TargetType          string               `bun:"target_type"`
	ServerID            *uuid.UUID           `bun:"server_id,type:uuid"`
	DatabaseID          *uuid.UUID           `bun:"database_id,type:uuid"`
	NextRunAt           time.Time            `bun:"next_run_at"`
	LastScheduledAt     BackupPolicyNullTime `bun:"last_scheduled_at"`
	BackupDestinationID uuid.UUID            `bun:"backup_destination_id,type:uuid"`
}

func (e *BackupPolicyEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil {
		builder.Add("id", "required", "backup policy ID is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		builder.Add("name", "required", "backup policy name is required")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(e.Schedule); err != nil {
		builder.Add("schedule", "invalid", "backup policy schedule must be a five-field cron expression")
	}
	if e.TargetType == "server" {
		if e.ServerID == nil || *e.ServerID == uuid.Nil || e.DatabaseID != nil {
			builder.Add("target_type", "incoherent", "server policies must target only one server")
		}
		if e.Strategy != "filesystem" || e.Driver != "restic" || e.Format != "restic" {
			builder.Add("driver", "incompatible", "server policies require filesystem, restic, and restic")
		}
	} else if e.TargetType == "database" {
		if e.ServerID != nil || e.DatabaseID == nil || *e.DatabaseID == uuid.Nil {
			builder.Add("target_type", "incoherent", "database policies must target one Database")
		}
		if e.Strategy != "logical" || e.Driver != "postgresql" || e.Format != "tar.age" {
			builder.Add("driver", "incompatible", "database policies require logical, postgresql, and tar.age")
		}
	} else {
		builder.Add("target_type", "unsupported", "backup policy target must be server or database")
	}
	if !validJSONObject(e.Retention) {
		builder.Add("retention", "invalid", "backup retention must be a JSON object")
	} else if !validRetentionDocument(e.Retention) {
		builder.Add("retention", "invalid", "backup retention must preserve at least one recovery point")
	}
	if !validJSONObject(e.Verification) {
		builder.Add("verification", "invalid", "backup verification settings must be a JSON object")
	}
	if !validJSONObject(e.Settings) {
		builder.Add("settings", "invalid", "backup settings must be a JSON object")
	}
	if e.ActivatedAt.Valid && e.ActivatedAt.Time.IsZero() {
		builder.Add("activated_at", "invalid", "backup policy activation time is invalid")
	}
	if e.NextRunAt.IsZero() {
		builder.Add("next_run_at", "required", "backup policy next run time is required")
	}
	if e.ActivatedAt.Valid && !e.NextRunAt.IsZero() &&
		!e.NextRunAt.After(e.ActivatedAt.Time) {
		builder.Add("next_run_at", "invalid", "active backup policy next run must follow activation")
	}
	if e.LastScheduledAt.Valid && !e.NextRunAt.IsZero() &&
		!e.NextRunAt.After(e.LastScheduledAt.Time) {
		builder.Add("next_run_at", "invalid", "backup policy next run must follow its last scheduled run")
	}
	if e.BackupDestinationID == uuid.Nil {
		builder.Add("backup_destination_id", "required", "backup destination is required")
	}
	return builder.Err()
}

func (e BackupPolicyEntity) Schedulable() bool {
	return e.ActivatedAt.Valid && !e.ArchivedAt.Valid
}

func validJSONObject(value json.RawMessage) bool {
	if len(value) == 0 || !json.Valid(value) {
		return false
	}
	var object map[string]any
	return json.Unmarshal(value, &object) == nil && object != nil
}

func validRetentionDocument(value json.RawMessage) bool {
	var retention BackupRetentionPolicy
	if json.Unmarshal(value, &retention) != nil {
		return false
	}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 ||
		retention.KeepWeekly < 0 || retention.KeepMonthly < 0 {
		return false
	}
	return retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly > 0
}

type BackupRetentionPolicy struct {
	KeepLast    int `json:"keep_last"`
	KeepDaily   int `json:"keep_daily"`
	KeepWeekly  int `json:"keep_weekly"`
	KeepMonthly int `json:"keep_monthly"`
}

func (e BackupPolicyEntity) RetentionPolicy() (BackupRetentionPolicy, error) {
	var retention BackupRetentionPolicy
	if err := json.Unmarshal(e.Retention, &retention); err != nil {
		return BackupRetentionPolicy{}, err
	}
	if !validRetentionDocument(e.Retention) {
		return BackupRetentionPolicy{}, errors.New("backup retention policy is invalid")
	}
	return retention, nil
}

func NextBackupRun(expression string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(after), nil
}

type ScheduledBackupPolicy struct {
	ID                         uuid.UUID  `bun:"id"`
	Schedule                   string     `bun:"schedule"`
	Strategy                   string     `bun:"strategy"`
	Driver                     string     `bun:"driver"`
	Format                     string     `bun:"format"`
	TargetType                 string     `bun:"target_type"`
	ServerID                   *uuid.UUID `bun:"server_id"`
	DatabaseID                 *uuid.UUID `bun:"database_id"`
	DatabaseClusterID          *uuid.UUID `bun:"database_cluster_id"`
	DatabaseClusterNodeID      *uuid.UUID `bun:"database_cluster_node_id"`
	DatabaseNodeInstallationID *uuid.UUID `bun:"database_node_installation_id"`
	ExecutionServerID          *uuid.UUID `bun:"execution_server_id"`
	BackupDestinationID        uuid.UUID  `bun:"backup_destination_id"`
	NextRunAt                  time.Time  `bun:"next_run_at"`
	EnvironmentID              uuid.UUID  `bun:"environment_id"`
}

type BackupPolicySchedule struct {
	ID        uuid.UUID `bun:"id"`
	NextRunAt time.Time `bun:"next_run_at"`
}

func (bp backupPolicy) ActiveSchedules(
	ctx context.Context,
	db storage.Executor,
) ([]BackupPolicySchedule, error) {
	var schedules []BackupPolicySchedule
	if err := db.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id, policy.next_run_at").
		Where("policy.archived_at IS NULL").
		Where("policy.activated_at IS NOT NULL").
		OrderExpr("policy.next_run_at ASC, policy.id ASC").
		Scan(ctx, &schedules); err != nil {
		return nil, err
	}
	return schedules, nil
}

func scheduledBackupPoliciesQuery(db storage.Executor) *bun.SelectQuery {
	return db.NewSelect().
		TableExpr("backup_policies AS policy").
		ColumnExpr("policy.id, policy.schedule, policy.strategy, policy.driver, policy.format").
		ColumnExpr("policy.target_type, policy.server_id, policy.database_id").
		ColumnExpr("database.database_cluster_id, database_node.id AS database_cluster_node_id, database_installation.id AS database_node_installation_id").
		ColumnExpr("database_installation.server_id AS execution_server_id").
		ColumnExpr("policy.backup_destination_id, policy.next_run_at").
		ColumnExpr("COALESCE(server_target.environment_id, system_environment.environment_id) AS environment_id").
		Join("LEFT JOIN LATERAL (SELECT environment_id FROM environment_targets WHERE server_id = policy.server_id AND detached_at IS NULL ORDER BY attached_at DESC LIMIT 1) AS server_target ON TRUE").
		Join("LEFT JOIN databases AS database ON database.id = policy.database_id AND database.archived_at IS NULL").
		Join("LEFT JOIN database_cluster_nodes AS database_node ON database_node.database_cluster_id = database.database_cluster_id AND database_node.role = 'primary' AND database_node.archived_at IS NULL").
		Join("LEFT JOIN database_node_installations AS database_installation ON database_installation.database_cluster_node_id = database_node.id AND database_installation.archived_at IS NULL").
		Join("LEFT JOIN LATERAL (SELECT environment.id AS environment_id FROM environments AS environment JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL WHERE application.slug = ? AND environment.archived_at IS NULL ORDER BY environment.created_at LIMIT 1) AS system_environment ON TRUE", SystemApplicationSlug)
}

func (bp backupPolicy) FindScheduled(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ScheduledBackupPolicy, error) {
	var policy ScheduledBackupPolicy
	if err := scheduledBackupPoliciesQuery(db).
		Where("policy.id = ?", id).
		Scan(ctx, &policy); err != nil {
		return ScheduledBackupPolicy{}, err
	}
	return policy, nil
}

func (bp backupPolicy) FindForUpdate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (BackupPolicyEntity, error) {
	var policy BackupPolicyEntity
	if err := db.NewSelect().
		Model(&policy).
		Where("id = ?", id).
		For("UPDATE").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}
	return policy, nil
}

func (bp backupPolicy) FindInactiveInstancePoliciesForUpdate(
	ctx context.Context,
	db storage.Executor,
	instanceID string,
) ([]BackupPolicyEntity, error) {
	var policies []BackupPolicyEntity
	if err := db.NewSelect().
		Model(&policies).
		Join("JOIN backup_destinations AS destination ON destination.id = backup_policies.backup_destination_id AND destination.archived_at IS NULL").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id AND credential.archived_at IS NULL").
		Where("credential.metadata ->> 'instance_id' = ?", instanceID).
		Where("backup_policies.archived_at IS NULL").
		Where("backup_policies.activated_at IS NULL").
		OrderExpr("backup_policies.created_at ASC").
		For("UPDATE OF backup_policies").
		Scan(ctx); err != nil {
		return nil, err
	}
	return policies, nil
}

func (bp backupPolicy) Activate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	activatedAt, nextRunAt time.Time,
) error {
	_, err := db.NewUpdate().
		Model((*BackupPolicyEntity)(nil)).
		Set("activated_at = ?", activatedAt).
		Set("next_run_at = ?", nextRunAt).
		Set("updated_at = ?", activatedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (bp backupPolicy) ReplaceSchedule(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	schedule string,
	nextRunAt, updatedAt time.Time,
) error {
	_, err := db.NewUpdate().
		Model((*BackupPolicyEntity)(nil)).
		Set("schedule = ?", schedule).
		Set("next_run_at = ?", nextRunAt).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (bp backupPolicy) AdvanceSchedule(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	lastRun, nextRun, updatedAt time.Time,
) error {
	_, err := db.NewUpdate().
		Model((*BackupPolicyEntity)(nil)).
		Set("last_scheduled_at = ?", lastRun).
		Set("next_run_at = ?", nextRun).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (bp backupPolicy) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (BackupPolicyEntity, error) {
	var entity BackupPolicyEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

type CreateBackupPolicyData struct {
	Name                string
	Schedule            string
	Strategy            string
	Driver              string
	Retention           json.RawMessage
	Format              string
	Verification        json.RawMessage
	Settings            json.RawMessage
	ArchivedAt          sql.NullTime
	ActivatedAt         sql.NullTime
	TargetType          string
	ServerID            *uuid.UUID
	DatabaseID          *uuid.UUID
	NextRunAt           time.Time
	LastScheduledAt     sql.NullTime
	BackupDestinationID uuid.UUID
}

func (bp backupPolicy) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupPolicyData,
) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Name:                data.Name,
		Schedule:            data.Schedule,
		Strategy:            data.Strategy,
		Driver:              data.Driver,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
		ActivatedAt:         data.ActivatedAt,
		TargetType:          data.TargetType,
		ServerID:            data.ServerID,
		DatabaseID:          data.DatabaseID,
		NextRunAt:           data.NextRunAt,
		LastScheduledAt:     data.LastScheduledAt,
		BackupDestinationID: data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if entity.TargetType == "database" && entity.DatabaseID != nil {
		if err := ensureActiveUnique(ctx, db, "database-backup-policy:"+entity.DatabaseID.String(), entity.ID, db.NewSelect().Model((*BackupPolicyEntity)(nil)).Where("target_type = 'database'").Where("database_id = ?", *entity.DatabaseID), "databaseId", "the Database already has an active backup policy"); err != nil {
			return BackupPolicyEntity{}, err
		}
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

type UpdateBackupPolicyData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	Name                string
	Schedule            string
	Strategy            string
	Driver              string
	Retention           json.RawMessage
	Format              string
	Verification        json.RawMessage
	Settings            json.RawMessage
	ArchivedAt          sql.NullTime
	ActivatedAt         sql.NullTime
	TargetType          string
	ServerID            *uuid.UUID
	DatabaseID          *uuid.UUID
	NextRunAt           time.Time
	LastScheduledAt     sql.NullTime
	BackupDestinationID uuid.UUID
}

func (bp backupPolicy) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateBackupPolicyData,
) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		Name:                data.Name,
		Schedule:            data.Schedule,
		Strategy:            data.Strategy,
		Driver:              data.Driver,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
		ActivatedAt:         data.ActivatedAt,
		TargetType:          data.TargetType,
		ServerID:            data.ServerID,
		DatabaseID:          data.DatabaseID,
		NextRunAt:           data.NextRunAt,
		LastScheduledAt:     data.LastScheduledAt,
		BackupDestinationID: data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if entity.TargetType == "database" && entity.DatabaseID != nil {
		if err := ensureActiveUnique(ctx, db, "database-backup-policy:"+entity.DatabaseID.String(), entity.ID, db.NewSelect().Model((*BackupPolicyEntity)(nil)).Where("target_type = 'database'").Where("database_id = ?", *entity.DatabaseID), "databaseId", "the Database already has an active backup policy"); err != nil {
			return BackupPolicyEntity{}, err
		}
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("schedule").
		Column("strategy").
		Column("driver").
		Column("retention").
		Column("format").
		Column("verification").
		Column("settings").
		Column("archived_at").
		Column("activated_at").
		Column("target_type").
		Column("server_id").
		Column("database_id").
		Column("next_run_at").
		Column("last_scheduled_at").
		Column("backup_destination_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

func (bp backupPolicy) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*BackupPolicyEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (bp backupPolicy) All(ctx context.Context, db storage.Executor) ([]BackupPolicyEntity, error) {
	var entities []BackupPolicyEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedBackupPolicies struct {
	BackupPolicies []BackupPolicyEntity
	TotalCount     int64
	Page           int64
	PageSize       int64
	TotalPages     int64
}

func (bp backupPolicy) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedBackupPolicies, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	totalCount, err := db.NewSelect().
		Model(&BackupPolicyEntity{}).Count(ctx)
	if err != nil {
		return PaginatedBackupPolicies{}, err
	}

	entities := make([]BackupPolicyEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedBackupPolicies{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedBackupPolicies{
		BackupPolicies: entities,
		TotalCount:     int64(totalCount),
		Page:           page,
		PageSize:       pageSize,
		TotalPages:     totalPages,
	}, nil
}

func (bp backupPolicy) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupPolicyData,
) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Name:                data.Name,
		Schedule:            data.Schedule,
		Strategy:            data.Strategy,
		Driver:              data.Driver,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
		ActivatedAt:         data.ActivatedAt,
		TargetType:          data.TargetType,
		ServerID:            data.ServerID,
		DatabaseID:          data.DatabaseID,
		NextRunAt:           data.NextRunAt,
		LastScheduledAt:     data.LastScheduledAt,
		BackupDestinationID: data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("schedule = excluded.schedule").
		Set("strategy = excluded.strategy").
		Set("driver = excluded.driver").
		Set("retention = excluded.retention").
		Set("format = excluded.format").
		Set("verification = excluded.verification").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Set("activated_at = excluded.activated_at").
		Set("target_type = excluded.target_type").
		Set("server_id = excluded.server_id").
		Set("database_id = excluded.database_id").
		Set("next_run_at = excluded.next_run_at").
		Set("last_scheduled_at = excluded.last_scheduled_at").
		Set("backup_destination_id = excluded.backup_destination_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}
