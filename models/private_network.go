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

type PrivateNetworkEntity struct {
	bun.BaseModel      `bun:"table:private_networks,alias:private_networks"`
	ID                 uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time    `bun:"created_at"`
	UpdatedAt          time.Time    `bun:"updated_at"`
	Name               string       `bun:"name"`
	ArchivedAt         sql.NullTime `bun:"archived_at"`
	OwnerEnvironmentID *uuid.UUID   `bun:"owner_environment_id,type:uuid"`
}

func (e *PrivateNetworkEntity) Validate() error {
	return nil
}

func (pn privateNetwork) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (PrivateNetworkEntity, error) {
	var entity PrivateNetworkEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return PrivateNetworkEntity{}, err
	}

	return entity, nil
}

type CreatePrivateNetworkData struct {
	Name               string
	ArchivedAt         sql.NullTime
	OwnerEnvironmentID *uuid.UUID
}

func (pn privateNetwork) Create(ctx context.Context, db storage.Executor, data CreatePrivateNetworkData) (PrivateNetworkEntity, error) {
	entity := PrivateNetworkEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return PrivateNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return PrivateNetworkEntity{}, err
	}

	return entity, nil
}

type UpdatePrivateNetworkData struct {
	ID                 uuid.UUID
	UpdatedAt          time.Time
	Name               string
	ArchivedAt         sql.NullTime
	OwnerEnvironmentID *uuid.UUID
}

func (pn privateNetwork) Update(ctx context.Context, db storage.Executor, data UpdatePrivateNetworkData) (PrivateNetworkEntity, error) {
	entity := PrivateNetworkEntity{
		ID:                 data.ID,
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return PrivateNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("archived_at").
		Column("owner_environment_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return PrivateNetworkEntity{}, err
	}

	return entity, nil
}

func (pn privateNetwork) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*PrivateNetworkEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (pn privateNetwork) All(ctx context.Context, db storage.Executor) ([]PrivateNetworkEntity, error) {
	var entities []PrivateNetworkEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedPrivateNetworks struct {
	PrivateNetworks []PrivateNetworkEntity
	TotalCount      int64
	Page            int64
	PageSize        int64
	TotalPages      int64
}

func (pn privateNetwork) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedPrivateNetworks, error) {
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
		Model(&PrivateNetworkEntity{}).Count(ctx)
	if err != nil {
		return PaginatedPrivateNetworks{}, err
	}

	entities := make([]PrivateNetworkEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedPrivateNetworks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedPrivateNetworks{
		PrivateNetworks: entities,
		TotalCount:      int64(totalCount),
		Page:            page,
		PageSize:        pageSize,
		TotalPages:      totalPages,
	}, nil
}

func (pn privateNetwork) Upsert(ctx context.Context, db storage.Executor, data CreatePrivateNetworkData) (PrivateNetworkEntity, error) {
	entity := PrivateNetworkEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return PrivateNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("archived_at = excluded.archived_at").
		Set("owner_environment_id = excluded.owner_environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return PrivateNetworkEntity{}, err
	}

	return entity, nil
}
