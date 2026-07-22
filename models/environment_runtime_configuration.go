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

type EnvironmentRuntimeConfigurationEntity struct {
	bun.BaseModel  `bun:"table:environment_runtime_configurations,alias:environment_runtime_configurations"`
	ID             int32           `bun:"id,pk,autoincrement"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	EnvironmentID  uuid.UUID       `bun:"environment_id,type:uuid"`
	Runtime        string          `bun:"runtime"`
	Command        sql.NullString  `bun:"command"`
	Arguments      json.RawMessage `bun:"arguments,type:jsonb"`
	Replicas       int32           `bun:"replicas"`
	Ports          json.RawMessage `bun:"ports,type:jsonb"`
	ResourceLimits json.RawMessage `bun:"resource_limits,type:jsonb"`
	RestartPolicy  string          `bun:"restart_policy"`
	Settings       json.RawMessage `bun:"settings,type:jsonb"`
}

func (e *EnvironmentRuntimeConfigurationEntity) Validate() error {
	return nil
}

func (erc environmentRuntimeConfiguration) Find(ctx context.Context, db storage.Executor, id int32) (EnvironmentRuntimeConfigurationEntity, error) {
	var entity EnvironmentRuntimeConfigurationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentRuntimeConfigurationData struct {
	EnvironmentID  uuid.UUID
	Runtime        string
	Command        sql.NullString
	Arguments      json.RawMessage
	Replicas       int32
	Ports          json.RawMessage
	ResourceLimits json.RawMessage
	RestartPolicy  string
	Settings       json.RawMessage
}

func (erc environmentRuntimeConfiguration) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentRuntimeConfigurationData) (EnvironmentRuntimeConfigurationEntity, error) {
	entity := EnvironmentRuntimeConfigurationEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		EnvironmentID:  data.EnvironmentID,
		Runtime:        data.Runtime,
		Command:        data.Command,
		Arguments:      data.Arguments,
		Replicas:       data.Replicas,
		Ports:          data.Ports,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentRuntimeConfigurationData struct {
	ID             int32
	UpdatedAt      time.Time
	EnvironmentID  uuid.UUID
	Runtime        string
	Command        sql.NullString
	Arguments      json.RawMessage
	Replicas       int32
	Ports          json.RawMessage
	ResourceLimits json.RawMessage
	RestartPolicy  string
	Settings       json.RawMessage
}

func (erc environmentRuntimeConfiguration) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentRuntimeConfigurationData) (EnvironmentRuntimeConfigurationEntity, error) {
	entity := EnvironmentRuntimeConfigurationEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		EnvironmentID:  data.EnvironmentID,
		Runtime:        data.Runtime,
		Command:        data.Command,
		Arguments:      data.Arguments,
		Replicas:       data.Replicas,
		Ports:          data.Ports,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("runtime").
		Column("command").
		Column("arguments").
		Column("replicas").
		Column("ports").
		Column("resource_limits").
		Column("restart_policy").
		Column("settings").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

func (erc environmentRuntimeConfiguration) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*EnvironmentRuntimeConfigurationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (erc environmentRuntimeConfiguration) All(ctx context.Context, db storage.Executor) ([]EnvironmentRuntimeConfigurationEntity, error) {
	var entities []EnvironmentRuntimeConfigurationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentRuntimeConfigurations struct {
	EnvironmentRuntimeConfigurations []EnvironmentRuntimeConfigurationEntity
	TotalCount                       int64
	Page                             int64
	PageSize                         int64
	TotalPages                       int64
}

func (erc environmentRuntimeConfiguration) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentRuntimeConfigurations, error) {
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
		Model(&EnvironmentRuntimeConfigurationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentRuntimeConfigurations{}, err
	}

	entities := make([]EnvironmentRuntimeConfigurationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentRuntimeConfigurations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentRuntimeConfigurations{
		EnvironmentRuntimeConfigurations: entities,
		TotalCount:                       int64(totalCount),
		Page:                             page,
		PageSize:                         pageSize,
		TotalPages:                       totalPages,
	}, nil
}

func (erc environmentRuntimeConfiguration) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentRuntimeConfigurationData) (EnvironmentRuntimeConfigurationEntity, error) {
	entity := EnvironmentRuntimeConfigurationEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		EnvironmentID:  data.EnvironmentID,
		Runtime:        data.Runtime,
		Command:        data.Command,
		Arguments:      data.Arguments,
		Replicas:       data.Replicas,
		Ports:          data.Ports,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("runtime = excluded.runtime").
		Set("command = excluded.command").
		Set("arguments = excluded.arguments").
		Set("replicas = excluded.replicas").
		Set("ports = excluded.ports").
		Set("resource_limits = excluded.resource_limits").
		Set("restart_policy = excluded.restart_policy").
		Set("settings = excluded.settings").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentRuntimeConfigurationEntity{}, err
	}

	return entity, nil
}
