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

type EnvironmentDomainEntity struct {
	bun.BaseModel `bun:"table:environment_domains,alias:environment_domains"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
	Hostname      string       `bun:"hostname"`
	IsPrimary     bool         `bun:"is_primary"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
}

func (e *EnvironmentDomainEntity) Validate() error {
	return nil
}

func (ed environmentDomain) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentDomainEntity, error) {
	var entity EnvironmentDomainEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentDomainData struct {
	EnvironmentID uuid.UUID
	Hostname      string
	IsPrimary     bool
	ArchivedAt    sql.NullTime
}

func (ed environmentDomain) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentDomainData) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentDomainData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	EnvironmentID uuid.UUID
	Hostname      string
	IsPrimary     bool
	ArchivedAt    sql.NullTime
}

func (ed environmentDomain) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentDomainData) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("hostname").
		Column("is_primary").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}

func (ed environmentDomain) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentDomainEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ed environmentDomain) All(ctx context.Context, db storage.Executor) ([]EnvironmentDomainEntity, error) {
	var entities []EnvironmentDomainEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentDomains struct {
	EnvironmentDomains []EnvironmentDomainEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (ed environmentDomain) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentDomains, error) {
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
		Model(&EnvironmentDomainEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentDomains{}, err
	}

	entities := make([]EnvironmentDomainEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentDomains{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentDomains{
		EnvironmentDomains: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (ed environmentDomain) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentDomainData) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("hostname = excluded.hostname").
		Set("is_primary = excluded.is_primary").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}
