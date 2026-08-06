package controllers

import (
	"encoding/json"
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

type Applications struct {
	service        *services.ApplicationSetup
	environmentSrv *services.EnvironmentSetup
	metricSrv      services.MetricRollupService
}

func NewApplications(
	service *services.ApplicationSetup,
	environments *services.EnvironmentSetup,
	metric services.MetricRollupService,
) Applications {
	return Applications{service: service, environmentSrv: environments, metricSrv: metric}
}

func (c Applications) RegisterRoutes(r *router.Router) error {
	auth := []echo.MiddlewareFunc{middleware.AuthOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.Applications, c.Index},
		{http.MethodGet, routes.ApplicationNew, c.New},
		{http.MethodPost, routes.ApplicationCreate, c.Create},
		{http.MethodGet, routes.ApplicationShow, c.Show},
		{http.MethodGet, routes.ApplicationEdit, c.Edit},
		{http.MethodPatch, routes.ApplicationUpdate, c.Update},
		{http.MethodGet, routes.ApplicationSourceEdit, c.EditSource},
		{http.MethodPatch, routes.ApplicationSourceUpdate, c.UpdateSource},
		{http.MethodDelete, routes.ApplicationDestroy, c.Destroy},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(
			echo.Route{
				Method:      definition.method,
				Path:        definition.route.Path(),
				Name:        definition.route.Name(),
				Handler:     definition.handler,
				Middlewares: auth,
			},
		)
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (c Applications) Index(etx *echo.Context) error {
	applications, err := c.service.List(etx.Request().Context())
	if err != nil {
		return c.renderError(etx, err)
	}
	return inertia.Page(
		etx,
		"Applications/Index",
		inertia.Props{"auth": authProps(etx), "applications": applications},
	)
}

func (c Applications) New(etx *echo.Context) error {
	options, err := c.service.Options(etx.Request().Context())
	if err != nil {
		return c.renderError(etx, err)
	}

	environmentOptions, err := c.environmentSrv.Options(etx.Request().Context())
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"could not get environment options",
			"error",
			err,
		)

		return c.renderError(etx, err)
	}

	return inertia.Page(
		etx,
		"Applications/New",
		inertia.Props{
			"auth":    authProps(etx),
			"options": applicationCreationOptionsProps(options, environmentOptions),
		},
	)
}

type registryResourceOptionProps struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Endpoint string    `json:"endpoint"`
}

func applicationSetupOptionsProps(options services.ApplicationSetupOptions) map[string]any {
	registries := make([]registryResourceOptionProps, 0, len(options.Registries))
	for _, registry := range options.Registries {
		registries = append(
			registries,
			registryResourceOptionProps{
				ID:       registry.ID,
				Name:     registry.Name,
				Endpoint: registry.Endpoint,
			},
		)
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
	props["dnsZones"] = environmentOptions.DNSZones

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
	return services.ApplicationSetupData{
		SourceType:           sourceType,
		ApplicationName:      payload.ApplicationName,
		ApplicationSlug:      payload.ApplicationSlug,
		EnvironmentName:      payload.EnvironmentName,
		EnvironmentSlug:      payload.EnvironmentSlug,
		EnvironmentKind:      payload.EnvironmentKind,
		GitHubInstallationID: installationID,
		GitHubRepositoryID:   repositoryID,
		Reference:            payload.Reference,
		AutoBuild:            payload.AutoBuild,
		ContextPath:          payload.ContextPath,
		BuilderReference:     payload.BuilderReference,
		BuildpackSettings:    payload.BuildpackSettings,
		RegistryResourceID:   registryID,
		ImageRepository:      payload.ImageRepository,
		BuildServerID:        buildServerID,
	}, nil
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
	Processes            []models.EnvironmentProcessInput         `json:"processes"`
	Resources            []services.EnvironmentSetupResourceInput `json:"resources"`
	Secrets              []services.EnvironmentSetupSecretInput   `json:"secrets"`
	Deploy               bool                                     `json:"deploy"`
	DNSMode              string                                   `json:"dnsMode"`
	DNSZoneID            string                                   `json:"dnsZoneId"`
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
	var dnsZoneID *uuid.UUID
	if strings.EqualFold(strings.TrimSpace(payload.DNSMode), services.DNSModeCloudflare) {
		parsed, err := uuid.Parse(payload.DNSZoneID)
		if err != nil {
			return preparedApplicationEnvironmentCreation{}, errors.New(
				"select a Cloudflare DNS zone",
			)
		}
		dnsZoneID = &parsed
	}
	return preparedApplicationEnvironmentCreation{
		source: source,
		setup: services.EnvironmentSetupInput{
			ServerIDs: serverIDs, Hostname: payload.Hostname, ContainerPort: payload.ContainerPort,
			HealthPath: payload.HealthPath, Processes: payload.Processes,
			Resources: payload.Resources, Secrets: payload.Secrets, Deploy: payload.Deploy,
			DNS: services.EnvironmentDNSInput{Mode: payload.DNSMode, ZoneID: dnsZoneID},
		},
		deploy: payload.Deploy,
	}, nil
}

func (c Applications) Create(etx *echo.Context) error {
	var payload applicationCreationPayload
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	prepared := make([]preparedApplicationEnvironmentCreation, 0, 2)
	if payload.Staging != nil {
		staging, err := payload.Staging.prepare(
			payload.ApplicationName,
			payload.ApplicationSlug,
			"Staging",
			"staging",
			"staging",
		)
		if err != nil {
			return c.renderCreationError(etx, err)
		}
		prepared = append(prepared, staging)
	}
	production, err := payload.Production.prepare(
		payload.ApplicationName,
		payload.ApplicationSlug,
		"Production",
		"production",
		"production",
	)
	if err != nil {
		return c.renderCreationError(etx, err)
	}
	prepared = append(prepared, production)
	sources := make([]services.ApplicationSetupData, 0, len(prepared))
	for _, environment := range prepared {
		sources = append(sources, environment.source)
	}
	result, err := c.service.CreateApplication(
		etx.Request().Context(),
		services.ApplicationCreationData{
			ApplicationName: payload.ApplicationName,
			ApplicationSlug: payload.ApplicationSlug,
			Environments:    sources,
		},
	)
	if err != nil {
		return c.renderCreationError(etx, err)
	}
	userID := cookies.ExtractFromCookieApp(etx).UserID
	setupResults := make([]services.EnvironmentSetupResult, 0, len(result.Environments))
	for index, environment := range result.Environments {
		completed, completeErr := c.environmentSrv.Complete(
			etx.Request().
				Context(),
			result.Application.ID,
			environment.Environment.ID,
			userID,
			prepared[index].setup,
		)
		if completeErr != nil {
			err = completeErr
			_ = c.environmentSrv.DeleteApplication(
				etx.Request().Context(),
				result.Application.ID,
			)
			return c.renderCreationError(etx, err)
		}
		setupResults = append(setupResults, completed)
	}
	message := "Application created"
	for index, completed := range setupResults {
		if prepared[index].deploy && completed.DeploymentDeferred {
			message = "Application created; selected deployments will start after managed DNS is ready"
			break
		}
		if prepared[index].deploy {
			message = "Application created and selected deployments queued"
		}
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	return inertia.Redirect(
		etx,
		routes.ApplicationShow.URL(result.Application.ID),
		http.StatusSeeOther,
	)
}

func (c Applications) Show(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := c.service.Overview(etx.Request().Context(), id)
	if err != nil {
		return c.renderError(etx, err)
	}
	telemetryRows := make([]services.AttributedTelemetryRow, 0)
	for _, environment := range details.Environments {
		if !environment.SetupComplete {
			continue
		}
		rows, telemetryErr := c.metricSrv.EnvironmentTelemetry(
			etx.Request().Context(),
			environment.EnvironmentID,
		)
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

func (c Applications) Edit(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := c.service.Details(etx.Request().Context(), id)
	if err != nil {
		return c.renderError(etx, err)
	}
	return inertia.Page(
		etx,
		"Applications/Edit",
		inertia.Props{"auth": authProps(etx), "application": details},
	)
}

func (c Applications) Update(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	var payload struct{ Name, Slug string }
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = c.service.UpdatePresentation(
			etx.Request().Context(),
			id,
			payload.Name,
			payload.Slug,
		)
	}
	if err != nil {
		return c.redirectWithError(etx, routes.ApplicationEdit.URL(id), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application updated")
	return inertia.Redirect(etx, routes.ApplicationShow.URL(id), http.StatusSeeOther)
}

func (c Applications) EditSource(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := c.service.Details(etx.Request().Context(), id)
	if err != nil {
		return c.renderError(etx, err)
	}
	options, err := c.service.Options(etx.Request().Context())
	if err != nil {
		return c.renderError(etx, err)
	}
	return inertia.Page(etx, "Applications/Source/Edit", inertia.Props{
		"auth": authProps(
			etx,
		),
		"application": details,
		"options":     applicationSetupOptionsProps(options),
		"updateUrl": routes.ApplicationSourceUpdate.URL(
			id,
		),
		"returnUrl": routes.ApplicationShow.URL(id),
	})
}

func (c Applications) UpdateSource(etx *echo.Context) error {
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
		err = c.service.UpdateSource(etx.Request().Context(), id, data)
	}
	if err != nil {
		return c.redirectWithError(etx, routes.ApplicationSourceEdit.URL(id), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Application source updated")
	return inertia.Redirect(etx, routes.ApplicationShow.URL(id), http.StatusSeeOther)
}

func (c Applications) Destroy(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		slog.ErrorContext(etx.Request().Context(), "could not parse id", "param", etx.Param("id"))
		if err := cookies.AddFlash(
			etx,
			cookies.FlashError,
			"Could not handle that request. Try again.",
		); err != nil {
			return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
		}

		return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
	}

	if err := c.environmentSrv.DeleteApplication(etx.Request().Context(), id); err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"could not delete application",
			"err",
			err,
		)

		if err := cookies.AddFlash(
			etx,
			cookies.FlashError,
			"Could not handle that request. Try again.",
		); err != nil {
			return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
		}

		return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
	}

	if err := cookies.AddFlash(
		etx,
		cookies.FlashSuccess,
		"Application permanently deleted",
	); err != nil {
		return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
	}

	return inertia.Redirect(etx, routes.Applications.URL(), http.StatusSeeOther)
}

func (c Applications) renderCreationError(etx *echo.Context, err error) error {
	options, optionsErr := c.service.Options(etx.Request().Context())
	environmentOptions, environmentOptionsErr := c.environmentSrv.Options(
		etx.Request().Context(),
	)
	if optionsErr != nil || environmentOptionsErr != nil {
		return c.renderError(etx, errors.Join(err, optionsErr, environmentOptionsErr))
	}
	props := inertia.Props{
		"auth": authProps(
			etx,
		),
		"options": applicationCreationOptionsProps(options, environmentOptions),
	}
	if validationErrors, ok := validation.As(err); ok {
		return inertia.Page(
			etx,
			"Applications/New",
			props,
			inertia.WithValidationErrors(validationErrors.ToMap()),
		)
	}

	props["setupError"] = err.Error()

	return inertia.Page(etx, "Applications/New", props)
}

func (c Applications) redirectWithError(
	etx *echo.Context,
	location string,
	err error,
) error {
	message := "Application operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func (c Applications) renderError(etx *echo.Context, err error) error {
	slog.ErrorContext(etx.Request().Context(), "failed to load application page", "error", err)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}
