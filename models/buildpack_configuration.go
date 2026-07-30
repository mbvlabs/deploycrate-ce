package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"path"
	"strings"
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
	ImageRepository     string          `bun:"image_repository"`
	Settings            json.RawMessage `bun:"settings,type:jsonb"`
	EnvironmentSourceID uuid.UUID       `bun:"environment_source_id,type:uuid"`
	ContainerRegistryID uuid.UUID       `bun:"container_registry_id,type:uuid"`
}

func (e *BuildpackConfigurationEntity) Validate() error {
	builder := validation.NewBuilder()
	contextPath, err := normalizeBuildContextPath(e.ContextPath)
	if err != nil {
		builder.Add("context_path", "invalid", err.Error())
	} else {
		e.ContextPath = contextPath
	}
	if strings.TrimSpace(e.ImageRepository) == "" {
		builder.Add("image_repository", "required", "image repository is required")
	}
	if e.EnvironmentSourceID == uuid.Nil || e.ContainerRegistryID == uuid.Nil {
		builder.Add("environment_source_id", "required", "source and container registry are required")
	}
	if len(e.Settings) == 0 || !json.Valid(e.Settings) {
		builder.Add("settings", "invalid", "Buildpacks settings must be valid JSON")
	} else {
		canonical, settingsErr := CanonicalBuildpackSettings(e.Settings)
		if settingsErr != nil {
			builder.Add("settings", "invalid", settingsErr.Error())
		} else {
			e.Settings = canonical
		}
	}
	return builder.Err()
}

func normalizeBuildContextPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return ".", nil
	}
	if strings.HasPrefix(value, "/") {
		return "", errors.New("build context must be relative")
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("build context cannot traverse outside the repository")
	}
	return cleaned, nil
}

func (bc buildpackConfiguration) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (BuildpackConfigurationEntity, error) {
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
	ImageRepository     string
	Settings            json.RawMessage
	EnvironmentSourceID uuid.UUID
	ContainerRegistryID uuid.UUID
}

func (bc buildpackConfiguration) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBuildpackConfigurationData,
) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		ImageRepository:     data.ImageRepository,
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
	ImageRepository     string
	Settings            json.RawMessage
	EnvironmentSourceID uuid.UUID
	ContainerRegistryID uuid.UUID
}

func (bc buildpackConfiguration) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateBuildpackConfigurationData,
) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		ImageRepository:     data.ImageRepository,
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
		Column("image_repository").
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

func (bc buildpackConfiguration) All(
	ctx context.Context,
	db storage.Executor,
) ([]BuildpackConfigurationEntity, error) {
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

func (bc buildpackConfiguration) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedBuildpackConfigurations, error) {
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

func (bc buildpackConfiguration) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateBuildpackConfigurationData,
) (BuildpackConfigurationEntity, error) {
	entity := BuildpackConfigurationEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ContextPath:         data.ContextPath,
		BuilderReference:    data.BuilderReference,
		ImageRepository:     data.ImageRepository,
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
		Set("image_repository = excluded.image_repository").
		Set("settings = excluded.settings").
		Set("environment_source_id = excluded.environment_source_id").
		Set("container_registry_id = excluded.container_registry_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BuildpackConfigurationEntity{}, err
	}

	return entity, nil
}
