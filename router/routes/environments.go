package routes

import (
	"deploycrate-ce/internal/routing"
)

const EnvironmentsPrefix = "/environments"

var Environments = routing.NewSimpleRoute("", "environments.index", EnvironmentsPrefix)

type EnvironmentParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
}

type ApplicationEnvironmentParams struct {
	ApplicationID string `param:"applicationID"`
}

type EnvironmentSecretParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	SecretID      string `param:"secretID"`
}

type EnvironmentDeploymentParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	DeploymentID  string `param:"deploymentID"`
}

type EnvironmentReleaseParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	ReleaseID     string `param:"releaseID"`
}

type EnvironmentDeploymentEventParams struct {
	EnvironmentID string `param:"environmentID"`
	DeploymentID  string `param:"deploymentID"`
}

type EnvironmentBuildParams struct {
	EnvironmentID string `param:"environmentID"`
	BuildID       string `param:"buildID"`
}

type EnvironmentBuildActionParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	BuildID       string `param:"buildID"`
}

type EnvironmentReleaseCommandParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	ExecutionID   string `param:"executionID"`
}

type EnvironmentTelemetryTraceParams struct {
	ApplicationID string `param:"applicationID"`
	EnvironmentID string `param:"environmentID"`
	TraceID       string `param:"traceID"`
}

var EnvironmentNew = routing.NewRouteWithParams[ApplicationEnvironmentParams](
	"/:applicationID/environments/new",
	"applications.environments.new",
	ApplicationsPrefix,
)

var EnvironmentCreate = routing.NewRouteWithParams[ApplicationEnvironmentParams](
	"/:applicationID/environments",
	"applications.environments.create",
	ApplicationsPrefix,
)

var EnvironmentShow = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID",
	"applications.environments.show",
	ApplicationsPrefix,
)

var EnvironmentTelemetry = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/telemetry",
	"applications.environments.telemetry",
	ApplicationsPrefix,
)

var EnvironmentTelemetryLogs = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/telemetry/logs",
	"applications.environments.telemetry.logs",
	ApplicationsPrefix,
)

var EnvironmentTelemetryQueries = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/telemetry/queries",
	"applications.environments.telemetry.queries",
	ApplicationsPrefix,
)

var EnvironmentTelemetryTrace = routing.NewRouteWithParams[EnvironmentTelemetryTraceParams](
	"/:applicationID/environments/:environmentID/telemetry/traces/:traceID",
	"applications.environments.telemetry.trace",
	ApplicationsPrefix,
)

var EnvironmentReleases = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/releases",
	"applications.environments.releases",
	ApplicationsPrefix,
)

var EnvironmentBuilds = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/builds",
	"applications.environments.builds",
	ApplicationsPrefix,
)

var EnvironmentRestart = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/restart",
	"applications.environments.restart",
	ApplicationsPrefix,
)

var EnvironmentStart = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/start",
	"applications.environments.start",
	ApplicationsPrefix,
)

var EnvironmentSecrets = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/secrets",
	"applications.environments.secrets",
	ApplicationsPrefix,
)

var EnvironmentLogs = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/logs",
	"applications.environments.logs",
	ApplicationsPrefix,
)

var EnvironmentEdit = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/edit",
	"applications.environments.edit",
	ApplicationsPrefix,
)

var EnvironmentUpdate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID",
	"applications.environments.update",
	ApplicationsPrefix,
)

var EnvironmentDestroy = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID",
	"applications.environments.destroy",
	ApplicationsPrefix,
)

var EnvironmentSourceEdit = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/source/edit",
	"applications.environments.source.edit",
	ApplicationsPrefix,
)

var EnvironmentSourceUpdate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/source",
	"applications.environments.source.update",
	ApplicationsPrefix,
)

var EnvironmentDeploymentsCreate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/deployments",
	"applications.environments.deployments.create",
	ApplicationsPrefix,
)

var EnvironmentPromoteToProduction = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/promote-to-production",
	"applications.environments.promote-to-production",
	ApplicationsPrefix,
)

var EnvironmentReleaseDeploymentsCreate = routing.NewRouteWithParams[EnvironmentReleaseParams](
	"/:applicationID/environments/:environmentID/releases/:releaseID/deployments",
	"applications.environments.releases.deployments.create",
	ApplicationsPrefix,
)

var EnvironmentDeploymentRetry = routing.NewRouteWithParams[EnvironmentDeploymentParams](
	"/:applicationID/environments/:environmentID/deployments/:deploymentID/retry",
	"applications.environments.deployments.retry",
	ApplicationsPrefix,
)

var EnvironmentDeploymentStop = routing.NewRouteWithParams[EnvironmentDeploymentParams](
	"/:applicationID/environments/:environmentID/deployments/:deploymentID/stop",
	"applications.environments.deployments.stop",
	ApplicationsPrefix,
)

var EnvironmentAPITokenRotate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/api-token",
	"applications.environments.api-token.rotate",
	ApplicationsPrefix,
)

var EnvironmentDNSAdopt = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/dns/adopt",
	"applications.environments.dns.adopt",
	ApplicationsPrefix,
)

var EnvironmentDNSRetry = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/dns/retry",
	"applications.environments.dns.retry",
	ApplicationsPrefix,
)

var EnvironmentDNSRefresh = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/dns/refresh",
	"applications.environments.dns.refresh",
	ApplicationsPrefix,
)

var EnvironmentDeploymentEvents = routing.NewRouteWithParams[EnvironmentDeploymentEventParams](
	"/:environmentID/deployments/:deploymentID/events",
	"environments.deployments.events",
	EnvironmentsPrefix,
)

var EnvironmentBuildLogs = routing.NewRouteWithParams[EnvironmentBuildParams](
	"/:environmentID/builds/:buildID/logs",
	"environments.builds.logs",
	EnvironmentsPrefix,
)

var EnvironmentBuildStart = routing.NewRouteWithParams[EnvironmentBuildActionParams](
	"/:applicationID/environments/:environmentID/builds/:buildID/start",
	"applications.environments.builds.start",
	ApplicationsPrefix,
)

var EnvironmentBuildStop = routing.NewRouteWithParams[EnvironmentBuildActionParams](
	"/:applicationID/environments/:environmentID/builds/:buildID/stop",
	"applications.environments.builds.stop",
	ApplicationsPrefix,
)

var EnvironmentBuildRetry = routing.NewRouteWithParams[EnvironmentBuildActionParams](
	"/:applicationID/environments/:environmentID/builds/:buildID/retry",
	"applications.environments.builds.retry",
	ApplicationsPrefix,
)

var EnvironmentReleaseCommandLogs = routing.NewRouteWithParams[EnvironmentReleaseCommandParams](
	"/:applicationID/environments/:environmentID/release-commands/:executionID/logs",
	"applications.environments.release_commands.logs",
	ApplicationsPrefix,
)

var EnvironmentReleaseCommandRetry = routing.NewRouteWithParams[EnvironmentReleaseCommandParams](
	"/:applicationID/environments/:environmentID/release-commands/:executionID/retry",
	"applications.environments.release_commands.retry",
	ApplicationsPrefix,
)

var EnvironmentSecretsCreate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/secrets",
	"applications.environments.secrets.create",
	ApplicationsPrefix,
)

var EnvironmentSecretsBulkCreate = routing.NewRouteWithParams[EnvironmentParams](
	"/:applicationID/environments/:environmentID/secrets/bulk",
	"applications.environments.secrets.bulk_create",
	ApplicationsPrefix,
)

var EnvironmentSecretRotate = routing.NewRouteWithParams[EnvironmentSecretParams](
	"/:applicationID/environments/:environmentID/secrets/:secretID/rotate",
	"applications.environments.secrets.rotate",
	ApplicationsPrefix,
)

var EnvironmentSecretDestroy = routing.NewRouteWithParams[EnvironmentSecretParams](
	"/:applicationID/environments/:environmentID/secrets/:secretID",
	"applications.environments.secrets.destroy",
	ApplicationsPrefix,
)
