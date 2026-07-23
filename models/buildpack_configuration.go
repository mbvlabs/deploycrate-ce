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

type BuildpackConfigurationEntity struct {
	bun.BaseModel       `bun:"table:buildpack_configurations,alias:buildpack_configurations"`
	ID                  int32           `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ContextPath         string          `bun:"context_path"`
	BuilderReference    sql.NullString  `bun:"builder_reference"`
	Repository          string          `bun:"repository"`
	Settings            json.RawMessage `bun:"settings,type:jsonb"`
	EnvironmentSourceID uuid.UUID       `bun:"environment_source_id,type:uuid"`
	ContainerRegistryID uuid.UUID       `bun:"container_registry_id,type:uuid"`
}

func (e *BuildpackConfigurationEntity) Validate() error {
	return nil
}

func (bc buildpackConfiguration) Find(ctx context.Context, db storage.Executor, id int32) (BuildpackConfigurationEntity, error) {
	var entity BuildpackConfigurationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return BuildpackConfigurationEntity{}, err
	}

	return entity, nil
}

type CreateBuildpackConfigurationData struct {
	ContextPath         string
	BuilderReference    sql.NullString
	Repository          string
	Settings            json.RawMessage
	EnvironmentSourceID uuid.UUID
	ContainerRegistryID uuid.UUID
}

func (bc buildpackConfiguration) Create(ctx context.Context, db storage.Executor, data CreateBuildpackConfigurationData) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		Repository:          data.Repository,
		Settings:            data.Settings,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ContainerRegistryID: data.ContainerRegistryID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildpackConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BuildpackConfigurationEntity{}, err
	}

	return entity, nil
}

type UpdateBuildpackConfigurationData struct {
	ID                  int32
	UpdatedAt           time.Time
	ContextPath         string
	BuilderReference    sql.NullString
	Repository          string
	Settings            json.RawMessage
	EnvironmentSourceID uuid.UUID
	ContainerRegistryID uuid.UUID
}

func (bc buildpackConfiguration) Update(ctx context.Context, db storage.Executor, data UpdateBuildpackConfigurationData) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		Repository:          data.Repository,
		Settings:            data.Settings,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ContainerRegistryID: data.ContainerRegistryID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildpackConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("context_path").
		Column("builder_reference").
		Column("repository").
		Column("settings").
		Column("environment_source_id").
		Column("container_registry_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return BuildpackConfigurationEntity{}, err
	}

	return entity, nil
}

func (bc buildpackConfiguration) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*BuildpackConfigurationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (bc buildpackConfiguration) All(ctx context.Context, db storage.Executor) ([]BuildpackConfigurationEntity, error) {
	var entities []BuildpackConfigurationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedBuildpackConfigurations struct {
	BuildpackConfigurations []BuildpackConfigurationEntity
	TotalCount              int64
	Page                    int64
	PageSize                int64
	TotalPages              int64
}

func (bc buildpackConfiguration) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedBuildpackConfigurations, error) {
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
		Model(&BuildpackConfigurationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedBuildpackConfigurations{}, err
	}

	entities := make([]BuildpackConfigurationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedBuildpackConfigurations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedBuildpackConfigurations{
		BuildpackConfigurations: entities,
		TotalCount:              int64(totalCount),
		Page:                    page,
		PageSize:                pageSize,
		TotalPages:              totalPages,
	}, nil
}

func (bc buildpackConfiguration) Upsert(ctx context.Context, db storage.Executor, data CreateBuildpackConfigurationData) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		Repository:          data.Repository,
		Settings:            data.Settings,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ContainerRegistryID: data.ContainerRegistryID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildpackConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("context_path = excluded.context_path").
		Set("builder_reference = excluded.builder_reference").
		Set("repository = excluded.repository").
		Set("settings = excluded.settings").
		Set("environment_source_id = excluded.environment_source_id").
		Set("container_registry_id = excluded.container_registry_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BuildpackConfigurationEntity{}, err
	}

	return entity, nil
}
