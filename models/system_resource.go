package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
)

type SystemResourceIndexItem struct {
	ID            string `bun:"id"`
	Name          string `bun:"name"`
	Category      string `bun:"category"`
	Kind          string `bun:"kind"`
	SharingScope  string `bun:"sharing_scope"`
	OriginAddress string `bun:"origin_address"`
	OriginPort    int32  `bun:"origin_port"`
	Health        string `bun:"health"`
}

func (application) FindSystemResourceIndex(ctx context.Context, db storage.Executor) ([]SystemResourceIndexItem, error) {
	items := make([]SystemResourceIndexItem, 0)
	err := db.NewSelect().
		TableExpr("resources AS resource").
		Distinct().
		ColumnExpr("resource.id::text AS id").
		ColumnExpr("resource.name AS name").
		ColumnExpr("resource.category AS category").
		ColumnExpr("resource.kind AS kind").
		ColumnExpr("resource.sharing_scope AS sharing_scope").
		ColumnExpr("COALESCE(origin.address, '') AS origin_address").
		ColumnExpr("COALESCE(origin.port, 0) AS origin_port").
		ColumnExpr("COALESCE(installation_status.health, 'unknown') AS health").
		Join("LEFT JOIN LATERAL (SELECT address, port, resource_installation_id FROM resource_endpoints WHERE resource_id = resource.id AND role = 'primary' AND archived_at IS NULL ORDER BY created_at LIMIT 1) AS origin ON TRUE").
		Join("LEFT JOIN resource_installation_statuses AS installation_status ON installation_status.resource_installation_id = origin.resource_installation_id").
		Where("resource.system_managed = TRUE").
		Where("resource.archived_at IS NULL").
		OrderExpr("resource.name").
		Scan(ctx, &items)
	return items, err
}

type SystemResourceBinding struct {
	ID              string          `bun:"id"`
	CreatedAt       time.Time       `bun:"created_at"`
	UpdatedAt       time.Time       `bun:"updated_at"`
	Alias           string          `bun:"alias"`
	Configuration   json.RawMessage `bun:"configuration"`
	EnvironmentID   string          `bun:"environment_id"`
	EnvironmentName string          `bun:"environment_name"`
	EnvironmentKind string          `bun:"environment_kind"`
	EndpointID      string          `bun:"endpoint_id"`
	CredentialID    string          `bun:"credential_id"`
}

type SystemResourceEndpoint struct {
	ID               string          `bun:"id"`
	CreatedAt        time.Time       `bun:"created_at"`
	UpdatedAt        time.Time       `bun:"updated_at"`
	Name             string          `bun:"name"`
	Role             string          `bun:"role"`
	Address          string          `bun:"address"`
	Port             int32           `bun:"port"`
	Protocol         string          `bun:"protocol"`
	TLSMode          string          `bun:"tls_mode"`
	Settings         json.RawMessage `bun:"settings"`
	InstallationID   string          `bun:"installation_id"`
	PrivateNetworkID string          `bun:"private_network_id"`
}

type SystemResourceCredential struct {
	ID                  string          `bun:"id"`
	Name                string          `bun:"name"`
	Username            string          `bun:"username"`
	Metadata            json.RawMessage `bun:"metadata"`
	HasEncryptedPayload bool            `bun:"has_encrypted_payload"`
	InstallationID      string          `bun:"installation_id"`
}

type SystemResourceInstallation struct {
	ID             string          `bun:"id"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	ImageReference string          `bun:"image_reference"`
	ImageDigest    string          `bun:"image_digest"`
	ContainerName  string          `bun:"container_name"`
	RestartPolicy  string          `bun:"restart_policy"`
	Configuration  json.RawMessage `bun:"configuration"`
	ServerID       string          `bun:"server_id"`
	ServerName     string          `bun:"server_name"`
	ServerAddress  string          `bun:"server_address"`
	State          string          `bun:"state"`
	ServiceState   string          `bun:"service_state"`
	Health         string          `bun:"health"`
	HealthReason   string          `bun:"health_reason"`
	ObservedAt     *time.Time      `bun:"observed_at"`
}

type SystemResourceVolumeMount struct {
	ID             string `bun:"id"`
	MountPath      string `bun:"mount_path"`
	ReadOnly       bool   `bun:"read_only"`
	InstallationID string `bun:"installation_id"`
}

type SystemResourceVolume struct {
	ID            string                      `bun:"id"`
	Name          string                      `bun:"name"`
	Driver        string                      `bun:"driver"`
	Configuration json.RawMessage             `bun:"configuration"`
	ServerID      string                      `bun:"server_id"`
	ServerName    string                      `bun:"server_name"`
	Mounts        []SystemResourceVolumeMount `bun:"-"`
}

type SystemResourceCheck struct {
	ID               string          `bun:"id"`
	Name             string          `bun:"name"`
	Kind             string          `bun:"kind"`
	Configuration    json.RawMessage `bun:"configuration"`
	IntervalSeconds  int32           `bun:"interval_seconds"`
	TimeoutSeconds   int32           `bun:"timeout_seconds"`
	FailureThreshold int32           `bun:"failure_threshold"`
	SuccessThreshold int32           `bun:"success_threshold"`
	Enabled          bool            `bun:"enabled"`
	State            string          `bun:"state"`
	Message          string          `bun:"message"`
	ObservedAt       *time.Time      `bun:"observed_at"`
}

type SystemWireGuardDeviceGrant struct {
	DeviceID          string     `bun:"device_id"`
	DeviceName        string     `bun:"device_name"`
	OwnerEmail        string     `bun:"owner_email"`
	PrivateAddress    string     `bun:"private_address"`
	GrantID           string     `bun:"grant_id"`
	GrantedAt         time.Time  `bun:"granted_at"`
	ApplicationState  string     `bun:"application_state"`
	ApplicationError  string     `bun:"application_error"`
	LatestHandshakeAt *time.Time `bun:"latest_handshake_at"`
	ObservedAt        *time.Time `bun:"observed_at"`
}

type SystemPrivateNetworkOption struct {
	ID   string `bun:"id"`
	Name string `bun:"name"`
}

type SystemWireGuardDeviceOption struct {
	ID             string `bun:"id"`
	Name           string `bun:"name"`
	PrivateAddress string `bun:"private_address"`
}

type SystemWireGuardDevice struct {
	ID                string     `bun:"id"`
	Name              string     `bun:"name"`
	OwnerEmail        string     `bun:"owner_email"`
	PrivateAddress    string     `bun:"private_address"`
	ActivatedAt       time.Time  `bun:"activated_at"`
	GrantCount        int32      `bun:"grant_count"`
	State             string     `bun:"state"`
	LatestHandshakeAt *time.Time `bun:"latest_handshake_at"`
	ObservedAt        *time.Time `bun:"observed_at"`
}

func (application) FindSystemWireGuardDevices(ctx context.Context, db storage.Executor) ([]SystemWireGuardDevice, error) {
	devices := make([]SystemWireGuardDevice, 0)
	err := db.NewSelect().TableExpr("wireguard_devices AS device").
		ColumnExpr("device.id::text AS id, device.name, owner.email AS owner_email, device.private_address::text AS private_address, device.activated_at").
		ColumnExpr("COUNT(resource_grant.id)::integer AS grant_count, COALESCE(status.state, 'unknown') AS state, status.latest_handshake_at, status.observed_at").
		Join("JOIN users AS owner ON owner.id = device.owner_user_id").
		Join("LEFT JOIN wireguard_device_resource_grants AS resource_grant ON resource_grant.wireguard_device_id = device.id AND resource_grant.revoked_at IS NULL").
		Join("LEFT JOIN wireguard_device_statuses AS status ON status.wireguard_device_id = device.id").
		Where("device.revoked_at IS NULL").
		GroupExpr("device.id, owner.email, status.state, status.latest_handshake_at, status.observed_at").
		OrderExpr("device.name").
		Scan(ctx, &devices)
	return devices, err
}

type SystemResourceDetail struct {
	ID               string                        `bun:"id"`
	CreatedAt        time.Time                     `bun:"created_at"`
	UpdatedAt        time.Time                     `bun:"updated_at"`
	Name             string                        `bun:"name"`
	Category         string                        `bun:"category"`
	Kind             string                        `bun:"kind"`
	SharingScope     string                        `bun:"sharing_scope"`
	Bindings         []SystemResourceBinding       `bun:"-"`
	Endpoints        []SystemResourceEndpoint      `bun:"-"`
	Credentials      []SystemResourceCredential    `bun:"-"`
	Installations    []SystemResourceInstallation  `bun:"-"`
	Volumes          []SystemResourceVolume        `bun:"-"`
	HealthChecks     []SystemResourceCheck         `bun:"-"`
	DeviceGrants     []SystemWireGuardDeviceGrant  `bun:"-"`
	PrivateNetworks  []SystemPrivateNetworkOption  `bun:"-"`
	AvailableDevices []SystemWireGuardDeviceOption `bun:"-"`
}

func (application) FindSystemResourceDetail(ctx context.Context, db storage.Executor, resourceID, currentUserID uuid.UUID) (SystemResourceDetail, error) {
	var detail SystemResourceDetail
	err := db.NewSelect().TableExpr("resources AS resource").
		ColumnExpr("resource.id::text AS id, resource.created_at, resource.updated_at, resource.name, resource.category, resource.kind, resource.sharing_scope").
		Where("resource.id = ?", resourceID).
		Where("resource.archived_at IS NULL").
		Where("resource.system_managed = TRUE").
		Scan(ctx, &detail)
	if errors.Is(err, sql.ErrNoRows) {
		return SystemResourceDetail{}, ErrNotFound
	}
	if err != nil {
		return SystemResourceDetail{}, err
	}
	detail.Bindings = make([]SystemResourceBinding, 0)
	detail.Endpoints = make([]SystemResourceEndpoint, 0)
	detail.Credentials = make([]SystemResourceCredential, 0)
	detail.Installations = make([]SystemResourceInstallation, 0)
	detail.Volumes = make([]SystemResourceVolume, 0)
	detail.HealthChecks = make([]SystemResourceCheck, 0)
	detail.DeviceGrants = make([]SystemWireGuardDeviceGrant, 0)
	detail.PrivateNetworks = make([]SystemPrivateNetworkOption, 0)
	detail.AvailableDevices = make([]SystemWireGuardDeviceOption, 0)
	queries := []func() error{
		func() error {
			return db.NewSelect().TableExpr("environment_resources AS binding").ColumnExpr("binding.id::text AS id, binding.created_at, binding.updated_at, binding.alias, binding.configuration, environment.id::text AS environment_id, environment.name AS environment_name, environment.kind AS environment_kind, binding.resource_endpoint_id::text AS endpoint_id, COALESCE(binding.resource_credential_id::text, '') AS credential_id").Join("JOIN environments AS environment ON environment.id = binding.environment_id AND environment.archived_at IS NULL").Where("binding.resource_id = ?", resourceID).Where("binding.archived_at IS NULL").OrderExpr("binding.created_at").Scan(ctx, &detail.Bindings)
		},
		func() error {
			return db.NewSelect().TableExpr("resource_endpoints AS endpoint").ColumnExpr("endpoint.id::text AS id, endpoint.created_at, endpoint.updated_at, endpoint.name, endpoint.role, endpoint.address, endpoint.port, endpoint.protocol, endpoint.tls_mode, endpoint.settings, COALESCE(endpoint.resource_installation_id::text, '') AS installation_id, COALESCE(endpoint.private_network_id::text, '') AS private_network_id").Where("endpoint.resource_id = ?", resourceID).Where("endpoint.archived_at IS NULL").OrderExpr("endpoint.role, endpoint.created_at").Scan(ctx, &detail.Endpoints)
		},
		func() error {
			return db.NewSelect().TableExpr("resource_credentials AS credential").ColumnExpr("credential.id::text AS id, credential.name, COALESCE(credential.username, '') AS username, credential.metadata, octet_length(credential.enc_payload) > 0 AS has_encrypted_payload, COALESCE(credential.resource_installation_id::text, '') AS installation_id").Where("credential.resource_id = ?", resourceID).Where("credential.archived_at IS NULL").OrderExpr("credential.created_at").Scan(ctx, &detail.Credentials)
		},
		func() error {
			return db.NewSelect().TableExpr("resource_installations AS installation").ColumnExpr("installation.id::text AS id, installation.created_at, installation.updated_at, installation.image_reference, COALESCE(installation.image_digest, '') AS image_digest, installation.container_name, installation.restart_policy, installation.configuration, server.id::text AS server_id, server.name AS server_name, server.address AS server_address, COALESCE(status.state, '') AS state, COALESCE(status.service_state, '') AS service_state, COALESCE(status.health, '') AS health, COALESCE(status.health_reason, '') AS health_reason, status.observed_at AS observed_at").Join("JOIN servers AS server ON server.id = installation.server_id AND server.archived_at IS NULL").Join("LEFT JOIN resource_installation_statuses AS status ON status.resource_installation_id = installation.id").Where("installation.resource_id = ?", resourceID).Where("installation.archived_at IS NULL").OrderExpr("installation.created_at").Scan(ctx, &detail.Installations)
		},
		func() error {
			return db.NewSelect().TableExpr("resource_volumes AS volume").ColumnExpr("volume.id::text AS id, volume.name, volume.driver, volume.configuration, server.id::text AS server_id, server.name AS server_name").Join("JOIN servers AS server ON server.id = volume.server_id").Where("volume.resource_id = ?", resourceID).Where("volume.archived_at IS NULL").OrderExpr("volume.created_at").Scan(ctx, &detail.Volumes)
		},
		func() error {
			return db.NewSelect().TableExpr("resource_health_checks AS health_check").ColumnExpr("health_check.id::text AS id, health_check.name, health_check.kind, health_check.configuration, health_check.interval_seconds, health_check.timeout_seconds, health_check.failure_threshold, health_check.success_threshold, health_check.enabled, COALESCE(status.state, '') AS state, COALESCE(status.message, '') AS message, status.observed_at AS observed_at").Join("JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id").Join("LEFT JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id").Where("installation.resource_id = ?", resourceID).Where("health_check.archived_at IS NULL").OrderExpr("health_check.created_at").Scan(ctx, &detail.HealthChecks)
		},
		func() error {
			return db.NewSelect().TableExpr("wireguard_device_resource_grants AS resource_grant").ColumnExpr("device.id::text AS device_id, device.name AS device_name, owner.email AS owner_email, device.private_address::text AS private_address, resource_grant.id::text AS grant_id, resource_grant.granted_at, COALESCE(application.state, 'pending') AS application_state, COALESCE(application.error, '') AS application_error, status.latest_handshake_at, status.observed_at").Join("JOIN wireguard_devices AS device ON device.id = resource_grant.wireguard_device_id AND device.revoked_at IS NULL").Join("JOIN users AS owner ON owner.id = device.owner_user_id").Join("LEFT JOIN wireguard_device_resource_grant_applications AS application ON application.wireguard_device_resource_grant_id = resource_grant.id").Join("LEFT JOIN wireguard_device_statuses AS status ON status.wireguard_device_id = device.id").Where("resource_grant.resource_id = ?", resourceID).Where("resource_grant.revoked_at IS NULL").OrderExpr("device.name").Scan(ctx, &detail.DeviceGrants)
		},
		func() error {
			return db.NewSelect().TableExpr("private_networks AS network").Distinct().ColumnExpr("network.id::text AS id, network.name AS name").Join("JOIN environment_networks AS binding ON binding.private_network_id = network.id AND binding.removed_at IS NULL").Join("JOIN environments AS environment ON environment.id = binding.environment_id AND environment.archived_at IS NULL").Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").Join("JOIN server_networks AS server_network ON server_network.private_network_id = network.id AND server_network.driver = 'wireguard' AND server_network.removed_at IS NULL").Where("application.slug = ?", SystemApplicationSlug).Where("network.archived_at IS NULL").OrderExpr("network.name").Scan(ctx, &detail.PrivateNetworks)
		},
		func() error {
			return db.NewSelect().TableExpr("wireguard_devices AS device").ColumnExpr("device.id::text AS id, device.name, device.private_address::text AS private_address").Where("device.owner_user_id = ?", currentUserID).Where("device.revoked_at IS NULL").Where("NOT EXISTS (SELECT 1 FROM wireguard_device_resource_grants resource_grant WHERE resource_grant.wireguard_device_id = device.id AND resource_grant.resource_id = ? AND resource_grant.revoked_at IS NULL)", resourceID).OrderExpr("device.name").Scan(ctx, &detail.AvailableDevices)
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			return SystemResourceDetail{}, err
		}
	}
	for index := range detail.Volumes {
		detail.Volumes[index].Mounts = make([]SystemResourceVolumeMount, 0)
		if err := db.NewSelect().TableExpr("resource_volume_mounts AS mount").ColumnExpr("mount.id::text AS id, mount.mount_path, mount.read_only, mount.resource_installation_id::text AS installation_id").Where("mount.resource_volume_id::text = ?", detail.Volumes[index].ID).Where("mount.archived_at IS NULL").OrderExpr("mount.created_at").Scan(ctx, &detail.Volumes[index].Mounts); err != nil {
			return SystemResourceDetail{}, err
		}
	}
	return detail, nil
}

type ResourceAccessTarget struct {
	ResourceID          uuid.UUID `bun:"resource_id"`
	ResourceName        string    `bun:"resource_name"`
	ResourceKind        string    `bun:"resource_kind"`
	OriginAddress       string    `bun:"origin_address"`
	OriginPort          int32     `bun:"origin_port"`
	WireGuardEndpointID uuid.UUID `bun:"wireguard_endpoint_id"`
	WireGuardAddress    string    `bun:"wireguard_address"`
	WireGuardPort       int32     `bun:"wireguard_port"`
	Protocol            string    `bun:"protocol"`
	ServerID            uuid.UUID `bun:"server_id"`
	PrivateNetworkID    uuid.UUID `bun:"private_network_id"`
	ServerPublicKey     string    `bun:"server_public_key"`
	ServerEndpoint      string    `bun:"server_endpoint"`
}

func (application) FindResourceAccessTarget(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (ResourceAccessTarget, error) {
	var target ResourceAccessTarget
	err := db.NewSelect().TableExpr("resources AS resource").
		ColumnExpr("resource.id AS resource_id, resource.name AS resource_name, resource.kind AS resource_kind").
		ColumnExpr("origin.address AS origin_address, origin.port AS origin_port").
		ColumnExpr("wireguard.id AS wireguard_endpoint_id, wireguard.address AS wireguard_address, wireguard.port AS wireguard_port, wireguard.protocol AS protocol").
		ColumnExpr("installation.server_id AS server_id, wireguard.private_network_id AS private_network_id, peer.public_key AS server_public_key, peer.endpoint AS server_endpoint").
		Join("JOIN resource_endpoints AS wireguard ON wireguard.resource_id = resource.id AND wireguard.private_network_id IS NOT NULL AND wireguard.archived_at IS NULL AND wireguard.address NOT IN ('127.0.0.1', '::1', 'localhost')").
		Join("JOIN resource_endpoints AS origin ON origin.resource_id = resource.id AND origin.role = 'primary' AND (wireguard.role = origin.role OR wireguard.role = 'wireguard') AND origin.resource_installation_id = wireguard.resource_installation_id AND origin.archived_at IS NULL AND origin.address IN ('127.0.0.1', '::1', 'localhost') AND origin.port = wireguard.port AND origin.protocol = wireguard.protocol AND origin.tls_mode = wireguard.tls_mode AND origin.id <> wireguard.id").
		Join("JOIN resource_installations AS installation ON installation.id = wireguard.resource_installation_id AND installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Join("JOIN wireguard_peers AS peer ON peer.server_id = installation.server_id AND peer.retired_at IS NULL").
		Where("resource.id = ?", resourceID).
		Where("resource.archived_at IS NULL").
		Where("resource.management_mode = 'managed'").
		Limit(1).
		Scan(ctx, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceAccessTarget{}, ErrNotFound
	}
	return target, err
}
