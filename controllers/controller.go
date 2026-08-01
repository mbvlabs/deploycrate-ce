// Package controllers provides HTTP handlers for the web application.
package controllers

import (
	"deploycrate-ce/controllers/api"
	"deploycrate-ce/router"

	"go.uber.org/fx"
)

var otherCache = NewCacheBuilder[string]().WithSize(2).Build

var constructors = fx.Provide(
	otherCache,
	NewPages,
	NewAssets,
	api.NewAPI,
	NewSessions,
	NewResetPasswords,
	NewSystem,
	NewSelfUpdates,
	NewGitHubConnections,
	NewRegistryResources,
	NewObjectStorage,
	NewApplications,
	NewEnvironments,
	NewResources,
	NewNetworks,
	NewDatabaseClusters,
	NewJobs,
	NewCaddyRoutes,
	NewNodes,
)

var Module = fx.Module(
	"controllers",
	constructors,
	fx.Invoke(func(r *router.Router, c Pages) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Assets) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c api.API) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Sessions) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c ResetPasswords) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c System) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c SelfUpdates) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c GitHubConnections) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c RegistryResources) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c ObjectStorage) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Applications) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Environments) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Resources) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Networks) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c DatabaseClusters) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Jobs) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c CaddyRoutes) error {
		return c.RegisterRoutes(r)
	}),
	fx.Invoke(func(r *router.Router, c Nodes) error {
		return c.RegisterRoutes(r)
	}),
)
