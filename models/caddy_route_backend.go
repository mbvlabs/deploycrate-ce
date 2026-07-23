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

type CaddyRouteBackendEntity struct {
	bun.BaseModel `bun:"table:caddy_route_backends,alias:caddy_route_backends"`
	ID            int32        `bun:"id,pk,autoincrement"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Weight        int32        `bun:"weight"`
	RemovedAt     sql.NullTime `bun:"removed_at"`
	CaddyRouteID  uuid.UUID    `bun:"caddy_route_id,type:uuid"`
	InstanceID    uuid.UUID    `bun:"instance_id,type:uuid"`
}

func (e *CaddyRouteBackendEntity) Validate() error {
	return nil
}

func (crb caddyRouteBackend) Find(ctx context.Context, db storage.Executor, id int32) (CaddyRouteBackendEntity, error) {
	var entity CaddyRouteBackendEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return CaddyRouteBackendEntity{}, err
	}

	return entity, nil
}

type CreateCaddyRouteBackendData struct {
	Weight       int32
	RemovedAt    sql.NullTime
	CaddyRouteID uuid.UUID
	InstanceID   uuid.UUID
}

func (crb caddyRouteBackend) Create(ctx context.Context, db storage.Executor, data CreateCaddyRouteBackendData) (CaddyRouteBackendEntity, error) {
	entity := CaddyRouteBackendEntity{
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Weight:       data.Weight,
		RemovedAt:    data.RemovedAt,
		CaddyRouteID: data.CaddyRouteID,
		InstanceID:   data.InstanceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteBackendEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return CaddyRouteBackendEntity{}, err
	}

	return entity, nil
}

type UpdateCaddyRouteBackendData struct {
	ID           int32
	UpdatedAt    time.Time
	Weight       int32
	RemovedAt    sql.NullTime
	CaddyRouteID uuid.UUID
	InstanceID   uuid.UUID
}

func (crb caddyRouteBackend) Update(ctx context.Context, db storage.Executor, data UpdateCaddyRouteBackendData) (CaddyRouteBackendEntity, error) {
	entity := CaddyRouteBackendEntity{
		ID:           data.ID,
		UpdatedAt:    time.Now(),
		Weight:       data.Weight,
		RemovedAt:    data.RemovedAt,
		CaddyRouteID: data.CaddyRouteID,
		InstanceID:   data.InstanceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteBackendEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("weight").
		Column("removed_at").
		Column("caddy_route_id").
		Column("instance_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return CaddyRouteBackendEntity{}, err
	}

	return entity, nil
}

func (crb caddyRouteBackend) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*CaddyRouteBackendEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (crb caddyRouteBackend) All(ctx context.Context, db storage.Executor) ([]CaddyRouteBackendEntity, error) {
	var entities []CaddyRouteBackendEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedCaddyRouteBackends struct {
	CaddyRouteBackends []CaddyRouteBackendEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (crb caddyRouteBackend) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedCaddyRouteBackends, error) {
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
		Model(&CaddyRouteBackendEntity{}).Count(ctx)
	if err != nil {
		return PaginatedCaddyRouteBackends{}, err
	}

	entities := make([]CaddyRouteBackendEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedCaddyRouteBackends{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedCaddyRouteBackends{
		CaddyRouteBackends: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (crb caddyRouteBackend) Upsert(ctx context.Context, db storage.Executor, data CreateCaddyRouteBackendData) (CaddyRouteBackendEntity, error) {
	entity := CaddyRouteBackendEntity{
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Weight:       data.Weight,
		RemovedAt:    data.RemovedAt,
		CaddyRouteID: data.CaddyRouteID,
		InstanceID:   data.InstanceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return CaddyRouteBackendEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("weight = excluded.weight").
		Set("removed_at = excluded.removed_at").
		Set("caddy_route_id = excluded.caddy_route_id").
		Set("instance_id = excluded.instance_id").
		Returning("*").
		Scan(ctx); err != nil {
		return CaddyRouteBackendEntity{}, err
	}

	return entity, nil
}
