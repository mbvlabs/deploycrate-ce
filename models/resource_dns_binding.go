package models

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceDNSBindingEntity struct {
	bun.BaseModel      `bun:"table:resource_dns_bindings,alias:resource_dns_bindings"`
	ID                 uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time      `bun:"created_at"`
	UpdatedAt          time.Time      `bun:"updated_at"`
	State              string         `bun:"state"`
	LastError          sql.NullString `bun:"last_error"`
	AppliedAt          sql.NullTime   `bun:"applied_at"`
	ArchivedAt         sql.NullTime   `bun:"archived_at"`
	ResourceEndpointID uuid.UUID      `bun:"resource_endpoint_id,type:uuid"`
	DNSZoneID          uuid.UUID      `bun:"dns_zone_id,type:uuid"`
}

type ResourceDNSStatusRow struct {
	BindingID      uuid.UUID      `bun:"binding_id"`
	ZoneID         uuid.UUID      `bun:"zone_id"`
	ZoneName       string         `bun:"zone_name"`
	ConnectionName string         `bun:"connection_name"`
	State          string         `bun:"state"`
	LastError      sql.NullString `bun:"last_error"`
	AppliedAt      sql.NullTime   `bun:"applied_at"`
}

func (entity *ResourceDNSBindingEntity) Validate() error {
	builder := validation.NewBuilder()
	if !slices.Contains(environmentDNSStates, entity.State) {
		builder.Add("state", "invalid", "DNS binding state is invalid")
	}
	if entity.ResourceEndpointID == uuid.Nil {
		builder.Add("resourceEndpointId", "required", "Resource endpoint is required")
	}
	if entity.DNSZoneID == uuid.Nil {
		builder.Add("dnsZoneId", "required", "DNS zone is required")
	}
	return builder.Err()
}

func (resourceDNSBinding) StatusForEndpoint(ctx context.Context, db storage.Executor, endpointID uuid.UUID) (ResourceDNSStatusRow, error) {
	var row ResourceDNSStatusRow
	err := db.NewSelect().TableExpr("resource_dns_bindings AS binding").
		ColumnExpr("binding.id AS binding_id, zone.id AS zone_id, zone.name AS zone_name, connection.name AS connection_name").
		ColumnExpr("binding.state, binding.last_error, binding.applied_at").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Where("binding.resource_endpoint_id = ?", endpointID).
		Where("binding.archived_at IS NULL").Limit(1).Scan(ctx, &row)
	return row, err
}

func (resourceDNSBinding) ActiveForEndpoint(ctx context.Context, db storage.Executor, endpointID uuid.UUID) (ResourceDNSBindingEntity, error) {
	var entity ResourceDNSBindingEntity
	err := db.NewSelect().Model(&entity).
		Where("resource_endpoint_id = ?", endpointID).
		Where("archived_at IS NULL").Limit(1).Scan(ctx)
	return entity, err
}

func (resourceDNSBinding) Create(ctx context.Context, db storage.Executor, endpointID, zoneID uuid.UUID) (ResourceDNSBindingEntity, error) {
	now := time.Now().UTC()
	entity := ResourceDNSBindingEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, State: EnvironmentDNSPending,
		ResourceEndpointID: endpointID, DNSZoneID: zoneID,
	}
	if err := validation.Validate(&entity); err != nil {
		return ResourceDNSBindingEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-dns-binding-endpoint:"+endpointID.String(), entity.ID,
		db.NewSelect().Model((*ResourceDNSBindingEntity)(nil)).Where("resource_endpoint_id = ?", endpointID),
		"resourceEndpointId", "the Resource endpoint already has an active DNS binding"); err != nil {
		return ResourceDNSBindingEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceDNSBindingEntity{}, err
	}
	return entity, nil
}

func (resourceDNSBinding) Reconfigure(ctx context.Context, db storage.Executor, id, zoneID uuid.UUID) (ResourceDNSBindingEntity, error) {
	var entity ResourceDNSBindingEntity
	err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", time.Now().UTC()).Set("dns_zone_id = ?", zoneID).
		Set("state = ?", EnvironmentDNSPending).Set("last_error = NULL").Set("applied_at = NULL").
		Where("id = ?", id).Where("archived_at IS NULL").Returning("*").Scan(ctx)
	return entity, err
}

func (resourceDNSBinding) MarkState(ctx context.Context, db storage.Executor, id uuid.UUID, state, message string, at time.Time) error {
	query := db.NewUpdate().Model((*ResourceDNSBindingEntity)(nil)).
		Set("updated_at = ?", at).Set("state = ?", state).Where("id = ?", id).Where("archived_at IS NULL")
	if message == "" {
		query = query.Set("last_error = NULL")
	} else {
		query = query.Set("last_error = ?", message)
	}
	if state == EnvironmentDNSApplied {
		query = query.Set("applied_at = ?", at)
	}
	_, err := query.Exec(ctx)
	return err
}

func (resourceDNSBinding) Archive(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().Model((*ResourceDNSBindingEntity)(nil)).
		Set("updated_at = ?", at).Set("archived_at = ?", at).Set("state = ?", EnvironmentDNSRemoving).
		Where("id = ?", id).Where("archived_at IS NULL").Exec(ctx)
	return err
}
