package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ServerStatusEntity struct {
	bun.BaseModel `bun:"table:server_statuses,alias:server_statuses"`
	ID            int32     `bun:"id,pk,autoincrement"`
	CreatedAt     time.Time `bun:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at"`
	State         string    `bun:"state"`
	ObservedAt    time.Time `bun:"observed_at"`
	ServerID      uuid.UUID `bun:"server_id,type:uuid"`
}

func (e *ServerStatusEntity) Validate() error {
	return nil
}

func (ss serverStatus) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (ServerStatusEntity, error) {
	var entity ServerStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ServerStatusEntity{}, err
	}

	return entity, nil
}

type CreateServerStatusData struct {
	State      string
	ObservedAt time.Time
	ServerID   uuid.UUID
}

func (ss serverStatus) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateServerStatusData,
) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		State:      data.State,
		ObservedAt: data.ObservedAt,
		ServerID:   data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ServerStatusEntity{}, err
	}

	return entity, nil
}

type UpdateServerStatusData struct {
	ID         int32
	UpdatedAt  time.Time
	State      string
	ObservedAt time.Time
	ServerID   uuid.UUID
}

func (ss serverStatus) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateServerStatusData,
) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		ID:         data.ID,
		UpdatedAt:  time.Now(),
		State:      data.State,
		ObservedAt: data.ObservedAt,
		ServerID:   data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("state").
		Column("observed_at").
		Column("server_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ServerStatusEntity{}, err
	}

	return entity, nil
}

func (ss serverStatus) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ServerStatusEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ss serverStatus) All(ctx context.Context, db storage.Executor) ([]ServerStatusEntity, error) {
	var entities []ServerStatusEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedServerStatuses struct {
	ServerStatuses []ServerStatusEntity
	TotalCount     int64
	Page           int64
	PageSize       int64
	TotalPages     int64
}

func (ss serverStatus) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedServerStatuses, error) {
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
		Model(&ServerStatusEntity{}).Count(ctx)
	if err != nil {
		return PaginatedServerStatuses{}, err
	}

	entities := make([]ServerStatusEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedServerStatuses{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedServerStatuses{
		ServerStatuses: entities,
		TotalCount:     int64(totalCount),
		Page:           page,
		PageSize:       pageSize,
		TotalPages:     totalPages,
	}, nil
}

func (ss serverStatus) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateServerStatusData,
) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		State:      data.State,
		ObservedAt: data.ObservedAt,
		ServerID:   data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("state = excluded.state").
		Set("observed_at = excluded.observed_at").
		Set("server_id = excluded.server_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ServerStatusEntity{}, err
	}

	return entity, nil
}
