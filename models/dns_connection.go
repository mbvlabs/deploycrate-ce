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

const (
	DNSProviderCloudflare             = "cloudflare"
	CloudflareAccountAPITokenProvider = "cloudflare_account_api_token"
	CloudflareCredentialSchemaVersion = 2
)

var cloudflareAccountIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func NormalizeCloudflareAccountID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !cloudflareAccountIDPattern.MatchString(value) {
		return "", errors.Join(ErrDomainValidation, validation.ValidationErrors{
			{
				Field:   "accountId",
				Code:    "format",
				Message: "Cloudflare account ID must be a 32-character hexadecimal ID",
			},
		})
	}
	return value, nil
}

type DNSConnectionEntity struct {
	bun.BaseModel `bun:"table:dns_connections,alias:dns_connections"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Name          string       `bun:"name"`
	Provider      string       `bun:"provider"`
	AccountID     string       `bun:"account_external_id"`
	VerifiedAt    sql.NullTime `bun:"verified_at"`
	LastSyncedAt  sql.NullTime `bun:"last_synced_at"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	CredentialID  uuid.UUID    `bun:"credential_id,type:uuid"`
}

func (entity *DNSConnectionEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.AccountID = strings.ToLower(strings.TrimSpace(entity.AccountID))
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	if entity.Provider != DNSProviderCloudflare {
		builder.Add("provider", "unsupported", "DNS provider must be Cloudflare")
	}
	if !cloudflareAccountIDPattern.MatchString(entity.AccountID) {
		builder.Add(
			"accountId",
			"format",
			"Cloudflare account ID must be a 32-character hexadecimal ID",
		)
	}
	if entity.CredentialID == uuid.Nil {
		builder.Add("credentialId", "required", "credential is required")
	}
	return builder.Err()
}

type CreateDNSConnectionData struct {
	Name         string
	Provider     string
	AccountID    string
	VerifiedAt   sql.NullTime
	LastSyncedAt sql.NullTime
	CredentialID uuid.UUID
}

func (dnsConnection) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateDNSConnectionData,
) (DNSConnectionEntity, error) {
	now := time.Now().UTC()
	entity := DNSConnectionEntity{
		ID:           uuid.New(),
		CreatedAt:    now,
		UpdatedAt:    now,
		Name:         data.Name,
		Provider:     data.Provider,
		AccountID:    data.AccountID,
		VerifiedAt:   data.VerifiedAt,
		LastSyncedAt: data.LastSyncedAt,
		CredentialID: data.CredentialID,
	}
	if err := validation.Validate(&entity); err != nil {
		return DNSConnectionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"dns-connection-name:"+strings.ToLower(entity.Name),
		entity.ID,
		db.NewSelect().
			Model((*DNSConnectionEntity)(nil)).
			Where("lower(name) = ?", strings.ToLower(entity.Name)),
		"name",
		"an active DNS connection already uses this name",
	); err != nil {
		return DNSConnectionEntity{}, err
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"dns-connection-credential:"+entity.CredentialID.String(),
		entity.ID,
		db.NewSelect().
			Model((*DNSConnectionEntity)(nil)).
			Where("credential_id = ?", entity.CredentialID),
		"credentialId",
		"an active DNS connection already uses this credential",
	); err != nil {
		return DNSConnectionEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DNSConnectionEntity{}, err
	}
	return entity, nil
}

func (dnsConnection) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (DNSConnectionEntity, error) {
	var entity DNSConnectionEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return DNSConnectionEntity{}, err
	}
	return entity, nil
}

func (dnsConnection) Active(
	ctx context.Context,
	db storage.Executor,
) ([]DNSConnectionEntity, error) {
	entities := make([]DNSConnectionEntity, 0)
	err := db.NewSelect().
		Model(&entities).
		Where("archived_at IS NULL").
		OrderExpr("lower(name)").
		Scan(ctx)
	return entities, err
}

func (dnsConnection) MarkSynchronized(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().Model((*DNSConnectionEntity)(nil)).
		Set("updated_at = ?", at).Set("verified_at = ?", at).Set("last_synced_at = ?", at).
		Where("id = ?", id).Where("archived_at IS NULL").Exec(ctx)
	return err
}

func (dnsConnection) Archive(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().Model((*DNSConnectionEntity)(nil)).
		Set("updated_at = ?", at).Set("archived_at = ?", at).
		Where("id = ?", id).Where("archived_at IS NULL").Exec(ctx)
	return err
}
