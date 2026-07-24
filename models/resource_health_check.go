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

type ResourceHealthCheckEntity struct {
	bun.BaseModel          `bun:"table:resource_health_checks,alias:resource_health_checks"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	Name                   string          `bun:"name"`
	Kind                   string          `bun:"kind"`
	Configuration          json.RawMessage `bun:"configuration,type:jsonb"`
	IntervalSeconds        int32           `bun:"interval_seconds"`
	TimeoutSeconds         int32           `bun:"timeout_seconds"`
	FailureThreshold       int32           `bun:"failure_threshold"`
	SuccessThreshold       int32           `bun:"success_threshold"`
	Enabled                bool            `bun:"enabled"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
	ResourceInstallationID uuid.UUID       `bun:"resource_installation_id,type:uuid"`
	ResourceEndpointID     *uuid.UUID      `bun:"resource_endpoint_id,type:uuid"`
	ResourceCredentialID   *uuid.UUID      `bun:"resource_credential_id,type:uuid"`
}

func (e *ResourceHealthCheckEntity) Validate() error {
	return nil
}

func (rhc resourceHealthCheck) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceHealthCheckEntity, error) {
	var entity ResourceHealthCheckEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

type CreateResourceHealthCheckData struct {
	Name                   string
	Kind                   string
	Configuration          json.RawMessage
	IntervalSeconds        int32
	TimeoutSeconds         int32
	FailureThreshold       int32
	SuccessThreshold       int32
	Enabled                bool
	ArchivedAt             sql.NullTime
	ResourceInstallationID uuid.UUID
	ResourceEndpointID     *uuid.UUID
	ResourceCredentialID   *uuid.UUID
}

func (rhc resourceHealthCheck) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

type UpdateResourceHealthCheckData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	Name                   string
	Kind                   string
	Configuration          json.RawMessage
	IntervalSeconds        int32
	TimeoutSeconds         int32
	FailureThreshold       int32
	SuccessThreshold       int32
	Enabled                bool
	ArchivedAt             sql.NullTime
	ResourceInstallationID uuid.UUID
	ResourceEndpointID     *uuid.UUID
	ResourceCredentialID   *uuid.UUID
}

func (rhc resourceHealthCheck) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("kind").
		Column("configuration").
		Column("interval_seconds").
		Column("timeout_seconds").
		Column("failure_threshold").
		Column("success_threshold").
		Column("enabled").
		Column("archived_at").
		Column("resource_installation_id").
		Column("resource_endpoint_id").
		Column("resource_credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

func (rhc resourceHealthCheck) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*ResourceHealthCheckEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rhc resourceHealthCheck) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceHealthCheckEntity, error) {
	var entities []ResourceHealthCheckEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceHealthChecks struct {
	ResourceHealthChecks []ResourceHealthCheckEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (rhc resourceHealthCheck) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceHealthChecks, error) {
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
		Model(&ResourceHealthCheckEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceHealthChecks{}, err
	}

	entities := make([]ResourceHealthCheckEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceHealthChecks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceHealthChecks{
		ResourceHealthChecks: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (rhc resourceHealthCheck) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("kind = excluded.kind").
		Set("configuration = excluded.configuration").
		Set("interval_seconds = excluded.interval_seconds").
		Set("timeout_seconds = excluded.timeout_seconds").
		Set("failure_threshold = excluded.failure_threshold").
		Set("success_threshold = excluded.success_threshold").
		Set("enabled = excluded.enabled").
		Set("archived_at = excluded.archived_at").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("resource_credential_id = excluded.resource_credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}
