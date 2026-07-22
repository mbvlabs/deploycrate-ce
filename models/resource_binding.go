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

type ResourceBindingEntity struct {
	bun.BaseModel           `bun:"table:resource_bindings,alias:resource_bindings"`
	ID                      uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt               time.Time       `bun:"created_at"`
	UpdatedAt               time.Time       `bun:"updated_at"`
	ResourceID              uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceEndpointID      uuid.UUID       `bun:"resource_endpoint_id,type:uuid"`
	EnvironmentDependencyID uuid.UUID       `bun:"environment_dependency_id,type:uuid"`
	ProvisioningMode        string          `bun:"provisioning_mode"`
	SecretManagementMode    string          `bun:"secret_management_mode"`
	Kind                    string          `bun:"kind"`
	ExternalDatabase        sql.NullString  `bun:"external_database"`
	ExternalPrincipal       sql.NullString  `bun:"external_principal"`
	Configuration           json.RawMessage `bun:"configuration,type:jsonb"`
	Status                  string          `bun:"status"`
	ArchivedAt              sql.NullTime    `bun:"archived_at"`
}

func (e *ResourceBindingEntity) Validate() error {
	return nil
}

func (rb resourceBinding) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceBindingEntity, error) {
	var entity ResourceBindingEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceBindingEntity{}, err
	}

	return entity, nil
}

type CreateResourceBindingData struct {
	ResourceID              uuid.UUID
	ResourceEndpointID      uuid.UUID
	EnvironmentDependencyID uuid.UUID
	ProvisioningMode        string
	SecretManagementMode    string
	Kind                    string
	ExternalDatabase        sql.NullString
	ExternalPrincipal       sql.NullString
	Configuration           json.RawMessage
	Status                  string
	ArchivedAt              sql.NullTime
}

func (rb resourceBinding) Create(ctx context.Context, db storage.Executor, data CreateResourceBindingData) (ResourceBindingEntity, error) {
	entity := ResourceBindingEntity{
		ID:                      uuid.New(),
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
		ResourceID:              data.ResourceID,
		ResourceEndpointID:      data.ResourceEndpointID,
		EnvironmentDependencyID: data.EnvironmentDependencyID,
		ProvisioningMode:        data.ProvisioningMode,
		SecretManagementMode:    data.SecretManagementMode,
		Kind:                    data.Kind,
		ExternalDatabase:        data.ExternalDatabase,
		ExternalPrincipal:       data.ExternalPrincipal,
		Configuration:           data.Configuration,
		Status:                  data.Status,
		ArchivedAt:              data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceBindingEntity{}, err
	}

	return entity, nil
}

type UpdateResourceBindingData struct {
	ID                      uuid.UUID
	UpdatedAt               time.Time
	ResourceID              uuid.UUID
	ResourceEndpointID      uuid.UUID
	EnvironmentDependencyID uuid.UUID
	ProvisioningMode        string
	SecretManagementMode    string
	Kind                    string
	ExternalDatabase        sql.NullString
	ExternalPrincipal       sql.NullString
	Configuration           json.RawMessage
	Status                  string
	ArchivedAt              sql.NullTime
}

func (rb resourceBinding) Update(ctx context.Context, db storage.Executor, data UpdateResourceBindingData) (ResourceBindingEntity, error) {
	entity := ResourceBindingEntity{
		ID:                      data.ID,
		UpdatedAt:               time.Now(),
		ResourceID:              data.ResourceID,
		ResourceEndpointID:      data.ResourceEndpointID,
		EnvironmentDependencyID: data.EnvironmentDependencyID,
		ProvisioningMode:        data.ProvisioningMode,
		SecretManagementMode:    data.SecretManagementMode,
		Kind:                    data.Kind,
		ExternalDatabase:        data.ExternalDatabase,
		ExternalPrincipal:       data.ExternalPrincipal,
		Configuration:           data.Configuration,
		Status:                  data.Status,
		ArchivedAt:              data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_id").
		Column("resource_endpoint_id").
		Column("environment_dependency_id").
		Column("provisioning_mode").
		Column("secret_management_mode").
		Column("kind").
		Column("external_database").
		Column("external_principal").
		Column("configuration").
		Column("status").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBindingEntity{}, err
	}

	return entity, nil
}

func (rb resourceBinding) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceBindingEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rb resourceBinding) All(ctx context.Context, db storage.Executor) ([]ResourceBindingEntity, error) {
	var entities []ResourceBindingEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceBindings struct {
	ResourceBindings []ResourceBindingEntity
	TotalCount       int64
	Page             int64
	PageSize         int64
	TotalPages       int64
}

func (rb resourceBinding) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceBindings, error) {
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
		Model(&ResourceBindingEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceBindings{}, err
	}

	entities := make([]ResourceBindingEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceBindings{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceBindings{
		ResourceBindings: entities,
		TotalCount:       int64(totalCount),
		Page:             page,
		PageSize:         pageSize,
		TotalPages:       totalPages,
	}, nil
}

func (rb resourceBinding) Upsert(ctx context.Context, db storage.Executor, data CreateResourceBindingData) (ResourceBindingEntity, error) {
	entity := ResourceBindingEntity{
		ID:                      uuid.New(),
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
		ResourceID:              data.ResourceID,
		ResourceEndpointID:      data.ResourceEndpointID,
		EnvironmentDependencyID: data.EnvironmentDependencyID,
		ProvisioningMode:        data.ProvisioningMode,
		SecretManagementMode:    data.SecretManagementMode,
		Kind:                    data.Kind,
		ExternalDatabase:        data.ExternalDatabase,
		ExternalPrincipal:       data.ExternalPrincipal,
		Configuration:           data.Configuration,
		Status:                  data.Status,
		ArchivedAt:              data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_id = excluded.resource_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("environment_dependency_id = excluded.environment_dependency_id").
		Set("provisioning_mode = excluded.provisioning_mode").
		Set("secret_management_mode = excluded.secret_management_mode").
		Set("kind = excluded.kind").
		Set("external_database = excluded.external_database").
		Set("external_principal = excluded.external_principal").
		Set("configuration = excluded.configuration").
		Set("status = excluded.status").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBindingEntity{}, err
	}

	return entity, nil
}
