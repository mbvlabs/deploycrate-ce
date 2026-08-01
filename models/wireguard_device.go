package models

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type WireGuardDeviceEntity struct {
	bun.BaseModel  `bun:"table:wireguard_devices,alias:wireguard_device"`
	ID             uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time    `bun:"created_at"`
	UpdatedAt      time.Time    `bun:"updated_at"`
	Name           string       `bun:"name"`
	PublicKey      string       `bun:"public_key"`
	PrivateAddress string       `bun:"private_address"`
	ActivatedAt    time.Time    `bun:"activated_at"`
	RevokedAt      sql.NullTime `bun:"revoked_at"`
	OwnerUserID    uuid.UUID    `bun:"owner_user_id,type:uuid"`
}

func (entity *WireGuardDeviceEntity) Validate() error {
	var errs []error
	if strings.TrimSpace(entity.Name) == "" {
		errs = append(errs, errors.New("device name is required"))
	}
	if len(entity.Name) > 120 {
		errs = append(errs, errors.New("device name must be at most 120 characters"))
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(entity.PublicKey))
	if err != nil || len(key) != 32 {
		errs = append(errs, errors.New("public key must be a base64-encoded 32-byte WireGuard key"))
	}
	address, err := netip.ParseAddr(strings.TrimSpace(entity.PrivateAddress))
	network := netip.MustParsePrefix("10.99.0.0/16")
	if err != nil || !address.Is4() || !network.Contains(address) || address.String() == "10.99.0.1" || address == network.Addr() {
		errs = append(errs, errors.New("private address must be an allocatable host in 10.99.0.0/16"))
	}
	if entity.ActivatedAt.IsZero() {
		errs = append(errs, errors.New("activation time is required"))
	}
	if entity.RevokedAt.Valid && entity.RevokedAt.Time.Before(entity.ActivatedAt) {
		errs = append(errs, errors.New("revocation cannot precede activation"))
	}
	if entity.OwnerUserID == uuid.Nil {
		errs = append(errs, errors.New("device owner is required"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid WireGuard device: %w", errors.Join(errs...))
	}
	return nil
}

type CreateWireGuardDeviceData struct {
	Name           string
	PublicKey      string
	PrivateAddress string
	OwnerUserID    uuid.UUID
}

func (wireGuardDevice) Create(ctx context.Context, db storage.Executor, data CreateWireGuardDeviceData) (WireGuardDeviceEntity, error) {
	now := time.Now()
	entity := WireGuardDeviceEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ActivatedAt: now,
		Name: strings.TrimSpace(data.Name), PublicKey: strings.TrimSpace(data.PublicKey),
		PrivateAddress: strings.TrimSpace(data.PrivateAddress), OwnerUserID: data.OwnerUserID,
	}
	if err := validation.Validate(&entity); err != nil {
		return WireGuardDeviceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return WireGuardDeviceEntity{}, err
	}
	return entity, nil
}

func (wireGuardDevice) FindActiveOwned(ctx context.Context, db storage.Executor, id, ownerUserID uuid.UUID) (WireGuardDeviceEntity, error) {
	var entity WireGuardDeviceEntity
	err := db.NewSelect().Model(&entity).
		Where("wireguard_device.id = ?", id).
		Where("wireguard_device.owner_user_id = ?", ownerUserID).
		Where("wireguard_device.revoked_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WireGuardDeviceEntity{}, ErrNotFound
	}
	return entity, err
}

func (wireGuardDevice) FindActive(ctx context.Context, db storage.Executor, id uuid.UUID) (WireGuardDeviceEntity, error) {
	var entity WireGuardDeviceEntity
	err := db.NewSelect().Model(&entity).
		Where("wireguard_device.id = ?", id).
		Where("wireguard_device.revoked_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WireGuardDeviceEntity{}, ErrNotFound
	}
	return entity, err
}

func (wireGuardDevice) ActiveForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) ([]WireGuardDeviceEntity, error) {
	entities := make([]WireGuardDeviceEntity, 0)
	err := db.NewSelect().Model(&entities).
		Join("JOIN wireguard_device_resource_grants AS resource_grant ON resource_grant.wireguard_device_id = wireguard_device.id AND resource_grant.revoked_at IS NULL").
		Where("resource_grant.resource_id = ?", resourceID).
		Where("wireguard_device.revoked_at IS NULL").
		OrderExpr("wireguard_device.created_at").
		Scan(ctx)
	return entities, err
}

func (wireGuardDevice) Revoke(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("wireguard_devices").
		Set("revoked_at = COALESCE(revoked_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
