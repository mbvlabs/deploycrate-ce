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

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type Networks struct {
	db     storage.Pool
	access *services.ResourcePrivateAccess
}

func NewNetworks(db storage.Pool, access *services.ResourcePrivateAccess) Networks {
	return Networks{db: db, access: access}
}

func (controller Networks) RegisterRoutes(r *router.Router) error {
	routesToRegister := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler     echo.HandlerFunc
		middlewares []echo.MiddlewareFunc
	}{
		{
			http.MethodGet,
			routes.Networks,
			controller.Index,
			[]echo.MiddlewareFunc{middleware.AuthOnly},
		},
		{
			http.MethodDelete,
			routes.NetworkWireGuardDeviceDestroy,
			controller.DestroyWireGuardDevice,
			[]echo.MiddlewareFunc{middleware.AdminOnly},
		},
	}

	errList := make([]error, 0, len(routesToRegister))
	for _, registered := range routesToRegister {
		_, err := r.AddRoute(echo.Route{
			Method: registered.method, Path: registered.route.Path(), Name: registered.route.Name(),
			Handler: registered.handler, Middlewares: registered.middlewares,
		})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller Networks) Index(etx *echo.Context) error {
	network, err := models.Application.FindSystemNetwork(
		etx.Request().Context(),
		controller.db.Executor(),
	)
	if err != nil {
		return controller.renderLoadError(etx, "network", err)
	}
	devices, err := models.Application.FindSystemWireGuardDevices(
		etx.Request().Context(),
		controller.db.Executor(),
	)
	if err != nil {
		return controller.renderLoadError(etx, "network devices", err)
	}

	return inertia.Page(etx, "Networks/Index", inertia.Props{
		"auth":    authProps(etx),
		"network": network,
		"devices": systemWireGuardDeviceProps(devices),
	})
}

func (controller Networks) DestroyWireGuardDevice(etx *echo.Context) error {
	deviceID, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.access.RevokeDevice(etx.Request().Context(), deviceID)
	}
	if err != nil {
		if flashErr := cookies.AddFlash(etx, cookies.FlashError, err.Error()); flashErr != nil {
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "WireGuard device revoked")
	}
	return inertia.Redirect(etx, routes.Networks.URL(), http.StatusSeeOther)
}

func (controller Networks) renderLoadError(etx *echo.Context, page string, err error) error {
	slog.ErrorContext(
		etx.Request().Context(),
		"failed to load DeployCrate CE networks page",
		"page",
		page,
		"error",
		err,
	)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}
