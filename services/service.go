// Package services provides application services for business workflows.
package services

import (
	caddyclients "deploycrate-ce/clients/caddy"
	githubclient "deploycrate-ce/clients/github"

	"go.uber.org/fx"
)

var Module = fx.Module(
	"services",
	fx.Provide(
		NewIdentity,
		NewSelfUpdate,
		NewClickHouseResource,
		NewSystemHealth,
		NewSystemLogs,
		func() CaddyClient { return caddyclients.New("") },
		NewCaddyRouteService,
		NewSSHCAService,
		NewMetricRollupService,
		NewClickHouseBackup,
		NewServerBackup,
		NewDatabaseBackup,
		NewDatabaseArtifact,
		NewDatabaseRestore,
		NewBackupExecutor,
		NewBackupVerifier,
		NewBackupRetention,
		NewBackupPolicyActivator,
		NewBackupScheduler,
		NewResourceBackups,
		NewResourceRestore,
		githubclient.NewClient,
		NewGitHubConnection,
		NewGitHubWebhook,
		NewApplicationSetup,
		NewContainerRegistries,
		NewEnvironmentSecrets,
		NewEnvironmentLogs,
		NewEnvironmentSetup,
		NewBuildExecution,
		NewDeploymentExecution,
		NewWorkloadReconciliation,
		NewResourceManagement,
		NewResourceHealth,
		NewResourcePrivateAccess,
	),
)
