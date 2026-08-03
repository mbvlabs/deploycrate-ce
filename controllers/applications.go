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
	service      *services.ApplicationSetup
	environments *services.EnvironmentSetup
	metric       services.MetricRollupService
}

func NewApplications(
	service *services.ApplicationSetup,
	environments *services.EnvironmentSetup,
	metric services.MetricRollupService,
) Applications {
	return Applications{service: service, environments: environments, metric: metric}
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
	return controller.newPage(etx)
}

func (controller Applications) newPage(etx *echo.Context) error {
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, err)
	}
	environmentOptions, err := controller.environments.Options(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/New", inertia.Props{"auth": authProps(etx), "options": applicationCreationOptionsProps(options, environmentOptions)})
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

func applicationCreationOptionsProps(
	options services.ApplicationSetupOptions,
	environmentOptions services.EnvironmentSetupOptions,
) map[string]any {
	props := applicationSetupOptionsProps(options)
	props["resources"] = environmentOptions.Resources
	props["servers"] = environmentOptions.Servers
	return props
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

type applicationEnvironmentCreationPayload struct {
	EnvironmentName      string                                   `json:"environmentName"`
	EnvironmentSlug      string                                   `json:"environmentSlug"`
	EnvironmentKind      string                                   `json:"environmentKind"`
	SourceType           string                                   `json:"sourceType"`
	GitHubInstallationID string                                   `json:"githubInstallationId"`
	GitHubRepositoryID   string                                   `json:"githubRepositoryId"`
	Reference            string                                   `json:"reference"`
	AutoBuild            bool                                     `json:"autoBuild"`
	ContextPath          string                                   `json:"contextPath"`
	BuilderReference     string                                   `json:"builderReference"`
	BuildpackSettings    json.RawMessage                          `json:"buildpackSettings"`
	RegistryResourceID   string                                   `json:"registryResourceId"`
	ImageRepository      string                                   `json:"imageRepository"`
	BuildServerID        string                                   `json:"buildServerId"`
	ServerIDs            []string                                 `json:"serverIds"`
	Hostname             string                                   `json:"hostname"`
	ContainerPort        int32                                    `json:"containerPort"`
	HealthPath           string                                   `json:"healthPath"`
	BPGOTargets          string                                   `json:"bpGoTargets"`
	Resources            []services.EnvironmentSetupResourceInput `json:"resources"`
	Secrets              []services.EnvironmentSetupSecretInput   `json:"secrets"`
	Deploy               bool                                     `json:"deploy"`
}

type applicationCreationPayload struct {
	ApplicationName string                                 `json:"applicationName"`
	ApplicationSlug string                                 `json:"applicationSlug"`
	Staging         *applicationEnvironmentCreationPayload `json:"staging"`
	Production      applicationEnvironmentCreationPayload  `json:"production"`
}

type preparedApplicationEnvironmentCreation struct {
	source services.ApplicationSetupData
	setup  services.EnvironmentSetupInput
	deploy bool
}

func (payload applicationEnvironmentCreationPayload) prepare(
	applicationName, applicationSlug, environmentName, environmentSlug, environmentKind string,
) (preparedApplicationEnvironmentCreation, error) {
	source, err := (applicationSetupPayload{
		SourceType: payload.SourceType, ApplicationName: applicationName, ApplicationSlug: applicationSlug,
		EnvironmentName: environmentName, EnvironmentSlug: environmentSlug, EnvironmentKind: environmentKind,
		GitHubInstallationID: payload.GitHubInstallationID, GitHubRepositoryID: payload.GitHubRepositoryID,
		Reference: payload.Reference, AutoBuild: payload.AutoBuild, ContextPath: payload.ContextPath,
		BuilderReference: payload.BuilderReference, BuildpackSettings: payload.BuildpackSettings,
		RegistryResourceID: payload.RegistryResourceID, ImageRepository: payload.ImageRepository,
		BuildServerID: payload.BuildServerID,
	}).serviceData()
	if err != nil {
		return preparedApplicationEnvironmentCreation{}, err
	}
	serverIDs := make([]uuid.UUID, 0, len(payload.ServerIDs))
	for _, value := range payload.ServerIDs {
		serverID, err := uuid.Parse(value)
		if err != nil {
			return preparedApplicationEnvironmentCreation{}, err
		}
		serverIDs = append(serverIDs, serverID)
	}
	return preparedApplicationEnvironmentCreation{
		source: source,
		setup: services.EnvironmentSetupInput{
			ServerIDs: serverIDs, Hostname: payload.Hostname, ContainerPort: payload.ContainerPort,
			HealthPath: payload.HealthPath, BPGOTargets: payload.BPGOTargets,
			Resources: payload.Resources, Secrets: payload.Secrets,
		},
		deploy: payload.Deploy,
	}, nil
}

func (controller Applications) Create(etx *echo.Context) error {
	var payload applicationCreationPayload
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	prepared := make([]preparedApplicationEnvironmentCreation, 0, 2)
	if payload.Staging != nil {
		staging, err := payload.Staging.prepare(payload.ApplicationName, payload.ApplicationSlug, "Staging", "staging", "staging")
		if err != nil {
			return controller.renderCreationError(etx, err)
		}
		prepared = append(prepared, staging)
	}
	production, err := payload.Production.prepare(payload.ApplicationName, payload.ApplicationSlug, "Production", "production", "production")
	if err != nil {
		return controller.renderCreationError(etx, err)
	}
	prepared = append(prepared, production)
	sources := make([]services.ApplicationSetupData, 0, len(prepared))
	for _, environment := range prepared {
		sources = append(sources, environment.source)
	}
	result, err := controller.service.CreateApplication(etx.Request().Context(), services.ApplicationCreationData{
		ApplicationName: payload.ApplicationName, ApplicationSlug: payload.ApplicationSlug, Environments: sources,
	})
	if err != nil {
		return controller.renderCreationError(etx, err)
	}
	userID := cookies.ExtractFromCookieApp(etx).UserID
	for index, environment := range result.Environments {
		if _, err = controller.environments.Complete(
			etx.Request().Context(), result.Application.ID, environment.Environment.ID, userID, prepared[index].setup,
		); err != nil {
			_ = controller.service.Archive(etx.Request().Context(), result.Application.ID)
			return controller.renderCreationError(etx, err)
		}
	}
	deploymentsQueued := 0
	for index, environment := range result.Environments {
		if !prepared[index].deploy {
			continue
		}
		if _, err = controller.environments.QueueSourceDeployment(
			etx.Request().Context(), result.Application.ID, environment.Environment.ID, &userID, "user", "",
		); err != nil {
			_ = cookies.AddFlash(etx, cookies.FlashError, "Application created, but an initial deployment could not be queued: "+err.Error())
			return inertia.Redirect(etx, routes.ApplicationShow.URL(result.Application.ID), http.StatusSeeOther)
		}
		deploymentsQueued++
	}
	message := "Application created"
	if deploymentsQueued > 0 {
		message = "Application created and selected deployments queued"
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	return inertia.Redirect(etx, routes.ApplicationShow.URL(result.Application.ID), http.StatusSeeOther)
}

func (controller Applications) Show(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := controller.service.Overview(etx.Request().Context(), id)
	if err != nil {
		return controller.renderError(etx, err)
	}
	telemetryRows := make([]services.AttributedTelemetryRow, 0)
	for _, environment := range details.Environments {
		if !environment.SetupComplete {
			continue
		}
		rows, telemetryErr := controller.metric.EnvironmentTelemetry(etx.Request().Context(), environment.EnvironmentID)
		if telemetryErr != nil {
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to load Application Environment telemetry",
				"application_id", id,
				"environment_id", environment.EnvironmentID,
				"error", telemetryErr,
			)
			continue
		}
		telemetryRows = append(telemetryRows, rows...)
	}
	return inertia.Page(etx, "Applications/Show", inertia.Props{
		"auth": authProps(etx), "application": details, "telemetry": telemetryRows,
	})
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

func (controller Applications) renderCreationError(etx *echo.Context, err error) error {
	options, optionsErr := controller.service.Options(etx.Request().Context())
	environmentOptions, environmentOptionsErr := controller.environments.Options(etx.Request().Context())
	if optionsErr != nil || environmentOptionsErr != nil {
		return controller.renderError(etx, errors.Join(err, optionsErr, environmentOptionsErr))
	}
	if validationErrors, ok := validation.As(err); ok {
		return inertia.Page(etx, "Applications/New", inertia.Props{
			"auth": authProps(etx), "options": applicationCreationOptionsProps(options, environmentOptions),
		}, inertia.WithValidationErrors(validationErrors.ToMap()))
	}
	return controller.redirectWithError(etx, routes.ApplicationNew.URL(), err)
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
