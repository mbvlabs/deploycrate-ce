package controllers

import (
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/validation"
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
	definitions := []routeDefinition{
		{http.MethodGet, routes.DnsConnections, controller.Index},
		{http.MethodPost, routes.DnsConnectionCreate, controller.Create},
		{http.MethodGet, routes.DnsConnectionShow, controller.Show},
		{http.MethodPost, routes.DnsConnectionSync, controller.Sync},
		{http.MethodPatch, routes.DnsConnectionTokenUpdate, controller.RotateToken},
		{http.MethodDelete, routes.DnsConnectionDestroy, controller.Destroy},
	}
	return registerRoutes(r, definitions, middleware.AdminOnly)
}

func (controller DNSConnections) Index(etx *echo.Context) error {
	connections, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/DNS", inertia.Props{
		"auth": authProps(etx), "connections": connections, "flash": environmentFlashProps(etx),
	})
}

func (controller DNSConnections) Show(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	connection, err := controller.service.Find(etx.Request().Context(), id)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	zones, err := controller.service.Zones(etx.Request().Context(), id)
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/DNS/Show", inertia.Props{
		"auth": authProps(
			etx,
		),
		"connection": connection,
		"zones":      zones,
		"flash":      environmentFlashProps(etx),
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
	if _, err := controller.service.Create(
		etx.Request().Context(),
		payload.Name,
		payload.AccountID,
		payload.Token,
	); err != nil {
		if handled, response := controller.validationResponse(etx, err); handled {
			return response
		}
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare DNS connection created")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) Sync(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.Synchronize(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare zones synchronized")
	return inertia.Redirect(etx, routes.DnsConnectionShow.URL(id), http.StatusSeeOther)
}

func (controller DNSConnections) RotateToken(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
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
		if handled, response := controller.showValidationResponse(etx, id, err); handled {
			return response
		}
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare account-owned API token rotated")
	return inertia.Redirect(etx, routes.DnsConnectionShow.URL(id), http.StatusSeeOther)
}

func (controller DNSConnections) Destroy(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.Archive(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Cloudflare DNS connection archived")
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}

func (controller DNSConnections) validationResponse(
	etx *echo.Context,
	operationErr error,
) (bool, error) {
	validationErrors, ok := validation.As(operationErr)
	if !ok {
		return false, nil
	}
	connections, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return true, inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return true, inertia.Page(etx, "Connections/DNS", inertia.Props{
		"auth": authProps(etx), "connections": connections, "flash": environmentFlashProps(etx),
	}, inertia.WithValidationErrors(validationErrors.ToMap()))
}

func (controller DNSConnections) showValidationResponse(
	etx *echo.Context,
	id uuid.UUID,
	operationErr error,
) (bool, error) {
	validationErrors, ok := validation.As(operationErr)
	if !ok {
		return false, nil
	}
	connection, err := controller.service.Find(etx.Request().Context(), id)
	if err != nil {
		return true, inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	zones, err := controller.service.Zones(etx.Request().Context(), id)
	if err != nil {
		return true, inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return true, inertia.Page(etx, "Connections/DNS/Show", inertia.Props{
		"auth": authProps(
			etx,
		),
		"connection": connection,
		"zones":      zones,
		"flash":      environmentFlashProps(etx),
	}, inertia.WithValidationErrors(validationErrors.ToMap()))
}

func (controller DNSConnections) redirectError(etx *echo.Context, operationErr error) error {
	message := strings.TrimSpace(operationErr.Error())
	if message == "" {
		message = "Cloudflare DNS operation failed"
	}
	_ = cookies.AddFlash(etx, cookies.FlashError, message)
	return inertia.Redirect(etx, routes.DnsConnections.URL(), http.StatusSeeOther)
}
