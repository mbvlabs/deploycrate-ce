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

type EnvironmentBuildConfigurationEntity struct {
	bun.BaseModel    `bun:"table:environment_build_configurations,alias:environment_build_configurations"`
	ID               int32           `bun:"id,pk,autoincrement"`
	CreatedAt        time.Time       `bun:"created_at"`
	UpdatedAt        time.Time       `bun:"updated_at"`
	EnvironmentID    uuid.UUID       `bun:"environment_id,type:uuid"`
	Method           string          `bun:"method"`
	ContextPath      string          `bun:"context_path"`
	DockerfilePath   sql.NullString  `bun:"dockerfile_path"`
	BuilderReference sql.NullString  `bun:"builder_reference"`
	Settings         json.RawMessage `bun:"settings,type:jsonb"`
}

func (e *EnvironmentBuildConfigurationEntity) Validate() error {
	return nil
}

func (ebc environmentBuildConfiguration) Find(ctx context.Context, db storage.Executor, id int32) (EnvironmentBuildConfigurationEntity, error) {
	var entity EnvironmentBuildConfigurationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentBuildConfigurationEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentBuildConfigurationData struct {
	EnvironmentID    uuid.UUID
	Method           string
	ContextPath      string
	DockerfilePath   sql.NullString
	BuilderReference sql.NullString
	Settings         json.RawMessage
}

func (ebc environmentBuildConfiguration) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentBuildConfigurationData) (EnvironmentBuildConfigurationEntity, error) {
	entity := EnvironmentBuildConfigurationEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		Method:           data.Method,
		ContextPath:      data.ContextPath,
		DockerfilePath:   data.DockerfilePath,
		BuilderReference: data.BuilderReference,
		Settings:         data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentBuildConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentBuildConfigurationEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentBuildConfigurationData struct {
	ID               int32
	UpdatedAt        time.Time
	EnvironmentID    uuid.UUID
	Method           string
	ContextPath      string
	DockerfilePath   sql.NullString
	BuilderReference sql.NullString
	Settings         json.RawMessage
}

func (ebc environmentBuildConfiguration) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentBuildConfigurationData) (EnvironmentBuildConfigurationEntity, error) {
	entity := EnvironmentBuildConfigurationEntity{
		ID:               data.ID,
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		Method:           data.Method,
		ContextPath:      data.ContextPath,
		DockerfilePath:   data.DockerfilePath,
		BuilderReference: data.BuilderReference,
		Settings:         data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentBuildConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("method").
		Column("context_path").
		Column("dockerfile_path").
		Column("builder_reference").
		Column("settings").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentBuildConfigurationEntity{}, err
	}

	return entity, nil
}

func (ebc environmentBuildConfiguration) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*EnvironmentBuildConfigurationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ebc environmentBuildConfiguration) All(ctx context.Context, db storage.Executor) ([]EnvironmentBuildConfigurationEntity, error) {
	var entities []EnvironmentBuildConfigurationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentBuildConfigurations struct {
	EnvironmentBuildConfigurations []EnvironmentBuildConfigurationEntity
	TotalCount                     int64
	Page                           int64
	PageSize                       int64
	TotalPages                     int64
}

func (ebc environmentBuildConfiguration) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentBuildConfigurations, error) {
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
		Model(&EnvironmentBuildConfigurationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentBuildConfigurations{}, err
	}

	entities := make([]EnvironmentBuildConfigurationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentBuildConfigurations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentBuildConfigurations{
		EnvironmentBuildConfigurations: entities,
		TotalCount:                     int64(totalCount),
		Page:                           page,
		PageSize:                       pageSize,
		TotalPages:                     totalPages,
	}, nil
}

func (ebc environmentBuildConfiguration) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentBuildConfigurationData) (EnvironmentBuildConfigurationEntity, error) {
	entity := EnvironmentBuildConfigurationEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		Method:           data.Method,
		ContextPath:      data.ContextPath,
		DockerfilePath:   data.DockerfilePath,
		BuilderReference: data.BuilderReference,
		Settings:         data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentBuildConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("method = excluded.method").
		Set("context_path = excluded.context_path").
		Set("dockerfile_path = excluded.dockerfile_path").
		Set("builder_reference = excluded.builder_reference").
		Set("settings = excluded.settings").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentBuildConfigurationEntity{}, err
	}

	return entity, nil
}
