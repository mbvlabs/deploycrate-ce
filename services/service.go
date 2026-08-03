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
		NewNodeEnrollment,
		NewServerExecution,
		NewContainerExecution,
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
		NewBuildExecution,
		NewDeploymentExecution,
		NewWorkloadReconciliation,
		NewResourceManagement,
		NewResourceHealth,
		NewResourcePrivateAccess,
	),
)
