package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ServerSSHCredentialEntity struct {
	bun.BaseModel           `bun:"table:server_ssh_credentials,alias:server_ssh_credentials"`
	ID                      int32        `bun:"id,pk,autoincrement"`
	CreatedAt               time.Time    `bun:"created_at"`
	UpdatedAt               time.Time    `bun:"updated_at"`
	Username                string       `bun:"username"`
	Port                    int32        `bun:"port"`
	EncPrivateKey           []byte       `bun:"enc_private_key"`
	EncPrivateKeyPassphrase []byte       `bun:"enc_private_key_passphrase"`
	KnownHostKey            string       `bun:"known_host_key"`
	HostKeyConfirmedAt      sql.NullTime `bun:"host_key_confirmed_at"`
	ServerID                uuid.UUID    `bun:"server_id,type:uuid"`
}

func (e *ServerSSHCredentialEntity) Validate() error {
	e.Username = strings.TrimSpace(e.Username)
	e.KnownHostKey = strings.TrimSpace(e.KnownHostKey)
	builder := validation.NewBuilder()
	builder.Required("username", e.Username)
	if e.Port < 1 || e.Port > 65535 {
		builder.Add("port", "range", "SSH port must be between 1 and 65535")
	}
	builder.Required("knownHostKey", e.KnownHostKey)
	if e.ServerID == uuid.Nil {
		builder.Add("serverId", "required", "Server is required")
	}
	return builder.Err()
}

func (ssshc serverSSHCredential) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (ServerSSHCredentialEntity, error) {
	var entity ServerSSHCredentialEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	return entity, nil
}

func (ssshc serverSSHCredential) FindForServer(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
) (ServerSSHCredentialEntity, error) {
	var entity ServerSSHCredentialEntity
	if err := db.NewSelect().Model(&entity).Where("server_id = ?", serverID).Scan(ctx); err != nil {
		return ServerSSHCredentialEntity{}, err
	}
	return entity, nil
}

func (ssshc serverSSHCredential) ConfirmHostKey(ctx context.Context, db storage.Executor, serverID uuid.UUID) error {
	_, err := db.NewUpdate().Model((*ServerSSHCredentialEntity)(nil)).
		Set("updated_at = ?", time.Now().UTC()).Set("host_key_confirmed_at = ?", time.Now().UTC()).
		Where("server_id = ?", serverID).Exec(ctx)
	return err
}

func (ssshc serverSSHCredential) CompleteTrustTransition(ctx context.Context, db storage.Executor, serverID uuid.UUID) error {
	_, err := db.NewUpdate().Model((*ServerSSHCredentialEntity)(nil)).
		Set("updated_at = ?", time.Now().UTC()).Set("username = 'admin'").
		Set("enc_private_key = NULL").Set("enc_private_key_passphrase = NULL").
		Where("server_id = ?", serverID).Exec(ctx)
	return err
}

type CreateServerSSHCredentialData struct {
	Username                string
	Port                    int32
	EncPrivateKey           []byte
	EncPrivateKeyPassphrase []byte
	KnownHostKey            string
	HostKeyConfirmedAt      sql.NullTime
	ServerID                uuid.UUID
}

func (ssshc serverSSHCredential) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateServerSSHCredentialData,
) (ServerSSHCredentialEntity, error) {
	entity := ServerSSHCredentialEntity{
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
		Username:                data.Username,
		Port:                    data.Port,
		EncPrivateKey:           data.EncPrivateKey,
		EncPrivateKeyPassphrase: data.EncPrivateKeyPassphrase,
		KnownHostKey:            data.KnownHostKey,
		HostKeyConfirmedAt:      data.HostKeyConfirmedAt,
		ServerID:                data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerSSHCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(
		ctx,
		db,
		"server-ssh-credential:"+entity.ServerID.String(),
		db.NewSelect().Model((*ServerSSHCredentialEntity)(nil)).Where("server_id = ?", entity.ServerID),
		"serverId",
		"the Server already has SSH credentials",
	); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	return entity, nil
}

type UpdateServerSSHCredentialData struct {
	ID                      int32
	UpdatedAt               time.Time
	Username                string
	Port                    int32
	EncPrivateKey           []byte
	EncPrivateKeyPassphrase []byte
	KnownHostKey            string
	HostKeyConfirmedAt      sql.NullTime
	ServerID                uuid.UUID
}

func (ssshc serverSSHCredential) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateServerSSHCredentialData,
) (ServerSSHCredentialEntity, error) {
	entity := ServerSSHCredentialEntity{
		ID:                      data.ID,
		UpdatedAt:               time.Now(),
		Username:                data.Username,
		Port:                    data.Port,
		EncPrivateKey:           data.EncPrivateKey,
		EncPrivateKeyPassphrase: data.EncPrivateKeyPassphrase,
		KnownHostKey:            data.KnownHostKey,
		HostKeyConfirmedAt:      data.HostKeyConfirmedAt,
		ServerID:                data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerSSHCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(
		ctx,
		db,
		"server-ssh-credential:"+entity.ServerID.String(),
		db.NewSelect().Model((*ServerSSHCredentialEntity)(nil)).
			Where("server_id = ?", entity.ServerID).
			Where("id <> ?", entity.ID),
		"serverId",
		"the Server already has SSH credentials",
	); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("username").
		Column("port").
		Column("enc_private_key").
		Column("enc_private_key_passphrase").
		Column("known_host_key").
		Column("host_key_confirmed_at").
		Column("server_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	return entity, nil
}

func (ssshc serverSSHCredential) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ServerSSHCredentialEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ssshc serverSSHCredential) All(
	ctx context.Context,
	db storage.Executor,
) ([]ServerSSHCredentialEntity, error) {
	var entities []ServerSSHCredentialEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedServerSSHCredentials struct {
	ServerSSHCredentials []ServerSSHCredentialEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (ssshc serverSSHCredential) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedServerSSHCredentials, error) {
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
		Model(&ServerSSHCredentialEntity{}).Count(ctx)
	if err != nil {
		return PaginatedServerSSHCredentials{}, err
	}

	entities := make([]ServerSSHCredentialEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedServerSSHCredentials{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedServerSSHCredentials{
		ServerSSHCredentials: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (ssshc serverSSHCredential) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateServerSSHCredentialData,
) (ServerSSHCredentialEntity, error) {
	entity := ServerSSHCredentialEntity{
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
		Username:                data.Username,
		Port:                    data.Port,
		EncPrivateKey:           data.EncPrivateKey,
		EncPrivateKeyPassphrase: data.EncPrivateKeyPassphrase,
		KnownHostKey:            data.KnownHostKey,
		HostKeyConfirmedAt:      data.HostKeyConfirmedAt,
		ServerID:                data.ServerID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ServerSSHCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := lockUnique(ctx, db, "server-ssh-credential:"+entity.ServerID.String()); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	var existing ServerSSHCredentialEntity
	err := db.NewSelect().Model(&existing).Where("server_id = ?", entity.ServerID).Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ServerSSHCredentialEntity{}, err
	}
	if err == nil {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
	}
	if existing.ID == 0 {
		if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
			return ServerSSHCredentialEntity{}, err
		}
		return entity, nil
	}
	if err := db.NewUpdate().Model(&entity).
		Column("updated_at", "username", "port", "enc_private_key", "enc_private_key_passphrase", "known_host_key", "host_key_confirmed_at", "server_id").
		WherePK().Returning("*").Scan(ctx); err != nil {
		return ServerSSHCredentialEntity{}, err
	}

	return entity, nil
}
