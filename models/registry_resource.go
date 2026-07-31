package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type RegistryResourceEntity struct {
	bun.BaseModel `bun:"table:registry_resources,alias:registry_resource"`
	ResourceID    uuid.UUID       `bun:"resource_id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	Provider      string          `bun:"provider"`
	Configuration json.RawMessage `bun:"configuration,type:jsonb"`
}

func (entity *RegistryResourceEntity) Validate() error {
	entity.Provider = strings.ToLower(strings.TrimSpace(entity.Provider))
	builder := validation.NewBuilder()
	if entity.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "Registry Resource is required")
	}
	if entity.Provider != "distribution" {
		builder.Add("provider", "unsupported", "Registry provider must be distribution")
	}
	if !validJSONObject(entity.Configuration) {
		builder.Add("configuration", "invalid", "Registry configuration must be a JSON object")
	}
	return builder.Err()
}

func (registryResource) Find(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (RegistryResourceEntity, error) {
	var entity RegistryResourceEntity
	if err := db.NewSelect().Model(&entity).Where("registry_resource.resource_id = ?", resourceID).Scan(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

type CreateRegistryResourceData struct {
	ResourceID    uuid.UUID
	Provider      string
	Configuration json.RawMessage
}

func (registryResource) Create(ctx context.Context, db storage.Executor, data CreateRegistryResourceData) (RegistryResourceEntity, error) {
	now := time.Now().UTC()
	entity := RegistryResourceEntity{
		ResourceID: data.ResourceID, CreatedAt: now, UpdatedAt: now,
		Provider: data.Provider, Configuration: data.Configuration,
	}
	if err := validation.Validate(&entity); err != nil {
		return RegistryResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	var kind string
	if err := db.NewSelect().TableExpr("resources").Column("kind").Where("id = ?", entity.ResourceID).Where("archived_at IS NULL").Scan(ctx, &kind); err != nil {
		return RegistryResourceEntity{}, err
	}
	if kind != "registry" {
		return RegistryResourceEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "resourceId", Code: "kind", Message: "Registry backing requires a Registry Resource"}})
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

func (registryResource) Update(ctx context.Context, db storage.Executor, data CreateRegistryResourceData) (RegistryResourceEntity, error) {
	entity := RegistryResourceEntity{ResourceID: data.ResourceID, UpdatedAt: time.Now().UTC(), Provider: data.Provider, Configuration: data.Configuration}
	if err := validation.Validate(&entity); err != nil {
		return RegistryResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := db.NewUpdate().Model(&entity).Column("updated_at", "provider", "configuration").WherePK().Returning("*").Scan(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

func (registryResource) Destroy(ctx context.Context, db storage.Executor, resourceID uuid.UUID) error {
	_, err := db.NewDelete().Model((*RegistryResourceEntity)(nil)).Where("resource_id = ?", resourceID).Exec(ctx)
	return err
}
