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

type DNSConnectionSummary struct {
	ID           uuid.UUID    `json:"id"           bun:"id"`
	Name         string       `json:"name"         bun:"name"`
	Provider     string       `json:"provider"     bun:"provider"`
	AccountID    string       `json:"accountId"    bun:"account_external_id"`
	VerifiedAt   sql.NullTime `json:"verifiedAt"   bun:"verified_at"`
	LastSyncedAt sql.NullTime `json:"lastSyncedAt" bun:"last_synced_at"`
	ArchivedAt   sql.NullTime `json:"archivedAt"   bun:"archived_at"`
	ActiveZones  int          `json:"activeZones"  bun:"active_zones"`
	BindingCount int          `json:"bindingCount" bun:"binding_count"`
}

func dnsConnectionSummaryQuery(db storage.Executor) *bun.SelectQuery {
	return db.NewSelect().
		TableExpr("dns_connections AS connection").
		ColumnExpr("connection.id, connection.name, connection.provider, connection.account_external_id, connection.verified_at, connection.last_synced_at, connection.archived_at").
		ColumnExpr("COUNT(DISTINCT zone.id) FILTER (WHERE zone.archived_at IS NULL AND zone.status = 'active') AS active_zones").
		ColumnExpr("COUNT(DISTINCT binding.id) FILTER (WHERE binding.archived_at IS NULL) AS binding_count").
		Join("LEFT JOIN dns_zones AS zone ON zone.dns_connection_id = connection.id").
		Join("LEFT JOIN environment_dns_bindings AS binding ON binding.dns_zone_id = zone.id").
		Where("connection.archived_at IS NULL")
}

func (dnsConnection) Summaries(
	ctx context.Context,
	db storage.Executor,
) ([]DNSConnectionSummary, error) {
	items := make([]DNSConnectionSummary, 0)
	err := dnsConnectionSummaryQuery(db).
		Group("connection.id").
		OrderExpr("lower(connection.name)").
		Scan(ctx, &items)
	return items, err
}

func (dnsConnection) Summary(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (DNSConnectionSummary, error) {
	var item DNSConnectionSummary
	err := dnsConnectionSummaryQuery(db).
		Where("connection.id = ?", id).
		Group("connection.id").
		Scan(ctx, &item)
	return item, err
}

func (dnsConnection) ActiveBindingCount(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (int, error) {
	return db.NewSelect().
		TableExpr("environment_dns_bindings AS binding").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Where("zone.dns_connection_id = ?", id).
		Where("binding.archived_at IS NULL").
		Count(ctx)
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
