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

type ServerEntity struct {
	bun.BaseModel       `bun:"table:servers,alias:servers"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ArchivedAt          sql.NullTime    `bun:"archived_at"`
	Name                string          `bun:"name"`
	Slug                string          `bun:"slug"`
	Kind                string          `bun:"kind"`
	Capabilities        json.RawMessage `bun:"capabilities,type:jsonb"`
	OperatingSystem     sql.NullString  `bun:"operating_system"`
	Distribution        sql.NullString  `bun:"distribution"`
	DistributionVersion sql.NullString  `bun:"distribution_version"`
	Architecture        sql.NullString  `bun:"architecture"`
	PackageManager      sql.NullString  `bun:"package_manager"`
	InitSystem          sql.NullString  `bun:"init_system"`
	Ipv4Address         string          `bun:"ipv4_address"`
	Ipv6Address         string          `bun:"ipv6_address"`
	IsConfigured        bool            `bun:"is_configured"`
	Address             string          `bun:"address"`
}

func (e *ServerEntity) Validate() error {
	return nil
}

func (s server) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ServerEntity, error) {
	var entity ServerEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ServerEntity{}, err
	}

	return entity, nil
}

type CreateServerData struct {
	ArchivedAt          sql.NullTime
	Name                string
	Slug                string
	Kind                string
	Capabilities        json.RawMessage
	OperatingSystem     sql.NullString
	Distribution        sql.NullString
	DistributionVersion sql.NullString
	Architecture        sql.NullString
	PackageManager      sql.NullString
	InitSystem          sql.NullString
	Ipv4Address         string
	Ipv6Address         string
	IsConfigured        bool
	Address             string
}

func (s server) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateServerData,
) (ServerEntity, error) {
	entity := ServerEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ArchivedAt:          data.ArchivedAt,
		Name:                data.Name,
		Slug:                data.Slug,
		Kind:                data.Kind,
		Capabilities:        data.Capabilities,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Ipv4Address:         data.Ipv4Address,
		Ipv6Address:         data.Ipv6Address,
		IsConfigured:        data.IsConfigured,
		Address:             data.Address,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ServerEntity{}, err
	}

	return entity, nil
}

type UpdateServerData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	ArchivedAt          sql.NullTime
	Name                string
	Slug                string
	Kind                string
	Capabilities        json.RawMessage
	OperatingSystem     sql.NullString
	Distribution        sql.NullString
	DistributionVersion sql.NullString
	Architecture        sql.NullString
	PackageManager      sql.NullString
	InitSystem          sql.NullString
	Ipv4Address         string
	Ipv6Address         string
	IsConfigured        bool
	Address             string
}

func (s server) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateServerData,
) (ServerEntity, error) {
	entity := ServerEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ArchivedAt:          data.ArchivedAt,
		Name:                data.Name,
		Slug:                data.Slug,
		Kind:                data.Kind,
		Capabilities:        data.Capabilities,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Ipv4Address:         data.Ipv4Address,
		Ipv6Address:         data.Ipv6Address,
		IsConfigured:        data.IsConfigured,
		Address:             data.Address,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("archived_at").
		Column("name").
		Column("slug").
		Column("kind").
		Column("capabilities").
		Column("operating_system").
		Column("distribution").
		Column("distribution_version").
		Column("architecture").
		Column("package_manager").
		Column("init_system").
		Column("ipv4_address").
		Column("ipv6_address").
		Column("is_configured").
		Column("address").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ServerEntity{}, err
	}

	return entity, nil
}

func (s server) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ServerEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (s server) All(ctx context.Context, db storage.Executor) ([]ServerEntity, error) {
	var entities []ServerEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedServers struct {
	Servers    []ServerEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (s server) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedServers, error) {
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
		Model(&ServerEntity{}).Count(ctx)
	if err != nil {
		return PaginatedServers{}, err
	}

	entities := make([]ServerEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedServers{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedServers{
		Servers:    entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s server) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateServerData,
) (ServerEntity, error) {
	entity := ServerEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ArchivedAt:          data.ArchivedAt,
		Name:                data.Name,
		Slug:                data.Slug,
		Kind:                data.Kind,
		Capabilities:        data.Capabilities,
		OperatingSystem:     data.OperatingSystem,
		Distribution:        data.Distribution,
		DistributionVersion: data.DistributionVersion,
		Architecture:        data.Architecture,
		PackageManager:      data.PackageManager,
		InitSystem:          data.InitSystem,
		Ipv4Address:         data.Ipv4Address,
		Ipv6Address:         data.Ipv6Address,
		IsConfigured:        data.IsConfigured,
		Address:             data.Address,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("archived_at = excluded.archived_at").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("kind = excluded.kind").
		Set("capabilities = excluded.capabilities").
		Set("operating_system = excluded.operating_system").
		Set("distribution = excluded.distribution").
		Set("distribution_version = excluded.distribution_version").
		Set("architecture = excluded.architecture").
		Set("package_manager = excluded.package_manager").
		Set("init_system = excluded.init_system").
		Set("ipv4_address = excluded.ipv4_address").
		Set("ipv6_address = excluded.ipv6_address").
		Set("is_configured = excluded.is_configured").
		Set("address = excluded.address").
		Returning("*").
		Scan(ctx); err != nil {
		return ServerEntity{}, err
	}

	return entity, nil
}
