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

type ResourceInstallationStatusEntity struct {
	bun.BaseModel          `bun:"table:resource_installation_statuses,alias:resource_installation_statuses"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	ExternalID             sql.NullString  `bun:"external_id"`
	State                  string          `bun:"state"`
	InstalledVersion       sql.NullString  `bun:"installed_version"`
	ServiceState           string          `bun:"service_state"`
	Health                 string          `bun:"health"`
	Source                 string          `bun:"source"`
	HealthReason           sql.NullString  `bun:"health_reason"`
	Details                json.RawMessage `bun:"details,type:jsonb"`
	ObservedAt             time.Time       `bun:"observed_at"`
	ExpiresAt              time.Time       `bun:"expires_at"`
	ResourceInstallationID uuid.UUID       `bun:"resource_installation_id,pk,type:uuid"`
}

func (e *ResourceInstallationStatusEntity) Validate() error {
	return nil
}

func (ris resourceInstallationStatus) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceInstallationStatusEntity, error) {
	var entity ResourceInstallationStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("resource_installation_id = ?", id).
		Scan(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}

type CreateResourceInstallationStatusData struct {
	ResourceInstallationID uuid.UUID
	ExternalID             sql.NullString
	State                  string
	InstalledVersion       sql.NullString
	ServiceState           string
	Health                 string
	Source                 string
	HealthReason           sql.NullString
	Details                json.RawMessage
	ObservedAt             time.Time
	ExpiresAt              time.Time
}

func (ris resourceInstallationStatus) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceInstallationStatusData,
) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		ResourceInstallationID: data.ResourceInstallationID,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Source:                 data.Source,
		HealthReason:           data.HealthReason,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
		ExpiresAt:              data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}

type UpdateResourceInstallationStatusData struct {
	ResourceInstallationID uuid.UUID
	UpdatedAt              time.Time
	ExternalID             sql.NullString
	State                  string
	InstalledVersion       sql.NullString
	ServiceState           string
	Health                 string
	Source                 string
	HealthReason           sql.NullString
	Details                json.RawMessage
	ObservedAt             time.Time
	ExpiresAt              time.Time
}

func (ris resourceInstallationStatus) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceInstallationStatusData,
) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		ResourceInstallationID: data.ResourceInstallationID,
		UpdatedAt:              time.Now(),
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Source:                 data.Source,
		HealthReason:           data.HealthReason,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
		ExpiresAt:              data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("external_id").
		Column("state").
		Column("installed_version").
		Column("service_state").
		Column("health").
		Column("source").
		Column("health_reason").
		Column("details").
		Column("observed_at").
		Column("expires_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}

func (ris resourceInstallationStatus) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*ResourceInstallationStatusEntity)(nil)).
		Where("resource_installation_id = ?", id).
		Exec(ctx)

	return err
}

func (ris resourceInstallationStatus) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceInstallationStatusEntity, error) {
	var entities []ResourceInstallationStatusEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceInstallationStatuses struct {
	ResourceInstallationStatuses []ResourceInstallationStatusEntity
	TotalCount                   int64
	Page                         int64
	PageSize                     int64
	TotalPages                   int64
}

func (ris resourceInstallationStatus) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceInstallationStatuses, error) {
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
		Model(&ResourceInstallationStatusEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceInstallationStatuses{}, err
	}

	entities := make([]ResourceInstallationStatusEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceInstallationStatuses{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceInstallationStatuses{
		ResourceInstallationStatuses: entities,
		TotalCount:                   int64(totalCount),
		Page:                         page,
		PageSize:                     pageSize,
		TotalPages:                   totalPages,
	}, nil
}

func (ris resourceInstallationStatus) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceInstallationStatusData,
) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		ResourceInstallationID: data.ResourceInstallationID,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Source:                 data.Source,
		HealthReason:           data.HealthReason,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
		ExpiresAt:              data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (resource_installation_id) DO UPDATE").
		Set("external_id = excluded.external_id").
		Set("state = excluded.state").
		Set("installed_version = excluded.installed_version").
		Set("service_state = excluded.service_state").
		Set("health = excluded.health").
		Set("source = excluded.source").
		Set("health_reason = excluded.health_reason").
		Set("details = excluded.details").
		Set("observed_at = excluded.observed_at").
		Set("expires_at = excluded.expires_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}
