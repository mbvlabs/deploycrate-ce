package seeds

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var systemServerCapabilities = json.RawMessage(`{"build":true,"runtime":true,"resource":true,"database":true,"repository":true,"telemetry":true}`)

func ensureSystemApplication(ctx context.Context, exec storage.Executor, now time.Time) error {
	if db, ok := exec.(*bun.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin DeployCrate CE system seed: %w", err)
		}
		defer tx.Rollback()
		if err := ensureSystemApplicationInTransaction(ctx, tx, now); err != nil {
			return err
		}
		return tx.Commit()
	}
	return ensureSystemApplicationInTransaction(ctx, exec, now)
}

func ensureSystemApplicationInTransaction(ctx context.Context, exec storage.Executor, now time.Time) error {
	if _, err := models.Application.FindSystem(ctx, exec); err == nil {
		if _, err := exec.NewUpdate().TableExpr("servers").Set("capabilities = ?", systemServerCapabilities).
			Set("updated_at = ?", now).Where("slug = ?", models.SystemApplicationSlug).
			Where("kind = 'self_hosted'").Where("archived_at IS NULL").Exec(ctx); err != nil {
			return fmt.Errorf("update DeployCrate CE system server capabilities: %w", err)
		}
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find DeployCrate CE system application: %w", err)
	}

	return createSystemApplication(ctx, exec, now)
}

func createSystemApplication(ctx context.Context, exec storage.Executor, now time.Time) error {
	appliedAt := sql.NullTime{Time: now, Valid: true}

	server, err := factories.CreateServer(
		ctx,
		exec,
		factories.WithServersName("DeployCrate CE Server"),
		factories.WithServersSlug(models.SystemApplicationSlug),
		factories.WithServersKind("self_hosted"),
		factories.WithServersCapabilities(systemServerCapabilities),
		factories.WithServersOperatingSystem(sql.NullString{String: "linux", Valid: true}),
		factories.WithServersDistribution(sql.NullString{String: "ubuntu", Valid: true}),
		factories.WithServersDistributionVersion(sql.NullString{String: "24.04", Valid: true}),
		factories.WithServersArchitecture(sql.NullString{String: "amd64", Valid: true}),
		factories.WithServersPackageManager(sql.NullString{String: "apt", Valid: true}),
		factories.WithServersInitSystem(sql.NullString{String: "systemd", Valid: true}),
		factories.WithServersIPv4Address("127.0.0.1"),
		factories.WithServersIPv6Address("::1"),
		factories.WithServersIsConfigured(true),
		factories.WithServersAddress("127.0.0.1"),
		factories.WithServersArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE system server: %w", err)
	}
	if _, err := factories.CreateServerStatus(ctx, exec, server.ID,
		factories.WithServerStatusesState("ready"),
		factories.WithServerStatusesObservedAt(now),
	); err != nil {
		return fmt.Errorf("create DeployCrate CE server status: %w", err)
	}

	application, err := factories.CreateApplication(ctx, exec,
		factories.WithApplicationsName("DeployCrate CE"),
		factories.WithApplicationsSlug(models.SystemApplicationSlug),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE system application: %w", err)
	}
	environment, err := factories.CreateEnvironment(ctx, exec, application.ID,
		factories.WithEnvironmentsName("Production"),
		factories.WithEnvironmentsSlug("production"),
		factories.WithEnvironmentsKind("production"),
		factories.WithEnvironmentsAPITokenPrefix(sql.NullString{}),
		factories.WithEnvironmentsAPITokenDigest([]byte{}),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE environment: %w", err)
	}
	target, err := factories.CreateEnvironmentTarget(ctx, exec, environment.ID, server.ID,
		factories.WithEnvironmentTargetsAttachedAt(now),
		factories.WithEnvironmentTargetsDetachedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE environment target: %w", err)
	}

	network, err := factories.CreatePrivateNetwork(ctx, exec, &environment.ID,
		factories.WithPrivateNetworksName("DeployCrate CE Host Network"),
		factories.WithPrivateNetworksArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE network: %w", err)
	}
	if _, err := factories.CreateEnvironmentNetwork(ctx, exec, environment.ID, network.ID,
		factories.WithEnvironmentNetworksRole("primary"),
		factories.WithEnvironmentNetworksRemovedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("attach DeployCrate CE environment network: %w", err)
	}
	if _, err := factories.CreateServerNetwork(ctx, exec,
		sql.NullString{String: "host", Valid: true}, server.ID, network.ID,
		factories.WithServerNetworksDriver("host"),
		factories.WithServerNetworksConfiguration(json.RawMessage(`{}`)),
		factories.WithServerNetworksState("applied"),
		factories.WithServerNetworksAppliedAt(appliedAt),
		factories.WithServerNetworksObservedAt(appliedAt),
		factories.WithServerNetworksError(sql.NullString{}),
		factories.WithServerNetworksRemovedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("create DeployCrate CE server network: %w", err)
	}
	if _, err := factories.CreateEnvironmentTargetNetwork(ctx, exec,
		sql.NullString{String: "host", Valid: true}, target.ID, network.ID,
		factories.WithEnvironmentTargetNetworksDriver("host"),
		factories.WithEnvironmentTargetNetworksConfiguration(json.RawMessage(`{}`)),
		factories.WithEnvironmentTargetNetworksState("applied"),
		factories.WithEnvironmentTargetNetworksAppliedAt(appliedAt),
		factories.WithEnvironmentTargetNetworksObservedAt(appliedAt),
		factories.WithEnvironmentTargetNetworksError(sql.NullString{}),
		factories.WithEnvironmentTargetNetworksRemovedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("create DeployCrate CE target network: %w", err)
	}

	resource, err := factories.CreateResource(ctx, exec,
		factories.WithResourcesName("DeployCrate CE PostgreSQL"),
		factories.WithResourcesSlug("deploycrate-ce-postgresql"),
		factories.WithResourcesResourceType(models.ResourceTypeDatabase),
		factories.WithResourcesConfiguration(json.RawMessage(`{"engine":"postgresql","databases":[{"name":"deploycrate"}]}`)),
		factories.WithResourcesSystemManaged(true),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE Database Resource: %w", err)
	}
	installation, err := factories.CreateResourceInstallation(ctx, exec, resource.ID, server.ID, nil,
		factories.WithResourceInstallationsImageReference("postgres:17-alpine"),
		factories.WithResourceInstallationsImageDigest(sql.NullString{}),
		factories.WithResourceInstallationsContainerName("deploycrate-ce-postgres"),
		factories.WithResourceInstallationsRestartPolicy("unless-stopped"),
		factories.WithResourceInstallationsConfiguration(json.RawMessage(`{"ports":[{"hostPort":5432,"containerPort":5432,"protocol":"tcp"}]}`)),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE PostgreSQL installation: %w", err)
	}
	volume, err := factories.CreateResourceVolume(ctx, exec, resource.ID, server.ID,
		factories.WithResourceVolumesName("PostgreSQL data"),
		factories.WithResourceVolumesDriver("docker"),
		factories.WithResourceVolumesConfiguration(json.RawMessage(`{"volume":"deploycrate-ce-postgres"}`)),
		factories.WithResourceVolumesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE PostgreSQL volume: %w", err)
	}
	if _, err := factories.CreateResourceVolumeMount(ctx, exec, volume.ID, installation.ID,
		factories.WithResourceVolumeMountsMountPath("/var/lib/postgresql/data"),
		factories.WithResourceVolumeMountsArchivedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("mount DeployCrate CE PostgreSQL volume: %w", err)
	}
	endpoint, err := factories.CreateResourceEndpoint(ctx, exec, resource.ID, &network.ID,
		factories.WithResourceEndpointsName("Primary PostgreSQL"), factories.WithResourceEndpointsRole("primary"),
		factories.WithResourceEndpointsAddress("127.0.0.1"), factories.WithResourceEndpointsPort(5432),
		factories.WithResourceEndpointsProtocol("postgresql"), factories.WithResourceEndpointsTlsMode("disable"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"database":"deploycrate"}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE Database Resource endpoint: %w", err)
	}
	_, err = factories.CreateResourceEndpoint(ctx, exec, resource.ID, &network.ID,
		factories.WithResourceEndpointsName("WireGuard PostgreSQL"), factories.WithResourceEndpointsRole("wireguard"),
		factories.WithResourceEndpointsAddress("10.99.0.1"), factories.WithResourceEndpointsPort(5432),
		factories.WithResourceEndpointsProtocol("postgresql"), factories.WithResourceEndpointsTlsMode("disable"),
		factories.WithResourceEndpointsSettings(json.RawMessage(`{"database":"deploycrate"}`)),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE WireGuard Database Resource endpoint: %w", err)
	}
	if _, err := factories.CreateEnvironmentResource(
		ctx,
		exec,
		environment.ID,
		resource.ID,
		endpoint.ID,
		nil,
		factories.WithEnvironmentResourcesAlias("database"),
		factories.WithEnvironmentResourcesConfiguration(
			json.RawMessage(`{"credential_source":"app_env","credential_record":"seeded"}`),
		),
		factories.WithEnvironmentResourcesArchivedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("bind DeployCrate CE database resource: %w", err)
	}

	clickHouse, err := factories.CreateResource(ctx, exec,
		factories.WithResourcesName("DeployCrate CE ClickHouse"),
		factories.WithResourcesSlug("deploycrate-ce-clickhouse"),
		factories.WithResourcesResourceType(models.ResourceTypeDatabase),
		factories.WithResourcesConfiguration(json.RawMessage(`{"engine":"clickhouse","databases":[{"name":"deploycrate"}]}`)),
		factories.WithResourcesSystemManaged(true),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE ClickHouse resource: %w", err)
	}
	_, err = factories.CreateResourceInstallation(
		ctx,
		exec,
		clickHouse.ID,
		server.ID,
		nil,
		factories.WithResourceInstallationsImageReference(
			"clickhouse/clickhouse-server:25.8.28.1",
		),
		factories.WithResourceInstallationsImageDigest(sql.NullString{}),
		factories.WithResourceInstallationsContainerName("deploycrate-ce-clickhouse"),
		factories.WithResourceInstallationsRestartPolicy("unless-stopped"),
		factories.WithResourceInstallationsConfiguration(
			json.RawMessage(
				`{"volume":"deploycrate-ce-clickhouse","bind":"127.0.0.1:8123"}`,
			),
		),
		factories.WithResourceInstallationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE ClickHouse installation: %w", err)
	}
	clickHouseEndpoint, err := factories.CreateResourceEndpoint(
		ctx,
		exec,
		clickHouse.ID,
		&network.ID,
		factories.WithResourceEndpointsName("ClickHouse HTTP"),
		factories.WithResourceEndpointsRole("primary"),
		factories.WithResourceEndpointsAddress("127.0.0.1"),
		factories.WithResourceEndpointsPort(8123),
		factories.WithResourceEndpointsProtocol("http"),
		factories.WithResourceEndpointsTlsMode("disable"),
		factories.WithResourceEndpointsSettings(
			json.RawMessage(`{"database":"deploycrate","user":"deploycrate"}`),
		),
		factories.WithResourceEndpointsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE ClickHouse endpoint: %w", err)
	}
	if _, err := factories.CreateEnvironmentResource(
		ctx,
		exec,
		environment.ID,
		clickHouse.ID,
		clickHouseEndpoint.ID,
		nil,
		factories.WithEnvironmentResourcesAlias("telemetry-storage"),
		factories.WithEnvironmentResourcesConfiguration(
			json.RawMessage(
				`{"credential_source":"app_env","password_env":"CLICKHOUSE_PASSWORD"}`,
			),
		),
		factories.WithEnvironmentResourcesArchivedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("bind DeployCrate CE ClickHouse resource: %w", err)
	}

	telemetry, err := models.Resource.Create(ctx, exec, models.CreateResourceData{
		Name: "DeployCrate Telemetry", Slug: "deploycrate-telemetry", ResourceType: models.ResourceTypeService,
		Configuration: json.RawMessage(`{"engine":"opentelemetry"}`), SystemManaged: true,
	})
	if err != nil {
		return fmt.Errorf("create DeployCrate Telemetry Resource: %w", err)
	}
	localTelemetryEndpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name: "Control-plane OTLP HTTP", Role: "local", Address: "127.0.0.1", Port: 4318,
		Protocol: "http", TlsMode: "disable",
		Settings:   json.RawMessage(`{"exposure":"system","transport":"http/protobuf","authentication":"none"}`),
		ResourceID: telemetry.ID,
	})
	if err != nil {
		return fmt.Errorf("create local DeployCrate Telemetry endpoint: %w", err)
	}
	wireGuardTelemetryEndpoint, err := models.ResourceEndpoint.Create(ctx, exec, models.CreateResourceEndpointData{
		Name: "WireGuard OTLP HTTP", Role: "wireguard", Address: "10.99.0.1", Port: 4318,
		Protocol: "http", TlsMode: "disable",
		Settings:   json.RawMessage(`{"exposure":"environment","transport":"http/protobuf","authentication":"signed_identity"}`),
		ResourceID: telemetry.ID, PrivateNetworkID: &network.ID,
	})
	if err != nil {
		return fmt.Errorf("create WireGuard DeployCrate Telemetry endpoint: %w", err)
	}
	if _, err := models.ResourceHealthCheck.Create(ctx, exec, models.CreateResourceHealthCheckData{
		Name: "OTLP receiver", Kind: "tcp", Configuration: json.RawMessage(`{}`),
		IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1,
		Enabled: true, ResourceID: telemetry.ID, ResourceEndpointID: &wireGuardTelemetryEndpoint.ID,
	}); err != nil {
		return fmt.Errorf("create DeployCrate Telemetry health check: %w", err)
	}
	if _, err := models.EnvironmentResource.Create(ctx, exec, models.CreateEnvironmentResourceData{
		Alias:         "telemetry",
		Configuration: json.RawMessage(`{"schema_version":1,"credential_source":"none","credential_projection":"individual_parts","environment_keys":{"endpoint":"OTEL_EXPORTER_OTLP_ENDPOINT","protocol":"OTEL_EXPORTER_OTLP_PROTOCOL","headers":"OTEL_EXPORTER_OTLP_HEADERS"}}`),
		EnvironmentID: environment.ID, ResourceID: telemetry.ID, ResourceEndpointID: localTelemetryEndpoint.ID,
	}); err != nil {
		return fmt.Errorf("bind DeployCrate Telemetry Resource: %w", err)
	}

	domain, err := factories.CreateEnvironmentDomain(ctx, exec, environment.ID,
		factories.WithEnvironmentDomainsHostname("deploycrate.localhost"),
		factories.WithEnvironmentDomainsIsPrimary(true),
		factories.WithEnvironmentDomainsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE domain: %w", err)
	}
	change, err := factories.CreateChange(ctx, exec, nil, uuid.New(), environment.ID, nil,
		factories.WithChangesSequence(1),
		factories.WithChangesKind("bootstrap"),
		factories.WithChangesTriggerType("installer"),
		factories.WithChangesActorType("system"),
		factories.WithChangesCauseSystem(sql.NullString{String: "deploycrate-ce-cli", Valid: true}),
		factories.WithChangesCauseReference(sql.NullString{String: "1.4.0", Valid: true}),
		factories.WithChangesCorrectionContext(json.RawMessage(`{}`)),
		factories.WithChangesSummary("Bootstrap DeployCrate CE"),
		factories.WithChangesStatus("completed"),
		factories.WithChangesRequestedAt(now),
		factories.WithChangesCommittedAt(appliedAt),
		factories.WithChangesStartedAt(appliedAt),
		factories.WithChangesFinishedAt(appliedAt),
		factories.WithChangesCancelledAt(sql.NullTime{}),
		factories.WithChangesError(sql.NullString{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE bootstrap change: %w", err)
	}
	release, err := factories.CreateRelease(
		ctx,
		exec,
		environment.ID,
		nil,
		nil,
		change.ID,
		factories.WithReleasesVersion(sql.NullString{String: "1.4.0", Valid: true}),
		factories.WithReleasesSourceRevision(sql.NullString{String: "seeded", Valid: true}),
		factories.WithReleasesArtifactReference(
			"/opt/deploycrate-ce/releases/1.4.0/deploycrate-ce",
		),
		factories.WithReleasesArtifactDigest([]byte("0123456789abcdef0123456789abcdef")),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE release: %w", err)
	}
	if _, err := factories.CreateChangeRelease(ctx, exec, change.ID, release.ID); err != nil {
		return fmt.Errorf("associate DeployCrate CE release: %w", err)
	}
	deployment, err := factories.CreateDeployment(
		ctx,
		exec,
		change.ID,
		release.ID,
		target.ID,
		factories.WithDeploymentsAttempt(1),
		factories.WithDeploymentsStrategy(
			json.RawMessage(`{"type":"blue_green","slots":{"blue":8080,"green":8081}}`),
		),
		factories.WithDeploymentsRuntimeConfiguration(
			json.RawMessage(`{"service_template":"deploycrate-ce@.service","active_slot":"blue"}`),
		),
		factories.WithDeploymentsStatus("succeeded"),
		factories.WithDeploymentsCurrentStep(sql.NullString{String: "health_check", Valid: true}),
		factories.WithDeploymentsStartedAt(appliedAt),
		factories.WithDeploymentsFinishedAt(appliedAt),
		factories.WithDeploymentsError(sql.NullString{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE deployment: %w", err)
	}
	instance, err := factories.CreateInstance(
		ctx,
		exec,
		"deploycrate-ce@blue.service",
		deployment.ID,
		release.ID,
		target.ID,
		factories.WithInstancesSlot("blue"),
		factories.WithInstancesReplicaKey("primary"),
		factories.WithInstancesState("serving"),
		factories.WithInstancesPorts(json.RawMessage(`{"http":8080}`)),
		factories.WithInstancesObservedAt(now),
		factories.WithInstancesRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE instance: %w", err)
	}
	route, err := factories.CreateCaddyRoute(
		ctx,
		exec,
		"deploycrate_ce_deploycrate_localhost",
		target.ID,
		domain.ID,
		release.ID,
		factories.WithCaddyRoutesState("applied"),
		factories.WithCaddyRoutesAppliedAt(appliedAt),
		factories.WithCaddyRoutesObservedAt(appliedAt),
		factories.WithCaddyRoutesRemovedAt(sql.NullTime{}),
	)
	if err != nil {
		return fmt.Errorf("create DeployCrate CE Caddy route: %w", err)
	}
	if _, err := factories.CreateCaddyRouteBackend(ctx, exec, route.ID, instance.ID,
		factories.WithCaddyRouteBackendsWeight(100),
		factories.WithCaddyRouteBackendsRemovedAt(sql.NullTime{}),
	); err != nil {
		return fmt.Errorf("create DeployCrate CE Caddy backend: %w", err)
	}

	return nil
}
