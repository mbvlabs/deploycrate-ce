package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
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

type Applications struct {
	service *services.ApplicationSetup
}

func NewApplications(service *services.ApplicationSetup) Applications {
	return Applications{service: service}
}

func (controller Applications) RegisterRoutes(r *router.Router) error {
	auth := []echo.MiddlewareFunc{middleware.AuthOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.Applications, controller.Index},
		{http.MethodGet, routes.ApplicationNew, controller.New},
		{http.MethodPost, routes.ApplicationCreate, controller.Create},
		{http.MethodGet, routes.EnvironmentNew, controller.NewEnvironment},
		{http.MethodPost, routes.EnvironmentCreate, controller.CreateEnvironment},
		{http.MethodGet, routes.ApplicationShow, controller.Show},
		{http.MethodGet, routes.ApplicationEdit, controller.Edit},
		{http.MethodPatch, routes.ApplicationUpdate, controller.Update},
		{http.MethodGet, routes.ApplicationSourceEdit, controller.EditSource},
		{http.MethodPatch, routes.ApplicationSourceUpdate, controller.UpdateSource},
		{http.MethodDelete, routes.ApplicationDestroy, controller.Destroy},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(), Handler: definition.handler, Middlewares: auth})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller Applications) Index(etx *echo.Context) error {
	applications, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/Index", inertia.Props{"auth": authProps(etx), "applications": applications})
}

func (controller Applications) New(etx *echo.Context) error {
	return controller.newPage(etx, false)
}

func (controller Applications) NewEnvironment(etx *echo.Context) error {
	return controller.newPage(etx, true)
}

func (controller Applications) newPage(etx *echo.Context, environmentIntent bool) error {
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/New", inertia.Props{"auth": authProps(etx), "options": applicationSetupOptionsProps(options), "environmentIntent": environmentIntent})
}

type registryResourceOptionProps struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Endpoint string    `json:"endpoint"`
}

func applicationSetupOptionsProps(options services.ApplicationSetupOptions) map[string]any {
	registries := make([]registryResourceOptionProps, 0, len(options.Registries))
	for _, registry := range options.Registries {
		registries = append(registries, registryResourceOptionProps{ID: registry.ID, Name: registry.Name, Endpoint: registry.Endpoint})
	}
	return map[string]any{
		"installations": options.Installations,
		"repositories":  options.Repositories,
		"registries":    registries,
		"buildServers":  options.BuildServers,
	}
}

type applicationSetupPayload struct {
	SourceType           string          `json:"sourceType"`
	ApplicationName      string          `json:"applicationName"`
	ApplicationSlug      string          `json:"applicationSlug"`
	EnvironmentName      string          `json:"environmentName"`
	EnvironmentSlug      string          `json:"environmentSlug"`
	EnvironmentKind      string          `json:"environmentKind"`
	GitHubInstallationID string          `json:"githubInstallationId"`
	GitHubRepositoryID   string          `json:"githubRepositoryId"`
	Reference            string          `json:"reference"`
	AutoBuild            bool            `json:"autoBuild"`
	ContextPath          string          `json:"contextPath"`
	BuilderReference     string          `json:"builderReference"`
	BuildpackSettings    json.RawMessage `json:"buildpackSettings"`
	RegistryResourceID   string          `json:"registryResourceId"`
	ImageRepository      string          `json:"imageRepository"`
	BuildServerID        string          `json:"buildServerId"`
}

func (payload applicationSetupPayload) serviceData() (services.ApplicationSetupData, error) {
	sourceType := strings.ToLower(strings.TrimSpace(payload.SourceType))
	if sourceType == "" {
		sourceType = "buildpacks"
	}
	var installationID, repositoryID, buildServerID uuid.UUID
	var err error
	if sourceType == "buildpacks" {
		installationID, err = uuid.Parse(payload.GitHubInstallationID)
		if err != nil {
			return services.ApplicationSetupData{}, err
		}
		repositoryID, err = uuid.Parse(payload.GitHubRepositoryID)
		if err != nil {
			return services.ApplicationSetupData{}, err
		}
		buildServerID, err = uuid.Parse(payload.BuildServerID)
		if err != nil {
			return services.ApplicationSetupData{}, err
		}
	}
	registryID, err := uuid.Parse(payload.RegistryResourceID)
	if err != nil {
		return services.ApplicationSetupData{}, err
	}
	return services.ApplicationSetupData{SourceType: sourceType, ApplicationName: payload.ApplicationName, ApplicationSlug: payload.ApplicationSlug, EnvironmentName: payload.EnvironmentName, EnvironmentSlug: payload.EnvironmentSlug, EnvironmentKind: payload.EnvironmentKind, GitHubInstallationID: installationID, GitHubRepositoryID: repositoryID, Reference: payload.Reference, AutoBuild: payload.AutoBuild, ContextPath: payload.ContextPath, BuilderReference: payload.BuilderReference, BuildpackSettings: payload.BuildpackSettings, RegistryResourceID: registryID, ImageRepository: payload.ImageRepository, BuildServerID: buildServerID}, nil
}

func (controller Applications) Create(etx *echo.Context) error {
	return controller.create(etx, false)
}

func (controller Applications) CreateEnvironment(etx *echo.Context) error {
	return controller.create(etx, true)
}

func (controller Applications) create(etx *echo.Context, environmentIntent bool) error {
	var payload applicationSetupPayload
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	data, err := payload.serviceData()
	if err == nil {
		var result services.ApplicationSetupResult
		result, err = controller.service.Create(etx.Request().Context(), data)
		if err == nil {
			if environmentIntent {
				_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment created. Complete its deployment setup")
				return inertia.Redirect(etx, routes.EnvironmentSetup.URL(routes.EnvironmentParams{ApplicationID: result.Application.ID.String(), EnvironmentID: result.Environment.ID.String()}), http.StatusSeeOther)
			}
			_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application created")
			return inertia.Redirect(etx, routes.ApplicationShow.URL(result.Application.ID), http.StatusSeeOther)
		}
	}
	return controller.renderSetupError(etx, "Applications/New", environmentIntent, err)
}

func (controller Applications) Show(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := controller.service.Details(etx.Request().Context(), id)
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/Show", inertia.Props{"auth": authProps(etx), "application": details})
}

func (controller Applications) Edit(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := controller.service.Details(etx.Request().Context(), id)
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/Edit", inertia.Props{"auth": authProps(etx), "application": details})
}

func (controller Applications) Update(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	var payload struct{ Name, Slug string }
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = controller.service.UpdatePresentation(etx.Request().Context(), id, payload.Name, payload.Slug)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.ApplicationEdit.URL(id), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application updated")
	return inertia.Redirect(etx, routes.ApplicationShow.URL(id), http.StatusSeeOther)
}

func (controller Applications) EditSource(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := controller.service.Details(etx.Request().Context(), id)
	if err != nil {
		return controller.renderError(etx, err)
	}
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/Source/Edit", inertia.Props{
		"auth": authProps(etx), "application": details, "options": applicationSetupOptionsProps(options),
		"updateUrl": routes.ApplicationSourceUpdate.URL(id), "returnUrl": routes.ApplicationShow.URL(id),
	})
}

func (controller Applications) UpdateSource(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	var payload applicationSetupPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var data services.ApplicationSetupData
	if err == nil {
		data, err = payload.serviceData()
	}
	if err == nil {
		err = controller.service.UpdateSource(etx.Request().Context(), id, data)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.ApplicationSourceEdit.URL(id), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application source updated")
	return inertia.Redirect(etx, routes.ApplicationShow.URL(id), http.StatusSeeOther)
}

func (controller Applications) Destroy(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.Archive(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.ApplicationShow.URL(id), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application archived")
	return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
}

func (controller Applications) renderSetupError(etx *echo.Context, page string, environmentIntent bool, err error) error {
	options, optionsErr := controller.service.Options(etx.Request().Context())
	if optionsErr != nil {
		return controller.renderError(etx, errors.Join(err, optionsErr))
	}
	if validationErrors, ok := validation.As(err); ok {
		return inertia.Page(etx, page, inertia.Props{"auth": authProps(etx), "options": applicationSetupOptionsProps(options), "environmentIntent": environmentIntent}, inertia.WithValidationErrors(validationErrors.ToMap()))
	}
	location := routes.ApplicationNew.URL()
	if environmentIntent {
		location = routes.EnvironmentNew.URL()
	}
	return controller.redirectWithError(etx, location, err)
}

func (controller Applications) redirectWithError(etx *echo.Context, location string, err error) error {
	message := "Application operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func (controller Applications) renderError(etx *echo.Context, err error) error {
	slog.ErrorContext(etx.Request().Context(), "failed to load application page", "error", err)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}
