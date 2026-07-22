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

type CaddyRouteEntity struct {
	bun.BaseModel       `bun:"table:caddy_routes,alias:caddy_routes"`
	ID                  uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time    `bun:"created_at"`
	UpdatedAt           time.Time    `bun:"updated_at"`
	EnvironmentTargetID uuid.UUID    `bun:"environment_target_id,type:uuid"`
	EnvironmentDomainID uuid.UUID    `bun:"environment_domain_id,type:uuid"`
	ReleaseID           uuid.UUID    `bun:"release_id,type:uuid"`
	ExternalID          string       `bun:"external_id"`
	State               string       `bun:"state"`
	AppliedAt           sql.NullTime `bun:"applied_at"`
	ObservedAt          sql.NullTime `bun:"observed_at"`
	RemovedAt           sql.NullTime `bun:"removed_at"`
}

func (e *CaddyRouteEntity) Validate() error {
	return nil
}

func (cr caddyRoute) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (CaddyRouteEntity, error) {
	var entity CaddyRouteEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return CaddyRouteEntity{}, err
	}

	return entity, nil
}

type CreateCaddyRouteData struct {
	EnvironmentTargetID uuid.UUID
	EnvironmentDomainID uuid.UUID
	ReleaseID           uuid.UUID
	ExternalID          string
	State               string
	AppliedAt           sql.NullTime
	ObservedAt          sql.NullTime
	RemovedAt           sql.NullTime
}

func (cr caddyRoute) Create(ctx context.Context, db storage.Executor, data CreateCaddyRouteData) (CaddyRouteEntity, error) {
	entity := CaddyRouteEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		EnvironmentDomainID: data.EnvironmentDomainID,
		ReleaseID:           data.ReleaseID,
		ExternalID:          data.ExternalID,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return CaddyRouteEntity{}, err
	}

	return entity, nil
}

type UpdateCaddyRouteData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	EnvironmentTargetID uuid.UUID
	EnvironmentDomainID uuid.UUID
	ReleaseID           uuid.UUID
	ExternalID          string
	State               string
	AppliedAt           sql.NullTime
	ObservedAt          sql.NullTime
	RemovedAt           sql.NullTime
}

func (cr caddyRoute) Update(ctx context.Context, db storage.Executor, data UpdateCaddyRouteData) (CaddyRouteEntity, error) {
	entity := CaddyRouteEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		EnvironmentDomainID: data.EnvironmentDomainID,
		ReleaseID:           data.ReleaseID,
		ExternalID:          data.ExternalID,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_target_id").
		Column("environment_domain_id").
		Column("release_id").
		Column("external_id").
		Column("state").
		Column("applied_at").
		Column("observed_at").
		Column("removed_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return CaddyRouteEntity{}, err
	}

	return entity, nil
}

func (cr caddyRoute) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*CaddyRouteEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (cr caddyRoute) All(ctx context.Context, db storage.Executor) ([]CaddyRouteEntity, error) {
	var entities []CaddyRouteEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedCaddyRoutes struct {
	CaddyRoutes []CaddyRouteEntity
	TotalCount  int64
	Page        int64
	PageSize    int64
	TotalPages  int64
}

func (cr caddyRoute) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedCaddyRoutes, error) {
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
		Model(&CaddyRouteEntity{}).Count(ctx)
	if err != nil {
		return PaginatedCaddyRoutes{}, err
	}

	entities := make([]CaddyRouteEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedCaddyRoutes{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedCaddyRoutes{
		CaddyRoutes: entities,
		TotalCount:  int64(totalCount),
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	}, nil
}

func (cr caddyRoute) Upsert(ctx context.Context, db storage.Executor, data CreateCaddyRouteData) (CaddyRouteEntity, error) {
	entity := CaddyRouteEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		EnvironmentDomainID: data.EnvironmentDomainID,
		ReleaseID:           data.ReleaseID,
		ExternalID:          data.ExternalID,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_target_id = excluded.environment_target_id").
		Set("environment_domain_id = excluded.environment_domain_id").
		Set("release_id = excluded.release_id").
		Set("external_id = excluded.external_id").
		Set("state = excluded.state").
		Set("applied_at = excluded.applied_at").
		Set("observed_at = excluded.observed_at").
		Set("removed_at = excluded.removed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return CaddyRouteEntity{}, err
	}

	return entity, nil
}
