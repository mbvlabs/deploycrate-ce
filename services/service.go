// Package services provides application services for business workflows.
package services

import (
	caddyclients "deploycrate-ce/clients/caddy"
	cloudflareclient "deploycrate-ce/clients/cloudflare"
	githubclient "deploycrate-ce/clients/github"

	"go.uber.org/fx"
)

var Module = fx.Module(
	"services",
	fx.Provide(
		NewDashboard,
		NewIdentity,
		NewSelfUpdate,
		NewClickHouseResource,
		NewSystemHealth,
		NewSystemLogs,
		NewSystemApplicationTelemetry,
		NewIPAPIGeoResolver,
		NewEnvironmentApplicationTelemetry,
		NewTelemetryIdentity,
		NewResourceCredentials,
		func() CloudflareDNSClient { return cloudflareclient.NewDNS() },
		NewDNSConnections,
		NewEnvironmentDNS,
		NewResourceDNS,
		func() CaddyClient { return caddyclients.New("") },
		NewCaddyRouteServiceWithDNS,
		NewSSHCAService,
		NewNodeEnrollment,
		NewServerExecution,
		NewContainerExecution,
		func(container *ContainerExecution) ResourceContainerService { return container },
		NewServerManagement,
		NewWorkloadExecution,
		NewMetricRollupService,
		NewClickHouseBackup,
		NewServerBackup,
		NewDatabaseBackup,
		NewDatabaseArtifact,
		NewDatabaseRestoreEngine,
		NewBackupExecutor,
		NewBackupVerifier,
		NewBackupRetention,
		NewBackupPolicyActivator,
		NewBackupScheduler,
		NewDatabaseBackups,
		NewDatabaseRestoreWorkflow,
		githubclient.NewClient,
		NewGitHubConnection,
		NewGitHubWebhook,
		NewApplicationSetup,
		NewRegistryResources,
		NewEnvironmentSecrets,
		NewEnvironmentLogs,
		NewEnvironmentSetup,
		NewReleaseDeployment,
		NewBuildExecution,
		NewReleaseCommandExecution,
		NewDeploymentExecution,
		NewWorkloadReconciliation,
		NewResourceManagement,
		NewResourceHealth,
		NewResourcePrivateAccess,
	),
	fx.Invoke(StartResourceCaddyReconciler),
)
