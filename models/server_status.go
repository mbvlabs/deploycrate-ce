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

type ServerStatusEntity struct {
	bun.BaseModel       `bun:"table:server_statuses,alias:server_statuses"`
	ID                  int32           `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ServerID            uuid.UUID       `bun:"server_id,type:uuid"`
	State               string          `bun:"state"`
	OperatingSystem     sql.NullString  `bun:"operating_system"`
	Distribution        sql.NullString  `bun:"distribution"`
	DistributionVersion sql.NullString  `bun:"distribution_version"`
	Architecture        sql.NullString  `bun:"architecture"`
	PackageManager      sql.NullString  `bun:"package_manager"`
	InitSystem          sql.NullString  `bun:"init_system"`
	Capabilities        json.RawMessage `bun:"capabilities,type:jsonb"`
	ObservedAt          time.Time       `bun:"observed_at"`
}

func (e *ServerStatusEntity) Validate() error {
	return nil
}

func (ss serverStatus) Find(ctx context.Context, db storage.Executor, id int32) (ServerStatusEntity, error) {
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
	ServerID            uuid.UUID
	State               string
	OperatingSystem     sql.NullString
	Distribution        sql.NullString
	DistributionVersion sql.NullString
	Architecture        sql.NullString
	PackageManager      sql.NullString
	InitSystem          sql.NullString
	Capabilities        json.RawMessage
	ObservedAt          time.Time
}

func (ss serverStatus) Create(ctx context.Context, db storage.Executor, data CreateServerStatusData) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ServerID:            data.ServerID,
		State:               data.State,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Capabilities:        data.Capabilities,
		ObservedAt:          data.ObservedAt,
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
	ID                  int32
	UpdatedAt           time.Time
	ServerID            uuid.UUID
	State               string
	OperatingSystem     sql.NullString
	Distribution        sql.NullString
	DistributionVersion sql.NullString
	Architecture        sql.NullString
	PackageManager      sql.NullString
	InitSystem          sql.NullString
	Capabilities        json.RawMessage
	ObservedAt          time.Time
}

func (ss serverStatus) Update(ctx context.Context, db storage.Executor, data UpdateServerStatusData) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ServerID:            data.ServerID,
		State:               data.State,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Capabilities:        data.Capabilities,
		ObservedAt:          data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("server_id").
		Column("state").
		Column("operating_system").
		Column("distribution").
		Column("distribution_version").
		Column("architecture").
		Column("package_manager").
		Column("init_system").
		Column("capabilities").
		Column("observed_at").
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

func (ss serverStatus) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedServerStatuses, error) {
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

func (ss serverStatus) Upsert(ctx context.Context, db storage.Executor, data CreateServerStatusData) (ServerStatusEntity, error) {
	entity := ServerStatusEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ServerID:            data.ServerID,
		State:               data.State,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Capabilities:        data.Capabilities,
		ObservedAt:          data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("server_id = excluded.server_id").
		Set("state = excluded.state").
		Set("operating_system = excluded.operating_system").
		Set("distribution = excluded.distribution").
		Set("distribution_version = excluded.distribution_version").
		Set("architecture = excluded.architecture").
		Set("package_manager = excluded.package_manager").
		Set("init_system = excluded.init_system").
		Set("capabilities = excluded.capabilities").
		Set("observed_at = excluded.observed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ServerStatusEntity{}, err
	}

	return entity, nil
}
