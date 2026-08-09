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
	internalwireguard "deploycrate-ce/internal/wireguard"

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
	network := netip.MustParsePrefix(internalwireguard.DeviceCIDR)
	if err != nil || !address.Is4() || !network.Contains(address) || address == network.Addr() {
		errs = append(
			errs,
			errors.New("private address must be an allocatable host in the WireGuard device pool"),
		)
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

func (wireGuardDevice) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateWireGuardDeviceData,
) (WireGuardDeviceEntity, error) {
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

func (wireGuardDevice) FindActiveOwned(
	ctx context.Context,
	db storage.Executor,
	id, ownerUserID uuid.UUID,
) (WireGuardDeviceEntity, error) {
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

func (wireGuardDevice) FindActive(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (WireGuardDeviceEntity, error) {
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

func (wireGuardDevice) ActiveForResource(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) ([]WireGuardDeviceEntity, error) {
	entities := make([]WireGuardDeviceEntity, 0)
	err := db.NewSelect().Model(&entities).
		Join("JOIN wireguard_device_resource_grants AS resource_grant ON resource_grant.wireguard_device_id = wireguard_device.id AND resource_grant.revoked_at IS NULL").
		Where("resource_grant.resource_id = ?", resourceID).
		Where("wireguard_device.revoked_at IS NULL").
		OrderExpr("wireguard_device.created_at").
		Scan(ctx)
	return entities, err
}

func (wireGuardDevice) PrivateAccessDetails(
	ctx context.Context,
	db storage.Executor,
	resourceID, currentUserID uuid.UUID,
) (ResourcePrivateAccessDetails, error) {
	detail := ResourcePrivateAccessDetails{
		DeviceGrants:     make([]SystemWireGuardDeviceGrant, 0),
		AvailableDevices: make([]SystemWireGuardDeviceOption, 0),
	}
	if err := db.NewSelect().TableExpr("wireguard_device_resource_grants AS resource_grant").
		ColumnExpr("device.id::text AS device_id, device.name AS device_name, owner.email AS owner_email, device.private_address::text AS private_address, resource_grant.id::text AS grant_id, resource_grant.granted_at, COALESCE(application.state, 'pending') AS application_state, COALESCE(application.error, '') AS application_error, status.latest_handshake_at, status.observed_at").
		Join("JOIN wireguard_devices AS device ON device.id = resource_grant.wireguard_device_id AND device.revoked_at IS NULL").
		Join("JOIN users AS owner ON owner.id = device.owner_user_id").
		Join("LEFT JOIN wireguard_device_resource_grant_applications AS application ON application.wireguard_device_resource_grant_id = resource_grant.id").
		Join("LEFT JOIN wireguard_device_statuses AS status ON status.wireguard_device_id = device.id").
		Where("resource_grant.resource_id = ?", resourceID).
		Where("resource_grant.revoked_at IS NULL").OrderExpr("device.name").
		Scan(ctx, &detail.DeviceGrants); err != nil {
		return ResourcePrivateAccessDetails{}, err
	}
	if err := db.NewSelect().TableExpr("wireguard_devices AS device").
		ColumnExpr("device.id::text AS id, device.name, device.private_address::text AS private_address").
		Where("device.owner_user_id = ?", currentUserID).
		Where("device.revoked_at IS NULL").
		Where("NOT EXISTS (SELECT 1 FROM wireguard_device_resource_grants resource_grant WHERE resource_grant.wireguard_device_id = device.id AND resource_grant.resource_id = ? AND resource_grant.revoked_at IS NULL)", resourceID).
		OrderExpr("device.name").Scan(ctx, &detail.AvailableDevices); err != nil {
		return ResourcePrivateAccessDetails{}, err
	}
	return detail, nil
}

func (wireGuardDevice) Revoke(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("wireguard_devices").
		Set("revoked_at = COALESCE(revoked_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
