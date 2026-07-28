package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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

func (service *ResourcePrivateAccess) requireOrdinaryManagedResource(ctx context.Context, resourceID uuid.UUID) error {
	count, err := service.db.Executor().NewSelect().TableExpr("resources").Where("id = ?", resourceID).
		Where("management_mode = 'managed'").Where("system_managed = FALSE").Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if count != 1 {
		return models.ErrNotFound
	}
	return nil
}

func (service *ResourcePrivateAccess) EnrollManaged(ctx context.Context, resourceID uuid.UUID, data ResourcePrivateAccessEnrollment) (ResourcePrivateAccessResult, error) {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	return service.Enroll(ctx, resourceID, data)
}

func (service *ResourcePrivateAccess) RevokeManagedGrant(ctx context.Context, resourceID, deviceID uuid.UUID) error {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return err
	}
	return service.RevokeGrant(ctx, resourceID, deviceID)
}

func (service *ResourcePrivateAccess) Enable(ctx context.Context, resourceID, privateNetworkID uuid.UUID) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()

	var resource models.ResourceEntity
	err = tx.NewSelect().Model(&resource).
		Where("id = ?", resourceID).
		Where("archived_at IS NULL").
		Where("system_managed = FALSE").
		For("UPDATE").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("private access is available only for managed Resources"))
	}

	var origin models.ResourceEndpointEntity
	err = tx.NewSelect().Model(&origin).
		Where("resource_id = ?", resourceID).
		Where("role = 'primary'").
		Where("private_network_id IS NULL").
		Where("archived_at IS NULL").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("managed Resource has no primary runtime origin"))
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if origin.ResourceInstallationID == nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("managed Resource primary endpoint is not installation-backed"))
	}

	privateEndpoints, err := tx.NewSelect().TableExpr("resource_endpoints").
		Where("resource_id = ?", resourceID).
		Where("private_network_id IS NOT NULL").
		Where("archived_at IS NULL").
		Count(ctx)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if privateEndpoints != 0 {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("managed Resource already has private access configured"))
	}

	var attachment struct {
		Address       string          `bun:"address"`
		Configuration json.RawMessage `bun:"configuration"`
	}
	err = tx.NewSelect().TableExpr("server_networks AS attachment").
		ColumnExpr("COALESCE(attachment.configuration ->> 'address', '') AS address, installation.configuration AS configuration").
		Join("JOIN resource_installations AS installation ON installation.server_id = attachment.server_id AND installation.id = ? AND installation.archived_at IS NULL", *origin.ResourceInstallationID).
		Join("JOIN private_networks AS network ON network.id = attachment.private_network_id AND network.archived_at IS NULL").
		Where("attachment.private_network_id = ?", privateNetworkID).
		Where("attachment.driver = 'wireguard'").
		Where("attachment.removed_at IS NULL").
		Limit(1).
		Scan(ctx, &attachment)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(attachment.Address) == "" {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("installation Server has no active WireGuard attachment for this private network"))
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if strings.TrimSpace(attachment.Address) != WireGuardPrivateAddress {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("private network attachment does not use the DeployCrate WireGuard listener address"))
	}
	mapping, err := primaryPortMapping(attachment.Configuration)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if mapping.Protocol != "tcp" || mapping.HostPort != origin.Port {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, errors.New("private access requires the primary origin to use its single TCP Docker port mapping"))
	}

	endpoint, err := models.ResourceEndpoint.Create(ctx, tx, models.CreateResourceEndpointData{
		Name: "Private access", Role: origin.Role, Address: strings.TrimSpace(attachment.Address),
		Port: origin.Port, Protocol: origin.Protocol, TlsMode: origin.TlsMode,
		Settings: json.RawMessage(`{}`), ResourceID: resourceID,
		ResourceInstallationID: origin.ResourceInstallationID, PrivateNetworkID: &privateNetworkID,
	})
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	return endpoint, nil
}

func (service *ResourcePrivateAccess) Disable(ctx context.Context, resourceID uuid.UUID) error {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return err
	}
	target, err := models.Application.FindResourceAccessTarget(ctx, service.db.Executor(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	dependencies, err := service.db.Executor().NewSelect().TableExpr("resource_endpoints AS endpoint").
		Where("endpoint.id = ?", target.WireGuardEndpointID).
		Where("EXISTS (SELECT 1 FROM environment_resources WHERE resource_endpoint_id = endpoint.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_endpoint_id = endpoint.id AND archived_at IS NULL)").
		Count(ctx)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return errors.Join(models.ErrDomainValidation, errors.New("private access is selected by an active Environment connection or health check"))
	}

	devices, err := models.WireGuardDevice.ActiveForResource(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	for _, device := range devices {
		if err := service.RevokeGrant(ctx, resourceID, device.ID); err != nil {
			return fmt.Errorf("revoke private access for device %q: %w", device.Name, err)
		}
	}
	if err := service.listener.RemoveListener(ctx, resourceID); err != nil {
		return fmt.Errorf("remove private resource listener: %w", err)
	}

	now := time.Now().UTC()
	result, err := service.db.Executor().NewUpdate().TableExpr("resource_endpoints").
		Set("archived_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", target.WireGuardEndpointID).
		Where("archived_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (service *ResourcePrivateAccess) Details(ctx context.Context, resourceID, currentUserID uuid.UUID) (models.ResourcePrivateAccessDetails, error) {
	detail := models.ResourcePrivateAccessDetails{
		DeviceGrants:     make([]models.SystemWireGuardDeviceGrant, 0),
		AvailableDevices: make([]models.SystemWireGuardDeviceOption, 0),
	}
	if err := service.db.Executor().NewSelect().TableExpr("wireguard_device_resource_grants AS resource_grant").
		ColumnExpr("device.id::text AS device_id, device.name AS device_name, owner.email AS owner_email, device.private_address::text AS private_address, resource_grant.id::text AS grant_id, resource_grant.granted_at, COALESCE(application.state, 'pending') AS application_state, COALESCE(application.error, '') AS application_error, status.latest_handshake_at, status.observed_at").
		Join("JOIN wireguard_devices AS device ON device.id = resource_grant.wireguard_device_id AND device.revoked_at IS NULL").
		Join("JOIN users AS owner ON owner.id = device.owner_user_id").
		Join("LEFT JOIN wireguard_device_resource_grant_applications AS application ON application.wireguard_device_resource_grant_id = resource_grant.id").
		Join("LEFT JOIN wireguard_device_statuses AS status ON status.wireguard_device_id = device.id").
		Where("resource_grant.resource_id = ?", resourceID).
		Where("resource_grant.revoked_at IS NULL").
		OrderExpr("device.name").
		Scan(ctx, &detail.DeviceGrants); err != nil {
		return models.ResourcePrivateAccessDetails{}, err
	}
	if err := service.db.Executor().NewSelect().TableExpr("wireguard_devices AS device").
		ColumnExpr("device.id::text AS id, device.name, device.private_address::text AS private_address").
		Where("device.owner_user_id = ?", currentUserID).
		Where("device.revoked_at IS NULL").
		Where("NOT EXISTS (SELECT 1 FROM wireguard_device_resource_grants resource_grant WHERE resource_grant.wireguard_device_id = device.id AND resource_grant.resource_id = ? AND resource_grant.revoked_at IS NULL)", resourceID).
		OrderExpr("device.name").
		Scan(ctx, &detail.AvailableDevices); err != nil {
		return models.ResourcePrivateAccessDetails{}, err
	}
	return detail, nil
}

func (service *ResourcePrivateAccess) Enroll(ctx context.Context, resourceID uuid.UUID, data ResourcePrivateAccessEnrollment) (ResourcePrivateAccessResult, error) {
	target, err := models.Application.FindResourceAccessTarget(ctx, service.db.Executor(), resourceID)
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
	} else if err := models.WireGuardGrantApplication.Mark(ctx, tx, application.ID, "pending", nil); err != nil {
		return ResourcePrivateAccessResult{}, err
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

func (service *ResourcePrivateAccess) cleanupFailedEnrollment(ctx context.Context, device models.WireGuardDeviceEntity, grant models.WireGuardDeviceResourceGrantEntity, target models.ResourceAccessTarget) {
	_ = service.firewall.RemoveRule(ctx, grant.ID, device.PrivateAddress, target.WireGuardPort)
	if count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(ctx, service.db.Executor(), target.ResourceID); err == nil && count == 1 {
		_ = service.listener.RemoveListener(ctx, target.ResourceID)
	}
	_ = service.wireguard.RemovePeer(ctx, device.PublicKey)
	now := time.Now()
	_ = models.WireGuardDeviceResourceGrant.Revoke(ctx, service.db.Executor(), grant.ID, now)
	_ = models.WireGuardDevice.Revoke(ctx, service.db.Executor(), device.ID, now)
}

func (service *ResourcePrivateAccess) apply(ctx context.Context, device models.WireGuardDeviceEntity, grant models.WireGuardDeviceResourceGrantEntity, target models.ResourceAccessTarget, applyPeer bool) error {
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
	target, err := models.Application.FindResourceAccessTarget(ctx, service.db.Executor(), resourceID)
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
		target, err := models.Application.FindResourceAccessTarget(ctx, service.db.Executor(), grant.ResourceID)
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
