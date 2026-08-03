package models

import (
	"context"
	"encoding/json"
	"time"

	"deploycrate-ce/internal/storage"
)

type SystemOverview struct {
	ApplicationName     string          `json:"applicationName"     bun:"application_name"`
	ApplicationSlug     string          `json:"applicationSlug"     bun:"application_slug"`
	EnvironmentName     string          `json:"environmentName"     bun:"environment_name"`
	EnvironmentKind     string          `json:"environmentKind"     bun:"environment_kind"`
	ServerID            string          `json:"-"                   bun:"server_id"`
	ServerName          string          `json:"serverName"          bun:"server_name"`
	ServerAddress       string          `json:"serverAddress"       bun:"server_address"`
	ServerStatus        string          `json:"serverStatus"        bun:"server_status"`
	ServerCapabilities  json.RawMessage `json:"serverCapabilities" bun:"server_capabilities"`
	OperatingSystem     string          `json:"operatingSystem"     bun:"operating_system"`
	Distribution        string          `json:"distribution"        bun:"distribution"`
	DistributionVersion string          `json:"distributionVersion" bun:"distribution_version"`
	Architecture        string          `json:"architecture"        bun:"architecture"`
	NetworkName         string          `json:"networkName"         bun:"network_name"`
	NetworkDriver       string          `json:"networkDriver"       bun:"network_driver"`
	NetworkState        string          `json:"networkState"        bun:"network_state"`
	ReleaseVersion      string          `json:"releaseVersion"      bun:"release_version"`
	ArtifactReference   string          `json:"artifactReference"   bun:"artifact_reference"`
	DeploymentStatus    string          `json:"deploymentStatus"    bun:"deployment_status"`
	DeploymentStep      string          `json:"deploymentStep"      bun:"deployment_step"`
	ActiveSlot          string          `json:"activeSlot"          bun:"active_slot"`
	ActiveService       string          `json:"activeService"       bun:"active_service"`
	ActiveState         string          `json:"activeState"         bun:"active_state"`
	ActivePort          int32           `json:"activePort"          bun:"active_port"`
	Domain              string          `json:"domain"              bun:"domain"`
	RouteExternalID     string          `json:"routeExternalId"     bun:"route_external_id"`
	RouteState          string          `json:"routeState"          bun:"route_state"`
	ObservedAt          time.Time       `json:"observedAt"          bun:"observed_at"`
}

func (a application) FindSystemOverview(
	ctx context.Context,
	db storage.Executor,
) (SystemOverview, error) {
	var overview SystemOverview
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.name AS application_name").
		ColumnExpr("application.slug AS application_slug").
		ColumnExpr("environment.name AS environment_name").
		ColumnExpr("environment.kind AS environment_kind").
		ColumnExpr("server.id::text AS server_id").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("server.address AS server_address").
		ColumnExpr("COALESCE(server_status.state, 'unknown') AS server_status").
		ColumnExpr("COALESCE(server.capabilities, '{}'::jsonb) AS server_capabilities").
		ColumnExpr("COALESCE(server.operating_system, '') AS operating_system").
		ColumnExpr("COALESCE(server.distribution, '') AS distribution").
		ColumnExpr("COALESCE(server.distribution_version, '') AS distribution_version").
		ColumnExpr("COALESCE(server.architecture, '') AS architecture").
		ColumnExpr("COALESCE(network.name, '') AS network_name").
		ColumnExpr("COALESCE(server_network.driver, '') AS network_driver").
		ColumnExpr("COALESCE(server_network.state, '') AS network_state").
		ColumnExpr("COALESCE(release.version, '') AS release_version").
		ColumnExpr("release.artifact_reference AS artifact_reference").
		ColumnExpr("deployment.status AS deployment_status").
		ColumnExpr("COALESCE(deployment.current_step, '') AS deployment_step").
		ColumnExpr("instance.slot AS active_slot").
		ColumnExpr("instance.external_id AS active_service").
		ColumnExpr("instance.state AS active_state").
		ColumnExpr("COALESCE((instance.ports ->> 'http')::integer, 0) AS active_port").
		ColumnExpr("domain.hostname AS domain").
		ColumnExpr("route.external_id AS route_external_id").
		ColumnExpr("route.state AS route_state").
		ColumnExpr("instance.observed_at AS observed_at").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN servers AS server ON server.id = target.server_id AND server.archived_at IS NULL").
		Join("LEFT JOIN LATERAL (SELECT state FROM server_statuses WHERE server_id = server.id ORDER BY observed_at DESC LIMIT 1) AS server_status ON TRUE").
		Join("JOIN environment_domains AS domain ON domain.environment_id = environment.id AND domain.is_primary = TRUE AND domain.archived_at IS NULL").
		Join("JOIN caddy_routes AS route ON route.environment_target_id = target.id AND route.environment_domain_id = domain.id AND route.removed_at IS NULL").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL AND backend.weight = 100").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Join("JOIN releases AS release ON release.id = instance.release_id").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id").
		Join("LEFT JOIN environment_networks AS environment_network ON environment_network.environment_id = environment.id AND environment_network.role = 'primary' AND environment_network.removed_at IS NULL").
		Join("LEFT JOIN private_networks AS network ON network.id = environment_network.private_network_id AND network.archived_at IS NULL").
		Join("LEFT JOIN server_networks AS server_network ON server_network.server_id = server.id AND server_network.private_network_id = network.id AND server_network.removed_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("route.created_at DESC").
		Limit(1).
		Scan(ctx, &overview); err != nil {
		return SystemOverview{}, err
	}
	return overview, nil
}

type SystemResourceOverview struct {
	ID               string `json:"id" bun:"id"`
	Name             string `json:"name" bun:"name"`
	ResourceType     string `json:"resourceType" bun:"resource_type"`
	Engine           string `json:"engine" bun:"engine"`
	SharingScope     string `json:"sharingScope" bun:"sharing_scope"`
	BindingAlias     string `json:"bindingAlias" bun:"binding_alias"`
	CredentialSource string `json:"credentialSource" bun:"credential_source"`
	HasCredential    bool   `json:"hasCredential" bun:"has_credential"`
	EndpointName     string `json:"endpointName" bun:"endpoint_name"`
	EndpointRole     string `json:"endpointRole" bun:"endpoint_role"`
	Address          string `json:"address" bun:"address"`
	Port             int32  `json:"port" bun:"port"`
	Protocol         string `json:"protocol" bun:"protocol"`
	TLSMode          string `json:"tlsMode" bun:"tls_mode"`
	External         bool   `json:"external" bun:"external"`
	HasInstallation  bool   `json:"hasInstallation" bun:"has_installation"`
	ImageReference   string `json:"imageReference" bun:"image_reference"`
	ContainerName    string `json:"containerName" bun:"container_name"`
	RestartPolicy    string `json:"restartPolicy" bun:"restart_policy"`
	Volume           string `json:"volume" bun:"volume"`
	Bind             string `json:"bind" bun:"bind"`
}

func (a application) FindSystemResources(
	ctx context.Context,
	db storage.Executor,
) ([]SystemResourceOverview, error) {
	resources := make([]SystemResourceOverview, 0)
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("resource.id::text AS id").
		ColumnExpr("resource.name AS name").
		ColumnExpr("resource.resource_type AS resource_type").
		ColumnExpr("resource.configuration ->> 'engine' AS engine").
		ColumnExpr("resource.sharing_scope AS sharing_scope").
		ColumnExpr("binding.alias AS binding_alias").
		ColumnExpr("COALESCE(binding.configuration ->> 'credential_source', '') AS credential_source").
		ColumnExpr("binding.resource_credential_id IS NOT NULL AS has_credential").
		ColumnExpr("endpoint.name AS endpoint_name").
		ColumnExpr("endpoint.role AS endpoint_role").
		ColumnExpr("endpoint.address AS address").
		ColumnExpr("endpoint.port AS port").
		ColumnExpr("endpoint.protocol AS protocol").
		ColumnExpr("endpoint.tls_mode AS tls_mode").
		ColumnExpr("COALESCE((endpoint.settings ->> 'external')::boolean, FALSE) AS external").
		ColumnExpr("installation.id IS NOT NULL AS has_installation").
		ColumnExpr("COALESCE(installation.image_reference, '') AS image_reference").
		ColumnExpr("COALESCE(installation.container_name, '') AS container_name").
		ColumnExpr("COALESCE(installation.restart_policy, '') AS restart_policy").
		ColumnExpr("COALESCE(installation.configuration ->> 'volume', '') AS volume").
		ColumnExpr("COALESCE(installation.configuration ->> 'bind', '') AS bind").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_resources AS binding ON binding.environment_id = environment.id AND binding.archived_at IS NULL").
		Join("JOIN resources AS resource ON resource.id = binding.resource_id AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.id = binding.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_installations AS installation ON installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("binding.created_at ASC").
		Scan(ctx, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

type SystemDeploymentEvent struct {
	ID         string          `json:"id" bun:"id"`
	Sequence   int64           `json:"sequence" bun:"sequence"`
	EventType  string          `json:"eventType" bun:"event_type"`
	Status     string          `json:"status" bun:"status"`
	Step       string          `json:"step" bun:"step"`
	Message    string          `json:"message" bun:"message"`
	Metadata   json.RawMessage `json:"metadata" bun:"metadata"`
	Error      string          `json:"error" bun:"error"`
	OccurredAt time.Time       `json:"occurredAt" bun:"occurred_at"`
	Deployment string          `json:"-" bun:"deployment_id"`
}

type SystemDeployment struct {
	ID                   string                  `json:"id" bun:"id"`
	CreatedAt            time.Time               `json:"createdAt" bun:"created_at"`
	UpdatedAt            time.Time               `json:"updatedAt" bun:"updated_at"`
	Attempt              int32                   `json:"attempt" bun:"attempt"`
	Strategy             json.RawMessage         `json:"strategy" bun:"strategy"`
	RuntimeConfiguration json.RawMessage         `json:"runtimeConfiguration" bun:"runtime_configuration"`
	Status               string                  `json:"status" bun:"status"`
	CurrentStep          string                  `json:"currentStep" bun:"current_step"`
	StartedAt            *time.Time              `json:"startedAt" bun:"started_at"`
	FinishedAt           *time.Time              `json:"finishedAt" bun:"finished_at"`
	Error                string                  `json:"error" bun:"error"`
	ReleaseID            string                  `json:"releaseId" bun:"release_id"`
	ReleaseVersion       string                  `json:"releaseVersion" bun:"release_version"`
	SourceRevision       string                  `json:"sourceRevision" bun:"source_revision"`
	ArtifactReference    string                  `json:"artifactReference" bun:"artifact_reference"`
	ArtifactDigest       string                  `json:"artifactDigest" bun:"artifact_digest"`
	ChangeID             string                  `json:"changeId" bun:"change_id"`
	ChangeSequence       int64                   `json:"changeSequence" bun:"change_sequence"`
	ChangeKind           string                  `json:"changeKind" bun:"change_kind"`
	ChangeSummary        string                  `json:"changeSummary" bun:"change_summary"`
	ChangeStatus         string                  `json:"changeStatus" bun:"change_status"`
	TriggerType          string                  `json:"triggerType" bun:"trigger_type"`
	RequestedAt          time.Time               `json:"requestedAt" bun:"requested_at"`
	InstanceID           string                  `json:"instanceId" bun:"instance_id"`
	InstanceService      string                  `json:"instanceService" bun:"instance_service"`
	InstanceSlot         string                  `json:"instanceSlot" bun:"instance_slot"`
	InstanceState        string                  `json:"instanceState" bun:"instance_state"`
	InstancePort         int32                   `json:"instancePort" bun:"instance_port"`
	InstanceObservedAt   *time.Time              `json:"instanceObservedAt" bun:"instance_observed_at"`
	Active               bool                    `json:"active" bun:"active"`
	Events               []SystemDeploymentEvent `json:"events" bun:"-"`
}

func (a application) FindSystemDeployments(
	ctx context.Context,
	db storage.Executor,
) ([]SystemDeployment, error) {
	deployments := make([]SystemDeployment, 0)
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("deployment.id::text AS id").
		ColumnExpr("deployment.created_at AS created_at").
		ColumnExpr("deployment.updated_at AS updated_at").
		ColumnExpr("deployment.attempt AS attempt").
		ColumnExpr("deployment.strategy AS strategy").
		ColumnExpr("deployment.runtime_configuration AS runtime_configuration").
		ColumnExpr("deployment.status AS status").
		ColumnExpr("COALESCE(deployment.current_step, '') AS current_step").
		ColumnExpr("deployment.started_at AS started_at").
		ColumnExpr("deployment.finished_at AS finished_at").
		ColumnExpr("COALESCE(deployment.error, '') AS error").
		ColumnExpr("release.id::text AS release_id").
		ColumnExpr("COALESCE(release.version, '') AS release_version").
		ColumnExpr("COALESCE(release.source_revision, '') AS source_revision").
		ColumnExpr("release.artifact_reference AS artifact_reference").
		ColumnExpr("encode(release.artifact_digest, 'hex') AS artifact_digest").
		ColumnExpr("change.id::text AS change_id").
		ColumnExpr("change.sequence AS change_sequence").
		ColumnExpr("change.kind AS change_kind").
		ColumnExpr("change.summary AS change_summary").
		ColumnExpr("change.status AS change_status").
		ColumnExpr("change.trigger_type AS trigger_type").
		ColumnExpr("change.requested_at AS requested_at").
		ColumnExpr("COALESCE(instance.id::text, '') AS instance_id").
		ColumnExpr("COALESCE(instance.external_id, '') AS instance_service").
		ColumnExpr("COALESCE(instance.slot, '') AS instance_slot").
		ColumnExpr("COALESCE(instance.state, '') AS instance_state").
		ColumnExpr("COALESCE((instance.ports ->> 'http')::integer, 0) AS instance_port").
		ColumnExpr("instance.observed_at AS instance_observed_at").
		ColumnExpr("COALESCE(instance.active, FALSE) AS active").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id").
		Join("JOIN deployments AS deployment ON deployment.environment_target_id = target.id").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Join("JOIN changes AS change ON change.id = deployment.change_id").
		Join(`LEFT JOIN LATERAL (
			SELECT candidate.*,
				EXISTS (
					SELECT 1
					FROM caddy_route_backends AS active_backend
					JOIN caddy_routes AS active_route ON active_route.id = active_backend.caddy_route_id
					WHERE active_backend.instance_id = candidate.id
						AND active_backend.weight = 100
						AND active_backend.removed_at IS NULL
						AND active_route.removed_at IS NULL
				) AS active
			FROM instances AS candidate
			WHERE candidate.deployment_id = deployment.id
			ORDER BY candidate.observed_at DESC
			LIMIT 1
		) AS instance ON TRUE`).
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("deployment.created_at DESC").
		Scan(ctx, &deployments); err != nil {
		return nil, err
	}

	events := make([]SystemDeploymentEvent, 0)
	if err := db.NewSelect().
		TableExpr("deployment_events AS event").
		ColumnExpr("event.id::text AS id").
		ColumnExpr("event.sequence AS sequence").
		ColumnExpr("event.event_type AS event_type").
		ColumnExpr("COALESCE(event.status, '') AS status").
		ColumnExpr("COALESCE(event.step, '') AS step").
		ColumnExpr("event.message AS message").
		ColumnExpr("event.metadata AS metadata").
		ColumnExpr("COALESCE(event.error, '') AS error").
		ColumnExpr("event.occurred_at AS occurred_at").
		ColumnExpr("event.deployment_id::text AS deployment_id").
		Join("JOIN deployments AS deployment ON deployment.id = event.deployment_id").
		Join("JOIN environment_targets AS target ON target.id = deployment.environment_target_id").
		Join("JOIN environments AS environment ON environment.id = target.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("event.sequence ASC").
		Scan(ctx, &events); err != nil {
		return nil, err
	}

	eventsByDeployment := make(map[string][]SystemDeploymentEvent, len(deployments))
	for _, event := range events {
		eventsByDeployment[event.Deployment] = append(eventsByDeployment[event.Deployment], event)
	}
	for index := range deployments {
		deploymentEvents := eventsByDeployment[deployments[index].ID]
		if deploymentEvents == nil {
			deploymentEvents = []SystemDeploymentEvent{}
		}
		deployments[index].Events = deploymentEvents
	}

	return deployments, nil
}

type SystemNetwork struct {
	NetworkID                 string          `json:"networkId" bun:"network_id"`
	NetworkCreatedAt          time.Time       `json:"networkCreatedAt" bun:"network_created_at"`
	NetworkUpdatedAt          time.Time       `json:"networkUpdatedAt" bun:"network_updated_at"`
	NetworkName               string          `json:"networkName" bun:"network_name"`
	OwnerEnvironmentID        string          `json:"ownerEnvironmentId" bun:"owner_environment_id"`
	EnvironmentID             string          `json:"environmentId" bun:"environment_id"`
	EnvironmentName           string          `json:"environmentName" bun:"environment_name"`
	EnvironmentBindingID      int32           `json:"environmentBindingId" bun:"environment_binding_id"`
	EnvironmentBindingRole    string          `json:"environmentBindingRole" bun:"environment_binding_role"`
	EnvironmentBindingCreated time.Time       `json:"environmentBindingCreated" bun:"environment_binding_created"`
	TargetID                  string          `json:"targetId" bun:"target_id"`
	TargetAttachedAt          time.Time       `json:"targetAttachedAt" bun:"target_attached_at"`
	ServerID                  string          `json:"serverId" bun:"server_id"`
	ServerName                string          `json:"serverName" bun:"server_name"`
	ServerAddress             string          `json:"serverAddress" bun:"server_address"`
	ServerNetworkID           int32           `json:"serverNetworkId" bun:"server_network_id"`
	ServerDriver              string          `json:"serverDriver" bun:"server_driver"`
	ServerExternalID          string          `json:"serverExternalId" bun:"server_external_id"`
	ServerConfiguration       json.RawMessage `json:"serverConfiguration" bun:"server_configuration"`
	ServerState               string          `json:"serverState" bun:"server_state"`
	ServerAppliedAt           *time.Time      `json:"serverAppliedAt" bun:"server_applied_at"`
	ServerObservedAt          *time.Time      `json:"serverObservedAt" bun:"server_observed_at"`
	ServerError               string          `json:"serverError" bun:"server_error"`
	TargetNetworkID           int32           `json:"targetNetworkId" bun:"target_network_id"`
	TargetDriver              string          `json:"targetDriver" bun:"target_driver"`
	TargetExternalID          string          `json:"targetExternalId" bun:"target_external_id"`
	TargetConfiguration       json.RawMessage `json:"targetConfiguration" bun:"target_configuration"`
	TargetState               string          `json:"targetState" bun:"target_state"`
	TargetAppliedAt           *time.Time      `json:"targetAppliedAt" bun:"target_applied_at"`
	TargetObservedAt          *time.Time      `json:"targetObservedAt" bun:"target_observed_at"`
	TargetError               string          `json:"targetError" bun:"target_error"`
	PeerID                    string          `json:"peerId" bun:"peer_id"`
	PeerPublicKey             string          `json:"peerPublicKey" bun:"peer_public_key"`
	PeerPrivateAddress        string          `json:"peerPrivateAddress" bun:"peer_private_address"`
	PeerEndpoint              string          `json:"peerEndpoint" bun:"peer_endpoint"`
	PeerListenPort            int32           `json:"peerListenPort" bun:"peer_listen_port"`
	PeerActivatedAt           *time.Time      `json:"peerActivatedAt" bun:"peer_activated_at"`
	PeerState                 string          `json:"peerState" bun:"peer_state"`
	PeerLatestHandshakeAt     *time.Time      `json:"peerLatestHandshakeAt" bun:"peer_latest_handshake_at"`
	PeerObservedAt            *time.Time      `json:"peerObservedAt" bun:"peer_observed_at"`
	PeerError                 string          `json:"peerError" bun:"peer_error"`
	Domain                    string          `json:"domain" bun:"domain"`
	RouteID                   string          `json:"routeId" bun:"route_id"`
	RouteExternalID           string          `json:"routeExternalId" bun:"route_external_id"`
	RouteState                string          `json:"routeState" bun:"route_state"`
	RouteCreatedAt            *time.Time      `json:"routeCreatedAt" bun:"route_created_at"`
	BackendWeight             int32           `json:"backendWeight" bun:"backend_weight"`
	BackendService            string          `json:"backendService" bun:"backend_service"`
	BackendState              string          `json:"backendState" bun:"backend_state"`
	BackendPort               int32           `json:"backendPort" bun:"backend_port"`
}

func (a application) FindSystemNetwork(
	ctx context.Context,
	db storage.Executor,
) (SystemNetwork, error) {
	var network SystemNetwork
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("network.id::text AS network_id").
		ColumnExpr("network.created_at AS network_created_at").
		ColumnExpr("network.updated_at AS network_updated_at").
		ColumnExpr("network.name AS network_name").
		ColumnExpr("network.owner_environment_id::text AS owner_environment_id").
		ColumnExpr("environment.id::text AS environment_id").
		ColumnExpr("environment.name AS environment_name").
		ColumnExpr("environment_network.id AS environment_binding_id").
		ColumnExpr("environment_network.role AS environment_binding_role").
		ColumnExpr("environment_network.created_at AS environment_binding_created").
		ColumnExpr("target.id::text AS target_id").
		ColumnExpr("target.attached_at AS target_attached_at").
		ColumnExpr("server.id::text AS server_id").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("server.address AS server_address").
		ColumnExpr("server_network.id AS server_network_id").
		ColumnExpr("server_network.driver AS server_driver").
		ColumnExpr("COALESCE(server_network.external_id, '') AS server_external_id").
		ColumnExpr("server_network.configuration AS server_configuration").
		ColumnExpr("server_network.state AS server_state").
		ColumnExpr("server_network.applied_at AS server_applied_at").
		ColumnExpr("server_network.observed_at AS server_observed_at").
		ColumnExpr("COALESCE(server_network.error, '') AS server_error").
		ColumnExpr("target_network.id AS target_network_id").
		ColumnExpr("target_network.driver AS target_driver").
		ColumnExpr("COALESCE(target_network.external_id, '') AS target_external_id").
		ColumnExpr("target_network.configuration AS target_configuration").
		ColumnExpr("target_network.state AS target_state").
		ColumnExpr("target_network.applied_at AS target_applied_at").
		ColumnExpr("target_network.observed_at AS target_observed_at").
		ColumnExpr("COALESCE(target_network.error, '') AS target_error").
		ColumnExpr("COALESCE(peer.id::text, '') AS peer_id").
		ColumnExpr("COALESCE(peer.public_key, '') AS peer_public_key").
		ColumnExpr("COALESCE(peer.private_address::text, '') AS peer_private_address").
		ColumnExpr("COALESCE(peer.endpoint, '') AS peer_endpoint").
		ColumnExpr("COALESCE(peer.listen_port, 0) AS peer_listen_port").
		ColumnExpr("peer.activated_at AS peer_activated_at").
		ColumnExpr("COALESCE(peer_status.state, '') AS peer_state").
		ColumnExpr("peer_status.latest_handshake_at AS peer_latest_handshake_at").
		ColumnExpr("peer_status.observed_at AS peer_observed_at").
		ColumnExpr("COALESCE(peer_status.error, '') AS peer_error").
		ColumnExpr("COALESCE(domain.hostname, '') AS domain").
		ColumnExpr("COALESCE(route.id::text, '') AS route_id").
		ColumnExpr("COALESCE(route.external_id, '') AS route_external_id").
		ColumnExpr("COALESCE(route.state, '') AS route_state").
		ColumnExpr("route.created_at AS route_created_at").
		ColumnExpr("COALESCE(backend.weight, 0) AS backend_weight").
		ColumnExpr("COALESCE(instance.external_id, '') AS backend_service").
		ColumnExpr("COALESCE(instance.state, '') AS backend_state").
		ColumnExpr("COALESCE((instance.ports ->> 'http')::integer, 0) AS backend_port").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_networks AS environment_network ON environment_network.environment_id = environment.id AND environment_network.role = 'primary' AND environment_network.removed_at IS NULL").
		Join("JOIN private_networks AS network ON network.id = environment_network.private_network_id AND network.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN servers AS server ON server.id = target.server_id AND server.archived_at IS NULL").
		Join("JOIN server_networks AS server_network ON server_network.server_id = server.id AND server_network.private_network_id = network.id AND server_network.removed_at IS NULL").
		Join("JOIN environment_target_networks AS target_network ON target_network.environment_target_id = target.id AND target_network.private_network_id = network.id AND target_network.removed_at IS NULL").
		Join("LEFT JOIN wireguard_peers AS peer ON peer.server_id = server.id AND peer.retired_at IS NULL").
		Join(`LEFT JOIN LATERAL (
			SELECT status.*
			FROM wireguard_peer_statuses AS status
			WHERE status.wireguard_peer_id = peer.id
			ORDER BY status.observed_at DESC
			LIMIT 1
		) AS peer_status ON TRUE`).
		Join("LEFT JOIN environment_domains AS domain ON domain.environment_id = environment.id AND domain.is_primary = TRUE AND domain.archived_at IS NULL").
		Join("LEFT JOIN caddy_routes AS route ON route.environment_target_id = target.id AND route.environment_domain_id = domain.id AND route.removed_at IS NULL").
		Join("LEFT JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL AND backend.weight = 100").
		Join("LEFT JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("route.created_at DESC NULLS LAST").
		Limit(1).
		Scan(ctx, &network); err != nil {
		return SystemNetwork{}, err
	}

	return network, nil
}
