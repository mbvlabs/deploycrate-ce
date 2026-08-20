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
	if e.Kind == "worker" {
		_, err := ParseServerCapabilities(e.Capabilities)
		return err
	}
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

func (server) ActiveConfigured(
	ctx context.Context,
	db storage.Executor,
) ([]ServerEntity, error) {
	servers := make([]ServerEntity, 0)
	err := db.NewSelect().
		Model(&servers).
		Where("archived_at IS NULL").
		Where("is_configured = TRUE").
		OrderExpr("created_at").
		Scan(ctx)
	return servers, err
}

func (server) ActiveWorkers(
	ctx context.Context,
	db storage.Executor,
) ([]ServerEntity, error) {
	servers := make([]ServerEntity, 0)
	err := db.NewSelect().
		Model(&servers).
		Where("kind = 'worker'").
		Where("archived_at IS NULL").
		OrderExpr("created_at DESC").
		Scan(ctx)
	return servers, err
}

func (server) ApplicationBuildOptions(
	ctx context.Context,
	db storage.Executor,
) ([]ApplicationBuildServerOption, error) {
	rows := make([]ApplicationBuildServerOption, 0)
	err := db.NewSelect().TableExpr("servers AS server").
		ColumnExpr("server.id, server.name, server.kind, server.address, COALESCE(server.architecture, '') AS architecture, server.capabilities").
		Where("server.archived_at IS NULL").Where("server.is_configured = TRUE").
		Where("server.kind IN ('self_hosted', 'worker')").
		Where("server.capabilities @> '{\"build\":true}'::jsonb").
		OrderExpr("CASE WHEN server.kind = 'self_hosted' THEN 0 ELSE 1 END, server.name").Scan(ctx, &rows)
	return rows, err
}

type ServerWireGuardMeshTarget struct {
	ServerID       uuid.UUID `bun:"server_id"`
	PrivateAddress string    `bun:"private_address"`
	SSHPort        int32     `bun:"ssh_port"`
	KnownHostKey   string    `bun:"known_host_key"`
}

func (server) ActiveWireGuardMeshTargets(
	ctx context.Context,
	db storage.Executor,
	includingServerID uuid.UUID,
) ([]ServerWireGuardMeshTarget, error) {
	targets := make([]ServerWireGuardMeshTarget, 0)
	err := db.NewSelect().
		TableExpr("servers AS server").
		ColumnExpr("server.id AS server_id, host(peer.private_address) AS private_address, credential.port AS ssh_port, credential.known_host_key").
		Join("JOIN wireguard_peers AS peer ON peer.server_id = server.id AND peer.retired_at IS NULL").
		Join("JOIN server_ssh_credentials AS credential ON credential.server_id = server.id").
		Where("server.kind = 'worker'").
		Where("server.archived_at IS NULL").
		Where("credential.host_key_confirmed_at IS NOT NULL").
		Where("(server.is_configured = TRUE OR server.id = ?)", includingServerID).
		OrderExpr("peer.private_address").
		Scan(ctx, &targets)
	return targets, err
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

func (s server) UpdateCapabilities(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	capabilities json.RawMessage,
) error {
	_, err := db.NewUpdate().
		Model((*ServerEntity)(nil)).
		Set("capabilities = ?", capabilities).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (server) UpdateIPv4Address(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	address string,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("servers").
		Set("ipv4_address = ?", address).Set("updated_at = ?", at).
		Where("id = ?", id).Exec(ctx)
	return err
}

func (s server) UpdateActiveSelfHostedCapabilitiesBySlug(
	ctx context.Context,
	db storage.Executor,
	slug string,
	capabilities json.RawMessage,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("servers").
		Set("capabilities = ?", capabilities).
		Set("updated_at = ?", at).
		Where("slug = ?", slug).
		Where("kind = 'self_hosted'").
		Where("archived_at IS NULL").
		Exec(ctx)
	return err
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
