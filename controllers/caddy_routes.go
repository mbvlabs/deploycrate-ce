package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type CaddyRoutes struct {
	service services.CaddyRouteService
}

func NewCaddyRoutes(service services.CaddyRouteService) CaddyRoutes {
	return CaddyRoutes{service: service}
}

func (controller CaddyRoutes) RegisterRoutes(r *router.Router) error {
	return errors.Join(
		registerRoutes(r, []routeDefinition{
			{http.MethodGet, routes.CaddyRoutes, controller.Index},
			{http.MethodGet, routes.CaddyRouteShow, controller.Show},
		}, middleware.AuthOnly),
		registerRoutes(r, []routeDefinition{
			{http.MethodPost, routes.CaddyRouteCreate, controller.Create},
			{http.MethodPatch, routes.CaddyRouteUpdate, controller.Update},
			{http.MethodPatch, routes.CaddyResourceRouteUpdate, controller.UpdateResource},
			{http.MethodPatch, routes.CaddyCustomRouteUpdate, controller.UpdateCustom},
			{http.MethodDelete, routes.CaddyCustomRouteDestroy, controller.DestroyCustom},
			{http.MethodDelete, routes.CaddyRouteDestroy, controller.Destroy},
		}, middleware.AdminOnly),
	)
}

func (controller CaddyRoutes) UpdateResource(etx *echo.Context) error {
	endpointID, err := uuidPathParam(etx, "id")
	var input services.ResourceCaddyRouteUpdateInput
	if err == nil {
		err = etx.Bind(&input)
	}
	var externalID string
	if err == nil {
		externalID, err = controller.service.UpdateResourceRoute(
			etx.Request().Context(),
			endpointID,
			input,
		)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource Caddy route updated and reconciled")
	return inertia.Redirect(etx, routes.CaddyRouteShow.URL(externalID), http.StatusSeeOther)
}

func (controller CaddyRoutes) UpdateCustom(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	var input services.CustomCaddyRouteInput
	if err == nil {
		err = etx.Bind(&input)
	}
	if err == nil {
		err = controller.service.UpdateCustom(etx.Request().Context(), id, input)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Caddy route updated and reconciled")
	return inertia.Redirect(etx, routes.CaddyRouteShow.URL(input.ExternalID), http.StatusSeeOther)
}

func (controller CaddyRoutes) DestroyCustom(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.DestroyCustom(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Caddy route removed")
	return inertia.Redirect(etx, routes.CaddyRoutes.URL(), http.StatusSeeOther)
}

func (controller CaddyRoutes) Show(etx *echo.Context) error {
	detail, err := controller.service.RouteDetail(etx.Request().Context(), etx.Param("id"))
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Caddy route details",
			"route_id",
			etx.Param("id"),
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(
		etx,
		"System/CaddyRoutes/Show",
		inertia.Props{"auth": authProps(etx), "route": detail},
	)
}

func (controller CaddyRoutes) Index(etx *echo.Context) error {
	snapshot, err := controller.service.ManagementSnapshot(etx.Request().Context())
	if err != nil {
		slog.ErrorContext(etx.Request().Context(), "failed to load Caddy routes", "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	resourceRoutes, err := controller.service.ResourceRouteSnapshot(etx.Request().Context())
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Resource Caddy routes",
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	customRoutes, err := controller.service.CustomRouteSnapshot(etx.Request().Context())
	if err != nil {
		slog.ErrorContext(etx.Request().Context(), "failed to load custom Caddy routes", "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "System/CaddyRoutes/Index", inertia.Props{
		"auth": authProps(
			etx,
		),
		"routes":         snapshot.Routes,
		"resourceRoutes": resourceRoutes,
		"customRoutes":   customRoutes,
		"options":        snapshot.Options,
	})
}

func (controller CaddyRoutes) Create(etx *echo.Context) error {
	var input services.CustomCaddyRouteInput
	err := etx.Bind(&input)
	if err == nil {
		_, err = controller.service.CreateCustom(etx.Request().Context(), input)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Caddy route created and reconciled")
	return inertia.Redirect(etx, routes.CaddyRoutes.URL(), http.StatusSeeOther)
}

func (controller CaddyRoutes) Update(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	var input services.ManagedCaddyRouteInput
	if err == nil {
		err = etx.Bind(&input)
	}
	if err == nil {
		err = controller.service.UpdateManaged(etx.Request().Context(), id, input)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Caddy route updated and reconciled")
	if etx.QueryParam("returnTo") == "details" {
		return inertia.Redirect(
			etx,
			routes.CaddyRouteShow.URL(input.ExternalID),
			http.StatusSeeOther,
		)
	}
	return inertia.Redirect(etx, routes.CaddyRoutes.URL(), http.StatusSeeOther)
}

func (controller CaddyRoutes) Destroy(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.DestroyManaged(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Caddy route removed")
	return inertia.Redirect(etx, routes.CaddyRoutes.URL(), http.StatusSeeOther)
}

func (controller CaddyRoutes) redirectWithError(etx *echo.Context, err error) error {
	message := "Caddy route operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, routes.CaddyRoutes.URL(), http.StatusSeeOther)
}
