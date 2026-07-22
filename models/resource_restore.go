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
	bun.BaseModel        `bun:"table:resource_restores,alias:resource_restores"`
	ID                   uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time      `bun:"created_at"`
	UpdatedAt            time.Time      `bun:"updated_at"`
	ChangeID             uuid.UUID      `bun:"change_id,type:uuid"`
	ChangeTaskID         uuid.UUID      `bun:"change_task_id,type:uuid"`
	ResourceBackupID     uuid.UUID      `bun:"resource_backup_id,type:uuid"`
	ResourceID           uuid.UUID      `bun:"resource_id,type:uuid"`
	SourceBindingID      *uuid.UUID     `bun:"source_binding_id,type:uuid"`
	TargetBindingID      *uuid.UUID     `bun:"target_binding_id,type:uuid"`
	TargetInstallationID *uuid.UUID     `bun:"target_installation_id,type:uuid"`
	Status               string         `bun:"status"`
	RequestedAt          time.Time      `bun:"requested_at"`
	StartedAt            sql.NullTime   `bun:"started_at"`
	FinishedAt           sql.NullTime   `bun:"finished_at"`
	VerifiedAt           sql.NullTime   `bun:"verified_at"`
	Error                sql.NullString `bun:"error"`
}

func (e *ResourceRestoreEntity) Validate() error {
	return nil
}

func (rr resourceRestore) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceRestoreEntity, error) {
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
	ChangeID             uuid.UUID
	ChangeTaskID         uuid.UUID
	ResourceBackupID     uuid.UUID
	ResourceID           uuid.UUID
	SourceBindingID      *uuid.UUID
	TargetBindingID      *uuid.UUID
	TargetInstallationID *uuid.UUID
	Status               string
	RequestedAt          time.Time
	StartedAt            sql.NullTime
	FinishedAt           sql.NullTime
	VerifiedAt           sql.NullTime
	Error                sql.NullString
}

func (rr resourceRestore) Create(ctx context.Context, db storage.Executor, data CreateResourceRestoreData) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ChangeID:             data.ChangeID,
		ChangeTaskID:         data.ChangeTaskID,
		ResourceBackupID:     data.ResourceBackupID,
		ResourceID:           data.ResourceID,
		SourceBindingID:      data.SourceBindingID,
		TargetBindingID:      data.TargetBindingID,
		TargetInstallationID: data.TargetInstallationID,
		Status:               data.Status,
		RequestedAt:          data.RequestedAt,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		VerifiedAt:           data.VerifiedAt,
		Error:                data.Error,
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
	ID                   uuid.UUID
	UpdatedAt            time.Time
	ChangeID             uuid.UUID
	ChangeTaskID         uuid.UUID
	ResourceBackupID     uuid.UUID
	ResourceID           uuid.UUID
	SourceBindingID      *uuid.UUID
	TargetBindingID      *uuid.UUID
	TargetInstallationID *uuid.UUID
	Status               string
	RequestedAt          time.Time
	StartedAt            sql.NullTime
	FinishedAt           sql.NullTime
	VerifiedAt           sql.NullTime
	Error                sql.NullString
}

func (rr resourceRestore) Update(ctx context.Context, db storage.Executor, data UpdateResourceRestoreData) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		ChangeID:             data.ChangeID,
		ChangeTaskID:         data.ChangeTaskID,
		ResourceBackupID:     data.ResourceBackupID,
		ResourceID:           data.ResourceID,
		SourceBindingID:      data.SourceBindingID,
		TargetBindingID:      data.TargetBindingID,
		TargetInstallationID: data.TargetInstallationID,
		Status:               data.Status,
		RequestedAt:          data.RequestedAt,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		VerifiedAt:           data.VerifiedAt,
		Error:                data.Error,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("change_id").
		Column("change_task_id").
		Column("resource_backup_id").
		Column("resource_id").
		Column("source_binding_id").
		Column("target_binding_id").
		Column("target_installation_id").
		Column("status").
		Column("requested_at").
		Column("started_at").
		Column("finished_at").
		Column("verified_at").
		Column("error").
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

func (rr resourceRestore) All(ctx context.Context, db storage.Executor) ([]ResourceRestoreEntity, error) {
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

func (rr resourceRestore) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceRestores, error) {
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

func (rr resourceRestore) Upsert(ctx context.Context, db storage.Executor, data CreateResourceRestoreData) (ResourceRestoreEntity, error) {
	entity := ResourceRestoreEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ChangeID:             data.ChangeID,
		ChangeTaskID:         data.ChangeTaskID,
		ResourceBackupID:     data.ResourceBackupID,
		ResourceID:           data.ResourceID,
		SourceBindingID:      data.SourceBindingID,
		TargetBindingID:      data.TargetBindingID,
		TargetInstallationID: data.TargetInstallationID,
		Status:               data.Status,
		RequestedAt:          data.RequestedAt,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		VerifiedAt:           data.VerifiedAt,
		Error:                data.Error,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("change_id = excluded.change_id").
		Set("change_task_id = excluded.change_task_id").
		Set("resource_backup_id = excluded.resource_backup_id").
		Set("resource_id = excluded.resource_id").
		Set("source_binding_id = excluded.source_binding_id").
		Set("target_binding_id = excluded.target_binding_id").
		Set("target_installation_id = excluded.target_installation_id").
		Set("status = excluded.status").
		Set("requested_at = excluded.requested_at").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("verified_at = excluded.verified_at").
		Set("error = excluded.error").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}

	return entity, nil
}
