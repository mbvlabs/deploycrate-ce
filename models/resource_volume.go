package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceVolumeEntity struct {
	bun.BaseModel `bun:"table:resource_volumes,alias:resource_volumes"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	Name          string          `bun:"name"`
	Driver        string          `bun:"driver"`
	Configuration json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt    sql.NullTime    `bun:"archived_at"`
	ResourceID    uuid.UUID       `bun:"resource_id,type:uuid"`
	ServerID      uuid.UUID       `bun:"server_id,type:uuid"`
}

func (e *ResourceVolumeEntity) Validate() error {
	return nil
}

func (rv resourceVolume) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceVolumeEntity, error) {
	var entity ResourceVolumeEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceVolumeEntity{}, err
	}

	return entity, nil
}

type CreateResourceVolumeData struct {
	Name          string
	Driver        string
	Configuration json.RawMessage
	ArchivedAt    sql.NullTime
	ResourceID    uuid.UUID
	ServerID      uuid.UUID
}

func (rv resourceVolume) Create(ctx context.Context, db storage.Executor, data CreateResourceVolumeData) (ResourceVolumeEntity, error) {
	entity := ResourceVolumeEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Name:          data.Name,
		Driver:        data.Driver,
		Configuration: data.Configuration,
		ArchivedAt:    data.ArchivedAt,
		ResourceID:    data.ResourceID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceVolumeEntity{}, err
	}

	return entity, nil
}

type UpdateResourceVolumeData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	Name          string
	Driver        string
	Configuration json.RawMessage
	ArchivedAt    sql.NullTime
	ResourceID    uuid.UUID
	ServerID      uuid.UUID
}

func (rv resourceVolume) Update(ctx context.Context, db storage.Executor, data UpdateResourceVolumeData) (ResourceVolumeEntity, error) {
	entity := ResourceVolumeEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		Name:          data.Name,
		Driver:        data.Driver,
		Configuration: data.Configuration,
		ArchivedAt:    data.ArchivedAt,
		ResourceID:    data.ResourceID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("driver").
		Column("configuration").
		Column("archived_at").
		Column("resource_id").
		Column("server_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceVolumeEntity{}, err
	}

	return entity, nil
}

func (rv resourceVolume) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceVolumeEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rv resourceVolume) All(ctx context.Context, db storage.Executor) ([]ResourceVolumeEntity, error) {
	var entities []ResourceVolumeEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceVolumes struct {
	ResourceVolumes []ResourceVolumeEntity
	TotalCount      int64
	Page            int64
	PageSize        int64
	TotalPages      int64
}

func (rv resourceVolume) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceVolumes, error) {
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
		Model(&ResourceVolumeEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceVolumes{}, err
	}

	entities := make([]ResourceVolumeEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceVolumes{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceVolumes{
		ResourceVolumes: entities,
		TotalCount:      int64(totalCount),
		Page:            page,
		PageSize:        pageSize,
		TotalPages:      totalPages,
	}, nil
}

func (rv resourceVolume) Upsert(ctx context.Context, db storage.Executor, data CreateResourceVolumeData) (ResourceVolumeEntity, error) {
	entity := ResourceVolumeEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Name:          data.Name,
		Driver:        data.Driver,
		Configuration: data.Configuration,
		ArchivedAt:    data.ArchivedAt,
		ResourceID:    data.ResourceID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceVolumeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("driver = excluded.driver").
		Set("configuration = excluded.configuration").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("server_id = excluded.server_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceVolumeEntity{}, err
	}

	return entity, nil
}
