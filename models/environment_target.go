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

type EnvironmentTargetEntity struct {
	bun.BaseModel `bun:"table:environment_targets,alias:environment_targets"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
	ServerID      uuid.UUID    `bun:"server_id,type:uuid"`
	AttachedAt    time.Time    `bun:"attached_at"`
	DetachedAt    sql.NullTime `bun:"detached_at"`
}

func (e *EnvironmentTargetEntity) Validate() error {
	return nil
}

func (et environmentTarget) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentTargetEntity, error) {
	var entity EnvironmentTargetEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentTargetData struct {
	EnvironmentID uuid.UUID
	ServerID      uuid.UUID
	AttachedAt    time.Time
	DetachedAt    sql.NullTime
}

func (et environmentTarget) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetData) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentTargetData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	EnvironmentID uuid.UUID
	ServerID      uuid.UUID
	AttachedAt    time.Time
	DetachedAt    sql.NullTime
}

func (et environmentTarget) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentTargetData) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("server_id").
		Column("attached_at").
		Column("detached_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}

func (et environmentTarget) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentTargetEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (et environmentTarget) All(ctx context.Context, db storage.Executor) ([]EnvironmentTargetEntity, error) {
	var entities []EnvironmentTargetEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentTargets struct {
	EnvironmentTargets []EnvironmentTargetEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (et environmentTarget) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentTargets, error) {
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
		Model(&EnvironmentTargetEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentTargets{}, err
	}

	entities := make([]EnvironmentTargetEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentTargets{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentTargets{
		EnvironmentTargets: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (et environmentTarget) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetData) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("server_id = excluded.server_id").
		Set("attached_at = excluded.attached_at").
		Set("detached_at = excluded.detached_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}
