package controllers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/request"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type Environments struct {
	setup        *services.EnvironmentSetup
	secrets      *services.EnvironmentSecrets
	applications *services.ApplicationSetup
	metric       services.MetricRollupService
}

func NewEnvironments(
	setup *services.EnvironmentSetup,
	secrets *services.EnvironmentSecrets,
	applications *services.ApplicationSetup,
	metric services.MetricRollupService,
) Environments {
	return Environments{setup: setup, secrets: secrets, applications: applications, metric: metric}
}

func (controller Environments) RegisterRoutes(router *router.Router) error {
	auth := []echo.MiddlewareFunc{middleware.AuthOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.Environments, controller.Index},
		{http.MethodGet, routes.EnvironmentShow, controller.Show},
		{http.MethodGet, routes.EnvironmentEdit, controller.Edit},
		{http.MethodPatch, routes.EnvironmentUpdate, controller.Update},
		{http.MethodDelete, routes.EnvironmentDestroy, controller.Destroy},
		{http.MethodGet, routes.EnvironmentSourceEdit, controller.EditSource},
		{http.MethodPatch, routes.EnvironmentSourceUpdate, controller.UpdateSource},
		{http.MethodGet, routes.EnvironmentBuildLogs, controller.BuildLogs},
		{http.MethodGet, routes.EnvironmentDeploymentEvents, controller.DeploymentEvents},
		{http.MethodPost, routes.EnvironmentBuildStart, controller.StartBuild},
		{http.MethodPost, routes.EnvironmentBuildStop, controller.StopBuild},
		{http.MethodPost, routes.EnvironmentBuildRetry, controller.RetryBuild},
		{http.MethodGet, routes.EnvironmentSetup, controller.Setup},
		{http.MethodPatch, routes.EnvironmentSetup, controller.CompleteSetup},
		{http.MethodPost, routes.EnvironmentDeploymentsCreate, controller.Deploy},
		{http.MethodPost, routes.EnvironmentReleaseDeploymentsCreate, controller.RedeployRelease},
		{http.MethodPost, routes.EnvironmentDeploymentRetry, controller.RetryDeployment},
		{http.MethodPost, routes.EnvironmentSecretsCreate, controller.CreateSecret},
		{http.MethodPost, routes.EnvironmentSecretRotate, controller.RotateSecret},
		{http.MethodDelete, routes.EnvironmentSecretDestroy, controller.ArchiveSecret},
	}
	registered := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := router.AddRoute(echo.Route{Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(), Handler: definition.handler, Middlewares: auth})
		registered = append(registered, err)
	}
	return errors.Join(registered...)
}

func (controller Environments) DeploymentEvents(etx *echo.Context) error {
	environmentID, environmentErr := uuid.Parse(etx.Param("environmentID"))
	deploymentID, deploymentErr := uuid.Parse(etx.Param("deploymentID"))
	after := int64(0)
	afterErr := error(nil)
	if value := etx.QueryParam("after"); value != "" {
		after, afterErr = strconv.ParseInt(value, 10, 64)
		if after < 0 {
			afterErr = errors.New("Deployment event cursor cannot be negative")
		}
	}
	if environmentErr != nil || deploymentErr != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Deployment not found"})
	}
	if afterErr != nil {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": "Deployment event cursor is invalid"})
	}
	snapshot, err := controller.setup.DeploymentEvents(etx.Request().Context(), environmentID, deploymentID, after)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Deployment not found"})
	}
	if err != nil {
		return etx.JSON(http.StatusInternalServerError, map[string]string{"error": "Deployment events could not be loaded"})
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (controller Environments) BuildLogs(etx *echo.Context) error {
	environmentID, environmentErr := uuid.Parse(etx.Param("environmentID"))
	buildID, buildErr := uuid.Parse(etx.Param("buildID"))
	after := int64(0)
	afterErr := error(nil)
	if value := etx.QueryParam("after"); value != "" {
		after, afterErr = strconv.ParseInt(value, 10, 64)
		if after < 0 {
			afterErr = errors.New("Build log cursor cannot be negative")
		}
	}
	if environmentErr != nil || buildErr != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Build not found"})
	}
	if afterErr != nil {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": "Build log cursor is invalid"})
	}
	snapshot, err := controller.setup.BuildLogs(etx.Request().Context(), environmentID, buildID, after)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Build not found"})
	}
	if err != nil {
		return etx.JSON(http.StatusInternalServerError, map[string]string{"error": "Build logs could not be loaded"})
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (controller Environments) Index(etx *echo.Context) error {
	environments, err := controller.setup.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Environments/Index", inertia.Props{
		"auth":         authProps(etx),
		"environments": environments,
		"flash":        environmentFlashProps(etx),
	})
}

func (controller Environments) RetryDeployment(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	deploymentID, parseErr := uuid.Parse(etx.Param("deploymentID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		_, err = controller.setup.RetryDeployment(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, deploymentID, cookies.ExtractFromCookieApp(etx).UserID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Deployment retry queued")
	}
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) Deploy(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		_, err = controller.setup.QueueManualDeploy(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, cookies.ExtractFromCookieApp(etx).UserID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Build & deploy queued")
	}
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) RedeployRelease(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	releaseID, parseErr := uuid.Parse(etx.Param("releaseID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		_, err = controller.setup.QueueReleaseDeployment(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, releaseID, cookies.ExtractFromCookieApp(etx).UserID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Release deployment queued with current Environment secrets")
	}
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) StartBuild(etx *echo.Context) error {
	return controller.buildAction(etx, "Build start requested", controller.setup.StartBuild)
}

func (controller Environments) StopBuild(etx *echo.Context) error {
	return controller.buildAction(etx, "Build cancellation requested", controller.setup.StopBuild)
}

func (controller Environments) RetryBuild(etx *echo.Context) error {
	return controller.buildAction(etx, "Build retry queued", controller.setup.RetryBuild)
}

func (controller Environments) buildAction(
	etx *echo.Context,
	success string,
	action func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error,
) error {
	params, err := environmentPathParams(etx)
	buildID, parseErr := uuid.Parse(etx.Param("buildID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		err = action(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, buildID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, success)
	}
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

type environmentPathIDs struct {
	ApplicationID uuid.UUID
	EnvironmentID uuid.UUID
}

type environmentIdentityProps struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Kind string    `json:"kind"`
}

func environmentOverviewProps(overview services.EnvironmentOverview) map[string]any {
	return map[string]any{
		"applicationId":   overview.ApplicationID,
		"applicationName": overview.ApplicationName,
		"environment": environmentIdentityProps{
			ID: overview.Environment.ID, Name: overview.Environment.Name, Kind: overview.Environment.Kind,
		},
		"setupComplete":    overview.SetupComplete,
		"repository":       overview.Repository,
		"reference":        overview.Reference,
		"contextPath":      overview.ContextPath,
		"registryName":     overview.RegistryName,
		"registryEndpoint": overview.RegistryEndpoint,
		"deployability":    overview.Deployability,
		"secrets":          overview.Secrets,
		"variables":        overview.Variables,
		"domain":           overview.Domain,
		"resources":        overview.Resources,
		"builds":           overview.Builds,
		"releases":         overview.Releases,
		"deployments":      overview.Deployments,
		"instances":        overview.Instances,
	}
}

func (ids environmentPathIDs) routeParams() routes.EnvironmentParams {
	return routes.EnvironmentParams{
		ApplicationID: ids.ApplicationID.String(),
		EnvironmentID: ids.EnvironmentID.String(),
	}
}

func environmentPathParams(etx *echo.Context) (environmentPathIDs, error) {
	applicationID, err := uuid.Parse(etx.Param("applicationID"))
	if err != nil {
		return environmentPathIDs{}, err
	}
	environmentID, err := uuid.Parse(etx.Param("environmentID"))
	return environmentPathIDs{ApplicationID: applicationID, EnvironmentID: environmentID}, err
}

func (controller Environments) Show(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	overview, err := controller.setup.Overview(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if !overview.SetupComplete {
		return inertia.Redirect(etx, routes.EnvironmentSetup.URL(params.routeParams()), http.StatusSeeOther)
	}
	telemetryRows, telemetryErr := controller.metric.EnvironmentTelemetry(etx.Request().Context(), params.EnvironmentID)
	if telemetryErr != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment telemetry",
			"environment_id", params.EnvironmentID,
			"error", telemetryErr,
		)
		telemetryRows = []services.AttributedTelemetryRow{}
	}
	return inertia.Page(etx, "Applications/Environments/Show", inertia.Props{
		"auth": authProps(etx), "environment": environmentOverviewProps(overview),
		"telemetry": telemetryRows, "flash": environmentFlashProps(etx),
	})
}

func (controller Environments) Edit(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	data, err := controller.setup.EditData(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	options, err := controller.setup.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Applications/Environments/Edit", inertia.Props{
		"auth": authProps(etx), "environment": environmentOverviewProps(data.Overview),
		"configuration": data.Configuration, "options": options,
	})
}

type environmentEditPayload struct {
	Name          string                                   `json:"name"`
	Slug          string                                   `json:"slug"`
	Kind          string                                   `json:"kind"`
	Hostname      string                                   `json:"hostname"`
	ContainerPort int32                                    `json:"containerPort"`
	HealthPath    string                                   `json:"healthPath"`
	BPGOTargets   string                                   `json:"bpGoTargets"`
	Resources     []services.EnvironmentSetupResourceInput `json:"resources"`
}

func (controller Environments) Update(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload environmentEditPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = controller.setup.UpdateEnvironment(
			etx.Request().Context(), params.ApplicationID, params.EnvironmentID,
			cookies.ExtractFromCookieApp(etx).UserID,
			services.EnvironmentEditInput{
				Name: payload.Name, Slug: payload.Slug, Kind: payload.Kind, Hostname: payload.Hostname,
				ContainerPort: payload.ContainerPort, HealthPath: payload.HealthPath,
				BPGOTargets: payload.BPGOTargets, Resources: payload.Resources,
			},
		)
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			data, dataErr := controller.setup.EditData(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
			options, optionsErr := controller.setup.Options(etx.Request().Context())
			if dataErr == nil && optionsErr == nil {
				return inertia.Page(etx, "Applications/Environments/Edit", inertia.Props{
					"auth": authProps(etx), "environment": environmentOverviewProps(data.Overview),
					"configuration": payload, "options": options,
				}, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(etx, routes.EnvironmentEdit.URL(params.routeParams()), http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment updated and replacement rollout queued")
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) EditSource(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := controller.applications.DetailsForEnvironment(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	options, err := controller.applications.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	returnURL := routes.EnvironmentSetup.URL(params.routeParams())
	if details.SetupComplete {
		returnURL = routes.EnvironmentEdit.URL(params.routeParams())
	}
	return inertia.Page(etx, "Applications/Source/Edit", inertia.Props{
		"auth": authProps(etx), "application": details, "options": applicationSetupOptionsProps(options),
		"updateUrl": routes.EnvironmentSourceUpdate.URL(params.routeParams()),
		"returnUrl": returnURL,
	})
}

func (controller Environments) UpdateSource(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	queued := false
	setupComplete := false
	var payload applicationSetupPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var data services.ApplicationSetupData
	if err == nil {
		data, err = payload.serviceData()
	}
	if err == nil {
		details, detailsErr := controller.applications.DetailsForEnvironment(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
		if detailsErr != nil {
			err = errors.New("Environment source is unavailable")
		} else {
			setupComplete = details.SetupComplete
			err = controller.applications.UpdateEnvironmentSource(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, data)
			if err == nil && details.SetupComplete {
				_, err = controller.setup.QueueManualDeploy(
					etx.Request().Context(), params.ApplicationID, params.EnvironmentID,
					cookies.ExtractFromCookieApp(etx).UserID,
				)
				queued = err == nil
			}
		}
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(etx, routes.EnvironmentSourceEdit.URL(params.routeParams()), http.StatusSeeOther)
	}
	message := "Environment source updated"
	if queued {
		message = "Environment source updated and replacement Build queued"
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	if !setupComplete {
		return inertia.Redirect(etx, routes.EnvironmentSetup.URL(params.routeParams()), http.StatusSeeOther)
	}
	return inertia.Redirect(etx, routes.EnvironmentEdit.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) Destroy(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = controller.setup.DeleteEnvironment(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		destination := routes.EnvironmentShow.URL(params.routeParams())
		if overview, overviewErr := controller.setup.Overview(etx.Request().Context(), params.ApplicationID, params.EnvironmentID); overviewErr == nil && !overview.SetupComplete {
			destination = routes.EnvironmentSetup.URL(params.routeParams())
		}
		return inertia.Redirect(etx, destination, http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment permanently deleted")
	return inertia.Redirect(etx, routes.Environments.URL(), http.StatusSeeOther)
}

func (controller Environments) Setup(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	overview, err := controller.setup.Overview(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if overview.SetupComplete {
		return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
	}
	options, err := controller.setup.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Applications/Environments/Setup", inertia.Props{
		"auth": authProps(etx), "environment": environmentOverviewProps(overview), "options": options,
		"setupUrl": routes.EnvironmentSetup.URL(params.routeParams()), "flash": environmentFlashProps(etx),
	})
}

func environmentFlashProps(etx *echo.Context) []inertia.Props {
	flashes := request.ExtractContext[[]cookies.FlashMessage](etx.Request().Context(), request.SessionFlashesKey)
	props := make([]inertia.Props, 0, len(flashes))
	for _, flash := range flashes {
		props = append(props, inertia.Props{"type": flash.Type, "message": flash.Message})
	}
	return props
}

type environmentSetupPayload struct {
	Hostname      string                                   `json:"hostname"`
	ContainerPort int32                                    `json:"containerPort"`
	HealthPath    string                                   `json:"healthPath"`
	BPGOTargets   string                                   `json:"bpGoTargets"`
	Resources     []services.EnvironmentSetupResourceInput `json:"resources"`
	Secrets       []services.EnvironmentSetupSecretInput   `json:"secrets"`
}

func (controller Environments) CompleteSetup(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload environmentSetupPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		_, err = controller.setup.Complete(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, cookies.ExtractFromCookieApp(etx).UserID, services.EnvironmentSetupInput{
			Hostname: payload.Hostname, ContainerPort: payload.ContainerPort,
			HealthPath: payload.HealthPath, BPGOTargets: payload.BPGOTargets,
			Resources: payload.Resources, Secrets: payload.Secrets,
		})
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			overview, overviewErr := controller.setup.Overview(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
			options, optionsErr := controller.setup.Options(etx.Request().Context())
			if overviewErr == nil && optionsErr == nil {
				return inertia.Page(etx, "Applications/Environments/Setup", inertia.Props{
					"auth": authProps(etx), "environment": environmentOverviewProps(overview), "options": options,
					"setupUrl": routes.EnvironmentSetup.URL(params.routeParams()),
				}, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
		}
		overview, overviewErr := controller.setup.Overview(etx.Request().Context(), params.ApplicationID, params.EnvironmentID)
		options, optionsErr := controller.setup.Options(etx.Request().Context())
		if overviewErr == nil && optionsErr == nil {
			return inertia.Page(etx, "Applications/Environments/Setup", inertia.Props{
				"auth": authProps(etx), "environment": environmentOverviewProps(overview), "options": options,
				"setupUrl": routes.EnvironmentSetup.URL(params.routeParams()), "setupError": err.Error(),
			})
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, "Environment setup failed")
		return inertia.Redirect(etx, routes.EnvironmentSetup.URL(params.routeParams()), http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment setup completed and the first build was queued")
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}

func (controller Environments) CreateSecret(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload struct{ Key, Value string }
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		_, err = controller.secrets.CreateUser(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, cookies.ExtractFromCookieApp(etx).UserID, payload.Key, payload.Value)
	}
	return controller.secretRedirect(etx, params, err, "Secret added")
}

func (controller Environments) RotateSecret(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	secretID, parseErr := uuid.Parse(etx.Param("secretID"))
	if err == nil {
		err = parseErr
	}
	var payload struct{ Value string }
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		_, err = controller.secrets.RotateUser(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, secretID, cookies.ExtractFromCookieApp(etx).UserID, payload.Value)
	}
	return controller.secretRedirect(etx, params, err, "Secret rotated")
}

func (controller Environments) ArchiveSecret(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	secretID, parseErr := uuid.Parse(etx.Param("secretID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		_, err = controller.secrets.ArchiveUser(etx.Request().Context(), params.ApplicationID, params.EnvironmentID, secretID, cookies.ExtractFromCookieApp(etx).UserID)
	}
	return controller.secretRedirect(etx, params, err, "Secret archived")
}

func (controller Environments) secretRedirect(etx *echo.Context, params environmentPathIDs, err error, success string) error {
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, success)
	}
	return inertia.Redirect(etx, routes.EnvironmentShow.URL(params.routeParams()), http.StatusSeeOther)
}
