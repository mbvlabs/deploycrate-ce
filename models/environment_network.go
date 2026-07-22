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

type EnvironmentNetworkEntity struct {
	bun.BaseModel    `bun:"table:environment_networks,alias:environment_networks"`
	ID               int32        `bun:"id,pk,autoincrement"`
	CreatedAt        time.Time    `bun:"created_at"`
	UpdatedAt        time.Time    `bun:"updated_at"`
	EnvironmentID    uuid.UUID    `bun:"environment_id,type:uuid"`
	PrivateNetworkID uuid.UUID    `bun:"private_network_id,type:uuid"`
	Role             string       `bun:"role"`
	RemovedAt        sql.NullTime `bun:"removed_at"`
}

func (e *EnvironmentNetworkEntity) Validate() error {
	return nil
}

func (en environmentNetwork) Find(ctx context.Context, db storage.Executor, id int32) (EnvironmentNetworkEntity, error) {
	var entity EnvironmentNetworkEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentNetworkEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentNetworkData struct {
	EnvironmentID    uuid.UUID
	PrivateNetworkID uuid.UUID
	Role             string
	RemovedAt        sql.NullTime
}

func (en environmentNetwork) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentNetworkData) (EnvironmentNetworkEntity, error) {
	entity := EnvironmentNetworkEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		PrivateNetworkID: data.PrivateNetworkID,
		Role:             data.Role,
		RemovedAt:        data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentNetworkEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentNetworkData struct {
	ID               int32
	UpdatedAt        time.Time
	EnvironmentID    uuid.UUID
	PrivateNetworkID uuid.UUID
	Role             string
	RemovedAt        sql.NullTime
}

func (en environmentNetwork) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentNetworkData) (EnvironmentNetworkEntity, error) {
	entity := EnvironmentNetworkEntity{
		ID:               data.ID,
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		PrivateNetworkID: data.PrivateNetworkID,
		Role:             data.Role,
		RemovedAt:        data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("private_network_id").
		Column("role").
		Column("removed_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentNetworkEntity{}, err
	}

	return entity, nil
}

func (en environmentNetwork) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*EnvironmentNetworkEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (en environmentNetwork) All(ctx context.Context, db storage.Executor) ([]EnvironmentNetworkEntity, error) {
	var entities []EnvironmentNetworkEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentNetworks struct {
	EnvironmentNetworks []EnvironmentNetworkEntity
	TotalCount          int64
	Page                int64
	PageSize            int64
	TotalPages          int64
}

func (en environmentNetwork) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentNetworks, error) {
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
		Model(&EnvironmentNetworkEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentNetworks{}, err
	}

	entities := make([]EnvironmentNetworkEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentNetworks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentNetworks{
		EnvironmentNetworks: entities,
		TotalCount:          int64(totalCount),
		Page:                page,
		PageSize:            pageSize,
		TotalPages:          totalPages,
	}, nil
}

func (en environmentNetwork) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentNetworkData) (EnvironmentNetworkEntity, error) {
	entity := EnvironmentNetworkEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EnvironmentID:    data.EnvironmentID,
		PrivateNetworkID: data.PrivateNetworkID,
		Role:             data.Role,
		RemovedAt:        data.RemovedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("private_network_id = excluded.private_network_id").
		Set("role = excluded.role").
		Set("removed_at = excluded.removed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentNetworkEntity{}, err
	}

	return entity, nil
}
