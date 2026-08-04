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

type DNSZoneEntity struct {
	bun.BaseModel   `bun:"table:dns_zones,alias:dns_zones"`
	ID              uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt       time.Time    `bun:"created_at"`
	UpdatedAt       time.Time    `bun:"updated_at"`
	ExternalID      string       `bun:"external_id"`
	Name            string       `bun:"name"`
	Status          string       `bun:"status"`
	LastSyncedAt    time.Time    `bun:"last_synced_at"`
	ArchivedAt      sql.NullTime `bun:"archived_at"`
	DNSConnectionID uuid.UUID    `bun:"dns_connection_id,type:uuid"`
}

func (entity *DNSZoneEntity) Validate() error {
	entity.ExternalID = strings.TrimSpace(entity.ExternalID)
	entity.Name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entity.Name), "."))
	entity.Status = strings.TrimSpace(entity.Status)
	builder := validation.NewBuilder()
	builder.Required("externalId", entity.ExternalID)
	builder.Required("name", entity.Name)
	builder.Required("status", entity.Status)
	if entity.DNSConnectionID == uuid.Nil {
		builder.Add("dnsConnectionId", "required", "DNS connection is required")
	}
	return builder.Err()
}

type UpsertDNSZoneData struct {
	ExternalID      string
	Name            string
	Status          string
	LastSyncedAt    time.Time
	DNSConnectionID uuid.UUID
}

func (dnsZone) Upsert(ctx context.Context, db storage.Executor, data UpsertDNSZoneData) (DNSZoneEntity, error) {
	now := time.Now().UTC()
	entity := DNSZoneEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ExternalID: data.ExternalID,
		Name: data.Name, Status: data.Status, LastSyncedAt: data.LastSyncedAt,
		DNSConnectionID: data.DNSConnectionID,
	}
	if err := validation.Validate(&entity); err != nil {
		return DNSZoneEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := db.NewInsert().Model(&entity).
		On("CONFLICT (dns_connection_id, external_id) DO UPDATE").
		Set("updated_at = excluded.updated_at").Set("name = excluded.name").
		Set("status = excluded.status").Set("last_synced_at = excluded.last_synced_at").
		Set("archived_at = NULL").Returning("*").Scan(ctx); err != nil {
		return DNSZoneEntity{}, err
	}
	return entity, nil
}

func (dnsZone) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (DNSZoneEntity, error) {
	var entity DNSZoneEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return DNSZoneEntity{}, err
	}
	return entity, nil
}

func (dnsZone) Active(ctx context.Context, db storage.Executor) ([]DNSZoneEntity, error) {
	entities := make([]DNSZoneEntity, 0)
	err := db.NewSelect().Model(&entities).Where("archived_at IS NULL").Where("status = 'active'").OrderExpr("name").Scan(ctx)
	return entities, err
}

func (dnsZone) ArchiveMissing(ctx context.Context, db storage.Executor, connectionID uuid.UUID, externalIDs []string, at time.Time) error {
	query := db.NewUpdate().Model((*DNSZoneEntity)(nil)).Set("updated_at = ?", at).Set("archived_at = ?", at).
		Where("dns_connection_id = ?", connectionID).Where("archived_at IS NULL")
	if len(externalIDs) > 0 {
		query = query.Where("external_id NOT IN (?)", bun.In(externalIDs))
	}
	_, err := query.Exec(ctx)
	return err
}
