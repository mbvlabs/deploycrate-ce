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

type ResourceVolumeMountEntity struct {
	bun.BaseModel          `bun:"table:resource_volume_mounts,alias:resource_volume_mounts"`
	ID                     uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time    `bun:"created_at"`
	UpdatedAt              time.Time    `bun:"updated_at"`
	MountPath              string       `bun:"mount_path"`
	ReadOnly               bool         `bun:"read_only"`
	ArchivedAt             sql.NullTime `bun:"archived_at"`
	ResourceVolumeID       uuid.UUID    `bun:"resource_volume_id,type:uuid"`
	ResourceInstallationID uuid.UUID    `bun:"resource_installation_id,type:uuid"`
}

func (e *ResourceVolumeMountEntity) Validate() error {
	return nil
}

func (rvm resourceVolumeMount) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceVolumeMountEntity, error) {
	var entity ResourceVolumeMountEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceVolumeMountEntity{}, err
	}

	return entity, nil
}

type CreateResourceVolumeMountData struct {
	MountPath              string
	ReadOnly               bool
	ArchivedAt             sql.NullTime
	ResourceVolumeID       uuid.UUID
	ResourceInstallationID uuid.UUID
}

func (rvm resourceVolumeMount) Create(ctx context.Context, db storage.Executor, data CreateResourceVolumeMountData) (ResourceVolumeMountEntity, error) {
	entity := ResourceVolumeMountEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		MountPath:              data.MountPath,
		ReadOnly:               data.ReadOnly,
		ArchivedAt:             data.ArchivedAt,
		ResourceVolumeID:       data.ResourceVolumeID,
		ResourceInstallationID: data.ResourceInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeMountEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceVolumeMountEntity{}, err
	}

	return entity, nil
}

type UpdateResourceVolumeMountData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	MountPath              string
	ReadOnly               bool
	ArchivedAt             sql.NullTime
	ResourceVolumeID       uuid.UUID
	ResourceInstallationID uuid.UUID
}

func (rvm resourceVolumeMount) Update(ctx context.Context, db storage.Executor, data UpdateResourceVolumeMountData) (ResourceVolumeMountEntity, error) {
	entity := ResourceVolumeMountEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		MountPath:              data.MountPath,
		ReadOnly:               data.ReadOnly,
		ArchivedAt:             data.ArchivedAt,
		ResourceVolumeID:       data.ResourceVolumeID,
		ResourceInstallationID: data.ResourceInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeMountEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("mount_path").
		Column("read_only").
		Column("archived_at").
		Column("resource_volume_id").
		Column("resource_installation_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceVolumeMountEntity{}, err
	}

	return entity, nil
}

func (rvm resourceVolumeMount) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceVolumeMountEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rvm resourceVolumeMount) All(ctx context.Context, db storage.Executor) ([]ResourceVolumeMountEntity, error) {
	var entities []ResourceVolumeMountEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceVolumeMounts struct {
	ResourceVolumeMounts []ResourceVolumeMountEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (rvm resourceVolumeMount) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceVolumeMounts, error) {
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
		Model(&ResourceVolumeMountEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceVolumeMounts{}, err
	}

	entities := make([]ResourceVolumeMountEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceVolumeMounts{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceVolumeMounts{
		ResourceVolumeMounts: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (rvm resourceVolumeMount) Upsert(ctx context.Context, db storage.Executor, data CreateResourceVolumeMountData) (ResourceVolumeMountEntity, error) {
	entity := ResourceVolumeMountEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		MountPath:              data.MountPath,
		ReadOnly:               data.ReadOnly,
		ArchivedAt:             data.ArchivedAt,
		ResourceVolumeID:       data.ResourceVolumeID,
		ResourceInstallationID: data.ResourceInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeMountEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("mount_path = excluded.mount_path").
		Set("read_only = excluded.read_only").
		Set("archived_at = excluded.archived_at").
		Set("resource_volume_id = excluded.resource_volume_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceVolumeMountEntity{}, err
	}

	return entity, nil
}
