package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type System struct {
	db     storage.Pool
	health *services.SystemHealth
}

func NewSystem(db storage.Pool, health *services.SystemHealth) System {
	return System{db: db, health: health}
}

func (s System) RegisterRoutes(r *router.Router) error {
	routesToRegister := []struct {
		route interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{route: routes.SystemOverview, handler: s.Overview},
		{route: routes.SystemDeployments, handler: s.Deployments},
		{route: routes.SystemDatabase, handler: s.Database},
		{route: routes.SystemNetwork, handler: s.Network},
	}

	errList := make([]error, 0, len(routesToRegister))
	for _, registered := range routesToRegister {
		_, err := r.AddRoute(echo.Route{
			Method:  http.MethodGet,
			Path:    registered.route.Path(),
			Name:    registered.route.Name(),
			Handler: registered.handler,
			Middlewares: []echo.MiddlewareFunc{
				middleware.AuthOnly,
			},
		})
		if err != nil {
			errList = append(errList, err)
		}
	}

	return errors.Join(errList...)
}

func (s System) Overview(etx *echo.Context) error {
	overview, err := models.Application.FindSystemOverview(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "overview", err)
	}
	backups, err := models.Backup.FindSystemHealth(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "backup health", err)
	}

	return inertia.Page(etx, "System/Overview", inertia.Props{
		"auth":    s.authProps(etx),
		"system":  overview,
		"health":  s.health.Run(etx.Request().Context()),
		"backups": backups,
	})
}

func (s System) Deployments(etx *echo.Context) error {
	deployments, err := models.Application.FindSystemDeployments(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "deployments", err)
	}

	return inertia.Page(etx, "System/Deployments", inertia.Props{
		"auth":        s.authProps(etx),
		"deployments": deployments,
	})
}

func (s System) Database(etx *echo.Context) error {
	database, err := models.Application.FindSystemDatabase(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "database", err)
	}

	return inertia.Page(etx, "System/Database", inertia.Props{
		"auth":     s.authProps(etx),
		"database": database,
	})
}

func (s System) Network(etx *echo.Context) error {
	network, err := models.Application.FindSystemNetwork(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "network", err)
	}

	return inertia.Page(etx, "System/Network", inertia.Props{
		"auth":    s.authProps(etx),
		"network": network,
	})
}

func (s System) authProps(etx *echo.Context) inertia.Props {
	return inertia.Props{"email": cookies.ExtractFromCookieApp(etx).Email}
}

func (s System) renderLoadError(etx *echo.Context, page string, err error) error {
	slog.ErrorContext(
		etx.Request().Context(),
		"failed to load DeployCrate CE system page",
		"page",
		page,
		"error",
		err,
	)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}
