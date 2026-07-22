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

type WireGuardPeerStatusEntity struct {
	bun.BaseModel     `bun:"table:wireguard_peer_statuses,alias:wireguard_peer_statuses"`
	ID                uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time      `bun:"created_at"`
	UpdatedAt         time.Time      `bun:"updated_at"`
	WireguardPeerID   uuid.UUID      `bun:"wireguard_peer_id,type:uuid"`
	State             string         `bun:"state"`
	LatestHandshakeAt sql.NullTime   `bun:"latest_handshake_at"`
	Error             sql.NullString `bun:"error"`
	ObservedAt        time.Time      `bun:"observed_at"`
}

func (e *WireGuardPeerStatusEntity) Validate() error {
	return nil
}

func (wgps wireGuardPeerStatus) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (WireGuardPeerStatusEntity, error) {
	var entity WireGuardPeerStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return WireGuardPeerStatusEntity{}, err
	}

	return entity, nil
}

type CreateWireGuardPeerStatusData struct {
	WireguardPeerID   uuid.UUID
	State             string
	LatestHandshakeAt sql.NullTime
	Error             sql.NullString
	ObservedAt        time.Time
}

func (wgps wireGuardPeerStatus) Create(ctx context.Context, db storage.Executor, data CreateWireGuardPeerStatusData) (WireGuardPeerStatusEntity, error) {
	entity := WireGuardPeerStatusEntity{
		ID:                uuid.New(),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		WireguardPeerID:   data.WireguardPeerID,
		State:             data.State,
		LatestHandshakeAt: data.LatestHandshakeAt,
		Error:             data.Error,
		ObservedAt:        data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return WireGuardPeerStatusEntity{}, err
	}

	return entity, nil
}

type UpdateWireGuardPeerStatusData struct {
	ID                uuid.UUID
	UpdatedAt         time.Time
	WireguardPeerID   uuid.UUID
	State             string
	LatestHandshakeAt sql.NullTime
	Error             sql.NullString
	ObservedAt        time.Time
}

func (wgps wireGuardPeerStatus) Update(ctx context.Context, db storage.Executor, data UpdateWireGuardPeerStatusData) (WireGuardPeerStatusEntity, error) {
	entity := WireGuardPeerStatusEntity{
		ID:                data.ID,
		UpdatedAt:         time.Now(),
		WireguardPeerID:   data.WireguardPeerID,
		State:             data.State,
		LatestHandshakeAt: data.LatestHandshakeAt,
		Error:             data.Error,
		ObservedAt:        data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("wireguard_peer_id").
		Column("state").
		Column("latest_handshake_at").
		Column("error").
		Column("observed_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return WireGuardPeerStatusEntity{}, err
	}

	return entity, nil
}

func (wgps wireGuardPeerStatus) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*WireGuardPeerStatusEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (wgps wireGuardPeerStatus) All(ctx context.Context, db storage.Executor) ([]WireGuardPeerStatusEntity, error) {
	var entities []WireGuardPeerStatusEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedWireGuardPeerStatuses struct {
	WireGuardPeerStatuses []WireGuardPeerStatusEntity
	TotalCount            int64
	Page                  int64
	PageSize              int64
	TotalPages            int64
}

func (wgps wireGuardPeerStatus) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedWireGuardPeerStatuses, error) {
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
		Model(&WireGuardPeerStatusEntity{}).Count(ctx)
	if err != nil {
		return PaginatedWireGuardPeerStatuses{}, err
	}

	entities := make([]WireGuardPeerStatusEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedWireGuardPeerStatuses{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedWireGuardPeerStatuses{
		WireGuardPeerStatuses: entities,
		TotalCount:            int64(totalCount),
		Page:                  page,
		PageSize:              pageSize,
		TotalPages:            totalPages,
	}, nil
}

func (wgps wireGuardPeerStatus) Upsert(ctx context.Context, db storage.Executor, data CreateWireGuardPeerStatusData) (WireGuardPeerStatusEntity, error) {
	entity := WireGuardPeerStatusEntity{
		ID:                uuid.New(),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		WireguardPeerID:   data.WireguardPeerID,
		State:             data.State,
		LatestHandshakeAt: data.LatestHandshakeAt,
		Error:             data.Error,
		ObservedAt:        data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("wireguard_peer_id = excluded.wireguard_peer_id").
		Set("state = excluded.state").
		Set("latest_handshake_at = excluded.latest_handshake_at").
		Set("error = excluded.error").
		Set("observed_at = excluded.observed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return WireGuardPeerStatusEntity{}, err
	}

	return entity, nil
}
