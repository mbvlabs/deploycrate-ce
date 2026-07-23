package services

import (
	"context"
	"fmt"
	"time"

	"deploycrate-ce/models"
)

type SystemOverview struct {
	ApplicationName     string    `json:"applicationName" bun:"application_name"`
	ApplicationSlug     string    `json:"applicationSlug" bun:"application_slug"`
	EnvironmentName     string    `json:"environmentName" bun:"environment_name"`
	EnvironmentKind     string    `json:"environmentKind" bun:"environment_kind"`
	ServerName          string    `json:"serverName" bun:"server_name"`
	ServerAddress       string    `json:"serverAddress" bun:"server_address"`
	ServerStatus        string    `json:"serverStatus" bun:"server_status"`
	OperatingSystem     string    `json:"operatingSystem" bun:"operating_system"`
	Distribution        string    `json:"distribution" bun:"distribution"`
	DistributionVersion string    `json:"distributionVersion" bun:"distribution_version"`
	Architecture        string    `json:"architecture" bun:"architecture"`
	NetworkName         string    `json:"networkName" bun:"network_name"`
	NetworkDriver       string    `json:"networkDriver" bun:"network_driver"`
	NetworkState        string    `json:"networkState" bun:"network_state"`
	DatabaseName        string    `json:"databaseName" bun:"database_name"`
	DatabaseKind        string    `json:"databaseKind" bun:"database_kind"`
	DatabaseAddress     string    `json:"databaseAddress" bun:"database_address"`
	DatabasePort        int32     `json:"databasePort" bun:"database_port"`
	ReleaseVersion      string    `json:"releaseVersion" bun:"release_version"`
	ArtifactReference   string    `json:"artifactReference" bun:"artifact_reference"`
	DeploymentStatus    string    `json:"deploymentStatus" bun:"deployment_status"`
	DeploymentStep      string    `json:"deploymentStep" bun:"deployment_step"`
	ActiveSlot          string    `json:"activeSlot" bun:"active_slot"`
	ActiveService       string    `json:"activeService" bun:"active_service"`
	ActiveState         string    `json:"activeState" bun:"active_state"`
	ActivePort          int32     `json:"activePort" bun:"active_port"`
	Domain              string    `json:"domain" bun:"domain"`
	RouteExternalID     string    `json:"routeExternalId" bun:"route_external_id"`
	RouteState          string    `json:"routeState" bun:"route_state"`
	ObservedAt          time.Time `json:"observedAt" bun:"observed_at"`
}

func (s *SelfUpdate) Overview(ctx context.Context) (SystemOverview, error) {
	if _, err := models.Application.FindSystem(ctx, s.db.Executor()); err != nil {
		return SystemOverview{}, fmt.Errorf("find DeployCrate CE system application: %w", err)
	}

	var overview SystemOverview
	if err := s.db.Executor().NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.name AS application_name").
		ColumnExpr("application.slug AS application_slug").
		ColumnExpr("environment.name AS environment_name").
		ColumnExpr("environment.kind AS environment_kind").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("server.address AS server_address").
		ColumnExpr("COALESCE(server_status.state, 'unknown') AS server_status").
		ColumnExpr("COALESCE(server.operating_system, '') AS operating_system").
		ColumnExpr("COALESCE(server.distribution, '') AS distribution").
		ColumnExpr("COALESCE(server.distribution_version, '') AS distribution_version").
		ColumnExpr("COALESCE(server.architecture, '') AS architecture").
		ColumnExpr("COALESCE(network.name, '') AS network_name").
		ColumnExpr("COALESCE(server_network.driver, '') AS network_driver").
		ColumnExpr("COALESCE(server_network.state, '') AS network_state").
		ColumnExpr("COALESCE(resource.name, '') AS database_name").
		ColumnExpr("COALESCE(resource.kind, '') AS database_kind").
		ColumnExpr("COALESCE(endpoint.address, '') AS database_address").
		ColumnExpr("COALESCE(endpoint.port, 0) AS database_port").
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
		Join("LEFT JOIN environment_resources AS environment_resource ON environment_resource.environment_id = environment.id AND environment_resource.archived_at IS NULL").
		Join("LEFT JOIN resources AS resource ON resource.id = environment_resource.resource_id AND resource.category = 'database' AND resource.archived_at IS NULL").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = environment_resource.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Where("application.slug = ?", models.SystemApplicationSlug).
		OrderExpr("route.created_at DESC").
		Limit(1).
		Scan(ctx, &overview); err != nil {
		return SystemOverview{}, fmt.Errorf("load DeployCrate CE system overview: %w", err)
	}

	return overview, nil
}
