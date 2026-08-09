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

type EnvironmentDNSRecordEntity struct {
	bun.BaseModel           `bun:"table:environment_dns_records,alias:environment_dns_records"`
	ID                      uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt               time.Time    `bun:"created_at"`
	UpdatedAt               time.Time    `bun:"updated_at"`
	ExternalID              string       `bun:"external_id"`
	RecordType              string       `bun:"record_type"`
	Content                 string       `bun:"content"`
	ObservedName            string       `bun:"observed_name"`
	ArchivedAt              sql.NullTime `bun:"archived_at"`
	EnvironmentDNSBindingID uuid.UUID    `bun:"environment_dns_binding_id,type:uuid"`
	DNSZoneID               uuid.UUID    `bun:"dns_zone_id,type:uuid"`
}

type DNSRecordStatus struct {
	Type    string `json:"type"    bun:"record_type"`
	Name    string `json:"name"    bun:"observed_name"`
	Content string `json:"content" bun:"content"`
}

type DNSTrackedRemoval struct {
	RecordID          uuid.UUID `bun:"record_id"`
	ExternalID        string    `bun:"external_id"`
	ObservedName      string    `bun:"observed_name"`
	ZoneID            uuid.UUID `bun:"zone_id"`
	ZoneExternalID    string    `bun:"zone_external_id"`
	CredentialPayload []byte    `bun:"credential_payload"`
}

func (environmentDNSRecord) StatusForBinding(
	ctx context.Context, db storage.Executor, bindingID uuid.UUID,
) ([]DNSRecordStatus, error) {
	records := make([]DNSRecordStatus, 0)
	err := db.NewSelect().TableExpr("environment_dns_records").
		ColumnExpr("record_type, observed_name, content").
		Where("environment_dns_binding_id = ?", bindingID).
		Where("archived_at IS NULL").OrderExpr("content").Scan(ctx, &records)
	return records, err
}

func (environmentDNSRecord) TrackedRemovals(
	ctx context.Context, db storage.Executor, bindingID uuid.UUID,
) ([]DNSTrackedRemoval, error) {
	records := make([]DNSTrackedRemoval, 0)
	err := db.NewSelect().TableExpr("environment_dns_records AS record").
		ColumnExpr("record.id AS record_id, record.external_id, record.observed_name, zone.id AS zone_id, zone.external_id AS zone_external_id, credential.enc_payload AS credential_payload").
		Join("JOIN dns_zones AS zone ON zone.id = record.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Join("JOIN credentials AS credential ON credential.id = connection.credential_id").
		Where("record.environment_dns_binding_id = ?", bindingID).
		Where("record.archived_at IS NULL").OrderExpr("record.id").Scan(ctx, &records)
	return records, err
}

func (entity *EnvironmentDNSRecordEntity) Validate() error {
	entity.ExternalID = strings.TrimSpace(entity.ExternalID)
	entity.Content = strings.TrimSpace(entity.Content)
	entity.ObservedName = strings.TrimSpace(entity.ObservedName)
	builder := validation.NewBuilder()
	if entity.ID == uuid.Nil {
		builder.Add("id", "required", "DNS record ID is required")
	}
	builder.Required("externalId", entity.ExternalID)
	if entity.RecordType != "A" {
		builder.Add("recordType", "invalid", "DNS record type must be A")
	}
	builder.Required("content", entity.Content)
	builder.Required("observedName", entity.ObservedName)
	if entity.EnvironmentDNSBindingID == uuid.Nil {
		builder.Add("environmentDnsBindingId", "required", "Environment DNS binding is required")
	}
	if entity.DNSZoneID == uuid.Nil {
		builder.Add("dnsZoneId", "required", "DNS zone is required")
	}
	return builder.Err()
}

func (environmentDNSRecord) ActiveForBinding(
	ctx context.Context,
	db storage.Executor,
	bindingID uuid.UUID,
) ([]EnvironmentDNSRecordEntity, error) {
	entities := make([]EnvironmentDNSRecordEntity, 0)
	err := db.NewSelect().Model(&entities).Where("environment_dns_binding_id = ?", bindingID).
		Where("archived_at IS NULL").OrderExpr("external_id").Scan(ctx)
	return entities, err
}

type UpsertEnvironmentDNSRecordData struct {
	ExternalID              string
	Content                 string
	ObservedName            string
	EnvironmentDNSBindingID uuid.UUID
	DNSZoneID               uuid.UUID
}

func (environmentDNSRecord) Upsert(
	ctx context.Context,
	db storage.Executor,
	data UpsertEnvironmentDNSRecordData,
) (EnvironmentDNSRecordEntity, error) {
	now := time.Now().UTC()
	entity := EnvironmentDNSRecordEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ExternalID: data.ExternalID,
		RecordType: "A", Content: data.Content, ObservedName: data.ObservedName,
		EnvironmentDNSBindingID: data.EnvironmentDNSBindingID, DNSZoneID: data.DNSZoneID,
	}
	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDNSRecordEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := lockUnique(
		ctx,
		db,
		"environment-dns-record:"+entity.EnvironmentDNSBindingID.String()+":"+entity.ExternalID,
	); err != nil {
		return EnvironmentDNSRecordEntity{}, err
	}

	var existing EnvironmentDNSRecordEntity
	err := db.NewSelect().Model(&existing).
		Where("environment_dns_binding_id = ?", entity.EnvironmentDNSBindingID).
		Where("external_id = ?", entity.ExternalID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EnvironmentDNSRecordEntity{}, err
	}
	if err == nil {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
	}
	if existing.ID == uuid.Nil {
		if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
			return EnvironmentDNSRecordEntity{}, err
		}
		return entity, nil
	}
	if err := db.NewUpdate().Model(&entity).
		Column("updated_at", "record_type", "content", "observed_name", "dns_zone_id", "archived_at").
		WherePK().Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSRecordEntity{}, err
	}
	return entity, nil
}

func (environmentDNSRecord) ArchiveMissing(
	ctx context.Context,
	db storage.Executor,
	bindingID uuid.UUID,
	externalIDs []string,
	at time.Time,
) error {
	query := db.NewUpdate().Model((*EnvironmentDNSRecordEntity)(nil)).Set("updated_at = ?", at).
		Set("archived_at = ?", at).
		Where("environment_dns_binding_id = ?", bindingID).Where("archived_at IS NULL")
	if len(externalIDs) > 0 {
		query = query.Where("external_id NOT IN (?)", bun.In(externalIDs))
	}
	_, err := query.Exec(ctx)
	return err
}
