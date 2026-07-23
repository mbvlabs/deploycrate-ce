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

type EnvironmentResourceEntity struct {
	bun.BaseModel        `bun:"table:environment_resources,alias:environment_resources"`
	ID                   uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time       `bun:"created_at"`
	UpdatedAt            time.Time       `bun:"updated_at"`
	Alias                string          `bun:"alias"`
	Configuration        json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt           sql.NullTime    `bun:"archived_at"`
	EnvironmentID        uuid.UUID       `bun:"environment_id,type:uuid"`
	ResourceID           uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceEndpointID   uuid.UUID       `bun:"resource_endpoint_id,type:uuid"`
	ResourceCredentialID *uuid.UUID      `bun:"resource_credential_id,type:uuid"`
}

func (e *EnvironmentResourceEntity) Validate() error {
	return nil
}

func (er environmentResource) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentResourceEntity, error) {
	var entity EnvironmentResourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentResourceData struct {
	Alias                string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	EnvironmentID        uuid.UUID
	ResourceID           uuid.UUID
	ResourceEndpointID   uuid.UUID
	ResourceCredentialID *uuid.UUID
}

func (er environmentResource) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentResourceData) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentResourceData struct {
	ID                   uuid.UUID
	UpdatedAt            time.Time
	Alias                string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	EnvironmentID        uuid.UUID
	ResourceID           uuid.UUID
	ResourceEndpointID   uuid.UUID
	ResourceCredentialID *uuid.UUID
}

func (er environmentResource) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentResourceData) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("alias").
		Column("configuration").
		Column("archived_at").
		Column("environment_id").
		Column("resource_id").
		Column("resource_endpoint_id").
		Column("resource_credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

func (er environmentResource) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentResourceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (er environmentResource) All(ctx context.Context, db storage.Executor) ([]EnvironmentResourceEntity, error) {
	var entities []EnvironmentResourceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentResources struct {
	EnvironmentResources []EnvironmentResourceEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (er environmentResource) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentResources, error) {
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
		Model(&EnvironmentResourceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentResources{}, err
	}

	entities := make([]EnvironmentResourceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentResources{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentResources{
		EnvironmentResources: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (er environmentResource) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentResourceData) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("alias = excluded.alias").
		Set("configuration = excluded.configuration").
		Set("archived_at = excluded.archived_at").
		Set("environment_id = excluded.environment_id").
		Set("resource_id = excluded.resource_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("resource_credential_id = excluded.resource_credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}
