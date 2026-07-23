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

type WireGuardPeerEntity struct {
	bun.BaseModel  `bun:"table:wireguard_peers,alias:wireguard_peers"`
	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time      `bun:"created_at"`
	UpdatedAt      time.Time      `bun:"updated_at"`
	PublicKey      string         `bun:"public_key"`
	EncPrivateKey  []byte         `bun:"enc_private_key"`
	PrivateAddress string         `bun:"private_address"`
	Endpoint       sql.NullString `bun:"endpoint"`
	ListenPort     int32          `bun:"listen_port"`
	ActivatedAt    time.Time      `bun:"activated_at"`
	RetiredAt      sql.NullTime   `bun:"retired_at"`
	ServerID       uuid.UUID      `bun:"server_id,type:uuid"`
}

func (e *WireGuardPeerEntity) Validate() error {
	return nil
}

func (wgp wireGuardPeer) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (WireGuardPeerEntity, error) {
	var entity WireGuardPeerEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return WireGuardPeerEntity{}, err
	}

	return entity, nil
}

type CreateWireGuardPeerData struct {
	PublicKey      string
	EncPrivateKey  []byte
	PrivateAddress string
	Endpoint       sql.NullString
	ListenPort     int32
	ActivatedAt    time.Time
	RetiredAt      sql.NullTime
	ServerID       uuid.UUID
}

func (wgp wireGuardPeer) Create(ctx context.Context, db storage.Executor, data CreateWireGuardPeerData) (WireGuardPeerEntity, error) {
	entity := WireGuardPeerEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		PublicKey:      data.PublicKey,
		EncPrivateKey:  data.EncPrivateKey,
		PrivateAddress: data.PrivateAddress,
		Endpoint:       data.Endpoint,
		ListenPort:     data.ListenPort,
		ActivatedAt:    data.ActivatedAt,
		RetiredAt:      data.RetiredAt,
		ServerID:       data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return WireGuardPeerEntity{}, err
	}

	return entity, nil
}

type UpdateWireGuardPeerData struct {
	ID             uuid.UUID
	UpdatedAt      time.Time
	PublicKey      string
	EncPrivateKey  []byte
	PrivateAddress string
	Endpoint       sql.NullString
	ListenPort     int32
	ActivatedAt    time.Time
	RetiredAt      sql.NullTime
	ServerID       uuid.UUID
}

func (wgp wireGuardPeer) Update(ctx context.Context, db storage.Executor, data UpdateWireGuardPeerData) (WireGuardPeerEntity, error) {
	entity := WireGuardPeerEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		PublicKey:      data.PublicKey,
		EncPrivateKey:  data.EncPrivateKey,
		PrivateAddress: data.PrivateAddress,
		Endpoint:       data.Endpoint,
		ListenPort:     data.ListenPort,
		ActivatedAt:    data.ActivatedAt,
		RetiredAt:      data.RetiredAt,
		ServerID:       data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("public_key").
		Column("enc_private_key").
		Column("private_address").
		Column("endpoint").
		Column("listen_port").
		Column("activated_at").
		Column("retired_at").
		Column("server_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return WireGuardPeerEntity{}, err
	}

	return entity, nil
}

func (wgp wireGuardPeer) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*WireGuardPeerEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (wgp wireGuardPeer) All(ctx context.Context, db storage.Executor) ([]WireGuardPeerEntity, error) {
	var entities []WireGuardPeerEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedWireGuardPeer struct {
	WireGuardPeer []WireGuardPeerEntity
	TotalCount    int64
	Page          int64
	PageSize      int64
	TotalPages    int64
}

func (wgp wireGuardPeer) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedWireGuardPeer, error) {
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
		Model(&WireGuardPeerEntity{}).Count(ctx)
	if err != nil {
		return PaginatedWireGuardPeer{}, err
	}

	entities := make([]WireGuardPeerEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedWireGuardPeer{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedWireGuardPeer{
		WireGuardPeer: entities,
		TotalCount:    int64(totalCount),
		Page:          page,
		PageSize:      pageSize,
		TotalPages:    totalPages,
	}, nil
}

func (wgp wireGuardPeer) Upsert(ctx context.Context, db storage.Executor, data CreateWireGuardPeerData) (WireGuardPeerEntity, error) {
	entity := WireGuardPeerEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		PublicKey:      data.PublicKey,
		EncPrivateKey:  data.EncPrivateKey,
		PrivateAddress: data.PrivateAddress,
		Endpoint:       data.Endpoint,
		ListenPort:     data.ListenPort,
		ActivatedAt:    data.ActivatedAt,
		RetiredAt:      data.RetiredAt,
		ServerID:       data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return WireGuardPeerEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("public_key = excluded.public_key").
		Set("enc_private_key = excluded.enc_private_key").
		Set("private_address = excluded.private_address").
		Set("endpoint = excluded.endpoint").
		Set("listen_port = excluded.listen_port").
		Set("activated_at = excluded.activated_at").
		Set("retired_at = excluded.retired_at").
		Set("server_id = excluded.server_id").
		Returning("*").
		Scan(ctx); err != nil {
		return WireGuardPeerEntity{}, err
	}

	return entity, nil
}
