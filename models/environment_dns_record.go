package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
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

func (environmentDNSRecord) ActiveForBinding(ctx context.Context, db storage.Executor, bindingID uuid.UUID) ([]EnvironmentDNSRecordEntity, error) {
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

func (environmentDNSRecord) Upsert(ctx context.Context, db storage.Executor, data UpsertEnvironmentDNSRecordData) (EnvironmentDNSRecordEntity, error) {
	now := time.Now().UTC()
	entity := EnvironmentDNSRecordEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ExternalID: data.ExternalID,
		RecordType: "A", Content: data.Content, ObservedName: data.ObservedName,
		EnvironmentDNSBindingID: data.EnvironmentDNSBindingID, DNSZoneID: data.DNSZoneID,
	}
	if err := db.NewInsert().Model(&entity).
		On("CONFLICT (environment_dns_binding_id, external_id) DO UPDATE").
		Set("updated_at = excluded.updated_at").Set("record_type = excluded.record_type").
		Set("content = excluded.content").Set("observed_name = excluded.observed_name").
		Set("dns_zone_id = excluded.dns_zone_id").
		Set("archived_at = NULL").Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSRecordEntity{}, err
	}
	return entity, nil
}

func (environmentDNSRecord) ArchiveMissing(ctx context.Context, db storage.Executor, bindingID uuid.UUID, externalIDs []string, at time.Time) error {
	query := db.NewUpdate().Model((*EnvironmentDNSRecordEntity)(nil)).Set("updated_at = ?", at).
		Set("archived_at = ?", at).Where("environment_dns_binding_id = ?", bindingID).Where("archived_at IS NULL")
	if len(externalIDs) > 0 {
		query = query.Where("external_id NOT IN (?)", bun.In(externalIDs))
	}
	_, err := query.Exec(ctx)
	return err
}
