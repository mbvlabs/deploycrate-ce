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
	AttachedAt    time.Time    `bun:"attached_at"`
	DetachedAt    sql.NullTime `bun:"detached_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
	ServerID      uuid.UUID    `bun:"server_id,type:uuid"`
}

type DNSServerAddress struct {
	ID   uuid.UUID `bun:"id"`
	Kind string    `bun:"kind"`
	IPv4 string    `bun:"ipv4_address"`
	Addr string    `bun:"address"`
}

func (environmentTarget) ActiveDNSServerAddresses(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) ([]DNSServerAddress, error) {
	servers := make([]DNSServerAddress, 0)
	err := db.NewSelect().TableExpr("environment_targets AS target").
		ColumnExpr("server.id, server.kind, server.ipv4_address, server.address").
		Join("JOIN servers AS server ON server.id = target.server_id AND server.archived_at IS NULL").
		Where("target.environment_id = ?", environmentID).
		Where("target.detached_at IS NULL").OrderExpr("server.ipv4_address").Scan(ctx, &servers)
	return servers, err
}

func (e *EnvironmentTargetEntity) Validate() error {
	return nil
}

func ensureEnvironmentTargetUnique(
	ctx context.Context,
	db storage.Executor,
	entity EnvironmentTargetEntity,
) error {
	if entity.DetachedAt.Valid {
		return nil
	}
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return errors.New("active Environment target uniqueness checks require a transaction")
	}
	lockKey := "environment-target:" + entity.EnvironmentID.String() + ":" + entity.ServerID.String()
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		lockKey,
	); err != nil {
		return err
	}
	count, err := db.NewSelect().Model((*EnvironmentTargetEntity)(nil)).
		Where("environment_id = ?", entity.EnvironmentID).Where("server_id = ?", entity.ServerID).
		Where("detached_at IS NULL").Where("id <> ?", entity.ID).Count(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverId",
					Code:    "taken",
					Message: "the Environment already has an active target on this Server",
				},
			},
		)
	}
	return nil
}

func ensureEnvironmentTargetRuntime(
	ctx context.Context,
	db storage.Executor,
	entity EnvironmentTargetEntity,
) error {
	if entity.DetachedAt.Valid {
		return nil
	}
	server, err := Server.Find(ctx, db, entity.ServerID)
	if err != nil || server.ArchivedAt.Valid || !server.IsConfigured ||
		(server.Kind != "self_hosted" && server.Kind != "worker") {
		return errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{Field: "serverId", Code: "unavailable", Message: "runtime Server is unavailable"},
			},
		)
	}
	capabilities, err := ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Runtime {
		return errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverId",
					Code:    "capability",
					Message: "runtime Server does not support application workloads",
				},
			},
		)
	}
	return nil
}

func (et environmentTarget) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentTargetEntity, error) {
	var entity EnvironmentTargetEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}

func (et environmentTarget) ActiveForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (EnvironmentTargetEntity, error) {
	var entity EnvironmentTargetEntity
	err := db.NewSelect().Model(&entity).Where("environment_id = ?", environmentID).
		Where("detached_at IS NULL").OrderExpr("attached_at DESC").Limit(1).Scan(ctx)
	return entity, err
}

func (et environmentTarget) ActiveForEnvironmentAll(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) ([]EnvironmentTargetEntity, error) {
	entities := make([]EnvironmentTargetEntity, 0)
	err := db.NewSelect().Model(&entities).Where("environment_id = ?", environmentID).
		Where("detached_at IS NULL").OrderExpr("attached_at, id").Scan(ctx)
	return entities, err
}

type CreateEnvironmentTargetData struct {
	AttachedAt    time.Time
	DetachedAt    sql.NullTime
	EnvironmentID uuid.UUID
	ServerID      uuid.UUID
}

func (et environmentTarget) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentTargetData,
) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentTargetRuntime(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}
	if err := ensureEnvironmentTargetUnique(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentTargetData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	AttachedAt    time.Time
	DetachedAt    sql.NullTime
	EnvironmentID uuid.UUID
	ServerID      uuid.UUID
}

func (et environmentTarget) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentTargetData,
) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentTargetRuntime(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}
	if err := ensureEnvironmentTargetUnique(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("attached_at").
		Column("detached_at").
		Column("environment_id").
		Column("server_id").
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

func (et environmentTarget) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentTargetEntity, error) {
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

func (et environmentTarget) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentTargets, error) {
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

func (et environmentTarget) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentTargetData,
) (EnvironmentTargetEntity, error) {
	entity := EnvironmentTargetEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		AttachedAt:    data.AttachedAt,
		DetachedAt:    data.DetachedAt,
		EnvironmentID: data.EnvironmentID,
		ServerID:      data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureEnvironmentTargetRuntime(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}
	if err := ensureEnvironmentTargetUnique(ctx, db, entity); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("attached_at = excluded.attached_at").
		Set("detached_at = excluded.detached_at").
		Set("environment_id = excluded.environment_id").
		Set("server_id = excluded.server_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetEntity{}, err
	}

	return entity, nil
}
