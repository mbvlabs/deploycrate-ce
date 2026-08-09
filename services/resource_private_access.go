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
	servers   *ServerExecution
}

func NewResourcePrivateAccess(db storage.Pool, servers *ServerExecution) *ResourcePrivateAccess {
	return &ResourcePrivateAccess{
		db: db, wireguard: wireguardclient.New(), firewall: firewallclient.New(),
		listener: listenerclient.New(), servers: servers,
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

func (service *ResourcePrivateAccess) requireOrdinaryManagedResource(
	ctx context.Context,
	resourceID uuid.UUID,
) error {
	exists, err := models.Resource.ActiveOrdinaryExists(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	if !exists {
		return models.ErrNotFound
	}
	return nil
}

func (service *ResourcePrivateAccess) EnrollManaged(
	ctx context.Context,
	resourceID uuid.UUID,
	data ResourcePrivateAccessEnrollment,
) (ResourcePrivateAccessResult, error) {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	return service.Enroll(ctx, resourceID, data)
}

func (service *ResourcePrivateAccess) RevokeManagedGrant(
	ctx context.Context,
	resourceID, deviceID uuid.UUID,
) error {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return err
	}
	return service.RevokeGrant(ctx, resourceID, deviceID)
}

func (service *ResourcePrivateAccess) Enable(
	ctx context.Context,
	resourceID, privateNetworkID uuid.UUID,
) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()

	_, err = models.Resource.LockActiveOrdinary(ctx, tx, resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	endpoint, err := createManagedResourcePrivateEndpoint(ctx, tx, resourceID, privateNetworkID)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	return endpoint, nil
}

func createManagedResourcePrivateEndpoint(
	ctx context.Context,
	db storage.Executor,
	resourceID, privateNetworkID uuid.UUID,
) (models.ResourceEndpointEntity, error) {
	privateEndpoints, err := models.ResourceEndpoint.ActivePrivateCount(ctx, db, resourceID, uuid.Nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if privateEndpoints != 0 {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"unique",
			"managed Resource already has private access configured",
		)
	}

	origin, err := models.ResourceEndpoint.FindActivePrimaryPublic(ctx, db, resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"topology",
			"managed Resource has no primary runtime origin",
		)
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	attachment, err := models.ServerNetwork.ResourceAttachment(ctx, db, resourceID, privateNetworkID)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(attachment.Address) == "" {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"topology",
			"installation Server has no active WireGuard attachment for this private network",
		)
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	attachment.Address = strings.TrimSpace(attachment.Address)
	originIsLoopback := origin.Address == "127.0.0.1" || origin.Address == "::1" ||
		origin.Address == "localhost"
	if attachment.Address == WireGuardPrivateAddress && !originIsLoopback {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"topology",
			"control-plane private access requires a loopback Resource origin",
		)
	}
	if attachment.Address != WireGuardPrivateAddress && origin.Address != attachment.Address {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"topology",
			"Node private access requires the Resource origin on its WireGuard address",
		)
	}
	mapping, err := primaryPortMapping(attachment.Configuration)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if mapping.Protocol != "tcp" || mapping.HostPort != origin.Port {
		return models.ResourceEndpointEntity{}, domainError(
			"privateNetworkId",
			"topology",
			"private access requires the primary origin to use its single TCP Docker port mapping",
		)
	}

	endpoint, err := models.ResourceEndpoint.Create(ctx, db, models.CreateResourceEndpointData{
		Name:     "Private access",
		Role:     "wireguard",
		Address:  attachment.Address,
		Port:     origin.Port,
		Protocol: origin.Protocol,
		TlsMode:  origin.TLSMode,
		Settings: json.RawMessage(
			`{}`,
		),
		ResourceID:       resourceID,
		PrivateNetworkID: &privateNetworkID,
	})
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	return endpoint, nil
}

func (service *ResourcePrivateAccess) Disable(ctx context.Context, resourceID uuid.UUID) error {
	if err := service.requireOrdinaryManagedResource(ctx, resourceID); err != nil {
		return err
	}
	target, err := models.Application.FindResourceAccessTarget(
		ctx,
		service.db.Executor(),
		resourceID,
	)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	privateEndpoints, err := models.ResourceEndpoint.ActivePrivateCount(
		ctx, service.db.Executor(), resourceID, target.WireGuardEndpointID,
	)
	if err != nil {
		return err
	}
	if privateEndpoints > 0 {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("archive every endpoint using WireGuard before turning off private access"),
		)
	}
	dependencies, err := models.ResourceEndpoint.ActiveDependencyCount(
		ctx, service.db.Executor(), target.WireGuardEndpointID,
	)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New(
				"private access is selected by an active Environment connection or health check",
			),
		)
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
	if targetUsesControlPlaneListener(target) {
		if err := service.listener.RemoveListener(ctx, resourceID); err != nil {
			return fmt.Errorf("remove private resource listener: %w", err)
		}
	}

	archived, err := models.ResourceEndpoint.ArchiveActive(
		ctx, service.db.Executor(), target.WireGuardEndpointID, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if !archived {
		return models.ErrNotFound
	}
	return nil
}

func (service *ResourcePrivateAccess) Details(
	ctx context.Context,
	resourceID, currentUserID uuid.UUID,
) (models.ResourcePrivateAccessDetails, error) {
	return models.WireGuardDevice.PrivateAccessDetails(
		ctx, service.db.Executor(), resourceID, currentUserID,
	)
}

func (service *ResourcePrivateAccess) Enroll(
	ctx context.Context,
	resourceID uuid.UUID,
	data ResourcePrivateAccessEnrollment,
) (ResourcePrivateAccessResult, error) {
	target, err := models.Application.FindResourceAccessTarget(
		ctx,
		service.db.Executor(),
		resourceID,
	)
	if err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	if target.Protocol == "" {
		return ResourcePrivateAccessResult{}, errors.New(
			"resource has no supported WireGuard endpoint",
		)
	}
	var controlPlane models.WireGuardPeerEntity
	if data.DeviceID == uuid.Nil {
		controlPlane, err = service.controlPlaneWireGuardPeer(ctx)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
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
			return ResourcePrivateAccessResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("device name is required"),
			)
		}
		address, err := AllocateWireGuardDeviceAddress(ctx, tx)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		var publicKey string
		privateKey, publicKey, err = generateWireGuardKeyPair()
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		device, err = models.WireGuardDevice.Create(ctx, tx, models.CreateWireGuardDeviceData{
			Name:           data.Name,
			PublicKey:      publicKey,
			PrivateAddress: address,
			OwnerUserID:    data.UserID,
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
		grant, err = models.WireGuardDeviceResourceGrant.Create(
			ctx,
			tx,
			device.ID,
			resourceID,
			data.UserID,
		)
		if err != nil {
			return ResourcePrivateAccessResult{}, err
		}
		application, err = models.WireGuardGrantApplication.CreatePending(
			ctx,
			tx,
			grant.ID,
			target.WireGuardEndpointID,
			target.ServerID,
		)
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
		_ = models.WireGuardGrantApplication.Mark(
			ctx,
			service.db.Executor(),
			application.ID,
			"failed",
			applyErr,
		)
		_ = models.WireGuardDeviceStatus.Upsert(
			ctx,
			service.db.Executor(),
			device.ID,
			"error",
			sql.NullTime{},
			applyErr,
		)
		if newDevice {
			service.cleanupFailedEnrollment(ctx, device, grant, target)
		}
		return ResourcePrivateAccessResult{}, applyErr
	}
	if err := models.WireGuardGrantApplication.Mark(
		ctx,
		service.db.Executor(),
		application.ID,
		"applied",
		nil,
	); err != nil {
		return ResourcePrivateAccessResult{}, err
	}
	if err := models.WireGuardDeviceStatus.Upsert(
		ctx,
		service.db.Executor(),
		device.ID,
		"active",
		sql.NullTime{},
		nil,
	); err != nil {
		return ResourcePrivateAccessResult{}, err
	}

	configuration := ""
	if newDevice {
		configuration = buildClientConfiguration(
			privateKey,
			device.PrivateAddress,
			controlPlane.PublicKey,
			controlPlane.Endpoint.String,
		)
	}
	return ResourcePrivateAccessResult{
		DeviceID:            device.ID,
		GrantID:             grant.ID,
		ClientConfiguration: configuration,
	}, nil
}

func (service *ResourcePrivateAccess) cleanupFailedEnrollment(
	ctx context.Context,
	device models.WireGuardDeviceEntity,
	grant models.WireGuardDeviceResourceGrantEntity,
	target models.ResourceAccessTarget,
) {
	_ = service.removeGrantNetworkAccess(ctx, device, grant, target)
	if count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(
		ctx,
		service.db.Executor(),
		target.ResourceID,
	); targetUsesControlPlaneListener(target) && err == nil &&
		count == 1 {
		_ = service.listener.RemoveListener(ctx, target.ResourceID)
	}
	_ = service.wireguard.RemovePeer(ctx, device.PublicKey)
	now := time.Now()
	_ = models.WireGuardDeviceResourceGrant.Revoke(ctx, service.db.Executor(), grant.ID, now)
	_ = models.WireGuardDevice.Revoke(ctx, service.db.Executor(), device.ID, now)
}

func (service *ResourcePrivateAccess) apply(
	ctx context.Context,
	device models.WireGuardDeviceEntity,
	grant models.WireGuardDeviceResourceGrantEntity,
	target models.ResourceAccessTarget,
	applyPeer bool,
) error {
	if applyPeer {
		if err := service.wireguard.ApplyPeer(
			ctx,
			device.PublicKey,
			device.PrivateAddress,
		); err != nil {
			return fmt.Errorf("apply WireGuard device peer: %w", err)
		}
	}
	if targetUsesControlPlaneListener(target) {
		if err := service.listener.ApplyListener(
			ctx,
			target.ResourceID,
			target.WireGuardAddress,
			target.WireGuardPort,
			target.OriginAddress,
			target.OriginPort,
		); err != nil {
			return fmt.Errorf("apply private resource listener: %w", err)
		}
		if err := service.firewall.ApplyRule(
			ctx,
			grant.ID,
			device.PrivateAddress,
			target.WireGuardPort,
		); err != nil {
			return fmt.Errorf("apply private resource firewall rule: %w", err)
		}
		return nil
	}
	if err := service.applyNodeGrantRule(ctx, device, grant, target); err != nil {
		return err
	}
	if err := service.firewall.ApplyRouteRule(
		ctx,
		grant.ID,
		device.PrivateAddress,
		target.WireGuardAddress,
		target.WireGuardPort,
	); err != nil {
		_ = service.removeNodeGrantRule(ctx, device, grant, target)
		return fmt.Errorf("apply private resource gateway route: %w", err)
	}
	return nil
}

func targetUsesControlPlaneListener(target models.ResourceAccessTarget) bool {
	return target.WireGuardAddress == WireGuardPrivateAddress
}

func (service *ResourcePrivateAccess) controlPlaneWireGuardPeer(
	ctx context.Context,
) (models.WireGuardPeerEntity, error) {
	peer, err := models.WireGuardPeer.FindActiveByPrivateAddress(
		ctx, service.db.Executor(), WireGuardPrivateAddress,
	)
	if err != nil {
		return models.WireGuardPeerEntity{}, fmt.Errorf(
			"load control-plane WireGuard identity: %w",
			err,
		)
	}
	if !peer.Endpoint.Valid {
		return models.WireGuardPeerEntity{}, errors.New(
			"control-plane WireGuard endpoint is unavailable",
		)
	}
	return peer, nil
}

func (service *ResourcePrivateAccess) applyNodeGrantRule(
	ctx context.Context,
	device models.WireGuardDeviceEntity,
	grant models.WireGuardDeviceResourceGrantEntity,
	target models.ResourceAccessTarget,
) error {
	execution, err := service.servers.Target(
		ctx,
		target.ServerID,
		managedResourceCapability(target.ResourceEngine),
	)
	if err != nil {
		return err
	}
	if !execution.Remote {
		return errors.New("private Resource gateway target must be a Node")
	}
	_, err = service.servers.RunRootCommand(
		ctx,
		execution,
		nil,
		"/usr/sbin/ufw",
		"allow",
		"in",
		"on",
		"wg0",
		"from",
		device.PrivateAddress,
		"to",
		target.WireGuardAddress,
		"port",
		fmt.Sprint(target.WireGuardPort),
		"proto",
		"tcp",
		"comment",
		"deploycrate-grant-"+grant.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("apply destination Node firewall rule: %w", err)
	}
	return nil
}

func (service *ResourcePrivateAccess) removeNodeGrantRule(
	ctx context.Context,
	device models.WireGuardDeviceEntity,
	grant models.WireGuardDeviceResourceGrantEntity,
	target models.ResourceAccessTarget,
) error {
	execution, err := service.servers.Target(
		ctx,
		target.ServerID,
		managedResourceCapability(target.ResourceEngine),
	)
	if err != nil {
		return err
	}
	_, err = service.servers.RunRootCommand(
		ctx,
		execution,
		nil,
		"/usr/sbin/ufw",
		"--force",
		"delete",
		"allow",
		"in",
		"on",
		"wg0",
		"from",
		device.PrivateAddress,
		"to",
		target.WireGuardAddress,
		"port",
		fmt.Sprint(target.WireGuardPort),
		"proto",
		"tcp",
		"comment",
		"deploycrate-grant-"+grant.ID.String(),
	)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "non-existent") || strings.Contains(message, "skipping") {
			return nil
		}
		return fmt.Errorf("remove destination Node firewall rule: %w", err)
	}
	return nil
}

func (service *ResourcePrivateAccess) removeGrantNetworkAccess(
	ctx context.Context,
	device models.WireGuardDeviceEntity,
	grant models.WireGuardDeviceResourceGrantEntity,
	target models.ResourceAccessTarget,
) error {
	if targetUsesControlPlaneListener(target) {
		return service.firewall.RemoveRule(
			ctx,
			grant.ID,
			device.PrivateAddress,
			target.WireGuardPort,
		)
	}
	if err := service.firewall.RemoveRouteRule(
		ctx,
		grant.ID,
		device.PrivateAddress,
		target.WireGuardAddress,
		target.WireGuardPort,
	); err != nil {
		return err
	}
	return service.removeNodeGrantRule(ctx, device, grant, target)
}

func (service *ResourcePrivateAccess) RevokeGrant(
	ctx context.Context,
	resourceID, deviceID uuid.UUID,
) error {
	device, err := models.WireGuardDevice.FindActive(ctx, service.db.Executor(), deviceID)
	if err != nil {
		return err
	}
	grant, err := models.WireGuardDeviceResourceGrant.FindActive(
		ctx,
		service.db.Executor(),
		deviceID,
		resourceID,
	)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	target, err := models.Application.FindResourceAccessTarget(
		ctx,
		service.db.Executor(),
		resourceID,
	)
	if err != nil {
		return err
	}
	application, err := models.WireGuardGrantApplication.FindByGrant(
		ctx,
		service.db.Executor(),
		grant.ID,
	)
	if err != nil {
		return err
	}
	if err := service.removeGrantNetworkAccess(ctx, device, grant, target); err != nil {
		_ = models.WireGuardGrantApplication.Mark(
			ctx,
			service.db.Executor(),
			application.ID,
			"failed",
			err,
		)
		return err
	}
	count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(
		ctx,
		service.db.Executor(),
		resourceID,
	)
	if err != nil {
		return err
	}
	if targetUsesControlPlaneListener(target) && count == 1 {
		if err := service.listener.RemoveListener(ctx, resourceID); err != nil {
			_ = models.WireGuardGrantApplication.Mark(
				ctx,
				service.db.Executor(),
				application.ID,
				"failed",
				err,
			)
			return err
		}
	}
	now := time.Now()
	if err := models.WireGuardDeviceResourceGrant.Revoke(
		ctx,
		service.db.Executor(),
		grant.ID,
		now,
	); err != nil {
		return err
	}
	return models.WireGuardGrantApplication.Mark(
		ctx,
		service.db.Executor(),
		application.ID,
		"removed",
		nil,
	)
}

func (service *ResourcePrivateAccess) RevokeDevice(ctx context.Context, deviceID uuid.UUID) error {
	device, err := models.WireGuardDevice.FindActive(ctx, service.db.Executor(), deviceID)
	if errors.Is(err, models.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	grants, err := models.WireGuardDeviceResourceGrant.ActiveForDevice(
		ctx,
		service.db.Executor(),
		deviceID,
	)
	if err != nil {
		return err
	}
	for _, grant := range grants {
		target, err := models.Application.FindResourceAccessTarget(
			ctx,
			service.db.Executor(),
			grant.ResourceID,
		)
		if err != nil {
			return err
		}
		if err := service.removeGrantNetworkAccess(ctx, device, grant, target); err != nil {
			return err
		}
		count, err := models.WireGuardDeviceResourceGrant.ActiveCountForResource(
			ctx,
			service.db.Executor(),
			grant.ResourceID,
		)
		if err != nil {
			return err
		}
		if targetUsesControlPlaneListener(target) && count == 1 {
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
		if err := models.WireGuardDeviceResourceGrant.Revoke(
			ctx,
			service.db.Executor(),
			grant.ID,
			now,
		); err != nil {
			return err
		}
		application, err := models.WireGuardGrantApplication.FindByGrant(
			ctx,
			service.db.Executor(),
			grant.ID,
		)
		if err == nil {
			_ = models.WireGuardGrantApplication.Mark(
				ctx,
				service.db.Executor(),
				application.ID,
				"removed",
				nil,
			)
		}
	}
	if err := models.WireGuardDevice.Revoke(ctx, service.db.Executor(), deviceID, now); err != nil {
		return err
	}
	return models.WireGuardDeviceStatus.Upsert(
		ctx,
		service.db.Executor(),
		deviceID,
		"revoked",
		sql.NullTime{},
		nil,
	)
}

func (service *ResourcePrivateAccess) ObserveResource(
	ctx context.Context,
	resourceID uuid.UUID,
) error {
	devices, err := models.WireGuardDevice.ActiveForResource(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	var observationErr error
	for _, device := range devices {
		handshake, present, err := service.wireguard.LatestHandshake(ctx, device.PublicKey)
		if err != nil {
			observationErr = errors.Join(observationErr, err)
			_ = models.WireGuardDeviceStatus.Upsert(
				ctx,
				service.db.Executor(),
				device.ID,
				"error",
				sql.NullTime{},
				err,
			)
			continue
		}
		observedHandshake := sql.NullTime{Time: handshake, Valid: present}
		if err := models.WireGuardDeviceStatus.Upsert(
			ctx,
			service.db.Executor(),
			device.ID,
			"active",
			observedHandshake,
			nil,
		); err != nil {
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
	return base64.StdEncoding.EncodeToString(
			private,
		), base64.StdEncoding.EncodeToString(
			public,
		), nil
}

func buildClientConfiguration(
	privateKey, privateAddress, serverPublicKey, serverEndpoint string,
) string {
	return fmt.Sprintf(
		"[Interface]\nPrivateKey = %s\nAddress = %s/32\n\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = 25\n",
		privateKey,
		privateAddress,
		serverPublicKey,
		serverEndpoint,
		WireGuardNodeCIDR,
	)
}
