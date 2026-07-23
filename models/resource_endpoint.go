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

type ResourceEndpointEntity struct {
	bun.BaseModel          `bun:"table:resource_endpoints,alias:resource_endpoints"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	Name                   string          `bun:"name"`
	Role                   string          `bun:"role"`
	Address                string          `bun:"address"`
	Port                   int32           `bun:"port"`
	Protocol               string          `bun:"protocol"`
	TlsMode                string          `bun:"tls_mode"`
	Settings               json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
	ResourceID             uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID      `bun:"resource_installation_id,type:uuid"`
	PrivateNetworkID       *uuid.UUID      `bun:"private_network_id,type:uuid"`
}

func (e *ResourceEndpointEntity) Validate() error {
	return nil
}

func (re resourceEndpoint) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceEndpointEntity, error) {
	var entity ResourceEndpointEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

type CreateResourceEndpointData struct {
	Name                   string
	Role                   string
	Address                string
	Port                   int32
	Protocol               string
	TlsMode                string
	Settings               json.RawMessage
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	PrivateNetworkID       *uuid.UUID
}

func (re resourceEndpoint) Create(ctx context.Context, db storage.Executor, data CreateResourceEndpointData) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

type UpdateResourceEndpointData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	Name                   string
	Role                   string
	Address                string
	Port                   int32
	Protocol               string
	TlsMode                string
	Settings               json.RawMessage
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	PrivateNetworkID       *uuid.UUID
}

func (re resourceEndpoint) Update(ctx context.Context, db storage.Executor, data UpdateResourceEndpointData) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("role").
		Column("address").
		Column("port").
		Column("protocol").
		Column("tls_mode").
		Column("settings").
		Column("archived_at").
		Column("resource_id").
		Column("resource_installation_id").
		Column("private_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

func (re resourceEndpoint) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceEndpointEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (re resourceEndpoint) All(ctx context.Context, db storage.Executor) ([]ResourceEndpointEntity, error) {
	var entities []ResourceEndpointEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceEndpoints struct {
	ResourceEndpoints []ResourceEndpointEntity
	TotalCount        int64
	Page              int64
	PageSize          int64
	TotalPages        int64
}

func (re resourceEndpoint) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceEndpoints, error) {
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
		Model(&ResourceEndpointEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceEndpoints{}, err
	}

	entities := make([]ResourceEndpointEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceEndpoints{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceEndpoints{
		ResourceEndpoints: entities,
		TotalCount:        int64(totalCount),
		Page:              page,
		PageSize:          pageSize,
		TotalPages:        totalPages,
	}, nil
}

func (re resourceEndpoint) Upsert(ctx context.Context, db storage.Executor, data CreateResourceEndpointData) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("role = excluded.role").
		Set("address = excluded.address").
		Set("port = excluded.port").
		Set("protocol = excluded.protocol").
		Set("tls_mode = excluded.tls_mode").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("private_network_id = excluded.private_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}
