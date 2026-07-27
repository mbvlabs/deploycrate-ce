package models

import (
	"context"
	"database/sql"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type WireGuardDeviceStatusEntity struct {
	bun.BaseModel     `bun:"table:wireguard_device_statuses,alias:wireguard_device_status"`
	ID                int32          `bun:"id,pk,autoincrement"`
	CreatedAt         time.Time      `bun:"created_at"`
	UpdatedAt         time.Time      `bun:"updated_at"`
	State             string         `bun:"state"`
	LatestHandshakeAt sql.NullTime   `bun:"latest_handshake_at"`
	ObservedAt        time.Time      `bun:"observed_at"`
	Error             sql.NullString `bun:"error"`
	WireGuardDeviceID uuid.UUID      `bun:"wireguard_device_id,type:uuid"`
}

func (wireGuardDeviceStatus) Upsert(ctx context.Context, db storage.Executor, deviceID uuid.UUID, state string, handshake sql.NullTime, operationErr error) error {
	now := time.Now()
	entity := WireGuardDeviceStatusEntity{
		CreatedAt: now, UpdatedAt: now, State: state, LatestHandshakeAt: handshake,
		ObservedAt: now, WireGuardDeviceID: deviceID,
	}
	if operationErr != nil {
		entity.Error = sql.NullString{String: operationErr.Error(), Valid: true}
	}
	result, err := db.NewUpdate().TableExpr("wireguard_device_statuses").
		Set("updated_at = ?", entity.UpdatedAt).
		Set("state = ?", entity.State).
		Set("latest_handshake_at = ?", entity.LatestHandshakeAt).
		Set("observed_at = ?", entity.ObservedAt).
		Set("error = ?", entity.Error).
		Where("wireguard_device_id = ?", deviceID).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows > 0 {
		return err
	}
	_, err = db.NewInsert().Model(&entity).Exec(ctx)
	return err
}
