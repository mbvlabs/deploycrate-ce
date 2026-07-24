package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceBackupEntity struct {
	bun.BaseModel          `bun:"table:resource_backups,alias:resource_backups"`
	ID                     uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time      `bun:"created_at"`
	UpdatedAt              time.Time      `bun:"updated_at"`
	TriggerType            string         `bun:"trigger_type"`
	Strategy               string         `bun:"strategy"`
	Driver                 string         `bun:"driver"`
	Format                 string         `bun:"format"`
	ObjectKey              string         `bun:"object_key"`
	Status                 string         `bun:"status"`
	RequestedAt            time.Time      `bun:"requested_at"`
	StartedAt              sql.NullTime   `bun:"started_at"`
	FinishedAt             sql.NullTime   `bun:"finished_at"`
	SizeBytes              sql.NullInt64  `bun:"size_bytes"`
	Digest                 []byte         `bun:"digest"`
	VerifiedAt             sql.NullTime   `bun:"verified_at"`
	Error                  sql.NullString `bun:"error"`
	ChangeID               uuid.UUID      `bun:"change_id,type:uuid"`
	ChangeTaskID           uuid.UUID      `bun:"change_task_id,type:uuid"`
	BackupPolicyID         uuid.UUID      `bun:"backup_policy_id,type:uuid"`
	ResourceID             uuid.UUID      `bun:"resource_id,type:uuid"`
	EnvironmentResourceID  *uuid.UUID     `bun:"environment_resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID     `bun:"resource_installation_id,type:uuid"`
	ResourceVolumeID       *uuid.UUID     `bun:"resource_volume_id,type:uuid"`
	BackupDestinationID    uuid.UUID      `bun:"backup_destination_id,type:uuid"`
}

func (e *ResourceBackupEntity) Validate() error {
	return nil
}

func (rb resourceBackup) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceBackupEntity, error) {
	var entity ResourceBackupEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceBackupEntity{}, err
	}

	return entity, nil
}

type CreateResourceBackupData struct {
	TriggerType            string
	Strategy               string
	Driver                 string
	Format                 string
	ObjectKey              string
	Status                 string
	RequestedAt            time.Time
	StartedAt              sql.NullTime
	FinishedAt             sql.NullTime
	SizeBytes              sql.NullInt64
	Digest                 []byte
	VerifiedAt             sql.NullTime
	Error                  sql.NullString
	ChangeID               uuid.UUID
	ChangeTaskID           uuid.UUID
	BackupPolicyID         uuid.UUID
	ResourceID             uuid.UUID
	EnvironmentResourceID  *uuid.UUID
	ResourceInstallationID *uuid.UUID
	ResourceVolumeID       *uuid.UUID
	BackupDestinationID    uuid.UUID
}

func (rb resourceBackup) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceBackupData,
) (ResourceBackupEntity, error) {
	entity := ResourceBackupEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		TriggerType:            data.TriggerType,
		Strategy:               data.Strategy,
		Driver:                 data.Driver,
		Format:                 data.Format,
		ObjectKey:              data.ObjectKey,
		Status:                 data.Status,
		RequestedAt:            data.RequestedAt,
		StartedAt:              data.StartedAt,
		FinishedAt:             data.FinishedAt,
		SizeBytes:              data.SizeBytes,
		Digest:                 data.Digest,
		VerifiedAt:             data.VerifiedAt,
		Error:                  data.Error,
		ChangeID:               data.ChangeID,
		ChangeTaskID:           data.ChangeTaskID,
		BackupPolicyID:         data.BackupPolicyID,
		ResourceID:             data.ResourceID,
		EnvironmentResourceID:  data.EnvironmentResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceVolumeID:       data.ResourceVolumeID,
		BackupDestinationID:    data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBackupEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceBackupEntity{}, err
	}

	return entity, nil
}

type UpdateResourceBackupData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	TriggerType            string
	Strategy               string
	Driver                 string
	Format                 string
	ObjectKey              string
	Status                 string
	RequestedAt            time.Time
	StartedAt              sql.NullTime
	FinishedAt             sql.NullTime
	SizeBytes              sql.NullInt64
	Digest                 []byte
	VerifiedAt             sql.NullTime
	Error                  sql.NullString
	ChangeID               uuid.UUID
	ChangeTaskID           uuid.UUID
	BackupPolicyID         uuid.UUID
	ResourceID             uuid.UUID
	EnvironmentResourceID  *uuid.UUID
	ResourceInstallationID *uuid.UUID
	ResourceVolumeID       *uuid.UUID
	BackupDestinationID    uuid.UUID
}

func (rb resourceBackup) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceBackupData,
) (ResourceBackupEntity, error) {
	entity := ResourceBackupEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		TriggerType:            data.TriggerType,
		Strategy:               data.Strategy,
		Driver:                 data.Driver,
		Format:                 data.Format,
		ObjectKey:              data.ObjectKey,
		Status:                 data.Status,
		RequestedAt:            data.RequestedAt,
		StartedAt:              data.StartedAt,
		FinishedAt:             data.FinishedAt,
		SizeBytes:              data.SizeBytes,
		Digest:                 data.Digest,
		VerifiedAt:             data.VerifiedAt,
		Error:                  data.Error,
		ChangeID:               data.ChangeID,
		ChangeTaskID:           data.ChangeTaskID,
		BackupPolicyID:         data.BackupPolicyID,
		ResourceID:             data.ResourceID,
		EnvironmentResourceID:  data.EnvironmentResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceVolumeID:       data.ResourceVolumeID,
		BackupDestinationID:    data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBackupEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("trigger_type").
		Column("strategy").
		Column("driver").
		Column("format").
		Column("object_key").
		Column("status").
		Column("requested_at").
		Column("started_at").
		Column("finished_at").
		Column("size_bytes").
		Column("digest").
		Column("verified_at").
		Column("error").
		Column("change_id").
		Column("change_task_id").
		Column("backup_policy_id").
		Column("resource_id").
		Column("environment_resource_id").
		Column("resource_installation_id").
		Column("resource_volume_id").
		Column("backup_destination_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBackupEntity{}, err
	}

	return entity, nil
}

func (rb resourceBackup) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceBackupEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rb resourceBackup) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceBackupEntity, error) {
	var entities []ResourceBackupEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceBackups struct {
	ResourceBackups []ResourceBackupEntity
	TotalCount      int64
	Page            int64
	PageSize        int64
	TotalPages      int64
}

func (rb resourceBackup) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceBackups, error) {
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
		Model(&ResourceBackupEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceBackups{}, err
	}

	entities := make([]ResourceBackupEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceBackups{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceBackups{
		ResourceBackups: entities,
		TotalCount:      int64(totalCount),
		Page:            page,
		PageSize:        pageSize,
		TotalPages:      totalPages,
	}, nil
}

func (rb resourceBackup) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceBackupData,
) (ResourceBackupEntity, error) {
	entity := ResourceBackupEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		TriggerType:            data.TriggerType,
		Strategy:               data.Strategy,
		Driver:                 data.Driver,
		Format:                 data.Format,
		ObjectKey:              data.ObjectKey,
		Status:                 data.Status,
		RequestedAt:            data.RequestedAt,
		StartedAt:              data.StartedAt,
		FinishedAt:             data.FinishedAt,
		SizeBytes:              data.SizeBytes,
		Digest:                 data.Digest,
		VerifiedAt:             data.VerifiedAt,
		Error:                  data.Error,
		ChangeID:               data.ChangeID,
		ChangeTaskID:           data.ChangeTaskID,
		BackupPolicyID:         data.BackupPolicyID,
		ResourceID:             data.ResourceID,
		EnvironmentResourceID:  data.EnvironmentResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceVolumeID:       data.ResourceVolumeID,
		BackupDestinationID:    data.BackupDestinationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBackupEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("trigger_type = excluded.trigger_type").
		Set("strategy = excluded.strategy").
		Set("driver = excluded.driver").
		Set("format = excluded.format").
		Set("object_key = excluded.object_key").
		Set("status = excluded.status").
		Set("requested_at = excluded.requested_at").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("size_bytes = excluded.size_bytes").
		Set("digest = excluded.digest").
		Set("verified_at = excluded.verified_at").
		Set("error = excluded.error").
		Set("change_id = excluded.change_id").
		Set("change_task_id = excluded.change_task_id").
		Set("backup_policy_id = excluded.backup_policy_id").
		Set("resource_id = excluded.resource_id").
		Set("environment_resource_id = excluded.environment_resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("resource_volume_id = excluded.resource_volume_id").
		Set("backup_destination_id = excluded.backup_destination_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBackupEntity{}, err
	}

	return entity, nil
}
