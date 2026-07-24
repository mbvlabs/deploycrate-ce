// Package services provides application services for business workflows.
package services

import (
	caddyclients "deploycrate-ce/clients/caddy"

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
		NewBackupScheduler,
		NewServerBackup,
		NewDatabaseBackup,
		NewBackupExecutor,
		NewBackupVerifier,
		NewBackupRetention,
	),
)
