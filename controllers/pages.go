package controllers

import (
	"log/slog"
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type Pages struct {
	db         storage.Pool
	insertOnly queue.InsertOnly
	metric     services.MetricRollupService
}

func NewPages(
	db storage.Pool,
	insertOnly queue.InsertOnly,
	metric services.MetricRollupService,
) Pages {
	return Pages{db: db, insertOnly: insertOnly, metric: metric}
}

func (p Pages) RegisterRoutes(r *router.Router) error {
	err := registerRoutes(r, []routeDefinition{
		{http.MethodGet, routes.HomePage, p.Home},
	}, middleware.AuthOnly)

	_ = r.AddRouteNotFound(middleware.AuthOnly(p.NotFound))

	return err
}

func (p Pages) Home(etx *echo.Context) error {
	ctx := etx.Request().Context()
	appSession := cookies.ExtractFromCookieApp(etx)
	if appSession.Email == "" {
		user, err := models.User.Find(
			ctx,
			p.db.Executor(),
			appSession.UserID,
		)
		if err != nil {
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to load authenticated user",
				"error",
				err,
			)
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}

		if err := cookies.CreateAppSession(etx, user); err != nil {
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to refresh authenticated user session",
				"error",
				err,
			)
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}
		appSession.Email = user.Email
	}
	dashboard, err := models.Dashboard.Snapshot(ctx, p.db.Executor())
	if err != nil {
		slog.ErrorContext(ctx, "failed to load dashboard", "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	overview, err := models.Application.FindSystemOverview(ctx, p.db.Executor())
	if err != nil {
		slog.ErrorContext(ctx, "failed to load system server for dashboard telemetry", "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	telemetry, err := p.metric.HostTelemetry(ctx, overview.ServerID)
	if err != nil {
		slog.WarnContext(ctx, "failed to load dashboard host telemetry", "error", err)
	}

	return inertia.Page(etx, "Home", inertia.Props{
		"auth": inertia.Props{
			"email": appSession.Email,
		},
		"dashboard": dashboard,
		"telemetry": telemetry,
	})
}

func (p Pages) NotFound(etx *echo.Context) error {
	return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
}
