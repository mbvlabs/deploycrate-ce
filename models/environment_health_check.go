package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentHealthCheckEntity struct {
	bun.BaseModel   `bun:"table:environment_health_checks,alias:environment_health_checks"`
	ID              uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt       time.Time    `bun:"created_at"`
	UpdatedAt       time.Time    `bun:"updated_at"`
	Name            string       `bun:"name"`
	Url             string       `bun:"url"`
	Method          string       `bun:"method"`
	ExpectedStatus  int32        `bun:"expected_status"`
	TimeoutSeconds  int32        `bun:"timeout_seconds"`
	IntervalSeconds int32        `bun:"interval_seconds"`
	Enabled         bool         `bun:"enabled"`
	ArchivedAt      sql.NullTime `bun:"archived_at"`
	EnvironmentID   uuid.UUID    `bun:"environment_id,type:uuid"`
}

func (e *EnvironmentHealthCheckEntity) Validate() error {
	return nil
}

func (ehc environmentHealthCheck) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentHealthCheckEntity, error) {
	var entity EnvironmentHealthCheckEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentHealthCheckData struct {
	Name            string
	Url             string
	Method          string
	ExpectedStatus  int32
	TimeoutSeconds  int32
	IntervalSeconds int32
	Enabled         bool
	ArchivedAt      sql.NullTime
	EnvironmentID   uuid.UUID
}

func (ehc environmentHealthCheck) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentHealthCheckData,
) (EnvironmentHealthCheckEntity, error) {
	entity := EnvironmentHealthCheckEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Name:            data.Name,
		Url:             data.Url,
		Method:          data.Method,
		ExpectedStatus:  data.ExpectedStatus,
		TimeoutSeconds:  data.TimeoutSeconds,
		IntervalSeconds: data.IntervalSeconds,
		Enabled:         data.Enabled,
		ArchivedAt:      data.ArchivedAt,
		EnvironmentID:   data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentHealthCheckEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentHealthCheckData struct {
	ID              uuid.UUID
	UpdatedAt       time.Time
	Name            string
	Url             string
	Method          string
	ExpectedStatus  int32
	TimeoutSeconds  int32
	IntervalSeconds int32
	Enabled         bool
	ArchivedAt      sql.NullTime
	EnvironmentID   uuid.UUID
}

func (ehc environmentHealthCheck) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentHealthCheckData,
) (EnvironmentHealthCheckEntity, error) {
	entity := EnvironmentHealthCheckEntity{
		ID:              data.ID,
		UpdatedAt:       time.Now(),
		Name:            data.Name,
		Url:             data.Url,
		Method:          data.Method,
		ExpectedStatus:  data.ExpectedStatus,
		TimeoutSeconds:  data.TimeoutSeconds,
		IntervalSeconds: data.IntervalSeconds,
		Enabled:         data.Enabled,
		ArchivedAt:      data.ArchivedAt,
		EnvironmentID:   data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("url").
		Column("method").
		Column("expected_status").
		Column("timeout_seconds").
		Column("interval_seconds").
		Column("enabled").
		Column("archived_at").
		Column("environment_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckEntity{}, err
	}

	return entity, nil
}

func (ehc environmentHealthCheck) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*EnvironmentHealthCheckEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ehc environmentHealthCheck) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentHealthCheckEntity, error) {
	var entities []EnvironmentHealthCheckEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentHealthChecks struct {
	EnvironmentHealthChecks []EnvironmentHealthCheckEntity
	TotalCount              int64
	Page                    int64
	PageSize                int64
	TotalPages              int64
}

func (ehc environmentHealthCheck) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentHealthChecks, error) {
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
		Model(&EnvironmentHealthCheckEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentHealthChecks{}, err
	}

	entities := make([]EnvironmentHealthCheckEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentHealthChecks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentHealthChecks{
		EnvironmentHealthChecks: entities,
		TotalCount:              int64(totalCount),
		Page:                    page,
		PageSize:                pageSize,
		TotalPages:              totalPages,
	}, nil
}

func (ehc environmentHealthCheck) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentHealthCheckData,
) (EnvironmentHealthCheckEntity, error) {
	entity := EnvironmentHealthCheckEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Name:            data.Name,
		Url:             data.Url,
		Method:          data.Method,
		ExpectedStatus:  data.ExpectedStatus,
		TimeoutSeconds:  data.TimeoutSeconds,
		IntervalSeconds: data.IntervalSeconds,
		Enabled:         data.Enabled,
		ArchivedAt:      data.ArchivedAt,
		EnvironmentID:   data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("url = excluded.url").
		Set("method = excluded.method").
		Set("expected_status = excluded.expected_status").
		Set("timeout_seconds = excluded.timeout_seconds").
		Set("interval_seconds = excluded.interval_seconds").
		Set("enabled = excluded.enabled").
		Set("archived_at = excluded.archived_at").
		Set("environment_id = excluded.environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckEntity{}, err
	}

	return entity, nil
}
