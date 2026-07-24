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
	DatabaseExternal    bool
	DatabaseHost        string
	DatabasePort        int
	DatabaseName        string
	DatabaseSSLMode     string
	WireGuard           BootstrapWireGuardInput
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

func (service BootstrapService) Bootstrap(ctx context.Context, input BootstrapInput) (BootstrapResult, error) {
	if err := validateBootstrapInput(input); err != nil {
		return BootstrapResult{}, err
	}

	result, found, err := service.findExisting(ctx, input.Domain)
	if err != nil {
		return BootstrapResult{}, err
	}
	if !found {
		result, err = service.createGraph(ctx, input)
		if err != nil {
			return BootstrapResult{}, err
		}
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

func (service BootstrapService) findExisting(ctx context.Context, domain string) (BootstrapResult, bool, error) {
	type graphRow struct {
		ApplicationID   uuid.UUID `bun:"application_id"`
		EnvironmentID   uuid.UUID `bun:"environment_id"`
		ServerID        uuid.UUID `bun:"server_id"`
		NetworkID       uuid.UUID `bun:"network_id"`
		ReleaseID       uuid.UUID `bun:"release_id"`
		InstanceID      uuid.UUID `bun:"instance_id"`
		CaddyRouteID    uuid.UUID `bun:"caddy_route_id"`
		ExternalRouteID string    `bun:"external_route_id"`
	}

	var row graphRow
	err := service.db.Executor().NewSelect().
		TableExpr("caddy_routes AS route").
		ColumnExpr("environment.application_id AS application_id").
		ColumnExpr("environment.id AS environment_id").
		ColumnExpr("target.server_id AS server_id").
		ColumnExpr("network.id AS network_id").
		ColumnExpr("route.release_id AS release_id").
		ColumnExpr("backend.instance_id AS instance_id").
		ColumnExpr("route.id AS caddy_route_id").
		ColumnExpr("route.external_id AS external_route_id").
		Join("JOIN environment_domains AS domain ON domain.id = route.environment_domain_id").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL").
		Join("JOIN private_networks AS network ON network.owner_environment_id = environment.id AND network.archived_at IS NULL").
		Where("domain.hostname = ?", domain).
		Where("domain.archived_at IS NULL").
		Where("route.removed_at IS NULL").
		OrderExpr("backend.id ASC").
		Limit(1).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return BootstrapResult{}, false, nil
	}
	if err != nil {
		return BootstrapResult{}, false, fmt.Errorf("find existing bootstrap topology: %w", err)
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

func (service BootstrapService) createGraph(ctx context.Context, input BootstrapInput) (BootstrapResult, error) {
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
		Name: "DeployCrate CE Server", Slug: "deploycrate-ce", Kind: "self_hosted",
		Capabilities:        json.RawMessage(fmt.Sprintf(`{"runtime":"systemd","proxy":"caddy","wireguard":true,"deployment_strategies":["blue_green"],"slots":{"blue":%d,"green":%d}}`, bootstrapBluePort, bootstrapGreenPort)),
		OperatingSystem:     sql.NullString{String: "linux", Valid: true},
		Distribution:        sql.NullString{String: input.Distribution, Valid: input.Distribution != ""},
		DistributionVersion: sql.NullString{String: input.DistributionVersion, Valid: input.DistributionVersion != ""},
		Architecture:        sql.NullString{String: input.Architecture, Valid: input.Architecture != ""},
		PackageManager:      sql.NullString{String: "apt", Valid: true},
		InitSystem:          sql.NullString{String: "systemd", Valid: true},
		Ipv4Address:         "127.0.0.1",
		Ipv6Address:         "::1",
		IsConfigured:        true, Address: input.Domain,
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
	networkConfiguration, err := json.Marshal(map[string]any{
		"address": input.WireGuard.PrivateAddress, "cidr": input.WireGuard.NetworkCIDR,
		"endpoint": input.WireGuard.Endpoint, "interface": input.WireGuard.Interface,
		"listen_port": input.WireGuard.ListenPort,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("encode bootstrap WireGuard network: %w", err)
	}
	if _, err := models.ServerNetwork.Create(ctx, tx, models.CreateServerNetworkData{
		Driver: "wireguard", ExternalID: sql.NullString{String: input.WireGuard.Interface, Valid: true}, Configuration: networkConfiguration,
		State: "applied", AppliedAt: applied, ObservedAt: applied, ServerID: server.ID, PrivateNetworkID: network.ID,
	}); err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap server network: %w", err)
	}
	if _, err := models.EnvironmentTargetNetwork.Create(ctx, tx, models.CreateEnvironmentTargetNetworkData{
		Driver: "wireguard", ExternalID: sql.NullString{String: input.WireGuard.Interface, Valid: true}, Configuration: networkConfiguration,
		State: "applied", AppliedAt: applied, ObservedAt: applied, EnvironmentTargetID: target.ID, PrivateNetworkID: network.ID,
	}); err != nil {
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

	if err := createBootstrapDatabaseResource(ctx, tx, input, environment.ID, server.ID, network.ID); err != nil {
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
		Sequence: 1, Kind: "bootstrap", TriggerType: "installer", ActorType: "system",
		CauseSystem:    sql.NullString{String: "deploycrate-ce-cli", Valid: true},
		CauseReference: sql.NullString{String: input.Version, Valid: input.Version != ""},
		CorrelationID:  uuid.New(), CorrectionContext: json.RawMessage(`{}`),
		Summary: "Bootstrap DeployCrate CE", Status: "completed", RequestedAt: now,
		CommittedAt: finished, StartedAt: finished, FinishedAt: finished, EnvironmentID: environment.ID,
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
		Attempt:              1,
		Strategy:             json.RawMessage(fmt.Sprintf(`{"type":"blue_green","slots":{"blue":%d,"green":%d}}`, bootstrapBluePort, bootstrapGreenPort)),
		RuntimeConfiguration: json.RawMessage(`{"service_template":"deploycrate-ce@.service","active_slot":"blue"}`),
		Status:               "succeeded", CurrentStep: sql.NullString{String: "health_check", Valid: true},
		StartedAt: finished, FinishedAt: finished, ChangeID: change.ID, ReleaseID: release.ID, EnvironmentTargetID: target.ID,
	})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("create bootstrap deployment: %w", err)
	}
	instance, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID: "deploycrate-ce@blue.service", Slot: "blue", ReplicaKey: "primary", State: "serving",
		Ports: json.RawMessage(fmt.Sprintf(`{"http":%d}`, bootstrapBluePort)), ObservedAt: now,
		DeploymentID: deployment.ID, ReleaseID: release.ID, EnvironmentTargetID: target.ID,
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

func createBootstrapDatabaseResource(
	ctx context.Context,
	exec storage.Executor,
	input BootstrapInput,
	environmentID, serverID, networkID uuid.UUID,
) error {
	resource, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name: "DeployCrate CE PostgreSQL", Category: "database", Kind: "postgresql",
		SharingScope: "environment", OwnerEnvironmentID: environmentID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap database resource: %w", err)
	}

	var installationID *uuid.UUID
	var endpointNetworkID *uuid.UUID
	if !input.DatabaseExternal {
		installation, err := models.ResourceInstallation.Create(ctx, exec, models.CreateResourceInstallationData{
			ImageReference: "postgres:17-alpine", ContainerName: "deploycrate-ce-postgres", RestartPolicy: "unless-stopped",
			Configuration: json.RawMessage(`{"volume":"deploycrate-ce-postgres","bind":"127.0.0.1:5432"}`),
			ResourceID:    resource.ID, ServerID: serverID,
		})
		if err != nil {
			return fmt.Errorf("create bootstrap database installation: %w", err)
		}
		installationID = &installation.ID
		endpointNetworkID = &networkID
	}
	endpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name: "Primary PostgreSQL", Role: "primary", Address: input.DatabaseHost, Port: int32(input.DatabasePort),
		Protocol: "postgresql", TlsMode: input.DatabaseSSLMode,
		Settings:   json.RawMessage(fmt.Sprintf(`{"database":%q,"external":%t}`, input.DatabaseName, input.DatabaseExternal)),
		ResourceID: resource.ID, ResourceInstallationID: installationID, PrivateNetworkID: endpointNetworkID,
	})
	if err != nil {
		return fmt.Errorf("create bootstrap database endpoint: %w", err)
	}
	if _, err := models.EnvironmentResource.Create(ctx, exec, models.CreateEnvironmentResourceData{
		Alias:         "database",
		Configuration: json.RawMessage(`{"credential_source":"app_env","credential_record":"pending_encryption_contract"}`),
		EnvironmentID: environmentID, ResourceID: resource.ID, ResourceEndpointID: endpoint.ID,
	}); err != nil {
		return fmt.Errorf("bind bootstrap database resource: %w", err)
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
	if strings.TrimSpace(input.DatabaseHost) == "" || input.DatabasePort < 1 || input.DatabasePort > 65535 {
		return errors.New("bootstrap database endpoint is invalid")
	}
	if strings.TrimSpace(input.WireGuard.Interface) == "" || strings.TrimSpace(input.WireGuard.Endpoint) == "" {
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
	if input.WireGuard.ListenPort < 1 || input.WireGuard.ListenPort > 65535 {
		return errors.New("bootstrap WireGuard listen port is invalid")
	}
	return nil
}

func routeID(domain string) string {
	id := strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToLower(domain))
	return "deploycrate_ce_" + id
}
