package api

import (
	"errors"
	"net/http"
	"strings"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/router"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type API struct {
	db           storage.Pool
	environments *services.EnvironmentSetup
	slot         string
	version      string
}

func NewAPI(
	db storage.Pool,
	environments *services.EnvironmentSetup,
	configuration config.Config,
	version services.CurrentVersion,
) API {
	return API{
		db: db, environments: environments,
		slot: configuration.App.Slot, version: string(version),
	}
}

func (a API) RegisterRoutes(r *router.Router) error {
	errs := []error{}

	_, err := r.AddRoute(echo.Route{
		Method:  http.MethodGet,
		Path:    routes.Health.Path(),
		Name:    routes.Health.Name(),
		Handler: a.Health,
	})
	if err != nil {
		errs = append(errs, err)
	}
	_, err = r.AddRoute(echo.Route{
		Method: http.MethodPost, Path: routes.APIEnvironmentDeploymentsCreate.Path(),
		Name: routes.APIEnvironmentDeploymentsCreate.Name(), Handler: a.DeployEnvironment,
	})
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (a API) DeployEnvironment(etx *echo.Context) error {
	environmentID, err := uuid.Parse(etx.Param("environmentID"))
	if err != nil {
		return etx.JSON(http.StatusNotFound, map[string]string{"error": "Environment not found"})
	}
	authorization := strings.Fields(etx.Request().Header.Get("Authorization"))
	if len(authorization) != 2 || !strings.EqualFold(authorization[0], "Bearer") {
		return etx.JSON(http.StatusUnauthorized, map[string]string{"error": "Bearer token is required"})
	}
	environment, err := a.environments.AuthenticateAPIToken(etx.Request().Context(), environmentID, authorization[1])
	if err != nil {
		return etx.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid bearer token"})
	}
	var payload struct {
		Reference string `json:"reference"`
	}
	if err := etx.Bind(&payload); err != nil {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": "Request body is invalid"})
	}
	result, err := a.environments.RequestSourceDeployment(etx.Request().Context(), environment.ApplicationID, environment.ID, nil, "api", payload.Reference)
	if err != nil {
		return etx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	response := map[string]any{"environmentId": environment.ID}
	if result.DeploymentDeferred {
		response["deploymentDeferred"] = true
	}
	if result.Release != nil {
		response["releaseId"] = result.Release.ID
	}
	if result.Deployment != nil {
		response["deploymentId"] = result.Deployment.ID
	}
	return etx.JSON(http.StatusAccepted, response)
}

func (a API) Health(etx *echo.Context) error {
	if err := a.db.Conn().PingContext(etx.Request().Context()); err != nil {
		return etx.JSON(http.StatusServiceUnavailable, "app database is unavailable")
	}
	etx.Response().Header().Set("X-DeployCrate-Slot", a.slot)
	etx.Response().Header().Set("X-DeployCrate-Version", a.version)
	return etx.JSON(http.StatusOK, "app is healthy and running")
}
