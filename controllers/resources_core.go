package controllers

import (
	"encoding/json"
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

type Resources struct {
	service     *services.ResourceManagement
	access      *services.ResourcePrivateAccess
	backups     *services.DatabaseBackups
	restore     *services.DatabaseRestoreWorkflow
	credentials *services.ResourceCredentials
	caddy       services.CaddyRouteService
}

func NewResources(
	service *services.ResourceManagement,
	access *services.ResourcePrivateAccess,
	backups *services.DatabaseBackups,
	restore *services.DatabaseRestoreWorkflow,
	credentials *services.ResourceCredentials,
	caddy services.CaddyRouteService,
) Resources {
	return Resources{
		service:     service,
		access:      access,
		backups:     backups,
		restore:     restore,
		credentials: credentials,
		caddy:       caddy,
	}
}

func (controller Resources) RegisterRoutes(r *router.Router) error {
	definitions := []routeDefinition{
		{http.MethodGet, routes.Resources, controller.Index},
		{http.MethodGet, routes.ResourceNew, controller.New},
		{http.MethodPost, routes.ResourceCreate, controller.Create},
		{http.MethodGet, routes.ResourceShow, controller.Show},
		{http.MethodGet, routes.ResourceDatabases, controller.Databases},
		{http.MethodGet, routes.ResourceBackups, controller.Backups},
		{http.MethodGet, routes.ResourceEndpoints, controller.Endpoints},
		{http.MethodGet, routes.ResourceCredentials, controller.Credentials},
		{http.MethodGet, routes.ResourceHealth, controller.Health},
		{http.MethodPost, routes.ResourceDeploy, controller.Deploy},
		{http.MethodGet, routes.ResourceSettings, controller.Edit},
		{http.MethodPatch, routes.ResourceUpdate, controller.Update},
		{http.MethodDelete, routes.ResourceDestroy, controller.Destroy},
		{
			http.MethodPatch,
			routes.ResourceConnectionEnvironmentKeysUpdate,
			controller.UpdateConnectionEnvironmentKeys,
		},
		{http.MethodPost, routes.ResourceEndpointCreate, controller.CreateEndpoint},
		{http.MethodPatch, routes.ResourceEndpointUpdate, controller.UpdateEndpoint},
		{http.MethodDelete, routes.ResourceEndpointDestroy, controller.DestroyEndpoint},
		{http.MethodPost, routes.ResourcePrivateAccessCreate, controller.EnablePrivateAccess},
		{http.MethodDelete, routes.ResourcePrivateAccessDestroy, controller.DisablePrivateAccess},
		{
			http.MethodPost,
			routes.ResourcePrivateAccessDeviceCreate,
			controller.CreatePrivateAccessDevice,
		},
		{
			http.MethodDelete,
			routes.ResourcePrivateAccessDeviceDestroy,
			controller.DestroyPrivateAccessDevice,
		},
		{http.MethodPost, routes.ResourceCredentialCreate, controller.CreateCredential},
		{http.MethodPatch, routes.ResourceCredentialUpdate, controller.UpdateCredential},
		{http.MethodDelete, routes.ResourceCredentialDestroy, controller.DestroyCredential},
		{
			http.MethodPost,
			routes.ResourceCredentialReveal,
			middleware.IPRateLimiter(5, routes.Resources)(controller.RevealCredential),
		},
		{http.MethodPost, routes.ResourceDatabaseCreateForResource, controller.CreateDatabase},
		{http.MethodDelete, routes.ResourceDatabaseDestroy, controller.DestroyDatabase},
		{http.MethodPost, routes.ResourceInstallationCreate, controller.CreateInstallation},
		{http.MethodGet, routes.ResourceInstallationLogs, controller.InstallationLogs},
		{http.MethodPost, routes.ResourceInstallationStart, controller.StartInstallation},
		{http.MethodPost, routes.ResourceInstallationStop, controller.StopInstallation},
		{http.MethodPost, routes.ResourceInstallationRestart, controller.RestartInstallation},
		{
			http.MethodDelete,
			routes.ResourceInstallationRemove,
			controller.RemoveInstallationContainer,
		},
		{http.MethodPatch, routes.ResourceInstallationUpdate, controller.UpdateInstallation},
		{http.MethodDelete, routes.ResourceInstallationDestroy, controller.DestroyInstallation},
		{http.MethodPost, routes.ResourceVolumeCreate, controller.CreateVolume},
		{http.MethodPatch, routes.ResourceVolumeUpdate, controller.UpdateVolume},
		{http.MethodDelete, routes.ResourceVolumeDestroy, controller.DestroyVolume},
		{http.MethodPost, routes.ResourceMountCreate, controller.CreateMount},
		{http.MethodPatch, routes.ResourceMountUpdate, controller.UpdateMount},
		{http.MethodDelete, routes.ResourceMountDestroy, controller.DestroyMount},
		{http.MethodPost, routes.ResourceHealthCheckCreate, controller.CreateHealthCheck},
		{http.MethodPatch, routes.ResourceHealthCheckUpdate, controller.UpdateHealthCheck},
		{http.MethodDelete, routes.ResourceHealthCheckDestroy, controller.DestroyHealthCheck},
		{http.MethodPost, routes.ResourceBackupPolicyCreate, controller.CreateBackupPolicy},
		{http.MethodPatch, routes.ResourceBackupPolicyUpdate, controller.UpdateBackupPolicy},
		{http.MethodPost, routes.ResourceBackupPolicyPause, controller.PauseBackupPolicy},
		{http.MethodPost, routes.ResourceBackupPolicyResume, controller.ResumeBackupPolicy},
		{http.MethodDelete, routes.ResourceBackupPolicyDestroy, controller.ArchiveBackupPolicy},
		{http.MethodPost, routes.ResourceBackupPolicyRun, controller.RunBackupPolicy},
		{http.MethodPost, routes.ResourceRestoreCreate, controller.CreateRestore},
	}
	return registerRoutes(r, definitions, middleware.AdminOnly)
}

func (controller Resources) Index(etx *echo.Context) error {
	filters := models.ResourceListFilters{
		Search: etx.QueryParam(
			"search",
		),
		Engine:       etx.QueryParam("engine"),
		ResourceType: etx.QueryParam("resourceType"),
	}
	items, err := controller.service.List(etx.Request().Context(), filters)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	return inertia.Page(etx, "Resources/Index", inertia.Props{
		"auth": authProps(etx), "resources": resourceListProps(items), "filters": inertia.Props{
			"search":       filters.Search,
			"engine":       filters.Engine,
			"resourceType": filters.ResourceType,
		},
	})
}

func (controller Resources) New(etx *echo.Context) error {
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	return inertia.Page(
		etx,
		"Resources/New",
		inertia.Props{
			"auth":    authProps(etx),
			"options": resourceOptionsProps(options),
			"flash":   resourceFlashProps(etx),
		},
	)
}

type resourcePayload struct {
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	ResourceType  string          `json:"resourceType"`
	Configuration json.RawMessage `json:"configuration"`
}

func (payload resourcePayload) serviceInput() (services.ResourceInput, error) {
	resourceType, err := models.ParseResourceTypeEnum(
		strings.ToLower(strings.TrimSpace(payload.ResourceType)),
	)
	if err != nil {
		return services.ResourceInput{}, domainPayloadError(
			"resourceType",
			"resource type is invalid",
		)
	}
	return services.ResourceInput{
		Name: payload.Name, Slug: payload.Slug, ResourceType: resourceType,
		Configuration: payload.Configuration,
	}, nil
}

type resourceCreatePayload struct {
	resourcePayload
	Endpoint         *resourceEndpointPayload     `json:"endpoint"`
	Credential       *resourceCredentialPayload   `json:"credential"`
	Installation     *resourceInstallationPayload `json:"installation"`
	Volume           *resourceVolumePayload       `json:"volume"`
	Mount            *resourceMountPayload        `json:"mount"`
	HealthCheck      *resourceHealthCheckPayload  `json:"healthCheck"`
	PrivateNetworkID string                       `json:"privateNetworkId"`
}

func (payload resourceCreatePayload) serviceInput() (services.CreateResourceInput, error) {
	resource, err := payload.resourcePayload.serviceInput()
	if err != nil {
		return services.CreateResourceInput{}, err
	}
	privateNetworkID, err := optionalUUID(payload.PrivateNetworkID)
	if err != nil {
		return services.CreateResourceInput{}, domainPayloadError(
			"privateNetworkId",
			"private network is invalid",
		)
	}
	input := services.CreateResourceInput{Resource: resource, PrivateNetworkID: privateNetworkID}
	if payload.Installation != nil {
		value, valueErr := payload.Installation.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(
				valueErr,
				"installation",
			)
		}
		input.Installation = &value
	}
	if payload.Endpoint != nil {
		value, valueErr := payload.Endpoint.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "endpoint")
		}
		input.Endpoint = &value
	}
	if payload.Credential != nil {
		value, valueErr := payload.Credential.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "credential")
		}
		input.Credential = &value
	}
	if payload.Volume != nil {
		value, valueErr := payload.Volume.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "volume")
		}
		if value.ServerID == uuid.Nil && input.Installation != nil {
			value.ServerID = input.Installation.ServerID
		}
		input.Volume = &value
	}
	if payload.Mount != nil {
		value, valueErr := payload.Mount.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "mount")
		}
		input.Mount = &value
	}
	if payload.HealthCheck != nil {
		value, valueErr := payload.HealthCheck.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "healthCheck")
		}
		input.HealthCheck = &value
	}
	return input, nil
}

func (controller Resources) Create(etx *echo.Context) error {
	var payload resourceCreatePayload
	err := etx.Bind(&payload)
	var input services.CreateResourceInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	var resource models.ResourceEntity
	if err == nil {
		resource, err = controller.service.CreateResource(etx.Request().Context(), input)
	}
	if err != nil {
		return controller.renderCreateError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource created")
	return inertia.Redirect(etx, routes.ResourceShow.URL(resource.ID), http.StatusSeeOther)
}

func (controller Resources) Show(etx *echo.Context) error {
	return controller.showSection(etx, "overview")
}

func (controller Resources) Databases(etx *echo.Context) error {
	return controller.showSection(etx, "databases")
}

func (controller Resources) Backups(etx *echo.Context) error {
	return controller.showSection(etx, "backups")
}

func (controller Resources) Endpoints(etx *echo.Context) error {
	return controller.showSection(etx, "endpoints")
}

func (controller Resources) Credentials(etx *echo.Context) error {
	return controller.showSection(etx, "credentials")
}

func (controller Resources) Health(etx *echo.Context) error {
	return controller.showSection(etx, "health")
}

func (controller Resources) showSection(etx *echo.Context, section string) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return controller.renderShowSection(etx, resourceID, section, nil, nil)
}

func (controller Resources) Deploy(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.DeployResource(etx.Request().Context(), resourceID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Resource deployed")
}

func (controller Resources) Edit(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return controller.renderEdit(etx, resourceID, nil)
}

func (controller Resources) Update(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	var payload resourcePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateResource(etx.Request().Context(), resourceID, input)
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return controller.renderEdit(
				etx,
				resourceID,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return controller.redirectError(etx, routes.ResourceSettings.URL(resourceID), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource updated")
	return inertia.Redirect(etx, routes.ResourceSettings.URL(resourceID), http.StatusSeeOther)
}

func (controller Resources) Destroy(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.ArchiveResource(etx.Request().Context(), resourceID)
	}
	if err != nil {
		return controller.redirectError(etx, routes.ResourceShow.URL(resourceID), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource archived")
	return inertia.Redirect(etx, routes.Resources.URL(), http.StatusSeeOther)
}

func (controller Resources) UpdateConnectionEnvironmentKeys(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	connectionID, connectionErr := uuidPathParam(etx, "connectionID")
	if err == nil {
		err = connectionErr
	}
	var payload struct {
		EnvironmentKeys map[string]string `json:"environmentKeys"`
	}
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = controller.service.UpdateConnectionEnvironmentKeys(
			etx.Request().Context(), resourceID, connectionID, payload.EnvironmentKeys,
		)
	}
	return controller.finishChildMutation(
		etx,
		resourceID,
		err,
		"Connection Environment keys updated",
	)
}
