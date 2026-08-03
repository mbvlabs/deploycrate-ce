package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type RegistryResources struct {
	service *services.RegistryResources
}

func NewRegistryResources(service *services.RegistryResources) RegistryResources {
	return RegistryResources{service: service}
}

func (controller RegistryResources) RegisterRoutes(r *router.Router) error {
	admin := []echo.MiddlewareFunc{middleware.AdminOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler     echo.HandlerFunc
		middlewares []echo.MiddlewareFunc
	}{
		{http.MethodGet, routes.RegistryResources, controller.Index, nil},
		{http.MethodPost, routes.RegistryResourceCreate, controller.Create, nil},
		{http.MethodGet, routes.RegistryResourceShow, controller.Show, nil},
		{http.MethodDelete, routes.RegistryResourceDestroy, controller.Destroy, nil},
		{http.MethodPost, routes.RegistryResourceCredentials, controller.Credentials, []echo.MiddlewareFunc{middleware.IPRateLimiter(5, routes.RegistryResources)}},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		middlewares := append([]echo.MiddlewareFunc{}, admin...)
		middlewares = append(middlewares, definition.middlewares...)
		_, err := r.AddRoute(echo.Route{Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(), Handler: definition.handler, Middlewares: middlewares})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller RegistryResources) Credentials(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Managed registry not found"})
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := etx.Bind(&payload); err != nil || payload.Password == "" || len(payload.Password) > 4096 {
		return etx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "Current password is required"})
	}

	credentials, err := controller.service.RevealManagedCredentials(
		etx.Request().Context(),
		resourceID,
		cookies.ExtractFromCookieApp(etx).UserID,
		payload.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCredentials):
			return etx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "Current password is incorrect"})
		case errors.Is(err, services.ErrManagedRegistryUnavailable):
			return etx.JSON(http.StatusNotFound, map[string]string{"error": "Managed registry not found"})
		default:
			slog.ErrorContext(etx.Request().Context(), "failed to reveal managed Registry credentials", "resource_id", resourceID, "error", err)
			return etx.JSON(http.StatusInternalServerError, map[string]string{"error": "Registry credentials could not be loaded"})
		}
	}

	return etx.JSON(http.StatusOK, credentials)
}

func (controller RegistryResources) Index(etx *echo.Context) error {
	registries, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/Registries", inertia.Props{"auth": authProps(etx), "registries": registries})
}

func (controller RegistryResources) Show(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	registry, err := controller.service.Find(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		slog.ErrorContext(etx.Request().Context(), "failed to load Registry", "resource_id", resourceID, "error", err)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/Registries/Show", inertia.Props{"auth": authProps(etx), "registry": registry})
}

func (controller RegistryResources) Create(etx *echo.Context) error {
	var payload struct {
		Name        string `json:"name"`
		Endpoint    string `json:"endpoint"`
		Username    string `json:"username"`
		AccessToken string `json:"accessToken"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	_, err := controller.service.CreateExternal(etx.Request().Context(), services.ExternalRegistryResourceInput{
		Name: payload.Name, Endpoint: payload.Endpoint, Username: payload.Username, AccessToken: payload.AccessToken,
	})
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			registries, listErr := controller.service.List(etx.Request().Context())
			if listErr == nil {
				return inertia.Page(etx, "Connections/Registries", inertia.Props{"auth": authProps(etx), "registries": registries}, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
		}
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Registry Resource connected")
	return inertia.Redirect(etx, routes.RegistryResources.URL(), http.StatusSeeOther)
}

func (controller RegistryResources) Destroy(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.ArchiveExternal(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Registry Resource archived")
	return inertia.Redirect(etx, routes.RegistryResources.URL(), http.StatusSeeOther)
}

func (controller RegistryResources) redirectWithError(etx *echo.Context, err error) error {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Registry Resource operation failed"
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, routes.RegistryResources.URL(), http.StatusSeeOther)
}
