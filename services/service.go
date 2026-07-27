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
		NewSystemHealth,
		func() CaddyClient { return caddyclients.New("") },
		NewCaddyRouteService,
		NewSSHCAService,
		NewMetricRollupService,
		NewClickHouseBackup,
		NewServerBackup,
		NewDatabaseBackup,
		NewBackupExecutor,
		NewBackupVerifier,
		NewBackupRetention,
		NewBackupPolicyActivator,
		NewBackupScheduler,
		githubclient.NewClient,
		NewGitHubConnection,
		NewGitHubWebhook,
		NewApplicationSetup,
	),
)
