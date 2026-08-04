package controllers

import (
	"errors"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type DNSConnections struct {
	service *services.DNSConnections
}

func NewDNSConnections(service *services.DNSConnections) DNSConnections {
	return DNSConnections{service: service}
}

func (controller DNSConnections) RegisterRoutes(r *router.Router) error {
	admin := []echo.MiddlewareFunc{middleware.AdminOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.DnsConnections, controller.Show},
		{http.MethodPost, routes.DnsConnectionCreate, controller.Create},
		{http.MethodPost, routes.DnsConnectionSync, controller.Sync},
		{http.MethodPatch, routes.DnsConnectionTokenUpdate, controller.RotateToken},
		{http.MethodDelete, routes.DnsConnectionDestroy, controller.Destroy},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(), Handler: definition.handler, Middlewares: admin})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller DNSConnections) Show(etx *echo.Context) error {
	connections, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/DNS", inertia.Props{
		"auth": authProps(etx), "connections": connections, "flash": environmentFlashProps(etx),
	})
}

func (controller DNSConnections) Create(etx *echo.Context) error {
	var payload struct {
		Name      string `json:"name"`
		AccountID string `json:"accountId"`
		Token     string `json:"token"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	if _, err := controller.service.Create(etx.Request().Context(), payload.Name, payload.AccountID, payload.Token); err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare DNS connection created")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) Sync(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.Synchronize(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare zones synchronized")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) RotateToken(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	var payload struct {
		Token string `json:"token"`
	}
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = controller.service.RotateToken(etx.Request().Context(), id, payload.Token)
	}
	if err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare account-owned API token rotated")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) Destroy(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.Archive(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare DNS connection archived")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) redirectError(etx *echo.Context, operationErr error) error {
	message := strings.TrimSpace(operationErr.Error())
	if message == "" {
		message = "Cloudflare DNS operation failed"
	}
	_ = cookies.AddFlash(etx, cookies.FlashError, message)
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}
