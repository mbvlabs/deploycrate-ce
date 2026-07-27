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

type ResourceRestoreEntity struct {
	bun.BaseModel               `bun:"table:resource_restores,alias:resource_restores"`
	ID                          uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt                   time.Time      `bun:"created_at"`
	UpdatedAt                   time.Time      `bun:"updated_at"`
	Status                      string         `bun:"status"`
	RequestedAt                 time.Time      `bun:"requested_at"`
	StartedAt                   sql.NullTime   `bun:"started_at"`
	FinishedAt                  sql.NullTime   `bun:"finished_at"`
	VerifiedAt                  sql.NullTime   `bun:"verified_at"`
	Error                       sql.NullString `bun:"error"`
	ChangeID                    uuid.UUID      `bun:"change_id,type:uuid"`
	ChangeTaskID                uuid.UUID      `bun:"change_task_id,type:uuid"`
	BackupID                    uuid.UUID      `bun:"backup_id,type:uuid"`
	ResourceID                  uuid.UUID      `bun:"resource_id,type:uuid"`
	SourceEnvironmentResourceID *uuid.UUID     `bun:"source_environment_resource_id,type:uuid"`
	TargetEnvironmentResourceID *uuid.UUID     `bun:"target_environment_resource_id,type:uuid"`
	TargetInstallationID        *uuid.UUID     `bun:"target_installation_id,type:uuid"`
}

func (e *ResourceRestoreEntity) Validate() error {
	return nil
}

func (rr resourceRestore) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceRestoreEntity, error) {
	var entity ResourceRestoreEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}

	return entity, nil
}

type CreateResourceRestoreData struct {
	Status                      string
	RequestedAt                 time.Time
	StartedAt                   sql.NullTime
	FinishedAt                  sql.NullTime
	VerifiedAt                  sql.NullTime
	Error                       sql.NullString
	ChangeID                    uuid.UUID
	ChangeTaskID                uuid.UUID
	BackupID                    uuid.UUID
	ResourceID                  uuid.UUID
	SourceEnvironmentResourceID *uuid.UUID
	TargetEnvironmentResourceID *uuid.UUID
	TargetInstallationID        *uuid.UUID
}

func (rr resourceRestore) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceRestoreData,
) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                          uuid.New(),
		CreatedAt:                   time.Now(),
		UpdatedAt:                   time.Now(),
		Status:                      data.Status,
		RequestedAt:                 data.RequestedAt,
		StartedAt:                   data.StartedAt,
		FinishedAt:                  data.FinishedAt,
		VerifiedAt:                  data.VerifiedAt,
		Error:                       data.Error,
		ChangeID:                    data.ChangeID,
		ChangeTaskID:                data.ChangeTaskID,
		BackupID:                    data.BackupID,
		ResourceID:                  data.ResourceID,
		SourceEnvironmentResourceID: data.SourceEnvironmentResourceID,
		TargetEnvironmentResourceID: data.TargetEnvironmentResourceID,
		TargetInstallationID:        data.TargetInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}

	return entity, nil
}

type UpdateResourceRestoreData struct {
	ID                          uuid.UUID
	UpdatedAt                   time.Time
	Status                      string
	RequestedAt                 time.Time
	StartedAt                   sql.NullTime
	FinishedAt                  sql.NullTime
	VerifiedAt                  sql.NullTime
	Error                       sql.NullString
	ChangeID                    uuid.UUID
	ChangeTaskID                uuid.UUID
	BackupID                    uuid.UUID
	ResourceID                  uuid.UUID
	SourceEnvironmentResourceID *uuid.UUID
	TargetEnvironmentResourceID *uuid.UUID
	TargetInstallationID        *uuid.UUID
}

func (rr resourceRestore) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceRestoreData,
) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                          data.ID,
		UpdatedAt:                   time.Now(),
		Status:                      data.Status,
		RequestedAt:                 data.RequestedAt,
		StartedAt:                   data.StartedAt,
		FinishedAt:                  data.FinishedAt,
		VerifiedAt:                  data.VerifiedAt,
		Error:                       data.Error,
		ChangeID:                    data.ChangeID,
		ChangeTaskID:                data.ChangeTaskID,
		BackupID:                    data.BackupID,
		ResourceID:                  data.ResourceID,
		SourceEnvironmentResourceID: data.SourceEnvironmentResourceID,
		TargetEnvironmentResourceID: data.TargetEnvironmentResourceID,
		TargetInstallationID:        data.TargetInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("status").
		Column("requested_at").
		Column("started_at").
		Column("finished_at").
		Column("verified_at").
		Column("error").
		Column("change_id").
		Column("change_task_id").
		Column("backup_id").
		Column("resource_id").
		Column("source_environment_resource_id").
		Column("target_environment_resource_id").
		Column("target_installation_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}

	return entity, nil
}

func (rr resourceRestore) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceRestoreEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rr resourceRestore) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceRestoreEntity, error) {
	var entities []ResourceRestoreEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceRestores struct {
	ResourceRestores []ResourceRestoreEntity
	TotalCount       int64
	Page             int64
	PageSize         int64
	TotalPages       int64
}

func (rr resourceRestore) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceRestores, error) {
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
		Model(&ResourceRestoreEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceRestores{}, err
	}

	entities := make([]ResourceRestoreEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceRestores{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceRestores{
		ResourceRestores: entities,
		TotalCount:       int64(totalCount),
		Page:             page,
		PageSize:         pageSize,
		TotalPages:       totalPages,
	}, nil
}

func (rr resourceRestore) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceRestoreData,
) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                          uuid.New(),
		CreatedAt:                   time.Now(),
		UpdatedAt:                   time.Now(),
		Status:                      data.Status,
		RequestedAt:                 data.RequestedAt,
		StartedAt:                   data.StartedAt,
		FinishedAt:                  data.FinishedAt,
		VerifiedAt:                  data.VerifiedAt,
		Error:                       data.Error,
		ChangeID:                    data.ChangeID,
		ChangeTaskID:                data.ChangeTaskID,
		BackupID:                    data.BackupID,
		ResourceID:                  data.ResourceID,
		SourceEnvironmentResourceID: data.SourceEnvironmentResourceID,
		TargetEnvironmentResourceID: data.TargetEnvironmentResourceID,
		TargetInstallationID:        data.TargetInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("status = excluded.status").
		Set("requested_at = excluded.requested_at").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("verified_at = excluded.verified_at").
		Set("error = excluded.error").
		Set("change_id = excluded.change_id").
		Set("change_task_id = excluded.change_task_id").
		Set("backup_id = excluded.backup_id").
		Set("resource_id = excluded.resource_id").
		Set("source_environment_resource_id = excluded.source_environment_resource_id").
		Set("target_environment_resource_id = excluded.target_environment_resource_id").
		Set("target_installation_id = excluded.target_installation_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}

	return entity, nil
}
