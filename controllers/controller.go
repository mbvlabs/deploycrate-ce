// Package controllers provides HTTP handlers for the web application.
package controllers

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"deploycrate-ce/controllers/api"
	"deploycrate-ce/router"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"go.uber.org/fx"
)

type namedRoute interface {
	Path() string
	Name() string
}

type routeDefinition struct {
	method  string
	route   namedRoute
	handler echo.HandlerFunc
}

func registerRoutes(
	r *router.Router,
	definitions []routeDefinition,
	middlewares ...echo.MiddlewareFunc,
) error {
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{
			Method:      definition.method,
			Path:        definition.route.Path(),
			Name:        definition.route.Name(),
			Handler:     definition.handler,
			Middlewares: middlewares,
		})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func uuidPathParam(etx *echo.Context, name string) (uuid.UUID, error) {
	return uuid.Parse(etx.Param(name))
}

func int64PathParam(etx *echo.Context, name string) (int64, error) {
	return strconv.ParseInt(etx.Param(name), 10, 64)
}

var otherCache = NewCacheBuilder[string]().WithSize(2).Build

func inertiaPropRequested(request *http.Request, component string, names ...string) bool {
	partialComponent := strings.TrimSpace(request.Header.Get("X-Inertia-Partial-Component"))
	if partialComponent == "" || partialComponent != component {
		return true
	}
	requested := strings.TrimSpace(request.Header.Get("X-Inertia-Partial-Data"))
	if requested == "" {
		return true
	}
	for value := range strings.SplitSeq(requested, ",") {
		value = strings.TrimSpace(value)
		if slices.Contains(names, value) {
			return true
		}
	}
	return false
}

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
	NewDNSConnections,
	NewRegistryResources,
	NewObjectStorage,
	NewApplications,
	NewEnvironments,
	NewResources,
	NewNetworks,
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
	fx.Invoke(func(r *router.Router, c DNSConnections) error {
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
