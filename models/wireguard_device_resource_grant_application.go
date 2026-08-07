package models

import (
	"context"
	"database/sql"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type WireGuardDeviceResourceGrantApplicationEntity struct {
	bun.BaseModel      `bun:"table:wireguard_device_resource_grant_applications,alias:grant_application"`
	ID                 uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time      `bun:"created_at"`
	UpdatedAt          time.Time      `bun:"updated_at"`
	Driver             string         `bun:"driver"`
	ExternalID         sql.NullString `bun:"external_id"`
	State              string         `bun:"state"`
	AppliedAt          sql.NullTime   `bun:"applied_at"`
	ObservedAt         sql.NullTime   `bun:"observed_at"`
	Error              sql.NullString `bun:"error"`
	GrantID            uuid.UUID      `bun:"wireguard_device_resource_grant_id,type:uuid"`
	ResourceEndpointID uuid.UUID      `bun:"resource_endpoint_id,type:uuid"`
	ServerID           uuid.UUID      `bun:"server_id,type:uuid"`
}

func (wireGuardGrantApplication) CreatePending(
	ctx context.Context,
	db storage.Executor,
	grantID, endpointID, serverID uuid.UUID,
) (WireGuardDeviceResourceGrantApplicationEntity, error) {
	now := time.Now()
	entity := WireGuardDeviceResourceGrantApplicationEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Driver: "wireguard-systemd-ufw",
		ExternalID: sql.NullString{String: grantID.String(), Valid: true},
		State:      "pending", GrantID: grantID, ResourceEndpointID: endpointID, ServerID: serverID,
	}
	_, err := db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

func (wireGuardGrantApplication) Mark(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	state string,
	operationErr error,
) error {
	now := time.Now()
	query := db.NewUpdate().TableExpr("wireguard_device_resource_grant_applications").
		Set("state = ?", state).
		Set("updated_at = ?", now).
		Set("observed_at = ?", now).
		Where("id = ?", id)
	if state == "applied" || state == "removed" {
		query = query.Set("applied_at = COALESCE(applied_at, ?)", now).Set("error = NULL")
	} else if state == "pending" {
		query = query.Set("error = NULL")
	} else if operationErr != nil {
		query = query.Set("error = ?", operationErr.Error())
	}
	_, err := query.Exec(ctx)
	return err
}

func (wireGuardGrantApplication) FindByGrant(
	ctx context.Context,
	db storage.Executor,
	grantID uuid.UUID,
) (WireGuardDeviceResourceGrantApplicationEntity, error) {
	var entity WireGuardDeviceResourceGrantApplicationEntity
	err := db.NewSelect().Model(&entity).
		Where("grant_application.wireguard_device_resource_grant_id = ?", grantID).
		Scan(ctx)
	return entity, err
}
