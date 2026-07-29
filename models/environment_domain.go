package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentDomainEntity struct {
	bun.BaseModel `bun:"table:environment_domains,alias:environment_domains"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Hostname      string       `bun:"hostname"`
	IsPrimary     bool         `bun:"is_primary"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
}

var environmentHostnamePattern = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`)

func (e *EnvironmentDomainEntity) Validate() error {
	e.Hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.Hostname), "."))
	builder := validation.NewBuilder()
	if len(e.Hostname) > 253 || !environmentHostnamePattern.MatchString(e.Hostname) {
		builder.Add("hostname", "format", "hostname must be a valid fully qualified domain name")
	}
	if !e.IsPrimary {
		builder.Add("isPrimary", "required", "this release requires a primary Environment domain")
	}
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environmentId", "required", "Environment is required")
	}
	return builder.Err()
}

func ensureEnvironmentDomainUnique(ctx context.Context, db storage.Executor, entity EnvironmentDomainEntity) error {
	if entity.ArchivedAt.Valid {
		return nil
	}
	if err := ensureActiveUnique(
		ctx, db, "environment-domain-hostname:"+entity.Hostname, entity.ID,
		db.NewSelect().Model((*EnvironmentDomainEntity)(nil)).Where("lower(hostname) = ?", strings.ToLower(entity.Hostname)),
		"hostname", "an active Environment domain already uses this hostname",
	); err != nil {
		return err
	}
	return ensureActiveUnique(
		ctx, db, "environment-domain-primary:"+entity.EnvironmentID.String(), entity.ID,
		db.NewSelect().Model((*EnvironmentDomainEntity)(nil)).Where("environment_id = ?", entity.EnvironmentID).Where("is_primary = TRUE"),
		"isPrimary", "the Environment already has an active primary domain",
	)
}

func (ed environmentDomain) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDomainEntity, error) {
	var entity EnvironmentDomainEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}

type BootstrapGraphRecord struct {
	ApplicationID   uuid.UUID `bun:"application_id"`
	EnvironmentID   uuid.UUID `bun:"environment_id"`
	ServerID        uuid.UUID `bun:"server_id"`
	NetworkID       uuid.UUID `bun:"network_id"`
	ReleaseID       uuid.UUID `bun:"release_id"`
	InstanceID      uuid.UUID `bun:"instance_id"`
	CaddyRouteID    uuid.UUID `bun:"caddy_route_id"`
	ExternalRouteID string    `bun:"external_route_id"`
}

func (ed environmentDomain) FindBootstrapGraphByHostname(
	ctx context.Context,
	db storage.Executor,
	hostname string,
) (BootstrapGraphRecord, bool, error) {
	var row BootstrapGraphRecord
	err := db.NewSelect().
		TableExpr("caddy_routes AS route").
		ColumnExpr("environment.application_id AS application_id").
		ColumnExpr("environment.id AS environment_id").
		ColumnExpr("target.server_id AS server_id").
		ColumnExpr("network.id AS network_id").
		ColumnExpr("route.release_id AS release_id").
		ColumnExpr("backend.instance_id AS instance_id").
		ColumnExpr("route.id AS caddy_route_id").
		ColumnExpr("route.external_id AS external_route_id").
		Join("JOIN environment_domains AS domain ON domain.id = route.environment_domain_id").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL").
		Join("JOIN private_networks AS network ON network.owner_environment_id = environment.id AND network.archived_at IS NULL").
		Where("domain.hostname = ?", hostname).
		Where("domain.archived_at IS NULL").
		Where("route.removed_at IS NULL").
		OrderExpr("backend.id ASC").
		Limit(1).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return BootstrapGraphRecord{}, false, nil
	}
	if err != nil {
		return BootstrapGraphRecord{}, false, err
	}
	return row, true, nil
}

type CreateEnvironmentDomainData struct {
	Hostname      string
	IsPrimary     bool
	ArchivedAt    sql.NullTime
	EnvironmentID uuid.UUID
}

func (ed environmentDomain) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentDomainData,
) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentDomainUnique(ctx, db, entity); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentDomainData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	Hostname      string
	IsPrimary     bool
	ArchivedAt    sql.NullTime
	EnvironmentID uuid.UUID
}

func (ed environmentDomain) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentDomainData,
) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentDomainUnique(ctx, db, entity); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("hostname").
		Column("is_primary").
		Column("archived_at").
		Column("environment_id").
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

func (ed environmentDomain) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentDomainEntity, error) {
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

func (ed environmentDomain) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentDomains, error) {
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

func (ed environmentDomain) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentDomainData,
) (EnvironmentDomainEntity, error) {
	entity := EnvironmentDomainEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Hostname:      data.Hostname,
		IsPrimary:     data.IsPrimary,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDomainEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentDomainUnique(ctx, db, entity); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("hostname = excluded.hostname").
		Set("is_primary = excluded.is_primary").
		Set("archived_at = excluded.archived_at").
		Set("environment_id = excluded.environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentDomainEntity{}, err
	}

	return entity, nil
}
