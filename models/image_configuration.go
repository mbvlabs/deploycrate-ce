package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ImageConfigurationEntity struct {
	bun.BaseModel       `bun:"table:image_configurations,alias:image_configurations"`
	ID                  uuid.UUID `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time `bun:"created_at"`
	UpdatedAt           time.Time `bun:"updated_at"`
	EnvironmentSourceID uuid.UUID `bun:"environment_source_id,type:uuid"`
	RegistryResourceID  uuid.UUID `bun:"registry_resource_id,type:uuid"`
}

func (e *ImageConfigurationEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil {
		builder.Add("id", "required", "image configuration ID is required")
	}
	if e.EnvironmentSourceID == uuid.Nil {
		builder.Add("environmentSourceId", "required", "environment source is required")
	}
	if e.RegistryResourceID == uuid.Nil {
		builder.Add("registryResourceId", "required", "registry resource is required")
	}
	return builder.Err()
}

func (ic imageConfiguration) FindForSource(ctx context.Context, db storage.Executor, sourceID uuid.UUID) (ImageConfigurationEntity, error) {
	var entity ImageConfigurationEntity
	err := db.NewSelect().Model(&entity).Where("environment_source_id = ?", sourceID).Scan(ctx)
	return entity, err
}

type CreateImageConfigurationData struct {
	EnvironmentSourceID uuid.UUID
	RegistryResourceID  uuid.UUID
}

func (ic imageConfiguration) Create(ctx context.Context, db storage.Executor, data CreateImageConfigurationData) (ImageConfigurationEntity, error) {
	entity := ImageConfigurationEntity{
		ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		EnvironmentSourceID: data.EnvironmentSourceID, RegistryResourceID: data.RegistryResourceID,
	}
	if err := validation.Validate(&entity); err != nil {
		return ImageConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(
		ctx,
		db,
		"image-configuration-source:"+entity.EnvironmentSourceID.String(),
		db.NewSelect().Model((*ImageConfigurationEntity)(nil)).Where("environment_source_id = ?", entity.EnvironmentSourceID),
		"environmentSourceId",
		"the Environment source already has an image configuration",
	); err != nil {
		return ImageConfigurationEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ImageConfigurationEntity{}, err
	}
	return entity, nil
}
