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

type SelfUpdates struct {
	service *services.SelfUpdate
	db      storage.Pool
}

func NewSelfUpdates(service *services.SelfUpdate, db storage.Pool) SelfUpdates {
	return SelfUpdates{service: service, db: db}
}

func (s SelfUpdates) RegisterRoutes(r *router.Router) error {
	errList := []error{}

	_, err := r.AddRoute(echo.Route{
		Method:  http.MethodGet,
		Path:    routes.SystemUpdate.Path(),
		Name:    routes.SystemUpdate.Name(),
		Handler: s.Show,
		Middlewares: []echo.MiddlewareFunc{
			middleware.AuthOnly,
		},
	})
	if err != nil {
		errList = append(errList, err)
	}

	_, err = r.AddRoute(echo.Route{
		Method:  http.MethodPost,
		Path:    routes.SystemUpdateCreate.Path(),
		Name:    routes.SystemUpdateCreate.Name(),
		Handler: s.Create,
		Middlewares: []echo.MiddlewareFunc{
			middleware.AuthOnly,
		},
	})
	if err != nil {
		errList = append(errList, err)
	}

	_, err = r.AddRoute(echo.Route{
		Method:  http.MethodGet,
		Path:    routes.SystemUpdateStatus.Path(),
		Name:    routes.SystemUpdateStatus.Name(),
		Handler: s.Status,
		Middlewares: []echo.MiddlewareFunc{
			middleware.AuthOnly,
		},
	})
	if err != nil {
		errList = append(errList, err)
	}

	return errors.Join(errList...)
}

func (s SelfUpdates) Show(etx *echo.Context) error {
	deployments, err := models.Application.FindSystemDeployments(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE updates",
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Page(etx, "System/Update", inertia.Props{
		"auth": inertia.Props{
			"email": cookies.ExtractFromCookieApp(etx).Email,
		},
		"currentVersion":  s.service.CurrentVersion(),
		"availableUpdate": s.service.CheckForUpdates(etx.Request().Context()),
		"update":          s.service.Status(),
		"deployments":     deployments,
	})
}

func (s SelfUpdates) Status(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, map[string]any{
		"currentVersion": s.service.CurrentVersion(),
		"update":         s.service.Status(),
	})
}

func (s SelfUpdates) Create(etx *echo.Context) error {
	app := cookies.ExtractFromCookieApp(etx)
	_, err := s.service.Start(etx.Request().Context(), app.UserID)
	switch {
	case err == nil:
		err = cookies.AddFlash(etx, cookies.FlashSuccess, "DeployCrate CE update started")
	case errors.Is(err, services.ErrUpdateInProgress),
		errors.Is(err, services.ErrNoUpdateAvailable):
		err = cookies.AddFlash(etx, cookies.FlashInfo, err.Error())
	default:
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to start DeployCrate CE update",
			"error",
			err,
		)
		err = cookies.AddFlash(etx, cookies.FlashError, "Failed to start update: "+err.Error())
	}
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Redirect(etx, routes.SystemUpdate.URL(), http.StatusSeeOther)
}
