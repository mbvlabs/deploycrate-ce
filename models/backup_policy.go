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

type BackupPolicyEntity struct {
	bun.BaseModel         `bun:"table:backup_policies,alias:backup_policies"`
	ID                    uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt             time.Time       `bun:"created_at"`
	UpdatedAt             time.Time       `bun:"updated_at"`
	Name                  string          `bun:"name"`
	Schedule              string          `bun:"schedule"`
	Strategy              string          `bun:"strategy"`
	Driver                string          `bun:"driver"`
	Retention             json.RawMessage `bun:"retention,type:jsonb"`
	Format                string          `bun:"format"`
	Verification          json.RawMessage `bun:"verification,type:jsonb"`
	Settings              json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt            sql.NullTime    `bun:"archived_at"`
	TargetType            string          `bun:"target_type"`
	ServerID              *uuid.UUID      `bun:"server_id,type:uuid"`
	ResourceID            *uuid.UUID      `bun:"resource_id,type:uuid"`
	EnvironmentResourceID *uuid.UUID      `bun:"environment_resource_id,type:uuid"`
	ResourceVolumeID      *uuid.UUID      `bun:"resource_volume_id,type:uuid"`
	NextRunAt             time.Time       `bun:"next_run_at"`
	LastScheduledAt       sql.NullTime    `bun:"last_scheduled_at"`
	BackupDestinationID   uuid.UUID       `bun:"backup_destination_id,type:uuid"`
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
		if e.ServerID == nil || *e.ServerID == uuid.Nil || e.ResourceID != nil ||
			e.EnvironmentResourceID != nil || e.ResourceVolumeID != nil {
			builder.Add("target_type", "incoherent", "server policies must target only one server")
		}
		if e.Strategy != "filesystem" || e.Driver != "restic" || e.Format != "restic" {
			builder.Add("driver", "incompatible", "server policies require filesystem, restic, and restic")
		}
	} else if e.TargetType == "resource" {
		if e.ServerID != nil || e.ResourceID == nil || *e.ResourceID == uuid.Nil {
			builder.Add("target_type", "incoherent", "resource policies must target one resource")
		}
		if e.Strategy != "logical" || e.Driver != "postgresql" || e.Format != "tar.age" {
			builder.Add("driver", "incompatible", "database policies require logical, postgresql, and tar.age")
		}
	} else {
		builder.Add("target_type", "unsupported", "backup policy target must be server or resource")
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
	if e.NextRunAt.IsZero() {
		builder.Add("next_run_at", "required", "backup policy next run time is required")
	}
	if e.BackupDestinationID == uuid.Nil {
		builder.Add("backup_destination_id", "required", "backup destination is required")
	}
	return builder.Err()
}

func validJSONObject(value json.RawMessage) bool {
	if len(value) == 0 || !json.Valid(value) {
		return false
	}
	var object map[string]any
	return json.Unmarshal(value, &object) == nil && object != nil
}

func validRetentionDocument(value json.RawMessage) bool {
	var retention struct {
		KeepLast    int `json:"keep_last"`
		KeepDaily   int `json:"keep_daily"`
		KeepWeekly  int `json:"keep_weekly"`
		KeepMonthly int `json:"keep_monthly"`
	}
	if json.Unmarshal(value, &retention) != nil {
		return false
	}
	if retention.KeepLast < 0 || retention.KeepDaily < 0 ||
		retention.KeepWeekly < 0 || retention.KeepMonthly < 0 {
		return false
	}
	return retention.KeepLast+retention.KeepDaily+retention.KeepWeekly+retention.KeepMonthly > 0
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
	Name                  string
	Schedule              string
	Strategy              string
	Driver                string
	Retention             json.RawMessage
	Format                string
	Verification          json.RawMessage
	Settings              json.RawMessage
	ArchivedAt            sql.NullTime
	TargetType            string
	ServerID              *uuid.UUID
	ResourceID            *uuid.UUID
	EnvironmentResourceID *uuid.UUID
	ResourceVolumeID      *uuid.UUID
	NextRunAt             time.Time
	LastScheduledAt       sql.NullTime
	BackupDestinationID   uuid.UUID
}

func (bp backupPolicy) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupPolicyData,
) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Schedule:              data.Schedule,
		Strategy:              data.Strategy,
		Driver:                data.Driver,
		Retention:             data.Retention,
		Format:                data.Format,
		Verification:          data.Verification,
		Settings:              data.Settings,
		ArchivedAt:            data.ArchivedAt,
		TargetType:            data.TargetType,
		ServerID:              data.ServerID,
		ResourceID:            data.ResourceID,
		EnvironmentResourceID: data.EnvironmentResourceID,
		ResourceVolumeID:      data.ResourceVolumeID,
		NextRunAt:             data.NextRunAt,
		LastScheduledAt:       data.LastScheduledAt,
		BackupDestinationID:   data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

type UpdateBackupPolicyData struct {
	ID                    uuid.UUID
	UpdatedAt             time.Time
	Name                  string
	Schedule              string
	Strategy              string
	Driver                string
	Retention             json.RawMessage
	Format                string
	Verification          json.RawMessage
	Settings              json.RawMessage
	ArchivedAt            sql.NullTime
	TargetType            string
	ServerID              *uuid.UUID
	ResourceID            *uuid.UUID
	EnvironmentResourceID *uuid.UUID
	ResourceVolumeID      *uuid.UUID
	NextRunAt             time.Time
	LastScheduledAt       sql.NullTime
	BackupDestinationID   uuid.UUID
}

func (bp backupPolicy) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateBackupPolicyData,
) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                    data.ID,
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Schedule:              data.Schedule,
		Strategy:              data.Strategy,
		Driver:                data.Driver,
		Retention:             data.Retention,
		Format:                data.Format,
		Verification:          data.Verification,
		Settings:              data.Settings,
		ArchivedAt:            data.ArchivedAt,
		TargetType:            data.TargetType,
		ServerID:              data.ServerID,
		ResourceID:            data.ResourceID,
		EnvironmentResourceID: data.EnvironmentResourceID,
		ResourceVolumeID:      data.ResourceVolumeID,
		NextRunAt:             data.NextRunAt,
		LastScheduledAt:       data.LastScheduledAt,
		BackupDestinationID:   data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
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
		Column("target_type").
		Column("server_id").
		Column("resource_id").
		Column("environment_resource_id").
		Column("resource_volume_id").
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
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Schedule:              data.Schedule,
		Strategy:              data.Strategy,
		Driver:                data.Driver,
		Retention:             data.Retention,
		Format:                data.Format,
		Verification:          data.Verification,
		Settings:              data.Settings,
		ArchivedAt:            data.ArchivedAt,
		TargetType:            data.TargetType,
		ServerID:              data.ServerID,
		ResourceID:            data.ResourceID,
		EnvironmentResourceID: data.EnvironmentResourceID,
		ResourceVolumeID:      data.ResourceVolumeID,
		NextRunAt:             data.NextRunAt,
		LastScheduledAt:       data.LastScheduledAt,
		BackupDestinationID:   data.BackupDestinationID,
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
		Set("target_type = excluded.target_type").
		Set("server_id = excluded.server_id").
		Set("resource_id = excluded.resource_id").
		Set("environment_resource_id = excluded.environment_resource_id").
		Set("resource_volume_id = excluded.resource_volume_id").
		Set("next_run_at = excluded.next_run_at").
		Set("last_scheduled_at = excluded.last_scheduled_at").
		Set("backup_destination_id = excluded.backup_destination_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}
