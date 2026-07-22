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
	ID                     int32           `bun:"id,pk,autoincrement"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	ResourceInstallationID uuid.UUID       `bun:"resource_installation_id,type:uuid"`
	ExternalID             sql.NullString  `bun:"external_id"`
	State                  string          `bun:"state"`
	InstalledVersion       sql.NullString  `bun:"installed_version"`
	ServiceState           sql.NullString  `bun:"service_state"`
	Health                 sql.NullString  `bun:"health"`
	Details                json.RawMessage `bun:"details,type:jsonb"`
	ObservedAt             time.Time       `bun:"observed_at"`
}

func (e *ResourceInstallationStatusEntity) Validate() error {
	return nil
}

func (ris resourceInstallationStatus) Find(ctx context.Context, db storage.Executor, id int32) (ResourceInstallationStatusEntity, error) {
	var entity ResourceInstallationStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
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
	ServiceState           sql.NullString
	Health                 sql.NullString
	Details                json.RawMessage
	ObservedAt             time.Time
}

func (ris resourceInstallationStatus) Create(ctx context.Context, db storage.Executor, data CreateResourceInstallationStatusData) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ResourceInstallationID: data.ResourceInstallationID,
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
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
	ID                     int32
	UpdatedAt              time.Time
	ResourceInstallationID uuid.UUID
	ExternalID             sql.NullString
	State                  string
	InstalledVersion       sql.NullString
	ServiceState           sql.NullString
	Health                 sql.NullString
	Details                json.RawMessage
	ObservedAt             time.Time
}

func (ris resourceInstallationStatus) Update(ctx context.Context, db storage.Executor, data UpdateResourceInstallationStatusData) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		ResourceInstallationID: data.ResourceInstallationID,
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_installation_id").
		Column("external_id").
		Column("state").
		Column("installed_version").
		Column("service_state").
		Column("health").
		Column("details").
		Column("observed_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}

func (ris resourceInstallationStatus) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ResourceInstallationStatusEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ris resourceInstallationStatus) All(ctx context.Context, db storage.Executor) ([]ResourceInstallationStatusEntity, error) {
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

func (ris resourceInstallationStatus) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceInstallationStatuses, error) {
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

func (ris resourceInstallationStatus) Upsert(ctx context.Context, db storage.Executor, data CreateResourceInstallationStatusData) (ResourceInstallationStatusEntity, error) {
	entity := ResourceInstallationStatusEntity{
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ResourceInstallationID: data.ResourceInstallationID,
		ExternalID:             data.ExternalID,
		State:                  data.State,
		InstalledVersion:       data.InstalledVersion,
		ServiceState:           data.ServiceState,
		Health:                 data.Health,
		Details:                data.Details,
		ObservedAt:             data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("external_id = excluded.external_id").
		Set("state = excluded.state").
		Set("installed_version = excluded.installed_version").
		Set("service_state = excluded.service_state").
		Set("health = excluded.health").
		Set("details = excluded.details").
		Set("observed_at = excluded.observed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationStatusEntity{}, err
	}

	return entity, nil
}
