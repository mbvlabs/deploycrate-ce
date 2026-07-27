package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type WireGuardDeviceResourceGrantEntity struct {
	bun.BaseModel     `bun:"table:wireguard_device_resource_grants,alias:wireguard_device_resource_grant"`
	ID                uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time    `bun:"created_at"`
	UpdatedAt         time.Time    `bun:"updated_at"`
	GrantedAt         time.Time    `bun:"granted_at"`
	RevokedAt         sql.NullTime `bun:"revoked_at"`
	WireGuardDeviceID uuid.UUID    `bun:"wireguard_device_id,type:uuid"`
	ResourceID        uuid.UUID    `bun:"resource_id,type:uuid"`
	GrantedByUserID   uuid.UUID    `bun:"granted_by_user_id,type:uuid"`
}

func (entity *WireGuardDeviceResourceGrantEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.GrantedAt.IsZero() {
		builder.Add("grantedAt", "required", "grant time is required")
	}
	if entity.WireGuardDeviceID == uuid.Nil {
		builder.Add("deviceId", "required", "device is required")
	}
	if entity.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "resource is required")
	}
	if entity.GrantedByUserID == uuid.Nil {
		builder.Add("grantedByUserId", "required", "granting user is required")
	}
	return builder.Err()
}

func (wireGuardDeviceResourceGrant) Create(ctx context.Context, db storage.Executor, deviceID, resourceID, userID uuid.UUID) (WireGuardDeviceResourceGrantEntity, error) {
	now := time.Now()
	entity := WireGuardDeviceResourceGrantEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, GrantedAt: now,
		WireGuardDeviceID: deviceID, ResourceID: resourceID, GrantedByUserID: userID,
	}
	if err := validation.Validate(&entity); err != nil {
		return WireGuardDeviceResourceGrantEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return WireGuardDeviceResourceGrantEntity{}, err
	}
	return entity, nil
}

func (wireGuardDeviceResourceGrant) FindActive(ctx context.Context, db storage.Executor, deviceID, resourceID uuid.UUID) (WireGuardDeviceResourceGrantEntity, error) {
	var entity WireGuardDeviceResourceGrantEntity
	err := db.NewSelect().Model(&entity).
		Where("wireguard_device_resource_grant.wireguard_device_id = ?", deviceID).
		Where("wireguard_device_resource_grant.resource_id = ?", resourceID).
		Where("wireguard_device_resource_grant.revoked_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WireGuardDeviceResourceGrantEntity{}, ErrNotFound
	}
	return entity, err
}

func (wireGuardDeviceResourceGrant) ActiveForDevice(ctx context.Context, db storage.Executor, deviceID uuid.UUID) ([]WireGuardDeviceResourceGrantEntity, error) {
	entities := make([]WireGuardDeviceResourceGrantEntity, 0)
	err := db.NewSelect().Model(&entities).
		Where("wireguard_device_resource_grant.wireguard_device_id = ?", deviceID).
		Where("wireguard_device_resource_grant.revoked_at IS NULL").
		OrderExpr("wireguard_device_resource_grant.created_at").
		Scan(ctx)
	return entities, err
}

func (wireGuardDeviceResourceGrant) Revoke(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("wireguard_device_resource_grants").
		Set("revoked_at = COALESCE(revoked_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (wireGuardDeviceResourceGrant) ActiveCountForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("wireguard_device_resource_grants").
		Where("resource_id = ?", resourceID).
		Where("revoked_at IS NULL").
		Count(ctx)
}
