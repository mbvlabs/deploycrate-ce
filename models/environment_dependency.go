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

type EnvironmentDependencyEntity struct {
	bun.BaseModel      `bun:"table:environment_dependencies,alias:environment_dependencies"`
	ID                 uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time       `bun:"created_at"`
	UpdatedAt          time.Time       `bun:"updated_at"`
	EnvironmentID      uuid.UUID       `bun:"environment_id,type:uuid"`
	ResourceID         uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceEndpointID uuid.UUID       `bun:"resource_endpoint_id,type:uuid"`
	PrivateNetworkID   *uuid.UUID      `bun:"private_network_id,type:uuid"`
	Alias              string          `bun:"alias"`
	Required           bool            `bun:"required"`
	SecretMapping      json.RawMessage `bun:"secret_mapping,type:jsonb"`
	Settings           json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt         sql.NullTime    `bun:"archived_at"`
}

func (e *EnvironmentDependencyEntity) Validate() error {
	return nil
}

func (ed environmentDependency) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentDependencyEntity, error) {
	var entity EnvironmentDependencyEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentDependencyEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentDependencyData struct {
	EnvironmentID      uuid.UUID
	ResourceID         uuid.UUID
	ResourceEndpointID uuid.UUID
	PrivateNetworkID   *uuid.UUID
	Alias              string
	Required           bool
	SecretMapping      json.RawMessage
	Settings           json.RawMessage
	ArchivedAt         sql.NullTime
}

func (ed environmentDependency) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentDependencyData) (EnvironmentDependencyEntity, error) {
	entity := EnvironmentDependencyEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		EnvironmentID:      data.EnvironmentID,
		ResourceID:         data.ResourceID,
		ResourceEndpointID: data.ResourceEndpointID,
		PrivateNetworkID:   data.PrivateNetworkID,
		Alias:              data.Alias,
		Required:           data.Required,
		SecretMapping:      data.SecretMapping,
		Settings:           data.Settings,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDependencyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentDependencyEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentDependencyData struct {
	ID                 uuid.UUID
	UpdatedAt          time.Time
	EnvironmentID      uuid.UUID
	ResourceID         uuid.UUID
	ResourceEndpointID uuid.UUID
	PrivateNetworkID   *uuid.UUID
	Alias              string
	Required           bool
	SecretMapping      json.RawMessage
	Settings           json.RawMessage
	ArchivedAt         sql.NullTime
}

func (ed environmentDependency) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentDependencyData) (EnvironmentDependencyEntity, error) {
	entity := EnvironmentDependencyEntity{
		ID:                 data.ID,
		UpdatedAt:          time.Now(),
		EnvironmentID:      data.EnvironmentID,
		ResourceID:         data.ResourceID,
		ResourceEndpointID: data.ResourceEndpointID,
		PrivateNetworkID:   data.PrivateNetworkID,
		Alias:              data.Alias,
		Required:           data.Required,
		SecretMapping:      data.SecretMapping,
		Settings:           data.Settings,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDependencyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("resource_id").
		Column("resource_endpoint_id").
		Column("private_network_id").
		Column("alias").
		Column("required").
		Column("secret_mapping").
		Column("settings").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentDependencyEntity{}, err
	}

	return entity, nil
}

func (ed environmentDependency) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentDependencyEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ed environmentDependency) All(ctx context.Context, db storage.Executor) ([]EnvironmentDependencyEntity, error) {
	var entities []EnvironmentDependencyEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentDependencies struct {
	EnvironmentDependencies []EnvironmentDependencyEntity
	TotalCount              int64
	Page                    int64
	PageSize                int64
	TotalPages              int64
}

func (ed environmentDependency) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentDependencies, error) {
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
		Model(&EnvironmentDependencyEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentDependencies{}, err
	}

	entities := make([]EnvironmentDependencyEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentDependencies{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentDependencies{
		EnvironmentDependencies: entities,
		TotalCount:              int64(totalCount),
		Page:                    page,
		PageSize:                pageSize,
		TotalPages:              totalPages,
	}, nil
}

func (ed environmentDependency) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentDependencyData) (EnvironmentDependencyEntity, error) {
	entity := EnvironmentDependencyEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		EnvironmentID:      data.EnvironmentID,
		ResourceID:         data.ResourceID,
		ResourceEndpointID: data.ResourceEndpointID,
		PrivateNetworkID:   data.PrivateNetworkID,
		Alias:              data.Alias,
		Required:           data.Required,
		SecretMapping:      data.SecretMapping,
		Settings:           data.Settings,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDependencyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("resource_id = excluded.resource_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("private_network_id = excluded.private_network_id").
		Set("alias = excluded.alias").
		Set("required = excluded.required").
		Set("secret_mapping = excluded.secret_mapping").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentDependencyEntity{}, err
	}

	return entity, nil
}
