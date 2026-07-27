package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const (
	bootstrapBluePort  = 8080
	bootstrapGreenPort = 8081
)

type BootstrapInput struct {
	Domain              string
	Version             string
	ArtifactReference   string
	ArtifactDigest      []byte
	Distribution        string
	DistributionVersion string
	Architecture        string
	Capabilities        BootstrapCapabilitiesInput
	DatabaseExternal    bool
	DatabaseHost        string
	DatabasePort        int
	DatabaseName        string
	DatabaseSSLMode     string
	WireGuard           BootstrapWireGuardInput
	Backup              BootstrapBackupInput
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
	return result, nil
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
		environment.ID,
		server.ID,
		network.ID,
	); err != nil {
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
	environmentID, serverID, networkID uuid.UUID,
) error {
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name: "DeployCrate CE ClickHouse", Category: "database", Kind: "clickhouse",
		SharingScope: "environment", OwnerEnvironmentID: environmentID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse resource: %w", err)
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
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name:                   "ClickHouse HTTP",
		Role:                   "primary",
		Address:                "127.0.0.1",
		Port:                   8123,
		Protocol:               "http",
		TlsMode:                "disable",
		Settings:               json.RawMessage(`{"database":"deploycrate","user":"deploycrate"}`),
		ResourceID:             resource.ID,
		ResourceInstallationID: &installation.ID,
		PrivateNetworkID:       &networkID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap ClickHouse endpoint: %w", err)
	}
	if _, err := models.EnvironmentResource.Create(
		ctx,
		exec,
		models.CreateEnvironmentResourceData{
			Alias: "telemetry",
			Configuration: json.RawMessage(
				`{"credential_source":"app_env","password_env":"CLICKHOUSE_PASSWORD"}`,
			),
			EnvironmentID:      environmentID,
			ResourceID:         resource.ID,
			ResourceEndpointID: endpoint.ID,
		},
	); err != nil {
		return fmt.Errorf("bind bootstrap ClickHouse resource: %w", err)
	}
	return nil
}

type bootstrapDatabaseTopology struct {
	ResourceID            uuid.UUID
	EnvironmentResourceID uuid.UUID
	InstallationID        *uuid.UUID
}

func createBootstrapDatabaseResource(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	environmentID, serverID, networkID uuid.UUID,
) (bootstrapDatabaseTopology, error) {
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name: "DeployCrate CE PostgreSQL", Category: "database", Kind: "postgresql",
		SharingScope: "environment", OwnerEnvironmentID: environmentID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap database resource: %w", err)
	}

	var installationID *uuid.UUID
	var endpointNetworkID *uuid.UUID
	if !input.DatabaseExternal {
		installation, err := models.ResourceInstallation.Create(
			ctx,
			exec,
			models.CreateResourceInstallationData{
				ImageReference: "postgres:17-alpine",
				ContainerName:  "deploycrate-ce-postgres",
				RestartPolicy:  "unless-stopped",
				Configuration: json.RawMessage(
					`{"volume":"deploycrate-ce-postgres","bind":"127.0.0.1:5432"}`,
				),
				ResourceID: resource.ID,
				ServerID:   serverID,
			},
		)
		if err != nil {
			return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap database installation: %w", err)
		}
		installationID = &installation.ID
		endpointNetworkID = &networkID
	}
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name:     "Primary PostgreSQL",
		Role:     "primary",
		Address:  input.DatabaseHost,
		Port:     int32(input.DatabasePort),
		Protocol: "postgresql",
		TlsMode:  input.DatabaseSSLMode,
		Settings: json.RawMessage(
			fmt.Sprintf(
				`{"database":%q,"external":%t}`,
				input.DatabaseName,
				input.DatabaseExternal,
			),
		),
		ResourceID:             resource.ID,
		ResourceInstallationID: installationID,
		PrivateNetworkID:       endpointNetworkID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("create bootstrap database endpoint: %w", err)
	}
	binding, err := models.EnvironmentResource.Create(ctx, exec, models.CreateEnvironmentResourceData{
		Alias: "database",
		Configuration: json.RawMessage(
			`{"credential_source":"app_env","credential_record":"pending_encryption_contract"}`,
		),
		EnvironmentID:      environmentID,
		ResourceID:         resource.ID,
		ResourceEndpointID: endpoint.ID,
	})
	if err != nil {
		return bootstrapDatabaseTopology{}, fmt.Errorf("bind bootstrap database resource: %w", err)
	}
	return bootstrapDatabaseTopology{
		ResourceID: resource.ID, EnvironmentResourceID: binding.ID, InstallationID: installationID,
	}, nil
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
		Name:                  "Control-plane PostgreSQL",
		Schedule:              input.Backup.DatabaseSchedule,
		Strategy:              "logical",
		Driver:                "postgresql",
		Retention:             input.Backup.DatabaseRetention,
		Format:                "tar.age",
		Verification:          json.RawMessage(`{"every_backup":true,"pg_restore_list":true}`),
		Settings:              json.RawMessage(`{"exclude_table_data":["river_*"]}`),
		TargetType:            "resource",
		ResourceID:            &database.ResourceID,
		EnvironmentResourceID: &database.EnvironmentResourceID,
		NextRunAt:             databaseNextRun,
		BackupDestinationID:   destination.ID,
	}); err != nil {
		return fmt.Errorf("create database backup policy: %w", err)
	}
	return nil
}

func validateBootstrapInput(input BootstrapInput) error {
	if strings.TrimSpace(input.Domain) == "" {
		return errors.New("bootstrap domain is required")
	}
	if strings.TrimSpace(input.ArtifactReference) == "" || len(input.ArtifactDigest) == 0 {
		return errors.New("bootstrap release artifact and digest are required")
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
