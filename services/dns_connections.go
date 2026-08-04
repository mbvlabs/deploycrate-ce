package services

import (
	"context"
	"database/sql"
	cloudflareclient "deploycrate-ce/clients/cloudflare"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const cloudflareTokenEncryptionPurpose = "cloudflare-api-token/v1"

type CloudflareDNSClient interface {
	VerifyAccountToken(context.Context, string, string) error
	ListZones(context.Context, string, string) ([]cloudflareclient.Zone, error)
	ListAddressRecords(context.Context, string, string, string) ([]cloudflareclient.DNSRecord, error)
	CreateARecord(context.Context, string, string, cloudflareclient.DNSRecordInput) (cloudflareclient.DNSRecord, error)
	UpdateARecord(context.Context, string, string, string, cloudflareclient.DNSRecordInput) (cloudflareclient.DNSRecord, error)
	DeleteRecord(context.Context, string, string, string) error
}

type DNSConnections struct {
	db     storage.Pool
	client CloudflareDNSClient
	config config.Config
}

type DNSConnectionSummary struct {
	ID           uuid.UUID    `json:"id" bun:"id"`
	Name         string       `json:"name" bun:"name"`
	Provider     string       `json:"provider" bun:"provider"`
	AccountID    string       `json:"accountId" bun:"account_external_id"`
	VerifiedAt   sql.NullTime `json:"verifiedAt" bun:"verified_at"`
	LastSyncedAt sql.NullTime `json:"lastSyncedAt" bun:"last_synced_at"`
	ArchivedAt   sql.NullTime `json:"archivedAt" bun:"archived_at"`
	ActiveZones  int          `json:"activeZones" bun:"active_zones"`
	BindingCount int          `json:"bindingCount" bun:"binding_count"`
}

func NewDNSConnections(db storage.Pool, client CloudflareDNSClient, cfg config.Config) *DNSConnections {
	return &DNSConnections{db: db, client: client, config: cfg}
}

func (service *DNSConnections) List(ctx context.Context) ([]DNSConnectionSummary, error) {
	items := make([]DNSConnectionSummary, 0)
	err := service.db.Executor().NewSelect().TableExpr("dns_connections AS connection").
		ColumnExpr("connection.id, connection.name, connection.provider, connection.account_external_id, connection.verified_at, connection.last_synced_at, connection.archived_at").
		ColumnExpr("COUNT(DISTINCT zone.id) FILTER (WHERE zone.archived_at IS NULL AND zone.status = 'active') AS active_zones").
		ColumnExpr("COUNT(DISTINCT binding.id) FILTER (WHERE binding.archived_at IS NULL) AS binding_count").
		Join("LEFT JOIN dns_zones AS zone ON zone.dns_connection_id = connection.id").
		Join("LEFT JOIN environment_dns_bindings AS binding ON binding.dns_zone_id = zone.id").
		Where("connection.archived_at IS NULL").Group("connection.id").OrderExpr("lower(connection.name)").Scan(ctx, &items)
	return items, err
}

func (service *DNSConnections) Create(ctx context.Context, name, accountID, token string) (models.DNSConnectionEntity, error) {
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	if name == "" {
		return models.DNSConnectionEntity{}, domainError("name", "required", "Connection name is required")
	}
	if token == "" {
		return models.DNSConnectionEntity{}, domainError("token", "required", "Account-owned API token is required")
	}
	accountID, err := models.NormalizeCloudflareAccountID(accountID)
	if err != nil {
		return models.DNSConnectionEntity{}, err
	}
	if err := service.client.VerifyAccountToken(ctx, accountID, token); err != nil {
		return models.DNSConnectionEntity{}, domainError("token", "unverified", "Cloudflare could not verify the account-owned API token")
	}
	zones, err := service.client.ListZones(ctx, accountID, token)
	if err != nil {
		return models.DNSConnectionEntity{}, domainError("token", "unverified", "The account-owned API token could not read Cloudflare zones")
	}
	if len(zones) == 0 {
		return models.DNSConnectionEntity{}, domainError("token", "unverified", "Account-owned API token does not expose any Cloudflare zones")
	}
	encrypted, err := secretcrypto.EncryptForPurpose([]byte(token), service.config.App.SessionEncryptionKey, cloudflareTokenEncryptionPurpose)
	if err != nil {
		return models.DNSConnectionEntity{}, fmt.Errorf("encrypt Cloudflare account-owned API token: %w", err)
	}
	metadata, _ := json.Marshal(map[string]any{
		"schema_version": models.CloudflareCredentialSchemaVersion, "credential_kind": "cloudflare_account_api_token", "account_id": accountID,
	})
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DNSConnectionEntity{}, err
	}
	defer tx.Rollback()
	credential, err := models.Credential.Create(ctx, tx, models.CreateCredentialData{
		Name: "Cloudflare DNS " + name, Provider: models.CloudflareAccountAPITokenProvider,
		Metadata: metadata, EncPayload: encrypted, VerifiedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return models.DNSConnectionEntity{}, err
	}
	connection, err := models.DNSConnection.Create(ctx, tx, models.CreateDNSConnectionData{
		Name: name, Provider: models.DNSProviderCloudflare, AccountID: accountID, CredentialID: credential.ID,
		VerifiedAt: sql.NullTime{Time: now, Valid: true}, LastSyncedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return models.DNSConnectionEntity{}, err
	}
	if err := service.persistZones(ctx, tx, connection.ID, zones, now); err != nil {
		return models.DNSConnectionEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.DNSConnectionEntity{}, err
	}
	return connection, nil
}

func (service *DNSConnections) Synchronize(ctx context.Context, id uuid.UUID) error {
	connection, token, err := service.connectionToken(ctx, service.db.Executor(), id)
	if err != nil {
		return err
	}
	if err := service.client.VerifyAccountToken(ctx, connection.AccountID, token); err != nil {
		return err
	}
	zones, err := service.client.ListZones(ctx, connection.AccountID, token)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := service.persistZones(ctx, tx, connection.ID, zones, now); err != nil {
		return err
	}
	if err := models.DNSConnection.MarkSynchronized(ctx, tx, connection.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *DNSConnections) RotateToken(ctx context.Context, id uuid.UUID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return domainError("token", "required", "Account-owned API token is required")
	}
	connection, err := models.DNSConnection.Find(ctx, service.db.Executor(), id)
	if err != nil || connection.ArchivedAt.Valid {
		return errors.New("DNS connection is unavailable")
	}
	if err := service.client.VerifyAccountToken(ctx, connection.AccountID, token); err != nil {
		return domainError("token", "unverified", "Cloudflare could not verify the account-owned API token")
	}
	zones, err := service.client.ListZones(ctx, connection.AccountID, token)
	if err != nil {
		return domainError("token", "unverified", "The account-owned API token could not read Cloudflare zones")
	}
	credential, err := models.Credential.Find(ctx, service.db.Executor(), connection.CredentialID)
	if err != nil {
		return err
	}
	encrypted, err := secretcrypto.EncryptForPurpose([]byte(token), service.config.App.SessionEncryptionKey, cloudflareTokenEncryptionPurpose)
	if err != nil {
		return fmt.Errorf("encrypt Cloudflare account-owned API token: %w", err)
	}
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := models.Credential.Update(ctx, tx, models.UpdateCredentialData{
		ID: credential.ID, Name: credential.Name, Provider: credential.Provider, Metadata: credential.Metadata,
		EncPayload: encrypted, VerifiedAt: sql.NullTime{Time: now, Valid: true},
		LastUsedAt: credential.LastUsedAt, ArchivedAt: credential.ArchivedAt,
	}); err != nil {
		return err
	}
	if err := service.persistZones(ctx, tx, connection.ID, zones, now); err != nil {
		return err
	}
	if err := models.DNSConnection.MarkSynchronized(ctx, tx, connection.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *DNSConnections) Archive(ctx context.Context, id uuid.UUID) error {
	connection, err := models.DNSConnection.Find(ctx, service.db.Executor(), id)
	if err != nil || connection.ArchivedAt.Valid {
		return errors.New("DNS connection is unavailable")
	}
	count, err := service.db.Executor().NewSelect().TableExpr("environment_dns_bindings AS binding").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Where("zone.dns_connection_id = ?", id).Where("binding.archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(models.ErrDomainValidation, errors.New("move managed Environment domains to another connection or manual DNS before archiving this connection"))
	}
	credential, err := models.Credential.Find(ctx, service.db.Executor(), connection.CredentialID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.DNSConnection.Archive(ctx, tx, id, now); err != nil {
		return err
	}
	if _, err := models.Credential.Update(ctx, tx, models.UpdateCredentialData{
		ID: credential.ID, Name: credential.Name, Provider: credential.Provider, Metadata: credential.Metadata,
		EncPayload: credential.EncPayload, VerifiedAt: credential.VerifiedAt, LastUsedAt: credential.LastUsedAt,
		ArchivedAt: sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *DNSConnections) persistZones(ctx context.Context, db storage.Executor, connectionID uuid.UUID, zones []cloudflareclient.Zone, now time.Time) error {
	externalIDs := make([]string, 0, len(zones))
	for _, zone := range zones {
		externalIDs = append(externalIDs, zone.ID)
		if _, err := models.DNSZone.Upsert(ctx, db, models.UpsertDNSZoneData{
			ExternalID: zone.ID, Name: zone.Name, Status: zone.Status,
			LastSyncedAt: now, DNSConnectionID: connectionID,
		}); err != nil {
			return err
		}
	}
	return models.DNSZone.ArchiveMissing(ctx, db, connectionID, externalIDs, now)
}

func (service *DNSConnections) connectionToken(ctx context.Context, db storage.Executor, id uuid.UUID) (models.DNSConnectionEntity, string, error) {
	connection, err := models.DNSConnection.Find(ctx, db, id)
	if err != nil || connection.ArchivedAt.Valid {
		return models.DNSConnectionEntity{}, "", errors.New("DNS connection is unavailable")
	}
	credential, err := models.Credential.Find(ctx, db, connection.CredentialID)
	if err != nil || credential.ArchivedAt.Valid || credential.Provider != models.CloudflareAccountAPITokenProvider {
		return models.DNSConnectionEntity{}, "", errors.New("Cloudflare credential is unavailable")
	}
	plaintext, err := secretcrypto.DecryptForPurpose(credential.EncPayload, service.config.App.SessionEncryptionKey, cloudflareTokenEncryptionPurpose)
	if err != nil {
		return models.DNSConnectionEntity{}, "", errors.New("Cloudflare account-owned API token could not be decrypted")
	}
	return connection, string(plaintext), nil
}
