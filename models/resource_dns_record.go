package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceDNSRecordEntity struct {
	bun.BaseModel        `bun:"table:resource_dns_records,alias:resource_dns_records"`
	ID                   uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time    `bun:"created_at"`
	UpdatedAt            time.Time    `bun:"updated_at"`
	ExternalID           string       `bun:"external_id"`
	RecordType           string       `bun:"record_type"`
	Content              string       `bun:"content"`
	ObservedName         string       `bun:"observed_name"`
	ArchivedAt           sql.NullTime `bun:"archived_at"`
	ResourceDNSBindingID uuid.UUID    `bun:"resource_dns_binding_id,type:uuid"`
	DNSZoneID            uuid.UUID    `bun:"dns_zone_id,type:uuid"`
}

func (entity *ResourceDNSRecordEntity) Validate() error {
	entity.ExternalID = strings.TrimSpace(entity.ExternalID)
	entity.Content = strings.TrimSpace(entity.Content)
	entity.ObservedName = strings.TrimSpace(entity.ObservedName)
	builder := validation.NewBuilder()
	builder.Required("externalId", entity.ExternalID)
	if entity.RecordType != "A" {
		builder.Add("recordType", "invalid", "DNS record type must be A")
	}
	builder.Required("content", entity.Content)
	builder.Required("observedName", entity.ObservedName)
	if entity.ResourceDNSBindingID == uuid.Nil {
		builder.Add("resourceDnsBindingId", "required", "Resource DNS binding is required")
	}
	if entity.DNSZoneID == uuid.Nil {
		builder.Add("dnsZoneId", "required", "DNS zone is required")
	}
	return builder.Err()
}

func (resourceDNSRecord) ActiveForBinding(ctx context.Context, db storage.Executor, bindingID uuid.UUID) ([]ResourceDNSRecordEntity, error) {
	rows := make([]ResourceDNSRecordEntity, 0)
	err := db.NewSelect().Model(&rows).Where("resource_dns_binding_id = ?", bindingID).
		Where("archived_at IS NULL").OrderExpr("external_id").Scan(ctx)
	return rows, err
}

func (resourceDNSRecord) TrackedRemovals(ctx context.Context, db storage.Executor, bindingID uuid.UUID) ([]DNSTrackedRemoval, error) {
	rows := make([]DNSTrackedRemoval, 0)
	err := db.NewSelect().TableExpr("resource_dns_records AS record").
		ColumnExpr("record.id AS record_id, record.external_id, record.observed_name, zone.id AS zone_id, zone.external_id AS zone_external_id, credential.enc_payload AS credential_payload").
		Join("JOIN dns_zones AS zone ON zone.id = record.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Join("JOIN credentials AS credential ON credential.id = connection.credential_id").
		Where("record.resource_dns_binding_id = ?", bindingID).Where("record.archived_at IS NULL").
		OrderExpr("record.id").Scan(ctx, &rows)
	return rows, err
}

type UpsertResourceDNSRecordData struct {
	ExternalID           string
	Content              string
	ObservedName         string
	ResourceDNSBindingID uuid.UUID
	DNSZoneID            uuid.UUID
}

func (resourceDNSRecord) Upsert(ctx context.Context, db storage.Executor, data UpsertResourceDNSRecordData) (ResourceDNSRecordEntity, error) {
	now := time.Now().UTC()
	entity := ResourceDNSRecordEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ExternalID: data.ExternalID,
		RecordType: "A", Content: data.Content, ObservedName: data.ObservedName,
		ResourceDNSBindingID: data.ResourceDNSBindingID, DNSZoneID: data.DNSZoneID,
	}
	if err := validation.Validate(&entity); err != nil {
		return ResourceDNSRecordEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := lockUnique(ctx, db, "resource-dns-record:"+data.ResourceDNSBindingID.String()+":"+data.ExternalID); err != nil {
		return ResourceDNSRecordEntity{}, err
	}
	var existing ResourceDNSRecordEntity
	err := db.NewSelect().Model(&existing).Where("resource_dns_binding_id = ?", data.ResourceDNSBindingID).
		Where("external_id = ?", data.ExternalID).Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ResourceDNSRecordEntity{}, err
	}
	if err == nil {
		entity.ID, entity.CreatedAt = existing.ID, existing.CreatedAt
	}
	if existing.ID == uuid.Nil {
		_, err = db.NewInsert().Model(&entity).Exec(ctx)
		return entity, err
	}
	err = db.NewUpdate().Model(&entity).
		Column("updated_at", "record_type", "content", "observed_name", "dns_zone_id", "archived_at").
		WherePK().Returning("*").Scan(ctx)
	return entity, err
}

func (resourceDNSRecord) ArchiveMissing(ctx context.Context, db storage.Executor, bindingID uuid.UUID, externalIDs []string, at time.Time) error {
	query := db.NewUpdate().Model((*ResourceDNSRecordEntity)(nil)).Set("updated_at = ?", at).
		Set("archived_at = ?", at).Where("resource_dns_binding_id = ?", bindingID).Where("archived_at IS NULL")
	if len(externalIDs) > 0 {
		query = query.Where("external_id NOT IN (?)", bun.In(externalIDs))
	}
	_, err := query.Exec(ctx)
	return err
}
