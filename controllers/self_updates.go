package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type SelfUpdates struct {
	service *services.SelfUpdate
	health  *services.SystemHealth
}

func NewSelfUpdates(service *services.SelfUpdate, health *services.SystemHealth) SelfUpdates {
	return SelfUpdates{service: service, health: health}
}

func (s SelfUpdates) RegisterRoutes(r *router.Router) error {
	errList := []error{}

	_, err := r.AddRoute(echo.Route{
		Method:  http.MethodGet,
		Path:    routes.SystemOverview.Path(),
		Name:    routes.SystemOverview.Name(),
		Handler: s.Overview,
		Middlewares: []echo.MiddlewareFunc{
			middleware.AuthOnly,
		},
	})
	if err != nil {
		errList = append(errList, err)
	}

	_, err = r.AddRoute(echo.Route{
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

	return errors.Join(errList...)
}

func (s SelfUpdates) Overview(etx *echo.Context) error {
	overview, err := s.service.Overview(etx.Request().Context())
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE system overview",
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Page(etx, "System/Overview", inertia.Props{
		"auth": inertia.Props{
			"email": cookies.ExtractFromCookieApp(etx).Email,
		},
		"system": overview,
		"health": s.health.Run(etx.Request().Context()),
	})
}

func (s SelfUpdates) Show(etx *echo.Context) error {
	return inertia.Page(etx, "System/Update", inertia.Props{
		"auth": inertia.Props{
			"email": cookies.ExtractFromCookieApp(etx).Email,
		},
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
	case errors.Is(err, services.ErrUpdateInProgress):
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
