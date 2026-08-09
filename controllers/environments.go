package controllers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/request"
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

type Environments struct {
	envSetupSvc        *services.EnvironmentSetup
	envSecretsSvc      *services.EnvironmentSecrets
	applicationsSvc    *services.ApplicationSetup
	metricsSvc         services.MetricRollupService
	logsSvc            *services.EnvironmentLogs
	appTelemetrySvc    *services.EnvironmentApplicationTelemetry
	dnsSvc             *services.EnvironmentDNS
	releaseCommandsSvc *services.ReleaseCommandExecution
}

func NewEnvironments(
	setup *services.EnvironmentSetup,
	secrets *services.EnvironmentSecrets,
	applications *services.ApplicationSetup,
	metric services.MetricRollupService,
	logs *services.EnvironmentLogs,
	appTelemetry *services.EnvironmentApplicationTelemetry,
	dns *services.EnvironmentDNS,
	releaseCommands *services.ReleaseCommandExecution,
) Environments {
	return Environments{
		envSetupSvc:        setup,
		envSecretsSvc:      secrets,
		applicationsSvc:    applications,
		metricsSvc:         metric,
		logsSvc:            logs,
		appTelemetrySvc:    appTelemetry,
		dnsSvc:             dns,
		releaseCommandsSvc: releaseCommands,
	}
}

func (c Environments) RegisterRoutes(router *router.Router) error {
	auth := []echo.MiddlewareFunc{middleware.AuthOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.Environments, c.Index},
		{http.MethodGet, routes.EnvironmentNew, c.New},
		{http.MethodPost, routes.EnvironmentCreate, c.Create},
		{http.MethodGet, routes.EnvironmentShow, c.Show},
		{http.MethodGet, routes.EnvironmentTelemetry, c.Telemetry},
		{http.MethodGet, routes.EnvironmentTelemetryLogs, c.TelemetryLogs},
		{http.MethodGet, routes.EnvironmentTelemetryQueries, c.TelemetryQueries},
		{http.MethodGet, routes.EnvironmentTelemetryTrace, c.TelemetryTrace},
		{http.MethodGet, routes.EnvironmentReleases, c.Releases},
		{http.MethodGet, routes.EnvironmentBuilds, c.Builds},
		{http.MethodGet, routes.EnvironmentSecrets, c.Secrets},
		{http.MethodPost, routes.EnvironmentRestart, c.Restart},
		{http.MethodPost, routes.EnvironmentStart, c.Start},
		{http.MethodGet, routes.EnvironmentLogs, c.Logs},
		{http.MethodGet, routes.EnvironmentEdit, c.Edit},
		{http.MethodPatch, routes.EnvironmentUpdate, c.Update},
		{http.MethodDelete, routes.EnvironmentDestroy, c.Destroy},
		{http.MethodGet, routes.EnvironmentSourceEdit, c.EditSource},
		{http.MethodPatch, routes.EnvironmentSourceUpdate, c.UpdateSource},
		{http.MethodGet, routes.EnvironmentBuildLogs, c.BuildLogs},
		{http.MethodGet, routes.EnvironmentDeploymentEvents, c.DeploymentEvents},
		{http.MethodPost, routes.EnvironmentBuildStart, c.StartBuild},
		{http.MethodPost, routes.EnvironmentBuildStop, c.StopBuild},
		{http.MethodPost, routes.EnvironmentBuildRetry, c.RetryBuild},
		{http.MethodGet, routes.EnvironmentReleaseCommandLogs, c.ReleaseCommandLogs},
		{http.MethodPost, routes.EnvironmentReleaseCommandRetry, c.RetryReleaseCommand},
		{http.MethodPost, routes.EnvironmentDeploymentsCreate, c.Deploy},
		{http.MethodPost, routes.EnvironmentPromoteToProduction, c.PromoteToProduction},
		{http.MethodPost, routes.EnvironmentReleaseDeploymentsCreate, c.RedeployRelease},
		{http.MethodPost, routes.EnvironmentDeploymentRetry, c.RetryDeployment},
		{http.MethodPost, routes.EnvironmentAPITokenRotate, c.RotateAPIToken},
		{http.MethodPost, routes.EnvironmentDNSAdopt, c.AdoptDNS},
		{http.MethodPost, routes.EnvironmentDNSRetry, c.RetryDNS},
		{http.MethodPost, routes.EnvironmentDNSRefresh, c.RefreshDNS},
		{http.MethodPost, routes.EnvironmentSecretsCreate, c.CreateSecret},
		{http.MethodPost, routes.EnvironmentSecretsBulkCreate, c.BulkCreateSecrets},
		{http.MethodPost, routes.EnvironmentSecretRotate, c.RotateSecret},
		{http.MethodDelete, routes.EnvironmentSecretDestroy, c.ArchiveSecret},
	}
	registered := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		middlewares := auth
		if definition.route.Name() == routes.EnvironmentReleaseCommandRetry.Name() {
			middlewares = []echo.MiddlewareFunc{middleware.AdminOnly}
		}
		_, err := router.AddRoute(
			echo.Route{
				Method:      definition.method,
				Path:        definition.route.Path(),
				Name:        definition.route.Name(),
				Handler:     definition.handler,
				Middlewares: middlewares,
			},
		)
		registered = append(registered, err)
	}
	return errors.Join(registered...)
}

func (c Environments) RotateAPIToken(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	token, err := c.envSetupSvc.RotateAPIToken(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if err != nil {
		return etx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusCreated, map[string]string{"token": token})
}

func (c Environments) Logs(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	snapshot, err := c.logsSvc.Snapshot(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
		etx.QueryParam("after"),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	if errors.Is(err, services.ErrInvalidEnvironmentLogCursor) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment logs",
			"environment_id", params.EnvironmentID,
			"error", err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Environment logs could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (c Environments) TelemetryLogs(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	snapshot, err := c.appTelemetrySvc.Logs(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
		etx.QueryParam("after"),
		services.ParseTelemetryRange(etx.QueryParam("range")),
		etx.QueryParam("search"),
	)
	if errors.Is(err, services.ErrInvalidSystemLogCursor) ||
		errors.Is(err, services.ErrInvalidSystemLogSearch) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	if errors.Is(err, services.ErrEnvironmentOpenTelemetryUnavailable) {
		return etx.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment OpenTelemetry logs",
			"environment_id", params.EnvironmentID,
			"error", err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "OpenTelemetry logs could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (c Environments) TelemetryQueries(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	queries, err := c.appTelemetrySvc.SlowQueries(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
		services.ParseTelemetryRange(etx.QueryParam("range")),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	if errors.Is(err, services.ErrEnvironmentOpenTelemetryUnavailable) {
		return etx.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment OpenTelemetry slow queries",
			"environment_id", params.EnvironmentID,
			"error", err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Slow queries could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, map[string]any{"queries": queries})
}

func (c Environments) TelemetryTrace(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	spans, err := c.appTelemetrySvc.Trace(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
		etx.Param("traceID"),
	)
	if errors.Is(err, services.ErrInvalidTraceID) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	if errors.Is(err, services.ErrEnvironmentOpenTelemetryUnavailable) {
		return etx.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment OpenTelemetry trace",
			"environment_id", params.EnvironmentID,
			"error", err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Trace could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, map[string]any{"spans": spans})
}

func (c Environments) DeploymentEvents(etx *echo.Context) error {
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
		return etx.JSON(
			http.StatusBadRequest,
			map[string]string{"error": "Deployment event cursor is invalid"},
		)
	}
	snapshot, err := c.envSetupSvc.DeploymentEvents(
		etx.Request().Context(),
		environmentID,
		deploymentID,
		after,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Deployment not found"})
	}
	if err != nil {
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Deployment events could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (c Environments) BuildLogs(etx *echo.Context) error {
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
		return etx.JSON(
			http.StatusBadRequest,
			map[string]string{"error": "Build log cursor is invalid"},
		)
	}
	snapshot, err := c.envSetupSvc.BuildLogs(
		etx.Request().Context(),
		environmentID,
		buildID,
		after,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Build not found"})
	}
	if err != nil {
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Build logs could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (c Environments) Index(etx *echo.Context) error {
	environments, err := c.envSetupSvc.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Environments/Index", inertia.Props{
		"auth":         authProps(etx),
		"environments": environments,
		"flash":        environmentFlashProps(etx),
	})
}

func (c Environments) New(etx *echo.Context) error {
	applicationID, err := uuid.Parse(etx.Param("applicationID"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return c.newEnvironmentPage(etx, applicationID, nil)
}

func (c Environments) Create(etx *echo.Context) error {
	applicationID, err := uuid.Parse(etx.Param("applicationID"))
	var payload applicationEnvironmentCreationPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	application, applicationErr := c.applicationsSvc.Overview(
		etx.Request().Context(),
		applicationID,
	)
	if err == nil {
		err = applicationErr
	}
	var prepared preparedApplicationEnvironmentCreation
	if err == nil {
		prepared, err = payload.prepare(
			application.Name,
			application.Slug,
			payload.EnvironmentName,
			payload.EnvironmentSlug,
			payload.EnvironmentKind,
		)
	}
	var created services.ApplicationSetupResult
	if err == nil {
		created, err = c.applicationsSvc.CreateEnvironment(
			etx.Request().Context(),
			applicationID,
			prepared.source,
		)
	}
	var completed services.EnvironmentSetupResult
	if err == nil {
		completed, err = c.envSetupSvc.Complete(
			etx.Request().Context(),
			applicationID,
			created.Environment.ID,
			cookies.ExtractFromCookieApp(etx).UserID,
			prepared.setup,
		)
		if err != nil {
			cleanupErr := c.applicationsSvc.DeleteIncompleteEnvironment(
				etx.Request().Context(),
				applicationID,
				created.Environment.ID,
			)
			if cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}
	if err != nil {
		return c.newEnvironmentPage(etx, applicationID, err)
	}
	message := "Environment created"
	if payload.Deploy && completed.DeploymentDeferred {
		message = "Environment created; deployment will start after managed DNS is ready"
	} else if payload.Deploy {
		message = "Environment created and deployment queued"
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(
			environmentPathIDs{
				ApplicationID: applicationID,
				EnvironmentID: created.Environment.ID,
			}.routeParams(),
		),
		http.StatusSeeOther,
	)
}

func (c Environments) newEnvironmentPage(
	etx *echo.Context,
	applicationID uuid.UUID,
	operationErr error,
) error {
	application, err := c.applicationsSvc.Overview(etx.Request().Context(), applicationID)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	applicationOptions, err := c.applicationsSvc.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	environmentOptions, err := c.envSetupSvc.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	props := inertia.Props{
		"auth": authProps(etx), "application": application,
		"options": applicationCreationOptionsProps(applicationOptions, environmentOptions),
	}
	if operationErr == nil {
		return inertia.Page(etx, "Applications/Environments/New", props)
	}
	if validationErrors, ok := validation.As(operationErr); ok {
		return inertia.Page(
			etx,
			"Applications/Environments/New",
			props,
			inertia.WithValidationErrors(validationErrors.ToMap()),
		)
	}
	props["setupError"] = operationErr.Error()
	return inertia.Page(etx, "Applications/Environments/New", props)
}

func (c Environments) RetryDeployment(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	deploymentID, parseErr := uuid.Parse(etx.Param("deploymentID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		_, err = c.envSetupSvc.RetryDeployment(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			deploymentID,
			cookies.ExtractFromCookieApp(etx).UserID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Deployment retry queued")
	}
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
}

func (c Environments) PromoteToProduction(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var result services.PromotionResult
	if err == nil {
		result, err = c.envSetupSvc.PromoteToProduction(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			cookies.ExtractFromCookieApp(etx).UserID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else if result.Deferred {
		_ = cookies.AddFlash(
			etx,
			cookies.FlashSuccess,
			"DNS reconciliation queued; promotion will complete when DNS is ready",
		)
	} else {
		_ = cookies.AddFlash(
			etx,
			cookies.FlashSuccess,
			"Staging release promoted to production; deployment queued",
		)
	}
	returnURL := routes.EnvironmentShow.URL(params.routeParams())
	if strings.EqualFold(
		strings.TrimSpace(etx.Request().Header.Get("X-Deploycrate-Return-To")),
		"application",
	) {
		returnURL = routes.ApplicationShow.URL(params.ApplicationID)
	}
	return inertia.Redirect(
		etx,
		returnURL,
		http.StatusSeeOther,
	)
}

func (c Environments) Deploy(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	deferred := false
	var payload struct {
		Reference string `json:"reference"`
	}
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		userID := cookies.ExtractFromCookieApp(etx).UserID
		var result services.SourceDeploymentResult
		result, err = c.envSetupSvc.RequestSourceDeployment(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			&userID,
			"user",
			payload.Reference,
		)
		deferred = result.DeploymentDeferred
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		message := "Deployment queued"
		if deferred {
			message = "DNS reconciliation queued; deployment will start when DNS is ready"
		}
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	}
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
}

func (c Environments) RedeployRelease(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	releaseID, parseErr := uuid.Parse(etx.Param("releaseID"))
	deferred := false
	if err == nil {
		err = parseErr
	}
	if err == nil {
		var result services.ReleaseDeploymentResult
		result, err = c.envSetupSvc.QueueReleaseDeployment(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			releaseID,
			cookies.ExtractFromCookieApp(etx).UserID,
		)
		deferred = result.DeploymentDeferred
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else if deferred {
		_ = cookies.AddFlash(
			etx,
			cookies.FlashSuccess,
			"DNS reconciliation queued; deployment will start when DNS is ready",
		)
	} else {
		_ = cookies.AddFlash(
			etx,
			cookies.FlashSuccess,
			"Release deployment queued with current Environment secrets",
		)
	}
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
}

func (c Environments) StartBuild(etx *echo.Context) error {
	return c.buildAction(etx, "Build start requested", c.envSetupSvc.StartBuild)
}

func (c Environments) StopBuild(etx *echo.Context) error {
	return c.buildAction(etx, "Build cancellation requested", c.envSetupSvc.StopBuild)
}

func (c Environments) RetryBuild(etx *echo.Context) error {
	return c.buildAction(etx, "Build retry queued", c.envSetupSvc.RetryBuild)
}

func (c Environments) buildAction(
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
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
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
		"sourceType":      overview.SourceType,
		"environment": environmentIdentityProps{
			ID:   overview.Environment.ID,
			Name: overview.Environment.Name,
			Kind: overview.Environment.Kind,
		},
		"setupComplete":                overview.SetupComplete,
		"repository":                   overview.Repository,
		"reference":                    overview.Reference,
		"contextPath":                  overview.ContextPath,
		"registryName":                 overview.RegistryName,
		"registryEndpoint":             overview.RegistryEndpoint,
		"runtimeServerIds":             overview.RuntimeServerIDs,
		"runtimeTargetIds":             overview.RuntimeTargetIDs,
		"runtimeServers":               overview.RuntimeServers,
		"deployability":                overview.Deployability,
		"secrets":                      overview.Secrets,
		"variables":                    overview.Variables,
		"domain":                       overview.Domain,
		"resources":                    overview.Resources,
		"builds":                       overview.Builds,
		"releases":                     overview.Releases,
		"deployments":                  overview.Deployments,
		"instances":                    overview.Instances,
		"processes":                    overview.Processes,
		"releaseCommands":              overview.ReleaseCommands,
		"apiTokenPrefix":               overview.APITokenPrefix,
		"dns":                          overview.DNS,
		"canPromoteToProduction":       overview.CanPromoteToProduction,
		"promotionTargetName":          overview.PromotionTargetName,
		"latestSuccessfulDeploymentId": overview.LatestSuccessfulDeploymentID,
		"latestSuccessfulReleaseId":    overview.LatestSuccessfulReleaseID,
	}
}

func (c Environments) ReleaseCommandLogs(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	executionID, parseErr := uuid.Parse(etx.Param("executionID"))
	if err == nil {
		err = parseErr
	}
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Release command not found"},
		)
	}
	logs, err := c.releaseCommandsSvc.Logs(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
		executionID,
	)
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Release command not found"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, map[string]any{"logs": logs})
}

func (c Environments) RetryReleaseCommand(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	executionID, parseErr := uuid.Parse(etx.Param("executionID"))
	if err == nil {
		err = parseErr
	}
	var payload struct {
		TargetID string `json:"targetId"`
	}
	if err == nil {
		err = etx.Bind(&payload)
	}
	var targetID *uuid.UUID
	if err == nil && strings.TrimSpace(payload.TargetID) != "" {
		parsed, targetErr := uuid.Parse(payload.TargetID)
		if targetErr != nil {
			err = targetErr
		} else {
			targetID = &parsed
		}
	}
	if err == nil {
		err = c.releaseCommandsSvc.Retry(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			executionID,
			cookies.ExtractFromCookieApp(etx).UserID,
			targetID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Release command retry queued")
	}
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
}

func (ids environmentPathIDs) routeParams() routes.EnvironmentParams {
	return routes.EnvironmentParams{
		ApplicationID: ids.ApplicationID.String(),
		EnvironmentID: ids.EnvironmentID.String(),
	}
}

func environmentSectionReturnURL(etx *echo.Context, params environmentPathIDs) string {
	switch strings.TrimSpace(etx.Request().Header.Get("X-Deploycrate-Section")) {
	case "releases":
		return routes.EnvironmentReleases.URL(params.routeParams())
	case "builds":
		return routes.EnvironmentBuilds.URL(params.routeParams())
	case "telemetry":
		return routes.EnvironmentTelemetry.URL(params.routeParams())
	case "secrets":
		return routes.EnvironmentSecrets.URL(params.routeParams())
	default:
		return routes.EnvironmentShow.URL(params.routeParams())
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

func (c Environments) Show(etx *echo.Context) error {
	return c.showSection(etx, "overview")
}

func (c Environments) Telemetry(etx *echo.Context) error {
	return c.showSection(etx, "telemetry")
}

func (c Environments) Releases(etx *echo.Context) error {
	return c.showSection(etx, "releases")
}

func (c Environments) Builds(etx *echo.Context) error {
	return c.showSection(etx, "builds")
}

func (c Environments) Secrets(etx *echo.Context) error {
	return c.showSection(etx, "secrets")
}

func (c Environments) showSection(etx *echo.Context, section string) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	overview, err := c.envSetupSvc.Overview(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if !overview.SetupComplete {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	telemetryRange := services.ParseTelemetryRange(etx.QueryParam("range"))
	openTelemetryAvailable, openTelemetryErr := c.appTelemetrySvc.Enabled(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if openTelemetryErr != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	applicationTelemetry := services.ApplicationTelemetry{
		History: []services.ApplicationTelemetryPoint{},
		Database: services.DatabaseTelemetry{
			History: []services.DatabaseTelemetryPoint{},
		},
		RecentTraces: []services.TraceSummary{},
		Routes:       []services.RouteTelemetry{},
		Queries:      []services.QueryTelemetry{},
	}
	if section == "telemetry" && openTelemetryAvailable {
		applicationTelemetry, err = c.appTelemetrySvc.Snapshot(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			telemetryRange,
		)
		if err != nil {
			slog.WarnContext(
				etx.Request().Context(),
				"failed to load Environment application telemetry",
				"environment_id", params.EnvironmentID,
				"error", err,
			)
		}
	}
	telemetry, telemetryErr := c.metricsSvc.EnvironmentTelemetry(
		etx.Request().Context(),
		params.EnvironmentID,
		telemetryRange,
	)
	if telemetryErr != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Environment telemetry",
			"environment_id", params.EnvironmentID,
			"error", telemetryErr,
		)
		telemetry = services.EnvironmentTelemetryResult{Rows: []services.AttributedTelemetryRow{}}
	}
	container, containerErr := c.envSetupSvc.ServingContainer(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if containerErr != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to inspect Environment serving container",
			"environment_id", params.EnvironmentID,
			"error", containerErr,
		)
		container = services.EnvironmentServingContainer{}
	}
	return inertia.Page(etx, "Applications/Environments/Show", inertia.Props{
		"auth": authProps(etx), "environment": environmentOverviewProps(overview),
		"telemetry": telemetry.Rows, "container": container,
		"host": telemetry.HostUsage, "telemetryRange": telemetryRange,
		"openTelemetryAvailable": openTelemetryAvailable,
		"applicationTelemetry":   applicationTelemetry,
		"section":                section, "flash": environmentFlashProps(etx),
	})
}

func (c Environments) Restart(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.envSetupSvc.RestartServingContainer(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Container restarted")
	}
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) Start(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.envSetupSvc.StartServingContainer(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Container started")
	}
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) Edit(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	data, err := c.envSetupSvc.EditData(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	options, err := c.envSetupSvc.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Applications/Environments/Edit", inertia.Props{
		"auth":          authProps(etx),
		"environment":   environmentOverviewProps(data.Overview),
		"configuration": data.Configuration,
		"options":       options,
		"flash":         environmentFlashProps(etx),
	})
}

type environmentEditPayload struct {
	Name          string                                   `json:"name"`
	Slug          string                                   `json:"slug"`
	Kind          string                                   `json:"kind"`
	Hostname      string                                   `json:"hostname"`
	ContainerPort int32                                    `json:"containerPort"`
	HealthPath    string                                   `json:"healthPath"`
	Processes     []models.EnvironmentProcessInput         `json:"processes"`
	Resources     []services.EnvironmentSetupResourceInput `json:"resources"`
	DNSMode       string                                   `json:"dnsMode"`
	DNSZoneID     string                                   `json:"dnsZoneId"`
}

func (c Environments) Update(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload environmentEditPayload
	var dnsZoneID *uuid.UUID
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		if strings.EqualFold(strings.TrimSpace(payload.DNSMode), services.DNSModeCloudflare) {
			parsed, parseErr := uuid.Parse(payload.DNSZoneID)
			if parseErr != nil {
				err = errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   "dnsZoneId",
							Code:    "required",
							Message: "select a Cloudflare DNS zone",
						},
					},
				)
			} else {
				dnsZoneID = &parsed
			}
		}
		if err == nil {
			err = c.envSetupSvc.UpdateEnvironment(
				etx.Request().Context(), params.ApplicationID, params.EnvironmentID,
				cookies.ExtractFromCookieApp(etx).UserID,
				services.EnvironmentEditInput{
					Name:          payload.Name,
					Slug:          payload.Slug,
					Kind:          payload.Kind,
					Hostname:      payload.Hostname,
					ContainerPort: payload.ContainerPort,
					HealthPath:    payload.HealthPath,
					Processes:     payload.Processes,
					Resources:     payload.Resources,
					DNS: services.EnvironmentDNSInput{
						Mode:   payload.DNSMode,
						ZoneID: dnsZoneID,
					},
				},
			)
		}
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			data, dataErr := c.envSetupSvc.EditData(
				etx.Request().Context(),
				params.ApplicationID,
				params.EnvironmentID,
			)
			options, optionsErr := c.envSetupSvc.Options(etx.Request().Context())
			if dataErr == nil && optionsErr == nil {
				configuration := data.Configuration
				configuration.Name = payload.Name
				configuration.Slug = payload.Slug
				configuration.Kind = payload.Kind
				configuration.Hostname = payload.Hostname
				configuration.ContainerPort = payload.ContainerPort
				configuration.HealthPath = payload.HealthPath
				configuration.Processes = payload.Processes
				configuration.Resources = payload.Resources
				configuration.DNSMode = payload.DNSMode
				configuration.DNSZoneID = dnsZoneID
				return inertia.Page(etx, "Applications/Environments/Edit", inertia.Props{
					"auth": authProps(etx), "environment": environmentOverviewProps(data.Overview),
					"configuration": configuration, "options": options,
				}, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(
			etx,
			routes.EnvironmentEdit.URL(params.routeParams()),
			http.StatusSeeOther,
		)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment changes saved")
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) AdoptDNS(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.dnsSvc.ConfirmAdoption(etx.Request().Context(), params.EnvironmentID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(
			etx,
			cookies.FlashSuccess,
			"Existing DNS records will be adopted and replaced",
		)
	}
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) RetryDNS(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.dnsSvc.Retry(etx.Request().Context(), params.EnvironmentID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "DNS reconciliation queued")
	}
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) RefreshDNS(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.dnsSvc.Refresh(etx.Request().Context(), params.EnvironmentID)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "DNS re-sync queued")
	}
	return inertia.Redirect(
		etx,
		routes.EnvironmentShow.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) EditSource(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	details, err := c.applicationsSvc.DetailsForEnvironment(
		etx.Request().Context(),
		params.ApplicationID,
		params.EnvironmentID,
	)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	options, err := c.applicationsSvc.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	returnURL := routes.EnvironmentEdit.URL(params.routeParams())
	return inertia.Page(etx, "Applications/Source/Edit", inertia.Props{
		"auth": authProps(
			etx,
		),
		"application": details,
		"options":     applicationSetupOptionsProps(options),
		"updateUrl":   routes.EnvironmentSourceUpdate.URL(params.routeParams()),
		"returnUrl":   returnURL,
		"navigation":  "environment",
	})
}

func (c Environments) UpdateSource(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	queued := false
	var payload applicationSetupPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var data services.ApplicationSetupData
	if err == nil {
		data, err = payload.serviceData()
	}
	if err == nil {
		details, detailsErr := c.applicationsSvc.DetailsForEnvironment(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
		)
		if detailsErr != nil {
			err = errors.New("Environment source is unavailable")
		} else {
			err = c.applicationsSvc.UpdateEnvironmentSource(
				etx.Request().Context(),
				params.ApplicationID,
				params.EnvironmentID,
				data,
			)
			if err == nil && details.SetupComplete {
				userID := cookies.ExtractFromCookieApp(etx).UserID
				_, err = c.envSetupSvc.QueueSourceDeployment(
					etx.Request().Context(), params.ApplicationID, params.EnvironmentID,
					&userID, "user", "",
				)
				queued = err == nil
			}
		}
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(
			etx,
			routes.EnvironmentSourceEdit.URL(params.routeParams()),
			http.StatusSeeOther,
		)
	}
	message := "Environment source updated"
	if queued {
		message = "Environment source updated and replacement Build queued"
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, message)
	return inertia.Redirect(
		etx,
		routes.EnvironmentEdit.URL(params.routeParams()),
		http.StatusSeeOther,
	)
}

func (c Environments) Destroy(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	if err == nil {
		err = c.envSetupSvc.DeleteEnvironment(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
		)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(
			etx,
			routes.EnvironmentSecrets.URL(params.routeParams()),
			http.StatusSeeOther,
		)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Environment permanently deleted")
	return inertia.Redirect(etx, routes.Environments.URL(), http.StatusSeeOther)
}

func environmentFlashProps(etx *echo.Context) []inertia.Props {
	flashes := request.ExtractContext[[]cookies.FlashMessage](
		etx.Request().Context(),
		request.SessionFlashesKey,
	)
	props := make([]inertia.Props, 0, len(flashes))
	for _, flash := range flashes {
		props = append(props, inertia.Props{"type": flash.Type, "message": flash.Message})
	}
	return props
}

func (c Environments) CreateSecret(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload struct{ Key, Value string }
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		_, err = c.envSecretsSvc.CreateUser(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			cookies.ExtractFromCookieApp(etx).UserID,
			payload.Key,
			payload.Value,
		)
	}
	return c.secretRedirect(etx, params, err, "Secret added")
}

func (c Environments) BulkCreateSecrets(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	var payload struct {
		Secrets []services.EnvironmentSecretInput `json:"secrets"`
	}
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		_, err = c.envSecretsSvc.BulkCreateUser(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			cookies.ExtractFromCookieApp(etx).UserID,
			payload.Secrets,
		)
	}
	return c.secretRedirect(etx, params, err, "Secrets added")
}

func (c Environments) RotateSecret(etx *echo.Context) error {
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
		_, err = c.envSecretsSvc.RotateUser(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			secretID,
			cookies.ExtractFromCookieApp(etx).UserID,
			payload.Value,
		)
	}
	return c.secretRedirect(etx, params, err, "Secret rotated")
}

func (c Environments) ArchiveSecret(etx *echo.Context) error {
	params, err := environmentPathParams(etx)
	secretID, parseErr := uuid.Parse(etx.Param("secretID"))
	if err == nil {
		err = parseErr
	}
	if err == nil {
		_, err = c.envSecretsSvc.ArchiveUser(
			etx.Request().Context(),
			params.ApplicationID,
			params.EnvironmentID,
			secretID,
			cookies.ExtractFromCookieApp(etx).UserID,
		)
	}
	return c.secretRedirect(etx, params, err, "Secret archived")
}

func (c Environments) secretRedirect(
	etx *echo.Context,
	params environmentPathIDs,
	err error,
	success string,
) error {
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, success)
	}
	return inertia.Redirect(
		etx,
		environmentSectionReturnURL(etx, params),
		http.StatusSeeOther,
	)
}
