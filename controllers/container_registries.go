package controllers

import (
	"errors"
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
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.RegistryResources, controller.Index},
		{http.MethodPost, routes.RegistryResourceCreate, controller.Create},
		{http.MethodDelete, routes.RegistryResourceDestroy, controller.Destroy},
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

func (controller RegistryResources) Index(etx *echo.Context) error {
	registries, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/Registries", inertia.Props{"auth": authProps(etx), "registries": registries})
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
