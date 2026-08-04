package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	bootstrapBluePort  = 8080
	bootstrapGreenPort = 8081
)

type BootstrapInput struct {
	Domain                 string
	Version                string
	ArtifactReference      string
	ArtifactDigest         []byte
	Distribution           string
	DistributionVersion    string
	Architecture           string
	SessionEncryptionKey   string
	Capabilities           BootstrapCapabilitiesInput
	ClickHouseUser         string
	ClickHousePassword     string
	DatabaseExternal       bool
	DatabaseHost           string
	DatabasePort           int
	DatabaseName           string
	DatabaseUser           string
	DatabasePassword       string
	DatabaseSSLMode        string
	DatabaseInstallationID uuid.UUID
	WireGuard              BootstrapWireGuardInput
	Backup                 BootstrapBackupInput
}

type BootstrapCapabilitiesInput struct {
	BuildpacksPackVersion string
	CaddyVersion          string
	DockerEngineVersion   string
	ResticVersion         string
	WireGuardToolsVersion string
}

type BootstrapBackupInput struct {
	Enabled                    bool
	InstanceID                 string
	Provider                   string
	Endpoint                   string
	Region                     string
	Bucket                     string
	Prefix                     string
	ForcePathStyle             bool
	EncryptedCredentialPayload []byte
	ValidatedAt                time.Time
	ServerSchedule             string
	ServerRetention            json.RawMessage
	DatabaseSchedule           string
	DatabaseRetention          json.RawMessage
}

type BootstrapWireGuardInput struct {
	Interface           string
	NetworkCIDR         string
	PrivateAddress      string
	PublicKey           string
	EncryptedPrivateKey []byte
	Endpoint            string
	ListenPort          int
}

type BootstrapResult struct {
	ApplicationID   uuid.UUID
	EnvironmentID   uuid.UUID
	ServerID        uuid.UUID
	NetworkID       uuid.UUID
	ReleaseID       uuid.UUID
	InstanceID      uuid.UUID
	CaddyRouteID    uuid.UUID
	ExternalRouteID string
}

type bootstrapRouteService interface {
	Reconcile(context.Context, uuid.UUID) (string, error)
	ReconcileRegistry(context.Context, string, string, string, string, string) error
	Verify(context.Context, string) error
}

type BootstrapService struct {
	db     storage.Pool
	routes bootstrapRouteService
}

func NewBootstrapService(db storage.Pool, routes bootstrapRouteService) BootstrapService {
	return BootstrapService{db: db, routes: routes}
}

func (service BootstrapService) Bootstrap(
	ctx context.Context,
	input BootstrapInput,
) (BootstrapResult, error) {
	if err := validateBootstrapInput(input); err != nil {
		return BootstrapResult{}, err
	}
	capabilities, err := bootstrapServerCapabilities(input.Capabilities)
	if err != nil {
		return BootstrapResult{}, err
	}

	result, found, err := service.findExisting(ctx, input.Domain)
	if err != nil {
		return BootstrapResult{}, err
	}
	if !found {
		result, err = service.createGraph(ctx, input, capabilities)
		if err != nil {
			return BootstrapResult{}, err
		}
	} else if err := models.Server.UpdateCapabilities(
		ctx,
		service.db.Executor(),
		result.ServerID,
		capabilities,
	); err != nil {
		return BootstrapResult{}, fmt.Errorf("update bootstrap server capabilities: %w", err)
	}

	externalID, err := service.routes.Reconcile(ctx, result.CaddyRouteID)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("reconcile bootstrap Caddy route: %w", err)
	}
	result.ExternalRouteID = externalID
	if err := service.reconcileRegistryContainer(ctx); err != nil {
		return BootstrapResult{}, err
	}
	if err := service.reconcileRegistry(ctx); err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}

func (service BootstrapService) reconcileRegistryContainer(ctx context.Context) error {
	registry, err := loadManagedRegistry(ctx, service.db.Executor())
	if err != nil {
		return err
	}
	var installation models.ResourceInstallationEntity
	if err := service.db.Executor().NewSelect().Model(&installation).Where("resource_id = ?", registry.ResourceID).
		Where("archived_at IS NULL").Limit(1).Scan(ctx); err != nil {
		return fmt.Errorf("load managed registry installation: %w", err)
	}
	var mounts []struct {
		Name      string `bun:"name"`
		MountPath string `bun:"mount_path"`
		ReadOnly  bool   `bun:"read_only"`
	}
	if err := service.db.Executor().NewSelect().TableExpr("resource_volume_mounts AS mount").
		ColumnExpr("volume.name, mount.mount_path, mount.read_only").
		Join("JOIN resource_volumes AS volume ON volume.id = mount.resource_volume_id AND volume.archived_at IS NULL").
		Where("mount.resource_installation_id = ?", installation.ID).Where("mount.archived_at IS NULL").Scan(ctx, &mounts); err != nil {
		return fmt.Errorf("load managed registry volume: %w", err)
	}
	volumeMounts := make([]containerclient.VolumeMount, 0, len(mounts))
	for _, mount := range mounts {
		volumeMounts = append(volumeMounts, containerclient.VolumeMount{Name: mount.Name, MountPath: mount.MountPath, ReadOnly: mount.ReadOnly})
	}
	imageReference := installation.ImageReference
	if installation.ImageDigest.Valid && !strings.Contains(imageReference, "@") {
		imageReference += "@" + installation.ImageDigest.String
	}
	if err := (containerclient.New()).Run(ctx, containerclient.RunSpec{
		InstallationID: installation.ID.String(), ContainerName: installation.ContainerName, ImageReference: imageReference,
		RestartPolicy: installation.RestartPolicy, PortMappings: []containerclient.PortMapping{{HostPort: 5000, ContainerPort: 5000, Protocol: "tcp"}},
		VolumeMounts: volumeMounts, Environment: map[string]string{},
	}); err != nil {
		return fmt.Errorf("run managed registry container: %w", err)
	}
	return nil
}

func (service BootstrapService) reconcileRegistry(ctx context.Context) error {
	registry, err := loadManagedRegistry(ctx, service.db.Executor())
	if err != nil {
		return err
	}
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), registry.ResourceEndpointID)
	if err != nil {
		return fmt.Errorf("load managed registry endpoint: %w", err)
	}
	return service.routes.ReconcileRegistry(
		ctx, registryRouteID(registry.RouteHost), registry.RouteHost,
		fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port), registry.Username, registry.BasicAuthHash,
	)
}

type managedRegistry struct {
	ResourceID         uuid.UUID
	ResourceEndpointID uuid.UUID
	RouteHost          string
	Username           string
	BasicAuthHash      string
}

func loadManagedRegistry(ctx context.Context, db storage.Executor) (managedRegistry, error) {
	var row struct {
		ResourceID         uuid.UUID       `bun:"resource_id"`
		ResourceEndpointID uuid.UUID       `bun:"resource_endpoint_id"`
		Configuration      json.RawMessage `bun:"configuration"`
		Username           string          `bun:"username"`
		CredentialMetadata json.RawMessage `bun:"credential_metadata"`
	}
	err := db.NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("registry.resource_id, registry.configuration").
		ColumnExpr("endpoint.id AS resource_endpoint_id").
		ColumnExpr("credential.username, credential.metadata AS credential_metadata").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.system_managed = TRUE AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Scan(ctx, &row)
	if err != nil {
		return managedRegistry{}, fmt.Errorf("load managed Registry: %w", err)
	}
	var configuration struct {
		RouteHost string `json:"route_host"`
	}
	var metadata struct {
		BasicAuthHash string `json:"basic_auth_hash"`
	}
	if json.Unmarshal(row.Configuration, &configuration) != nil || strings.TrimSpace(configuration.RouteHost) == "" ||
		json.Unmarshal(row.CredentialMetadata, &metadata) != nil || strings.TrimSpace(row.Username) == "" || strings.TrimSpace(metadata.BasicAuthHash) == "" {
		return managedRegistry{}, errors.New("managed Registry authentication is invalid")
	}
	return managedRegistry{ResourceID: row.ResourceID, ResourceEndpointID: row.ResourceEndpointID, RouteHost: configuration.RouteHost, Username: row.Username, BasicAuthHash: metadata.BasicAuthHash}, nil
}

func (service BootstrapService) VerifyRoute(ctx context.Context, externalID string) error {
	return service.routes.Verify(ctx, externalID)
}

func (service BootstrapService) findExisting(
	ctx context.Context,
	domain string,
) (BootstrapResult, bool, error) {
	row, found, err := models.EnvironmentDomain.FindBootstrapGraphByHostname(
		ctx, service.db.Executor(), domain,
	)
	if err != nil {
		return BootstrapResult{}, false, fmt.Errorf("find existing bootstrap topology: %w", err)
	}
	if !found {
		return BootstrapResult{}, false, nil
	}
	return BootstrapResult{
		ApplicationID:   row.ApplicationID,
		EnvironmentID:   row.EnvironmentID,
		ServerID:        row.ServerID,
		NetworkID:       row.NetworkID,
		ReleaseID:       row.ReleaseID,
		InstanceID:      row.InstanceID,
		CaddyRouteID:    row.CaddyRouteID,
		ExternalRouteID: row.ExternalRouteID,
	}, true, nil
}

func bootstrapServerCapabilities(
	input BootstrapCapabilitiesInput,
) (json.RawMessage, error) {
	capabilities, err := json.Marshal(map[string]any{
		"build": true, "runtime": true, "resource": true, "database": true, "repository": true, "telemetry": true,
		"container_engine": map[string]string{
			"name":    "docker",
			"version": input.DockerEngineVersion,
		},
		"buildpacks": map[string]string{
			"tool":    "pack",
			"version": input.BuildpacksPackVersion,
		},
		"filesystem_backups": map[string]string{
			"tool":    "restic",
			"version": input.ResticVersion,
		},
		"proxy": map[string]string{
			"name":    "caddy",
			"version": input.CaddyVersion,
		},
		"networking": map[string]any{
			"wireguard": map[string]string{
				"tool":    "wireguard-tools",
				"version": input.WireGuardToolsVersion,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode bootstrap server capabilities: %w", err)
	}
	return capabilities, nil
}

func (service BootstrapService) createGraph(
	ctx context.Context,
	input BootstrapInput,
	capabilities json.RawMessage,
) (BootstrapResult, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("begin bootstrap topology transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	application, err := models.Application.Create(ctx, tx, models.CreateApplicationData{
		Name: "DeployCrate CE", Slug: "deploycrate-ce",
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap application: %w", err)
	}
	environment, err := models.Environment.Create(ctx, tx, models.CreateEnvironmentData{
		Name: "Production", Slug: "production", Kind: "production", ApplicationID: application.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap environment: %w", err)
	}

	server, err := models.Server.Create(ctx, tx, models.CreateServerData{
		Name:            "DeployCrate CE Server",
		Slug:            "deploycrate-ce",
		Kind:            "self_hosted",
		Capabilities:    capabilities,
		OperatingSystem: sql.NullString{String: "linux", Valid: true},
		Distribution: sql.NullString{
			String: input.Distribution,
			Valid:  input.Distribution != "",
		},
		DistributionVersion: sql.NullString{
			String: input.DistributionVersion,
			Valid:  input.DistributionVersion != "",
		},
		Architecture: sql.NullString{
			String: input.Architecture,
			Valid:  input.Architecture != "",
		},
		PackageManager: sql.NullString{String: "apt", Valid: true},
		InitSystem:     sql.NullString{String: "systemd", Valid: true},
		Ipv4Address:    "127.0.0.1",
		Ipv6Address:    "::1",
		IsConfigured:   true,
		Address:        input.Domain,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap server: %w", err)
	}
	if _, err := models.ServerStatus.Create(ctx, tx, models.CreateServerStatusData{
		State: "ready", ObservedAt: now, ServerID: server.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap server status: %w", err)
	}
	target, err := models.EnvironmentTarget.Create(ctx, tx, models.CreateEnvironmentTargetData{
		AttachedAt: now, EnvironmentID: environment.ID, ServerID: server.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap environment target: %w", err)
	}

	network, err := models.PrivateNetwork.Create(ctx, tx, models.CreatePrivateNetworkData{
		Name: "DeployCrate CE WireGuard Mesh", OwnerEnvironmentID: &environment.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap private network: %w", err)
	}
	if _, err := models.EnvironmentNetwork.Create(ctx, tx, models.CreateEnvironmentNetworkData{
		Role: "primary", EnvironmentID: environment.ID, PrivateNetworkID: network.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("attach bootstrap environment network: %w", err)
	}
	applied := sql.NullTime{Time: now, Valid: true}
	desiredPeers, err := BuildWireGuardDesiredState(nil)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("build initial WireGuard desired state: %w", err)
	}
	networkConfiguration, err := json.Marshal(map[string]any{
		"address": input.WireGuard.PrivateAddress, "cidr": input.WireGuard.NetworkCIDR,
		"endpoint": input.WireGuard.Endpoint, "interface": input.WireGuard.Interface,
		"listen_port": input.WireGuard.ListenPort, "peer_revision": desiredPeers.Revision,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("encode bootstrap WireGuard network: %w", err)
	}
	if _, err := models.ServerNetwork.Create(ctx, tx, models.CreateServerNetworkData{
		Driver:           "wireguard",
		ExternalID:       sql.NullString{String: input.WireGuard.Interface, Valid: true},
		Configuration:    networkConfiguration,
		State:            "applied",
		AppliedAt:        applied,
		ObservedAt:       applied,
		ServerID:         server.ID,
		PrivateNetworkID: network.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap server network: %w", err)
	}
	if _, err := models.EnvironmentTargetNetwork.Create(
		ctx,
		tx,
		models.CreateEnvironmentTargetNetworkData{
			Driver:              "wireguard",
			ExternalID:          sql.NullString{String: input.WireGuard.Interface, Valid: true},
			Configuration:       networkConfiguration,
			State:               "applied",
			AppliedAt:           applied,
			ObservedAt:          applied,
			EnvironmentTargetID: target.ID,
			PrivateNetworkID:    network.ID,
		},
	); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap target network: %w", err)
	}
	peer, err := models.WireGuardPeer.Create(ctx, tx, models.CreateWireGuardPeerData{
		PublicKey: input.WireGuard.PublicKey, EncPrivateKey: input.WireGuard.EncryptedPrivateKey,
		PrivateAddress: input.WireGuard.PrivateAddress,
		Endpoint:       sql.NullString{String: input.WireGuard.Endpoint, Valid: true},
		ListenPort:     int32(input.WireGuard.ListenPort), ActivatedAt: now, ServerID: server.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap WireGuard peer: %w", err)
	}
	if _, err := models.WireGuardPeerStatus.Create(ctx, tx, models.CreateWireGuardPeerStatusData{
		State: "ready", ObservedAt: now, WireguardPeerID: peer.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap WireGuard peer status: %w", err)
	}

	databaseTopology, err := createBootstrapDatabaseResource(
		ctx,
		tx,
		input,
		environment.ID,
		server.ID,
		network.ID,
	)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := createBootstrapClickHouseResource(
		ctx,
		tx,
		input,
		environment.ID,
		server.ID,
		network.ID,
	); err != nil {
		return BootstrapResult{}, err
	}
	if err := createBootstrapRegistryResource(ctx, tx, input, server.ID); err != nil {
		return BootstrapResult{}, err
	}
	if err := createBootstrapBackups(
		ctx,
		tx,
		input,
		server.ID,
		databaseTopology,
		now,
	); err != nil {
		return BootstrapResult{}, err
	}
	domain, err := models.EnvironmentDomain.Create(ctx, tx, models.CreateEnvironmentDomainData{
		Hostname: input.Domain, IsPrimary: true, EnvironmentID: environment.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap domain: %w", err)
	}

	finished := sql.NullTime{Time: now, Valid: true}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:          1,
		Kind:              "bootstrap",
		TriggerType:       "installer",
		ActorType:         "system",
		CauseSystem:       sql.NullString{String: "deploycrate-ce-cli", Valid: true},
		CauseReference:    sql.NullString{String: input.Version, Valid: input.Version != ""},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           "Bootstrap DeployCrate CE",
		Status:            "completed",
		RequestedAt:       now,
		CommittedAt:       finished,
		StartedAt:         finished,
		FinishedAt:        finished,
		EnvironmentID:     environment.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap change: %w", err)
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version:           sql.NullString{String: input.Version, Valid: input.Version != ""},
		ArtifactReference: input.ArtifactReference, ArtifactDigest: input.ArtifactDigest,
		EnvironmentID: environment.ID, CreatedByChangeID: change.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap release: %w", err)
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{
		ChangeID: change.ID, ReleaseID: release.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("associate bootstrap change and release: %w", err)
	}
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: 1,
		Strategy: json.RawMessage(
			fmt.Sprintf(
				`{"type":"blue_green","slots":{"blue":%d,"green":%d}}`,
				bootstrapBluePort,
				bootstrapGreenPort,
			),
		),
		RuntimeConfiguration: json.RawMessage(
			`{"service_template":"deploycrate-ce@.service","active_slot":"blue"}`,
		),
		Status:              "succeeded",
		CurrentStep:         sql.NullString{String: "health_check", Valid: true},
		StartedAt:           finished,
		FinishedAt:          finished,
		ChangeID:            change.ID,
		ReleaseID:           release.ID,
		EnvironmentTargetID: target.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap deployment: %w", err)
	}
	instance, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID:          "deploycrate-ce@blue.service",
		Slot:                "blue",
		ReplicaKey:          "primary",
		State:               "serving",
		Ports:               json.RawMessage(fmt.Sprintf(`{"http":%d}`, bootstrapBluePort)),
		ObservedAt:          now,
		DeploymentID:        deployment.ID,
		ReleaseID:           release.ID,
		EnvironmentTargetID: target.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap instance: %w", err)
	}
	externalRouteID := routeID(input.Domain)
	route, err := models.CaddyRoute.Create(ctx, tx, models.CreateCaddyRouteData{
		ExternalID: externalRouteID, State: "pending", EnvironmentTargetID: target.ID,
		EnvironmentDomainID: domain.ID, ReleaseID: release.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap Caddy route: %w", err)
	}
	if _, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{
		Weight: 100, CaddyRouteID: route.ID, InstanceID: instance.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap Caddy backend: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return BootstrapResult{}, fmt.Errorf("commit bootstrap topology: %w", err)
	}
	committed = true
	return BootstrapResult{
		ApplicationID: application.ID, EnvironmentID: environment.ID, ServerID: server.ID,
		NetworkID: network.ID, ReleaseID: release.ID, InstanceID: instance.ID,
		CaddyRouteID: route.ID, ExternalRouteID: externalRouteID,
	}, nil
}

func createBootstrapClickHouseResource(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	environmentID, serverID, networkID uuid.UUID,
) error {
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name:         "DeployCrate CE ClickHouse",
		Slug:         "deploycrate-ce-clickhouse",
		ResourceType: models.ResourceTypeDatabase, Configuration: json.RawMessage(`{"engine":"clickhouse","engine_version":"25.8.28.1","databases":[{"name":"deploycrate"}]}`),
		SystemManaged: true,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse resource: %w", err)
	}
	credentialPayload, err := json.Marshal(map[string]any{
		"schema_version": 1, "values": map[string]string{"password": input.ClickHousePassword},
	})
	if err != nil {
		return fmt.Errorf("encode bootstrap ClickHouse administrator credential: %w", err)
	}
	encryptedCredential, err := secretcrypto.EncryptForPurpose(
		credentialPayload,
		input.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return fmt.Errorf("encrypt bootstrap ClickHouse administrator credential: %w", err)
	}
	credentialKey, err := hex.DecodeString(input.SessionEncryptionKey)
	if err != nil || len(credentialKey) != 32 {
		return errors.New("bootstrap ClickHouse credential digest key is invalid")
	}
	credentialDigest := hmac.New(sha256.New, credentialKey)
	_, _ = credentialDigest.Write(credentialPayload)
	credential, err := models.ResourceCredential.Create(ctx, exec, models.CreateResourceCredentialData{
		Name: "Database administrator", Username: sql.NullString{String: input.ClickHouseUser, Valid: true},
		Metadata:   json.RawMessage(`{"schema_version":1,"purpose":"administrator","superuser":true}`),
		EncPayload: encryptedCredential, Digest: credentialDigest.Sum(nil), ResourceID: resource.ID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse administrator credential: %w", err)
	}
	installation, err := models.ResourceInstallation.Create(
		ctx,
		exec,
		models.CreateResourceInstallationData{
			ImageReference: "clickhouse/clickhouse-server:25.8.28.1",
			ContainerName:  "deploycrate-ce-clickhouse",
			RestartPolicy:  "unless-stopped",
			Configuration: json.RawMessage(
				`{"volume":"deploycrate-ce-clickhouse","bind":"127.0.0.1:8123"}`,
			),
			ResourceID: resource.ID,
			ServerID:   serverID,
		},
	)
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse installation: %w", err)
	}
	if err := createBootstrapResourceVolume(
		ctx,
		exec,
		resource.ID,
		installation.ID,
		serverID,
		"deploycrate-ce-clickhouse",
		"/var/lib/clickhouse",
	); err != nil {
		return fmt.Errorf("create bootstrap ClickHouse volume: %w", err)
	}
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name:             "ClickHouse HTTP",
		Role:             "primary",
		Address:          "127.0.0.1",
		Port:             8123,
		Protocol:         "http",
		TlsMode:          "disable",
		Settings:         json.RawMessage(`{"database":"deploycrate","user":"deploycrate"}`),
		ResourceID:       resource.ID,
		PrivateNetworkID: &networkID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse endpoint: %w", err)
	}
	if _, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name:             "WireGuard ClickHouse HTTP",
		Role:             "wireguard",
		Address:          WireGuardPrivateAddress,
		Port:             8123,
		Protocol:         "http",
		TlsMode:          "disable",
		Settings:         json.RawMessage(`{"database":"deploycrate","user":"deploycrate","external":false}`),
		ResourceID:       resource.ID,
		PrivateNetworkID: &networkID,
	}); err != nil {
		return fmt.Errorf("create bootstrap ClickHouse WireGuard endpoint: %w", err)
	}
	if _, err := models.EnvironmentResource.Create(
		ctx,
		exec,
		models.CreateEnvironmentResourceData{
			Alias:                "telemetry",
			Configuration:        json.RawMessage(`{"credential_source":"managed","database":"deploycrate"}`),
			EnvironmentID:        environmentID,
			ResourceID:           resource.ID,
			ResourceEndpointID:   endpoint.ID,
			ResourceCredentialID: &credential.ID,
		},
	); err != nil {
		return fmt.Errorf("bind bootstrap ClickHouse resource: %w", err)
	}
	return nil
}

const (
	registryImageReference = "registry@sha256:1be55279f18a2fe1a74edf2664cac61c1bea305b7b4642dab412e7affdcb3e33"
)

func createBootstrapRegistryResource(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	serverID uuid.UUID,
) error {
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name:         "DeployCrate CE Registry",
		Slug:         "deploycrate-ce-registry",
		ResourceType: models.ResourceTypeService, Configuration: json.RawMessage(`{"engine":"registry"}`),
		SystemManaged: true,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap registry Resource: %w", err)
	}
	registryDomain := "registry-" + strings.TrimSpace(input.Domain)
	registryConfiguration, err := json.Marshal(map[string]any{"schema_version": 1, "route_host": registryDomain})
	if err != nil {
		return fmt.Errorf("encode managed Registry configuration: %w", err)
	}
	if _, err := models.RegistryResource.Create(ctx, exec, models.CreateRegistryResourceData{
		ResourceID: resource.ID, Provider: "distribution", Configuration: registryConfiguration,
	}); err != nil {
		return fmt.Errorf("create bootstrap Registry backing: %w", err)
	}
	installation, err := models.ResourceInstallation.Create(ctx, exec, models.CreateResourceInstallationData{
		ImageReference: registryImageReference,
		ImageDigest:    sql.NullString{String: "sha256:1be55279f18a2fe1a74edf2664cac61c1bea305b7b4642dab412e7affdcb3e33", Valid: true},
		ContainerName:  "deploycrate-ce-registry", RestartPolicy: "unless-stopped",
		Configuration: json.RawMessage(`{"portMappings":[{"hostPort":5000,"containerPort":5000,"protocol":"tcp"}]}`),
		ResourceID:    resource.ID, ServerID: serverID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap registry installation: %w", err)
	}
	if err := createBootstrapResourceVolume(ctx, exec, resource.ID, installation.ID, serverID, "deploycrate-ce-registry", "/var/lib/registry"); err != nil {
		return fmt.Errorf("create bootstrap registry volume: %w", err)
	}
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name: "Registry API", Role: "primary", Address: "127.0.0.1", Port: 5000,
		Protocol: "http", TlsMode: "disable", Settings: json.RawMessage(`{"health_path":"/v2/"}`),
		ResourceID: resource.ID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap registry endpoint: %w", err)
	}
	username := "deploycrate"
	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate registry password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(passwordBytes)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash registry password: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"schema_version": 1, "values": map[string]string{"password": password},
	})
	if err != nil {
		return fmt.Errorf("encode registry credential: %w", err)
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, input.SessionEncryptionKey, registryCredentialPurpose)
	if err != nil {
		return fmt.Errorf("encrypt registry credential: %w", err)
	}
	masterKey, err := hex.DecodeString(input.SessionEncryptionKey)
	if err != nil || len(masterKey) != 32 {
		return errors.New("registry credential digest key is invalid")
	}
	digest := hmac.New(sha256.New, masterKey)
	_, _ = digest.Write(payload)
	resourceCredentialMetadata, err := json.Marshal(map[string]any{
		"schema_version": 1, "roles": []string{"push", "pull"}, "basic_auth_hash": string(passwordHash),
	})
	if err != nil {
		return fmt.Errorf("encode registry credential metadata: %w", err)
	}
	_, err = models.ResourceCredential.Create(ctx, exec, models.CreateResourceCredentialData{
		Name: "Registry publisher", Username: sql.NullString{String: username, Valid: true},
		Metadata: resourceCredentialMetadata, EncPayload: encrypted, Digest: digest.Sum(nil),
		ResourceID: resource.ID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap registry credential: %w", err)
	}
	if _, err := models.ResourceHealthCheck.Create(ctx, exec, models.CreateResourceHealthCheckData{
		Name: "Registry API", Kind: "http", Configuration: json.RawMessage(`{"path":"/v2/","expected_status":200}`),
		IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1, Enabled: true,
		ResourceID: resource.ID, ResourceEndpointID: &endpoint.ID,
	}); err != nil {
		return fmt.Errorf("create bootstrap registry health check: %w", err)
	}
	return nil
}

type bootstrapDatabaseTopology struct {
	ResourceID            uuid.UUID
	EnvironmentResourceID uuid.UUID
}

func createBootstrapDatabaseResource(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	environmentID, serverID, networkID uuid.UUID,
) (bootstrapDatabaseTopology, error) {
	configuration, err := json.Marshal(models.ResourceConfiguration{
		Engine: "postgresql", EngineVersion: "17",
		Databases: []models.ResourceDatabaseDefinition{{Name: input.DatabaseName}},
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("encode bootstrap PostgreSQL Resource configuration: %w", err)
	}
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name:         "DeployCrate CE PostgreSQL",
		Slug:         "deploycrate-ce-postgresql",
		ResourceType: models.ResourceTypeDatabase, Configuration: configuration,
		SystemManaged: true,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap PostgreSQL Resource: %w", err)
	}
	var endpointNetworkID *uuid.UUID
	if !input.DatabaseExternal {
		installation, err := models.ResourceInstallation.Create(ctx, exec, models.CreateResourceInstallationData{
			ID: input.DatabaseInstallationID, ImageReference: "postgres:17-alpine",
			ContainerName: "deploycrate-ce-postgres", RestartPolicy: "unless-stopped",
			Configuration: json.RawMessage(`{"portMappings":[{"hostPort":5432,"containerPort":5432,"protocol":"tcp"}]}`),
			ResourceID:    resource.ID, ServerID: serverID,
		})
		if err != nil {
			return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap PostgreSQL Resource installation: %w", err)
		}
		if err := createBootstrapResourceVolume(ctx, exec, resource.ID, installation.ID, serverID, "deploycrate-ce-postgres", "/var/lib/postgresql/data"); err != nil {
			return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap PostgreSQL Resource volume: %w", err)
		}
		endpointNetworkID = &networkID
	}

	credentialPayload, err := json.Marshal(map[string]any{
		"schema_version": 1, "values": map[string]string{"password": input.DatabasePassword},
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("encode bootstrap Resource administrator credential: %w", err)
	}
	encryptedCredential, err := secretcrypto.EncryptForPurpose(credentialPayload, input.SessionEncryptionKey, resourceCredentialPurpose)
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("encrypt bootstrap Resource administrator credential: %w", err)
	}
	credentialKey, err := hex.DecodeString(input.SessionEncryptionKey)
	if err != nil || len(credentialKey) != 32 {
		return bootstrapDatabaseTopology{}, errors.New("bootstrap Resource credential digest key is invalid")
	}
	credentialDigest := hmac.New(sha256.New, credentialKey)
	_, _ = credentialDigest.Write(credentialPayload)
	if _, err := models.ResourceCredential.Create(ctx, exec, models.CreateResourceCredentialData{
		Name: "Database administrator", Username: sql.NullString{String: input.DatabaseUser, Valid: true},
		Metadata: json.RawMessage(`{"schema_version":1,"purpose":"administrator","superuser":true}`), EncPayload: encryptedCredential,
		Digest: credentialDigest.Sum(nil), ResourceID: resource.ID,
	}); err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap Resource administrator credential: %w", err)
	}
	resourceCredential, err := models.ResourceCredential.Create(ctx, exec, models.CreateResourceCredentialData{
		Name: "Database user", Username: sql.NullString{String: input.DatabaseUser, Valid: true},
		Metadata:   json.RawMessage(fmt.Sprintf(`{"schema_version":1,"purpose":"application","database":%q}`, input.DatabaseName)),
		EncPayload: encryptedCredential, Digest: credentialDigest.Sum(nil), ResourceID: resource.ID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap Database Resource credential: %w", err)
	}
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name: "Primary PostgreSQL", Role: "primary", Address: input.DatabaseHost,
		Port: int32(input.DatabasePort), Protocol: "postgresql", TlsMode: input.DatabaseSSLMode,
		Settings:   json.RawMessage(`{}`),
		ResourceID: resource.ID, PrivateNetworkID: endpointNetworkID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("publish bootstrap Database Resource endpoint: %w", err)
	}
	if _, err := models.ResourceHealthCheck.Create(ctx, exec, models.CreateResourceHealthCheckData{
		Name: "PostgreSQL readiness", Kind: "postgresql", Configuration: json.RawMessage(fmt.Sprintf(`{"database":%q}`, input.DatabaseName)),
		IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1, Enabled: true,
		ResourceID: resource.ID, ResourceEndpointID: &endpoint.ID, ResourceCredentialID: &resourceCredential.ID,
	}); err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap PostgreSQL Resource health check: %w", err)
	}
	if endpointNetworkID != nil {
		_, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
			Name: "WireGuard PostgreSQL", Role: "wireguard", Address: WireGuardPrivateAddress,
			Port: int32(input.DatabasePort), Protocol: "postgresql", TlsMode: input.DatabaseSSLMode,
			Settings:   json.RawMessage(`{}`),
			ResourceID: resource.ID, PrivateNetworkID: endpointNetworkID,
		})
		if err != nil {
			return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap WireGuard PostgreSQL Resource endpoint: %w", err)
		}
	}
	binding, err := models.EnvironmentResource.Create(ctx, exec, models.CreateEnvironmentResourceData{
		Alias:                "database",
		Configuration:        json.RawMessage(fmt.Sprintf(`{"database":%q}`, input.DatabaseName)),
		EnvironmentID:        environmentID,
		ResourceID:           resource.ID,
		ResourceEndpointID:   endpoint.ID,
		ResourceCredentialID: &resourceCredential.ID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("bind bootstrap database resource: %w", err)
	}
	return bootstrapDatabaseTopology{
		ResourceID: resource.ID, EnvironmentResourceID: binding.ID,
	}, nil
}

func createBootstrapResourceVolume(
	ctx context.Context,
	exec storage.Executor,
	resourceID, installationID, serverID uuid.UUID,
	name, mountPath string,
) error {
	configuration, err := json.Marshal(map[string]string{"volume": name})
	if err != nil {
		return fmt.Errorf("encode volume configuration: %w", err)
	}
	volume, err := models.ResourceVolume.Create(ctx, exec, models.CreateResourceVolumeData{
		Name:          name,
		Driver:        "docker",
		Configuration: configuration,
		ResourceID:    resourceID,
		ServerID:      serverID,
	})
	if err != nil {
		return fmt.Errorf("create resource volume: %w", err)
	}
	if _, err := models.ResourceVolumeMount.Create(ctx, exec, models.CreateResourceVolumeMountData{
		MountPath:              mountPath,
		ReadOnly:               false,
		ResourceVolumeID:       volume.ID,
		ResourceInstallationID: installationID,
	}); err != nil {
		return fmt.Errorf("create resource volume mount: %w", err)
	}
	return nil
}

func createBootstrapBackups(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	serverID uuid.UUID,
	database bootstrapDatabaseTopology,
	now time.Time,
) error {
	if !input.Backup.Enabled {
		return nil
	}
	metadata, err := json.Marshal(map[string]any{
		"schema_version":   1,
		"credential_kind":  "object_storage_backup_access",
		"instance_id":      input.Backup.InstanceID,
		"provider":         input.Backup.Provider,
		"endpoint":         input.Backup.Endpoint,
		"region":           input.Backup.Region,
		"bucket":           input.Backup.Bucket,
		"prefix":           input.Backup.Prefix,
		"force_path_style": input.Backup.ForcePathStyle,
	})
	if err != nil {
		return err
	}
	credential, err := models.Credential.Create(ctx, exec, models.CreateCredentialData{
		Name:       "Control-plane backup storage",
		Provider:   "backup_" + input.Backup.Provider,
		Metadata:   metadata,
		EncPayload: input.Backup.EncryptedCredentialPayload,
		VerifiedAt: sql.NullTime{Time: input.Backup.ValidatedAt, Valid: !input.Backup.ValidatedAt.IsZero()},
	})
	if err != nil {
		return fmt.Errorf("create backup credential: %w", err)
	}
	destination, err := models.BackupDestination.Create(ctx, exec, models.CreateBackupDestinationData{
		Name:           "Control-plane backup storage",
		Provider:       input.Backup.Provider,
		Endpoint:       sql.NullString{String: input.Backup.Endpoint, Valid: input.Backup.Endpoint != ""},
		Region:         sql.NullString{String: input.Backup.Region, Valid: input.Backup.Region != ""},
		Bucket:         input.Backup.Bucket,
		Prefix:         sql.NullString{String: input.Backup.Prefix, Valid: input.Backup.Prefix != ""},
		ForcePathStyle: input.Backup.ForcePathStyle,
		CredentialID:   credential.ID,
	})
	if err != nil {
		return fmt.Errorf("create backup destination: %w", err)
	}
	serverNextRun, err := models.NextBackupRun(input.Backup.ServerSchedule, now)
	if err != nil {
		return fmt.Errorf("calculate first server backup: %w", err)
	}
	if _, err := models.BackupPolicy.Create(ctx, exec, models.CreateBackupPolicyData{
		Name:                "Control-plane server state",
		Schedule:            input.Backup.ServerSchedule,
		Strategy:            "filesystem",
		Driver:              "restic",
		Retention:           input.Backup.ServerRetention,
		Format:              "restic",
		Verification:        json.RawMessage(`{"snapshot":true,"manifests":true,"repository_check":"weekly","data_subset_parts":7}`),
		Settings:            json.RawMessage(`{"source_manifest_version":1,"exclude_manifest_version":1}`),
		TargetType:          "server",
		Target:              json.RawMessage(`{}`),
		ServerID:            &serverID,
		NextRunAt:           serverNextRun,
		BackupDestinationID: destination.ID,
	}); err != nil {
		return fmt.Errorf("create server backup policy: %w", err)
	}
	if input.DatabaseExternal {
		return nil
	}
	databaseNextRun, err := models.NextBackupRun(input.Backup.DatabaseSchedule, now)
	if err != nil {
		return fmt.Errorf("calculate first database backup: %w", err)
	}
	if _, err := models.BackupPolicy.Create(ctx, exec, models.CreateBackupPolicyData{
		Name: "Control-plane PostgreSQL", Schedule: input.Backup.DatabaseSchedule,
		Strategy: "logical", Driver: "postgresql", Retention: input.Backup.DatabaseRetention,
		Format: "tar.age", Verification: json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`),
		Settings: json.RawMessage(`{"exclude_table_data":["river_*"]}`), TargetType: "resource",
		Target: json.RawMessage(fmt.Sprintf(`{"database":%q}`, input.DatabaseName)), ResourceID: &database.ResourceID,
		NextRunAt: databaseNextRun, BackupDestinationID: destination.ID,
	}); err != nil {
		return fmt.Errorf("create database backup policy: %w", err)
	}
	return nil
}

func validateBootstrapInput(input BootstrapInput) error {
	if !input.DatabaseExternal && input.DatabaseInstallationID == uuid.Nil {
		return errors.New("bootstrap Database installation ID is required")
	}
	if strings.TrimSpace(input.Domain) == "" {
		return errors.New("bootstrap domain is required")
	}
	if strings.TrimSpace(input.ArtifactReference) == "" || len(input.ArtifactDigest) == 0 {
		return errors.New("bootstrap release artifact and digest are required")
	}
	key, err := hex.DecodeString(input.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return errors.New("bootstrap session encryption key must be a hex-encoded 32-byte key")
	}
	capabilityVersions := []struct {
		name    string
		version string
	}{
		{name: "Buildpacks pack", version: input.Capabilities.BuildpacksPackVersion},
		{name: "Caddy", version: input.Capabilities.CaddyVersion},
		{name: "Docker Engine", version: input.Capabilities.DockerEngineVersion},
		{name: "Restic", version: input.Capabilities.ResticVersion},
		{name: "WireGuard tools", version: input.Capabilities.WireGuardToolsVersion},
	}
	for _, capability := range capabilityVersions {
		if strings.TrimSpace(capability.version) == "" {
			return fmt.Errorf("bootstrap %s version is required", capability.name)
		}
	}
	if strings.TrimSpace(input.DatabaseHost) == "" || input.DatabasePort < 1 ||
		input.DatabasePort > 65535 {
		return errors.New("bootstrap database endpoint is invalid")
	}
	if strings.TrimSpace(input.DatabaseName) == "" || strings.TrimSpace(input.DatabaseUser) == "" || input.DatabasePassword == "" {
		return errors.New("bootstrap database identity and administrator credential are required")
	}
	if strings.TrimSpace(input.ClickHouseUser) == "" || input.ClickHousePassword == "" {
		return errors.New("bootstrap ClickHouse identity and administrator credential are required")
	}
	if strings.TrimSpace(input.WireGuard.Interface) == "" ||
		strings.TrimSpace(input.WireGuard.Endpoint) == "" {
		return errors.New("bootstrap WireGuard interface and endpoint are required")
	}
	prefix, err := netip.ParsePrefix(input.WireGuard.NetworkCIDR)
	if err != nil {
		return errors.New("bootstrap WireGuard network CIDR is invalid")
	}
	address, err := netip.ParseAddr(input.WireGuard.PrivateAddress)
	if err != nil || !prefix.Contains(address) {
		return errors.New("bootstrap WireGuard private address is invalid")
	}
	publicKey, err := base64.StdEncoding.DecodeString(input.WireGuard.PublicKey)
	if err != nil || len(publicKey) != 32 {
		return errors.New("bootstrap WireGuard public key is invalid")
	}
	if len(input.WireGuard.EncryptedPrivateKey) == 0 {
		return errors.New("bootstrap encrypted WireGuard private key is required")
	}
	if input.Backup.Enabled {
		if input.Backup.InstanceID == "" || input.Backup.Provider == "" || input.Backup.Region == "" ||
			input.Backup.Bucket == "" || len(input.Backup.EncryptedCredentialPayload) == 0 ||
			input.Backup.ValidatedAt.IsZero() || input.Backup.ServerSchedule == "" ||
			len(input.Backup.ServerRetention) == 0 {
			return errors.New("bootstrap backup configuration is incomplete")
		}
		if !input.DatabaseExternal &&
			(input.Backup.DatabaseSchedule == "" || len(input.Backup.DatabaseRetention) == 0) {
			return errors.New("bootstrap database backup policy is incomplete")
		}
	}
	if input.WireGuard.ListenPort < 1 || input.WireGuard.ListenPort > 65535 {
		return errors.New("bootstrap WireGuard listen port is invalid")
	}
	return nil
}

func routeID(domain string) string {
	id := strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToLower(domain))
	return "deploycrate_ce_" + id
}

func registryRouteID(domain string) string {
	id := strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToLower(domain))
	return "deploycrate_registry_" + id
}
