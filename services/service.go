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
		func() CaddyClient { return caddyclients.New("") },
		NewCaddyRouteService,
	),
)
