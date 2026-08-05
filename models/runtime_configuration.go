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

type RuntimeConfigurationEntity struct {
	bun.BaseModel  `bun:"table:runtime_configurations,alias:runtime_configurations"`
	ID             int32           `bun:"id,pk,autoincrement"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	Runtime        string          `bun:"runtime"`
	ResourceLimits json.RawMessage `bun:"resource_limits,type:jsonb"`
	RestartPolicy  string          `bun:"restart_policy"`
	Settings       json.RawMessage `bun:"settings,type:jsonb"`
	EnvironmentID  uuid.UUID       `bun:"environment_id,type:uuid"`
}

func (e *RuntimeConfigurationEntity) Validate() error {
	e.Runtime = strings.ToLower(strings.TrimSpace(e.Runtime))
	e.RestartPolicy = strings.ToLower(strings.TrimSpace(e.RestartPolicy))
	builder := validation.NewBuilder()
	if e.Runtime != "go" {
		builder.Add("runtime", "unsupported", "only the Go runtime is supported")
	}
	if e.RestartPolicy != "unless-stopped" {
		builder.Add("restartPolicy", "unsupported", "restart policy must be unless-stopped")
	}
	for field, value := range map[string]json.RawMessage{"resourceLimits": e.ResourceLimits, "settings": e.Settings} {
		if len(value) == 0 || !json.Valid(value) {
			builder.Add(field, "invalid", field+" must be valid JSON")
		}
	}
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environmentId", "required", "Environment is required")
	}
	return builder.Err()
}

func ensureRuntimeConfigurationUnique(ctx context.Context, db storage.Executor, entity RuntimeConfigurationEntity) error {
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return errors.New("runtime configuration uniqueness checks require a transaction")
	}
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "runtime-configuration:"+entity.EnvironmentID.String()); err != nil {
		return err
	}
	count, err := db.NewSelect().Model((*RuntimeConfigurationEntity)(nil)).
		Where("environment_id = ?", entity.EnvironmentID).Where("id <> ?", entity.ID).Count(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "environmentId", Code: "taken", Message: "the Environment already has a runtime configuration"}})
	}
	return nil
}

func (rc runtimeConfiguration) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (RuntimeConfigurationEntity, error) {
	var entity RuntimeConfigurationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

type CreateRuntimeConfigurationData struct {
	Runtime        string
	ResourceLimits json.RawMessage
	RestartPolicy  string
	Settings       json.RawMessage
	EnvironmentID  uuid.UUID
}

func (rc runtimeConfiguration) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateRuntimeConfigurationData,
) (RuntimeConfigurationEntity, error) {
	entity := RuntimeConfigurationEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Runtime:        data.Runtime,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
		EnvironmentID:  data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return RuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureRuntimeConfigurationUnique(ctx, db, entity); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

type UpdateRuntimeConfigurationData struct {
	ID             int32
	UpdatedAt      time.Time
	Runtime        string
	ResourceLimits json.RawMessage
	RestartPolicy  string
	Settings       json.RawMessage
	EnvironmentID  uuid.UUID
}

func (rc runtimeConfiguration) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateRuntimeConfigurationData,
) (RuntimeConfigurationEntity, error) {
	entity := RuntimeConfigurationEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		Runtime:        data.Runtime,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
		EnvironmentID:  data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return RuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureRuntimeConfigurationUnique(ctx, db, entity); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("runtime").
		Column("resource_limits").
		Column("restart_policy").
		Column("settings").
		Column("environment_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	return entity, nil
}

func (rc runtimeConfiguration) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*RuntimeConfigurationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rc runtimeConfiguration) All(
	ctx context.Context,
	db storage.Executor,
) ([]RuntimeConfigurationEntity, error) {
	var entities []RuntimeConfigurationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedRuntimeConfigurations struct {
	RuntimeConfigurations []RuntimeConfigurationEntity
	TotalCount            int64
	Page                  int64
	PageSize              int64
	TotalPages            int64
}

func (rc runtimeConfiguration) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedRuntimeConfigurations, error) {
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
		Model(&RuntimeConfigurationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedRuntimeConfigurations{}, err
	}

	entities := make([]RuntimeConfigurationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedRuntimeConfigurations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedRuntimeConfigurations{
		RuntimeConfigurations: entities,
		TotalCount:            int64(totalCount),
		Page:                  page,
		PageSize:              pageSize,
		TotalPages:            totalPages,
	}, nil
}

func (rc runtimeConfiguration) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateRuntimeConfigurationData,
) (RuntimeConfigurationEntity, error) {
	entity := RuntimeConfigurationEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Runtime:        data.Runtime,
		ResourceLimits: data.ResourceLimits,
		RestartPolicy:  data.RestartPolicy,
		Settings:       data.Settings,
		EnvironmentID:  data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return RuntimeConfigurationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureRuntimeConfigurationUnique(ctx, db, entity); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("runtime = excluded.runtime").
		Set("resource_limits = excluded.resource_limits").
		Set("restart_policy = excluded.restart_policy").
		Set("settings = excluded.settings").
		Set("environment_id = excluded.environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return RuntimeConfigurationEntity{}, err
	}

	return entity, nil
}
