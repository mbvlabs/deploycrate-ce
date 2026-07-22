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

type ResourceInstallationEntity struct {
	bun.BaseModel  `bun:"table:resource_installations,alias:resource_installations"`
	ID             uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	ResourceID     uuid.UUID       `bun:"resource_id,type:uuid"`
	ServerID       *uuid.UUID      `bun:"server_id,type:uuid"`
	Mode           string          `bun:"mode"`
	Driver         string          `bun:"driver"`
	DesiredVersion sql.NullString  `bun:"desired_version"`
	Configuration  json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt     sql.NullTime    `bun:"archived_at"`
}

func (e *ResourceInstallationEntity) Validate() error {
	return nil
}

func (ri resourceInstallation) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceInstallationEntity, error) {
	var entity ResourceInstallationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

type CreateResourceInstallationData struct {
	ResourceID     uuid.UUID
	ServerID       *uuid.UUID
	Mode           string
	Driver         string
	DesiredVersion sql.NullString
	Configuration  json.RawMessage
	ArchivedAt     sql.NullTime
}

func (ri resourceInstallation) Create(ctx context.Context, db storage.Executor, data CreateResourceInstallationData) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ResourceID:     data.ResourceID,
		ServerID:       data.ServerID,
		Mode:           data.Mode,
		Driver:         data.Driver,
		DesiredVersion: data.DesiredVersion,
		Configuration:  data.Configuration,
		ArchivedAt:     data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

type UpdateResourceInstallationData struct {
	ID             uuid.UUID
	UpdatedAt      time.Time
	ResourceID     uuid.UUID
	ServerID       *uuid.UUID
	Mode           string
	Driver         string
	DesiredVersion sql.NullString
	Configuration  json.RawMessage
	ArchivedAt     sql.NullTime
}

func (ri resourceInstallation) Update(ctx context.Context, db storage.Executor, data UpdateResourceInstallationData) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		ResourceID:     data.ResourceID,
		ServerID:       data.ServerID,
		Mode:           data.Mode,
		Driver:         data.Driver,
		DesiredVersion: data.DesiredVersion,
		Configuration:  data.Configuration,
		ArchivedAt:     data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_id").
		Column("server_id").
		Column("mode").
		Column("driver").
		Column("desired_version").
		Column("configuration").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

func (ri resourceInstallation) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceInstallationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ri resourceInstallation) All(ctx context.Context, db storage.Executor) ([]ResourceInstallationEntity, error) {
	var entities []ResourceInstallationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceInstallations struct {
	ResourceInstallations []ResourceInstallationEntity
	TotalCount            int64
	Page                  int64
	PageSize              int64
	TotalPages            int64
}

func (ri resourceInstallation) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceInstallations, error) {
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
		Model(&ResourceInstallationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceInstallations{}, err
	}

	entities := make([]ResourceInstallationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceInstallations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceInstallations{
		ResourceInstallations: entities,
		TotalCount:            int64(totalCount),
		Page:                  page,
		PageSize:              pageSize,
		TotalPages:            totalPages,
	}, nil
}

func (ri resourceInstallation) Upsert(ctx context.Context, db storage.Executor, data CreateResourceInstallationData) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ResourceID:     data.ResourceID,
		ServerID:       data.ServerID,
		Mode:           data.Mode,
		Driver:         data.Driver,
		DesiredVersion: data.DesiredVersion,
		Configuration:  data.Configuration,
		ArchivedAt:     data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_id = excluded.resource_id").
		Set("server_id = excluded.server_id").
		Set("mode = excluded.mode").
		Set("driver = excluded.driver").
		Set("desired_version = excluded.desired_version").
		Set("configuration = excluded.configuration").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}
