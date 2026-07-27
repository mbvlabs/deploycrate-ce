package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	firewallclient "deploycrate-ce/clients/firewall"
	listenerclient "deploycrate-ce/clients/resourceaccess"
	wireguardclient "deploycrate-ce/clients/wireguard"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/curve25519"
)

type ResourcePrivateAccess struct {
	db        storage.Pool
	wireguard wireguardclient.Client
	firewall  firewallclient.Client
	listener  listenerclient.Client
}

func NewResourcePrivateAccess(db storage.Pool) *ResourcePrivateAccess {
	return &ResourcePrivateAccess{
		db: db, wireguard: wireguardclient.New(), firewall: firewallclient.New(),
		listener: listenerclient.New(),
	}
}

type ResourcePrivateAccessEnrollment struct {
	DeviceID uuid.UUID
	Name     string
	UserID   uuid.UUID
}

type ResourcePrivateAccessResult struct {
	DeviceID            uuid.UUID
	GrantID             uuid.UUID
	ClientConfiguration string
}

func (service *ResourcePrivateAccess) Enroll(ctx context.Context, resourceID uuid.UUID, data ResourcePrivateAccessEnrollment) (ResourcePrivateAccessResult, error) {
	target, err := models.Application.FindSystemResourceAccessTarget(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	if target.Protocol == "" {
		return ResourcePrivateAccessResult{}, errors.New("resource has no supported WireGuard endpoint")
	}

	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var device models.WireGuardDeviceEntity
	var grant models.WireGuardDeviceResourceGrantEntity
	var application models.WireGuardDeviceResourceGrantApplicationEntity
	var privateKey string
	retrying := false
	newDevice := data.DeviceID == uuid.Nil
	if newDevice {
		if strings.TrimSpace(data.Name) == "" {
			return ResourcePrivateAccessResult{}, errors.Join(models.ErrDomainValidation, errors.New("device name is required"))
		}
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext('deploycrate-wireguard-device-address'))"); err != nil {
			return ResourcePrivateAccessResult{}, fmt.Errorf("lock WireGuard address allocation: %w", err)
		}
		allocated, err := models.WireGuardDevice.AllocatedAddresses(ctx, tx)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		address, err := NextWireGuardPrivateAddress(allocated)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		var publicKey string
		privateKey, publicKey, err = generateWireGuardKeyPair()
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		device, err = models.WireGuardDevice.Create(ctx, tx, models.CreateWireGuardDeviceData{
			Name: data.Name, PublicKey: publicKey, PrivateAddress: address, OwnerUserID: data.UserID,
		})
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
	} else {
		device, err = models.WireGuardDevice.FindActiveOwned(ctx, tx, data.DeviceID, data.UserID)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		grant, err = models.WireGuardDeviceResourceGrant.FindActive(ctx, tx, device.ID, resourceID)
		if err == nil {
			application, err = models.WireGuardGrantApplication.FindByGrant(ctx, tx, grant.ID)
			if err != nil {
				return ResourcePrivateAccessResult{}, err
			}
			retrying = true
		} else if !errors.Is(err, models.ErrNotFound) {
			return ResourcePrivateAccessResult{}, err
		}
	}

	if !retrying {
		grant, err = models.WireGuardDeviceResourceGrant.Create(ctx, tx, device.ID, resourceID, data.UserID)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		application, err = models.WireGuardGrantApplication.CreatePending(ctx, tx, grant.ID, target.WireGuardEndpointID, target.ServerID)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	committed = true

	applyErr := service.apply(ctx, device, grant, target, true)
	if applyErr != nil {
		_ = models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "failed", applyErr)
		_ = models.WireGuardDeviceStatus.Upsert(ctx, service.db.Executor(), device.ID, "error", sql.NullTime{}, applyErr)
		if newDevice {
			service.cleanupFailedEnrollment(ctx, device, grant, target)
		}
		return ResourcePrivateAccessResult{}, applyErr
	}
	if err := models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "applied", nil); err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	if err := models.WireGuardDeviceStatus.Upsert(ctx, service.db.Executor(), device.ID, "active", sql.NullTime{}, nil); err != nil {
		return ResourcePrivateAccessResult{}, err
	}

	configuration := ""
	if newDevice {
		configuration = buildClientConfiguration(privateKey, device.PrivateAddress, target.ServerPublicKey, target.ServerEndpoint)
	}
	return ResourcePrivateAccessResult{DeviceID: device.ID, GrantID: grant.ID, ClientConfiguration: configuration}, nil
}

func (service *ResourcePrivateAccess) cleanupFailedEnrollment(ctx context.Context, device models.WireGuardDeviceEntity, grant models.WireGuardDeviceResourceGrantEntity, target models.SystemResourceAccessTarget) {
	_ = service.firewall.RemoveRule(ctx, grant.ID, device.PrivateAddress, target.WireGuardPort)
	if count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(ctx, service.db.Executor(), target.ResourceID); err == nil && count == 1 {
		_ = service.listener.RemoveListener(ctx, target.ResourceID)
	}
	_ = service.wireguard.RemovePeer(ctx, device.PublicKey)
	now := time.Now()
	_ = models.WireGuardDeviceResourceGrant.Revoke(ctx, service.db.Executor(), grant.ID, now)
	_ = models.WireGuardDevice.Revoke(ctx, service.db.Executor(), device.ID, now)
}

func (service *ResourcePrivateAccess) apply(ctx context.Context, device models.WireGuardDeviceEntity, grant models.WireGuardDeviceResourceGrantEntity, target models.SystemResourceAccessTarget, applyPeer bool) error {
	if applyPeer {
		if err := service.wireguard.ApplyPeer(ctx, device.PublicKey, device.PrivateAddress); err != nil {
			return fmt.Errorf("apply WireGuard device peer: %w", err)
		}
	}
	if err := service.listener.ApplyListener(ctx, target.ResourceID, target.WireGuardAddress, target.WireGuardPort, target.OriginAddress, target.OriginPort); err != nil {
		return fmt.Errorf("apply private resource listener: %w", err)
	}
	if err := service.firewall.ApplyRule(ctx, grant.ID, device.PrivateAddress, target.WireGuardPort); err != nil {
		return fmt.Errorf("apply private resource firewall rule: %w", err)
	}
	return nil
}

func (service *ResourcePrivateAccess) RevokeGrant(ctx context.Context, resourceID, deviceID uuid.UUID) error {
	device, err := models.WireGuardDevice.FindActive(ctx, service.db.Executor(), deviceID)
	if err != nil {
		return err
	}
	grant, err := models.WireGuardDeviceResourceGrant.FindActive(ctx, service.db.Executor(), deviceID, resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	target, err := models.Application.FindSystemResourceAccessTarget(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	application, err := models.WireGuardGrantApplication.FindByGrant(ctx, service.db.Executor(), grant.ID)
	if err != nil {
		return err
	}
	if err := service.firewall.RemoveRule(ctx, grant.ID, device.PrivateAddress, target.WireGuardPort); err != nil {
		_ = models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "failed", err)
		return err
	}
	count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	if count == 1 {
		if err := service.listener.RemoveListener(ctx, resourceID); err != nil {
			_ = models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "failed", err)
			return err
		}
	}
	now := time.Now()
	if err := models.WireGuardDeviceResourceGrant.Revoke(ctx, service.db.Executor(), grant.ID, now); err != nil {
		return err
	}
	return models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "removed", nil)
}

func (service *ResourcePrivateAccess) RevokeDevice(ctx context.Context, deviceID uuid.UUID) error {
	device, err := models.WireGuardDevice.FindActive(ctx, service.db.Executor(), deviceID)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	grants, err := models.WireGuardDeviceResourceGrant.ActiveForDevice(ctx, service.db.Executor(), deviceID)
	if err != nil {
		return err
	}
	for _, grant := range grants {
		target, err := models.Application.FindSystemResourceAccessTarget(ctx, service.db.Executor(), grant.ResourceID)
		if err != nil {
			return err
		}
		if err := service.firewall.RemoveRule(ctx, grant.ID, device.PrivateAddress, target.WireGuardPort); err != nil {
			return err
		}
		count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(ctx, service.db.Executor(), grant.ResourceID)
		if err != nil {
			return err
		}
		if count == 1 {
			if err := service.listener.RemoveListener(ctx, grant.ResourceID); err != nil {
				return err
			}
		}
	}
	if err := service.wireguard.RemovePeer(ctx, device.PublicKey); err != nil {
		return err
	}
	now := time.Now()
	for _, grant := range grants {
		if err := models.WireGuardDeviceResourceGrant.Revoke(ctx, service.db.Executor(), grant.ID, now); err != nil {
			return err
		}
		application, err := models.WireGuardGrantApplication.FindByGrant(ctx, service.db.Executor(), grant.ID)
		if err == nil {
			_ = models.WireGuardGrantApplication.Mark(ctx, service.db.Executor(), application.ID, "removed", nil)
		}
	}
	if err := models.WireGuardDevice.Revoke(ctx, service.db.Executor(), deviceID, now); err != nil {
		return err
	}
	return models.WireGuardDeviceStatus.Upsert(ctx, service.db.Executor(), deviceID, "revoked", sql.NullTime{}, nil)
}

func (service *ResourcePrivateAccess) ObserveResource(ctx context.Context, resourceID uuid.UUID) error {
	devices, err := models.WireGuardDevice.ActiveForResource(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	var observationErr error
	for _, device := range devices {
		handshake, present, err := service.wireguard.LatestHandshake(ctx, device.PublicKey)
		if err != nil {
			observationErr = errors.Join(observationErr, err)
			_ = models.WireGuardDeviceStatus.Upsert(ctx, service.db.Executor(), device.ID, "error", sql.NullTime{}, err)
			continue
		}
		observedHandshake := sql.NullTime{Time: handshake, Valid: present}
		if err := models.WireGuardDeviceStatus.Upsert(ctx, service.db.Executor(), device.ID, "active", observedHandshake, nil); err != nil {
			observationErr = errors.Join(observationErr, err)
		}
	}
	return observationErr
}

func generateWireGuardKeyPair() (string, string, error) {
	private := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(private); err != nil {
		return "", "", fmt.Errorf("generate WireGuard private key: %w", err)
	}
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("derive WireGuard public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(private), base64.StdEncoding.EncodeToString(public), nil
}

func buildClientConfiguration(privateKey, privateAddress, serverPublicKey, serverEndpoint string) string {
	return fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s/32\n\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = 10.99.0.1/32\nPersistentKeepalive = 25\n", privateKey, privateAddress, serverPublicKey, serverEndpoint)
}
