package controllers

import (
	"encoding/json"
	"errors"
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

	"github.com/labstack/echo/v5"
)

type DatabaseClusters struct {
	service *services.DatabaseClusters
}

func NewDatabaseClusters(service *services.DatabaseClusters) DatabaseClusters {
	return DatabaseClusters{service: service}
}

func (controller DatabaseClusters) RegisterRoutes(r *router.Router) error {
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.ResourceDatabaseNew, controller.New},
		{http.MethodPost, routes.ResourceDatabaseCreate, controller.Create},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{
			Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(),
			Handler: definition.handler, Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller DatabaseClusters) New(etx *echo.Context) error {
	return controller.renderNew(etx, nil)
}

func (controller DatabaseClusters) renderNew(etx *echo.Context, options ...inertia.PageOption) error {
	clusterOptions, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Resources/DatabaseNew", inertia.Props{
		"auth": authProps(etx), "options": clusterOptions,
	}, options...)
}

type databaseClusterPayload struct {
	Name                      string                                  `json:"name"`
	Slug                      string                                  `json:"slug"`
	Engine                    string                                  `json:"engine"`
	EngineVersion             string                                  `json:"engineVersion"`
	SharingMode               string                                  `json:"sharingMode"`
	DesiredInstallationMethod string                                  `json:"desiredInstallationMethod"`
	AdministratorUsername     string                                  `json:"administratorUsername"`
	AdministratorPassword     string                                  `json:"administratorPassword"`
	Endpoint                  services.DatabaseClusterEndpointInput   `json:"endpoint"`
	Placement                 *services.DatabaseClusterPlacementInput `json:"placement"`
	Database                  *databaseResourcePayload                `json:"database"`
}

type databaseResourcePayload struct {
	Name         string `json:"name"`
	Encoding     string `json:"encoding"`
	Collation    string `json:"collation"`
	ResourceName string `json:"resourceName"`
	ResourceSlug string `json:"resourceSlug"`
	SharingScope string `json:"sharingScope"`
}

func (controller DatabaseClusters) Create(etx *echo.Context) error {
	var payload databaseClusterPayload
	err := etx.Bind(&payload)
	sharingMode := strings.ToLower(strings.TrimSpace(payload.SharingMode))
	var resource models.ResourceEntity
	if err == nil && payload.Database == nil {
		err = domainPayloadError("database", "Database Resource details are required")
	}
	if err == nil {
		_, resource, err = controller.service.CreateResource(etx.Request().Context(), services.CreateDatabaseClusterInput{
			Name: payload.Name, Slug: payload.Slug, Engine: payload.Engine, EngineVersion: payload.EngineVersion,
			SharingMode: sharingMode, DesiredInstallationMethod: payload.DesiredInstallationMethod,
			Topology: json.RawMessage(`{"primary_count":1,"replica_count":0}`), MaintenancePolicy: json.RawMessage(`{}`),
			AdministratorUsername: payload.AdministratorUsername, AdministratorPassword: payload.AdministratorPassword,
			Endpoint: payload.Endpoint, Placement: payload.Placement,
		}, services.PublishDatabaseInput{
			Name: payload.Database.Name, Encoding: payload.Database.Encoding, Collation: payload.Database.Collation,
			Settings: json.RawMessage(`{}`), ResourceName: payload.Database.ResourceName,
			ResourceSlug: payload.Database.ResourceSlug,
			SharingScope: models.ResourceSharingScopeEnum(payload.Database.SharingScope),
		})
	}
	if err != nil {
		if fieldErrors, ok := validation.As(err); ok {
			return controller.renderNew(etx, inertia.WithValidationErrors(fieldErrors.ToMap()))
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(etx, routes.ResourceDatabaseNew.URL(), http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Database Resource created")
	return inertia.Redirect(etx, routes.ResourceShow.URL(resource.ID), http.StatusSeeOther)
}
