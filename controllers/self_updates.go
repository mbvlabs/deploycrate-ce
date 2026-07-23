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
}

func NewSelfUpdates(service *services.SelfUpdate) SelfUpdates {
	return SelfUpdates{service: service}
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
		Path:    routes.SelfUpdateSettings.Path(),
		Name:    routes.SelfUpdateSettings.Name(),
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
		Path:    routes.SelfUpdateSettingsCreate.Path(),
		Name:    routes.SelfUpdateSettingsCreate.Name(),
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
		slog.ErrorContext(etx.Request().Context(), "failed to load DeployCrate CE system overview", "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Page(etx, "System/Overview", inertia.Props{
		"auth": inertia.Props{
			"email": cookies.ExtractFromCookieApp(etx).Email,
		},
		"system": overview,
	})
}

func (s SelfUpdates) Show(etx *echo.Context) error {
	release, releaseErr := s.service.Check(etx.Request().Context())
	releaseError := ""
	if releaseErr != nil {
		releaseError = releaseErr.Error()
		slog.WarnContext(etx.Request().Context(), "failed to check for DeployCrate CE updates", "error", releaseErr)
	}

	return inertia.Page(etx, "Settings/Updates", inertia.Props{
		"auth": inertia.Props{
			"email": cookies.ExtractFromCookieApp(etx).Email,
		},
		"release":      release,
		"releaseError": releaseError,
		"update":       s.service.Status(),
	})
}

func (s SelfUpdates) Create(etx *echo.Context) error {
	app := cookies.ExtractFromCookieApp(etx)
	_, err := s.service.Start(etx.Request().Context(), app.UserID)
	switch {
	case err == nil:
		err = cookies.AddFlash(etx, cookies.FlashSuccess, "DeployCrate CE update started")
	case errors.Is(err, services.ErrAlreadyCurrent):
		err = cookies.AddFlash(etx, cookies.FlashInfo, err.Error())
	case errors.Is(err, services.ErrUpdateInProgress):
		err = cookies.AddFlash(etx, cookies.FlashInfo, err.Error())
	default:
		slog.ErrorContext(etx.Request().Context(), "failed to start DeployCrate CE update", "error", err)
		err = cookies.AddFlash(etx, cookies.FlashError, "Failed to start update: "+err.Error())
	}
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Redirect(etx, routes.SelfUpdateSettings.URL(), http.StatusSeeOther)
}
