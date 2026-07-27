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

type ServerNetworkEntity struct {
	bun.BaseModel    `bun:"table:server_networks,alias:server_networks"`
	ID               int32           `bun:"id,pk,autoincrement"`
	CreatedAt        time.Time       `bun:"created_at"`
	UpdatedAt        time.Time       `bun:"updated_at"`
	Driver           string          `bun:"driver"`
	ExternalID       sql.NullString  `bun:"external_id"`
	Configuration    json.RawMessage `bun:"configuration,type:jsonb"`
	State            string          `bun:"state"`
	AppliedAt        sql.NullTime    `bun:"applied_at"`
	ObservedAt       sql.NullTime    `bun:"observed_at"`
	Error            sql.NullString  `bun:"error"`
	RemovedAt        sql.NullTime    `bun:"removed_at"`
	ServerID         uuid.UUID       `bun:"server_id,type:uuid"`
	PrivateNetworkID uuid.UUID       `bun:"private_network_id,type:uuid"`
}

func (e *ServerNetworkEntity) Validate() error {
	return nil
}

func (sn serverNetwork) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (ServerNetworkEntity, error) {
	var entity ServerNetworkEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ServerNetworkEntity{}, err
	}

	return entity, nil
}

type CreateServerNetworkData struct {
	Driver           string
	ExternalID       sql.NullString
	Configuration    json.RawMessage
	State            string
	AppliedAt        sql.NullTime
	ObservedAt       sql.NullTime
	Error            sql.NullString
	RemovedAt        sql.NullTime
	ServerID         uuid.UUID
	PrivateNetworkID uuid.UUID
}

func (sn serverNetwork) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateServerNetworkData,
) (ServerNetworkEntity, error) {
	entity := ServerNetworkEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Driver:           data.Driver,
		ExternalID:       data.ExternalID,
		Configuration:    data.Configuration,
		State:            data.State,
		AppliedAt:        data.AppliedAt,
		ObservedAt:       data.ObservedAt,
		Error:            data.Error,
		RemovedAt:        data.RemovedAt,
		ServerID:         data.ServerID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ServerNetworkEntity{}, err
	}

	return entity, nil
}

type UpdateServerNetworkData struct {
	ID               int32
	UpdatedAt        time.Time
	Driver           string
	ExternalID       sql.NullString
	Configuration    json.RawMessage
	State            string
	AppliedAt        sql.NullTime
	ObservedAt       sql.NullTime
	Error            sql.NullString
	RemovedAt        sql.NullTime
	ServerID         uuid.UUID
	PrivateNetworkID uuid.UUID
}

func (sn serverNetwork) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateServerNetworkData,
) (ServerNetworkEntity, error) {
	entity := ServerNetworkEntity{
		ID:               data.ID,
		UpdatedAt:        time.Now(),
		Driver:           data.Driver,
		ExternalID:       data.ExternalID,
		Configuration:    data.Configuration,
		State:            data.State,
		AppliedAt:        data.AppliedAt,
		ObservedAt:       data.ObservedAt,
		Error:            data.Error,
		RemovedAt:        data.RemovedAt,
		ServerID:         data.ServerID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("driver").
		Column("external_id").
		Column("configuration").
		Column("state").
		Column("applied_at").
		Column("observed_at").
		Column("error").
		Column("removed_at").
		Column("server_id").
		Column("private_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ServerNetworkEntity{}, err
	}

	return entity, nil
}

func (sn serverNetwork) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ServerNetworkEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (sn serverNetwork) All(
	ctx context.Context,
	db storage.Executor,
) ([]ServerNetworkEntity, error) {
	var entities []ServerNetworkEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedServerNetworks struct {
	ServerNetworks []ServerNetworkEntity
	TotalCount     int64
	Page           int64
	PageSize       int64
	TotalPages     int64
}

func (sn serverNetwork) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedServerNetworks, error) {
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
		Model(&ServerNetworkEntity{}).Count(ctx)
	if err != nil {
		return PaginatedServerNetworks{}, err
	}

	entities := make([]ServerNetworkEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedServerNetworks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedServerNetworks{
		ServerNetworks: entities,
		TotalCount:     int64(totalCount),
		Page:           page,
		PageSize:       pageSize,
		TotalPages:     totalPages,
	}, nil
}

func (sn serverNetwork) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateServerNetworkData,
) (ServerNetworkEntity, error) {
	entity := ServerNetworkEntity{
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Driver:           data.Driver,
		ExternalID:       data.ExternalID,
		Configuration:    data.Configuration,
		State:            data.State,
		AppliedAt:        data.AppliedAt,
		ObservedAt:       data.ObservedAt,
		Error:            data.Error,
		RemovedAt:        data.RemovedAt,
		ServerID:         data.ServerID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("driver = excluded.driver").
		Set("external_id = excluded.external_id").
		Set("configuration = excluded.configuration").
		Set("state = excluded.state").
		Set("applied_at = excluded.applied_at").
		Set("observed_at = excluded.observed_at").
		Set("error = excluded.error").
		Set("removed_at = excluded.removed_at").
		Set("server_id = excluded.server_id").
		Set("private_network_id = excluded.private_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ServerNetworkEntity{}, err
	}

	return entity, nil
}
