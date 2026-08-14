package controllers

import (
	"encoding/json"
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
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
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
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return controller.renderShowSection(etx, resourceID, section, nil, nil)
}

func (controller Resources) Deploy(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.DeployResource(etx.Request().Context(), resourceID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Resource deployed")
}

func (controller Resources) Edit(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return controller.renderEdit(etx, resourceID, nil)
}

func (controller Resources) Update(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
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
	resourceID, err := uuid.Parse(etx.Param("id"))
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
	resourceID, err := uuid.Parse(etx.Param("id"))
	connectionID, connectionErr := uuid.Parse(etx.Param("connectionID"))
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

type resourceEndpointPayload struct {
	Name             string                                 `json:"name"`
	Role             string                                 `json:"role"`
	Address          string                                 `json:"address"`
	Port             int32                                  `json:"port"`
	Protocol         string                                 `json:"protocol"`
	TLSMode          string                                 `json:"tlsMode"`
	Settings         json.RawMessage                        `json:"settings"`
	PrivateNetworkID string                                 `json:"privateNetworkId"`
	Publication      services.ResourceCaddyPublicationInput `json:"publication"`
}

func (payload resourceEndpointPayload) serviceInput() (services.ResourceEndpointInput, error) {
	networkID, err := optionalUUID(payload.PrivateNetworkID)
	if err != nil {
		return services.ResourceEndpointInput{}, domainPayloadError(
			"privateNetworkId",
			"private network is invalid",
		)
	}
	return services.ResourceEndpointInput{
		Name: payload.Name, Role: payload.Role, Address: payload.Address, Port: payload.Port,
		Protocol: payload.Protocol, TLSMode: payload.TLSMode, Settings: payload.Settings,
		PrivateNetworkID: networkID,
	}, nil
}

func (controller Resources) CreateEndpoint(etx *echo.Context) error {
	var payload resourceEndpointPayload
	resourceID, input, err := bindResourceChild(
		etx,
		func() (services.ResourceEndpointInput, error) {
			if bindErr := etx.Bind(&payload); bindErr != nil {
				return services.ResourceEndpointInput{}, bindErr
			}
			return payload.serviceInput()
		},
	)
	if err == nil {
		input, err = controller.caddy.PrepareResourcePublication(
			etx.Request().Context(),
			resourceID,
			uuid.Nil,
			input,
			payload.Publication,
		)
	}
	if err == nil {
		var endpoint models.ResourceEndpointEntity
		endpoint, err = controller.service.CreateEndpoint(
			etx.Request().Context(),
			resourceID,
			input,
		)
		if err == nil && payload.Publication.Enabled {
			err = controller.caddy.SyncResourcePublication(
				etx.Request().Context(),
				resourceID,
				endpoint.ID,
				payload.Publication,
			)
		}
	}
	return controller.finishChildMutation(etx, resourceID, err, "Endpoint created")
}

func (controller Resources) UpdateEndpoint(etx *echo.Context) error {
	resourceID, endpointID, err := parseChildIDs(etx, "endpointID")
	var payload resourceEndpointPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceEndpointInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		input, err = controller.caddy.PrepareResourcePublication(
			etx.Request().Context(),
			resourceID,
			endpointID,
			input,
			payload.Publication,
		)
	}
	if err == nil {
		if !payload.Publication.Enabled {
			err = controller.caddy.RemoveResourcePublication(
				etx.Request().Context(),
				resourceID,
				endpointID,
			)
		}
	}
	if err == nil {
		_, err = controller.service.UpdateEndpoint(
			etx.Request().Context(),
			resourceID,
			endpointID,
			input,
		)
	}
	if err == nil {
		if payload.Publication.Enabled {
			err = controller.caddy.SyncResourcePublication(
				etx.Request().Context(),
				resourceID,
				endpointID,
				payload.Publication,
			)
		} else {
			err = controller.caddy.RemoveResourcePublication(
				etx.Request().Context(),
				resourceID,
				endpointID,
			)
		}
	}
	return controller.finishChildMutation(etx, resourceID, err, "Endpoint updated")
}

func (controller Resources) DestroyEndpoint(etx *echo.Context) error {
	resourceID, endpointID, err := parseChildIDs(etx, "endpointID")
	if err == nil {
		err = controller.caddy.RemoveResourcePublication(
			etx.Request().Context(),
			resourceID,
			endpointID,
		)
	}
	if err == nil {
		err = controller.service.ArchiveEndpoint(etx.Request().Context(), resourceID, endpointID)
	}
	if err == nil {
		err = controller.caddy.RemoveResourcePublication(
			etx.Request().Context(),
			resourceID,
			endpointID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Endpoint archived")
}

type resourcePrivateAccessPayload struct {
	PrivateNetworkID string `json:"privateNetworkId"`
}

func (controller Resources) EnablePrivateAccess(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourcePrivateAccessPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var privateNetworkID uuid.UUID
	if err == nil {
		privateNetworkID, err = uuid.Parse(payload.PrivateNetworkID)
		if err != nil {
			err = domainPayloadError("privateNetworkId", "private network is required")
		}
	}
	if err == nil {
		_, err = controller.access.Enable(etx.Request().Context(), resourceID, privateNetworkID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Resource added to private network")
}

func (controller Resources) DisablePrivateAccess(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.access.Disable(etx.Request().Context(), resourceID)
	}
	return controller.finishChildMutation(
		etx,
		resourceID,
		err,
		"Resource removed from private network",
	)
}

type resourcePrivateAccessDevicePayload struct {
	Name     string `json:"name"`
	DeviceID string `json:"deviceId"`
}

func (controller Resources) CreatePrivateAccessDevice(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourcePrivateAccessDevicePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	deviceID := uuid.Nil
	if err == nil && strings.TrimSpace(payload.DeviceID) != "" {
		deviceID, err = uuid.Parse(payload.DeviceID)
	}
	var result services.ResourcePrivateAccessResult
	if err == nil {
		result, err = controller.access.EnrollManaged(
			etx.Request().Context(),
			resourceID,
			services.ResourcePrivateAccessEnrollment{
				DeviceID: deviceID,
				Name:     payload.Name,
				UserID:   cookies.ExtractFromCookieApp(etx).UserID,
			},
		)
	}
	if err != nil {
		return controller.finishChildMutation(etx, resourceID, err, "")
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Private Resource access granted")
	etx.Response().Header().Set("Cache-Control", "no-store")
	enrollment := inertia.Props{
		"deviceId": result.DeviceID.String(), "grantId": result.GrantID.String(),
		"clientConfiguration": result.ClientConfiguration,
	}
	return controller.renderShowSection(
		etx,
		resourceID,
		resourceReturnSection(etx),
		enrollment,
		nil,
	)
}

func (controller Resources) DestroyPrivateAccessDevice(etx *echo.Context) error {
	resourceID, deviceID, err := parseChildIDs(etx, "deviceID")
	if err == nil {
		err = controller.access.RevokeManagedGrant(etx.Request().Context(), resourceID, deviceID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Private Resource access revoked")
}

type resourceCredentialPayload struct {
	Name         string            `json:"name"`
	Username     string            `json:"username"`
	Metadata     json.RawMessage   `json:"metadata"`
	SecretValues map[string]string `json:"secretValues"`
	Rotate       bool              `json:"rotate"`
}

func (payload resourceCredentialPayload) serviceInput() (services.ResourceCredentialInput, error) {
	return services.ResourceCredentialInput{
		Name: payload.Name, Username: payload.Username, Metadata: payload.Metadata,
		SecretValues: payload.SecretValues,
	}, nil
}

func (controller Resources) CreateCredential(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceCredentialPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceCredentialInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateCredential(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Credential created")
}

type resourceDatabasePayload struct {
	Name         string                     `json:"name"`
	Encoding     string                     `json:"encoding"`
	Collation    string                     `json:"collation"`
	CredentialID string                     `json:"credentialId"`
	Credential   *resourceCredentialPayload `json:"credential"`
}

func (controller Resources) CreateDatabase(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceDatabasePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		credentialID, credentialIDErr := optionalUUID(payload.CredentialID)
		if credentialIDErr != nil {
			err = domainPayloadError("credentialId", "credential is invalid")
		} else {
			input := services.CreateResourceDatabaseInput{
				Database: models.ResourceDatabaseDefinition{
					Name: payload.Name, Encoding: payload.Encoding, Collation: payload.Collation,
				},
				CredentialID: credentialID,
			}
			if payload.Credential != nil {
				var credential services.ResourceCredentialInput
				credential, err = payload.Credential.serviceInput()
				if err == nil {
					input.Credential = &credential
				}
			}
			if err == nil {
				_, err = controller.service.CreateDatabase(
					etx.Request().Context(),
					resourceID,
					input,
				)
			}
		}
	}
	return controller.finishChildMutation(
		etx,
		resourceID,
		err,
		"Database and credential access created",
	)
}

func (controller Resources) DestroyDatabase(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	if err == nil && databaseName == "" {
		err = domainPayloadError("database", "Database is required")
	}
	if err == nil {
		err = controller.service.DestroyDatabase(
			etx.Request().Context(),
			resourceID,
			databaseName,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Database deleted")
}

func (controller Resources) UpdateCredential(etx *echo.Context) error {
	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	var payload resourceCredentialPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceCredentialInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil && payload.Rotate {
		_, err = controller.service.RotateCredential(
			etx.Request().Context(),
			resourceID,
			credentialID,
			input,
		)
	} else if err == nil {
		_, err = controller.service.UpdateCredentialMetadata(
			etx.Request().Context(),
			resourceID,
			credentialID,
			input,
		)
	}
	message := "Credential updated"
	if payload.Rotate {
		message = "Credential rotated"
	}
	return controller.finishChildMutation(etx, resourceID, err, message)
}

func (controller Resources) DestroyCredential(etx *echo.Context) error {
	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	if err == nil {
		err = controller.service.ArchiveCredential(
			etx.Request().Context(),
			resourceID,
			credentialID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Credential archived")
}

func (controller Resources) RevealCredential(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Resource credential not found"},
		)
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := etx.Bind(
		&payload,
	); err != nil || payload.Password == "" ||
		len(payload.Password) > 4096 {
		return etx.JSON(
			http.StatusUnprocessableEntity,
			map[string]string{"error": "Current password is required"},
		)
	}

	credential, err := controller.credentials.RevealManaged(
		etx.Request().Context(), resourceID, credentialID,
		cookies.ExtractFromCookieApp(etx).UserID, payload.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCredentials):
			return etx.JSON(
				http.StatusUnprocessableEntity,
				map[string]string{"error": "Current password is incorrect"},
			)
		case errors.Is(err, services.ErrResourceCredentialUnavailable):
			return etx.JSON(
				http.StatusNotFound,
				map[string]string{"error": "Resource credential not found"},
			)
		default:
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to reveal Resource credential",
				"resource_id",
				resourceID,
				"credential_id",
				credentialID,
				"error",
				err,
			)
			return etx.JSON(
				http.StatusInternalServerError,
				map[string]string{"error": "Resource credential could not be loaded"},
			)
		}
	}
	return etx.JSON(http.StatusOK, credential)
}

type resourceInstallationPayload struct {
	ImageReference       string                                    `json:"imageReference"`
	ImageDigest          string                                    `json:"imageDigest"`
	ContainerName        string                                    `json:"containerName"`
	RestartPolicy        string                                    `json:"restartPolicy"`
	Configuration        json.RawMessage                           `json:"configuration"`
	PortMappings         *[]models.ResourceInstallationPortMapping `json:"portMappings"`
	ServerID             string                                    `json:"serverId"`
	RegistryCredentialID string                                    `json:"registryCredentialId"`
}

func (payload resourceInstallationPayload) serviceInput() (services.ResourceInstallationInput, error) {
	serverID, err := uuid.Parse(payload.ServerID)
	if err != nil {
		return services.ResourceInstallationInput{}, domainPayloadError(
			"serverId",
			"server is required",
		)
	}
	registryID, err := optionalUUID(payload.RegistryCredentialID)
	if err != nil {
		return services.ResourceInstallationInput{}, domainPayloadError(
			"registryCredentialId",
			"registry credential is invalid",
		)
	}
	return services.ResourceInstallationInput{
		ImageReference: payload.ImageReference, ImageDigest: payload.ImageDigest,
		ContainerName: payload.ContainerName, RestartPolicy: payload.RestartPolicy,
		Configuration: payload.Configuration, PortMappings: payload.PortMappings,
		ServerID: serverID, RegistryCredentialID: registryID,
	}, nil
}

func (controller Resources) CreateInstallation(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceInstallationPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceInstallationInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateInstallation(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Installation created")
}

func (controller Resources) StartInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.RunInstallation(
			etx.Request().Context(),
			resourceID,
			installationID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container is running")
}

func (controller Resources) InstallationLogs(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	tail := 200
	if value := strings.TrimSpace(etx.QueryParam("tail")); err == nil && value != "" {
		tail, err = strconv.Atoi(value)
	}
	if err != nil {
		return etx.JSON(
			http.StatusBadRequest,
			map[string]string{"error": "Resource installation or log tail is invalid"},
		)
	}
	logs, err := controller.service.InstallationLogs(
		etx.Request().Context(),
		resourceID,
		installationID,
		tail,
	)
	if errors.Is(err, models.ErrNotFound) {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Resource installation not found"},
		)
	}
	if err != nil {
		return etx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	return etx.JSON(http.StatusOK, map[string]string{"logs": logs})
}

func (controller Resources) StopInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.StopInstallation(
			etx.Request().Context(),
			resourceID,
			installationID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container stopped")
}

func (controller Resources) RestartInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.RestartInstallation(
			etx.Request().Context(),
			resourceID,
			installationID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container restarted")
}

func (controller Resources) RemoveInstallationContainer(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.RemoveInstallationContainer(
			etx.Request().Context(),
			resourceID,
			installationID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container removed")
}

func (controller Resources) UpdateInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	var payload resourceInstallationPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceInstallationInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateInstallation(
			etx.Request().Context(),
			resourceID,
			installationID,
			input,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Installation updated")
}

func (controller Resources) DestroyInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.ArchiveInstallation(
			etx.Request().Context(),
			resourceID,
			installationID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Installation archived")
}

type resourceVolumePayload struct {
	Name          string          `json:"name"`
	Driver        string          `json:"driver"`
	Configuration json.RawMessage `json:"configuration"`
	ServerID      string          `json:"serverId"`
}

func (payload resourceVolumePayload) serviceInput() (services.ResourceVolumeInput, error) {
	serverID := uuid.Nil
	var err error
	if strings.TrimSpace(payload.ServerID) != "" {
		serverID, err = uuid.Parse(payload.ServerID)
	}
	if err != nil {
		return services.ResourceVolumeInput{}, domainPayloadError("serverId", "server is invalid")
	}
	return services.ResourceVolumeInput{
		Name:          payload.Name,
		Driver:        payload.Driver,
		Configuration: payload.Configuration,
		ServerID:      serverID,
	}, nil
}

func (controller Resources) CreateVolume(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceVolumePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceVolumeInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateVolume(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Volume created")
}

func (controller Resources) UpdateVolume(etx *echo.Context) error {
	resourceID, volumeID, err := parseChildIDs(etx, "volumeID")
	var payload resourceVolumePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceVolumeInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateVolume(
			etx.Request().Context(),
			resourceID,
			volumeID,
			input,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Volume updated")
}

func (controller Resources) DestroyVolume(etx *echo.Context) error {
	resourceID, volumeID, err := parseChildIDs(etx, "volumeID")
	if err == nil {
		err = controller.service.ArchiveVolume(etx.Request().Context(), resourceID, volumeID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Volume archived")
}

type resourceMountPayload struct {
	MountPath              string `json:"mountPath"`
	ReadOnly               bool   `json:"readOnly"`
	ResourceVolumeID       string `json:"resourceVolumeId"`
	ResourceInstallationID string `json:"resourceInstallationId"`
}

func (payload resourceMountPayload) serviceInput() (services.ResourceMountInput, error) {
	volumeID, volumeErr := optionalUUID(payload.ResourceVolumeID)
	installationID, installationErr := optionalUUID(payload.ResourceInstallationID)
	if volumeErr != nil || installationErr != nil {
		return services.ResourceMountInput{}, domainPayloadError(
			"mount",
			"volume or installation is invalid",
		)
	}
	input := services.ResourceMountInput{MountPath: payload.MountPath, ReadOnly: payload.ReadOnly}
	if volumeID != nil {
		input.ResourceVolumeID = *volumeID
	}
	if installationID != nil {
		input.ResourceInstallationID = *installationID
	}
	return input, nil
}

func (controller Resources) CreateMount(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceMountPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceMountInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateMount(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Mount created")
}

func (controller Resources) UpdateMount(etx *echo.Context) error {
	resourceID, mountID, err := parseChildIDs(etx, "mountID")
	var payload resourceMountPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceMountInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateMount(etx.Request().Context(), resourceID, mountID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Mount updated")
}

func (controller Resources) DestroyMount(etx *echo.Context) error {
	resourceID, mountID, err := parseChildIDs(etx, "mountID")
	if err == nil {
		err = controller.service.ArchiveMount(etx.Request().Context(), resourceID, mountID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Mount archived")
}

type resourceHealthCheckPayload struct {
	Name                 string          `json:"name"`
	Kind                 string          `json:"kind"`
	Configuration        json.RawMessage `json:"configuration"`
	IntervalSeconds      int32           `json:"intervalSeconds"`
	TimeoutSeconds       int32           `json:"timeoutSeconds"`
	FailureThreshold     int32           `json:"failureThreshold"`
	SuccessThreshold     int32           `json:"successThreshold"`
	Enabled              bool            `json:"enabled"`
	ResourceEndpointID   string          `json:"resourceEndpointId"`
	ResourceCredentialID string          `json:"resourceCredentialId"`
}

func (payload resourceHealthCheckPayload) serviceInput() (services.ResourceHealthCheckInput, error) {
	endpointID, endpointErr := optionalUUID(payload.ResourceEndpointID)
	credentialID, credentialErr := optionalUUID(payload.ResourceCredentialID)
	if endpointErr != nil || credentialErr != nil {
		return services.ResourceHealthCheckInput{}, domainPayloadError(
			"healthCheck",
			"endpoint or credential is invalid",
		)
	}
	input := services.ResourceHealthCheckInput{
		Name:                 payload.Name,
		Kind:                 payload.Kind,
		Configuration:        payload.Configuration,
		IntervalSeconds:      payload.IntervalSeconds,
		TimeoutSeconds:       payload.TimeoutSeconds,
		FailureThreshold:     payload.FailureThreshold,
		SuccessThreshold:     payload.SuccessThreshold,
		Enabled:              payload.Enabled,
		ResourceEndpointID:   endpointID,
		ResourceCredentialID: credentialID,
	}
	return input, nil
}

func (controller Resources) CreateHealthCheck(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceHealthCheckPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceHealthCheckInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateHealthCheck(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Health check created")
}

func (controller Resources) UpdateHealthCheck(etx *echo.Context) error {
	resourceID, healthCheckID, err := parseChildIDs(etx, "healthCheckID")
	var payload resourceHealthCheckPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceHealthCheckInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateHealthCheck(
			etx.Request().Context(),
			resourceID,
			healthCheckID,
			input,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Health check updated")
}

func (controller Resources) DestroyHealthCheck(etx *echo.Context) error {
	resourceID, healthCheckID, err := parseChildIDs(etx, "healthCheckID")
	if err == nil {
		err = controller.service.ArchiveHealthCheck(
			etx.Request().Context(),
			resourceID,
			healthCheckID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Health check archived")
}

type resourceBackupPolicyPayload struct {
	Schedule            string `json:"schedule"`
	KeepLast            int    `json:"keepLast"`
	KeepDaily           int    `json:"keepDaily"`
	KeepWeekly          int    `json:"keepWeekly"`
	KeepMonthly         int    `json:"keepMonthly"`
	BackupDestinationID string `json:"backupDestinationId"`
}

func (payload resourceBackupPolicyPayload) serviceInput() (services.DatabaseBackupPolicyInput, error) {
	destinationID, err := uuid.Parse(payload.BackupDestinationID)
	if err != nil {
		return services.DatabaseBackupPolicyInput{}, domainPayloadError(
			"backupDestinationId",
			"Object Storage destination is required",
		)
	}
	return services.DatabaseBackupPolicyInput{
		Schedule: payload.Schedule, KeepLast: payload.KeepLast, KeepDaily: payload.KeepDaily,
		KeepWeekly: payload.KeepWeekly, KeepMonthly: payload.KeepMonthly,
		BackupDestinationID: destinationID,
	}, nil
}

func (controller Resources) CreateBackupPolicy(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	if err == nil && databaseName == "" {
		err = domainPayloadError("database", "Database is required")
	}
	var payload resourceBackupPolicyPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.DatabaseBackupPolicyInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.backups.CreateForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			input,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup policy created")
}

func (controller Resources) UpdateBackupPolicy(etx *echo.Context) error {
	resourceID, databaseName, policyID, err := parseResourceDatabasePolicyIDs(etx)
	var payload resourceBackupPolicyPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.DatabaseBackupPolicyInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.backups.UpdateForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			policyID,
			input,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup policy updated")
}

func (controller Resources) PauseBackupPolicy(etx *echo.Context) error {
	resourceID, databaseName, policyID, err := parseResourceDatabasePolicyIDs(etx)
	if err == nil {
		err = controller.backups.SetStateForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			policyID,
			"pause",
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup policy paused")
}

func (controller Resources) ResumeBackupPolicy(etx *echo.Context) error {
	resourceID, databaseName, policyID, err := parseResourceDatabasePolicyIDs(etx)
	if err == nil {
		err = controller.backups.SetStateForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			policyID,
			"resume",
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup policy resumed")
}

func (controller Resources) ArchiveBackupPolicy(etx *echo.Context) error {
	resourceID, databaseName, policyID, err := parseResourceDatabasePolicyIDs(etx)
	if err == nil {
		err = controller.backups.SetStateForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			policyID,
			"archive",
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup policy archived")
}

func (controller Resources) RunBackupPolicy(etx *echo.Context) error {
	resourceID, databaseName, policyID, err := parseResourceDatabasePolicyIDs(etx)
	if err == nil {
		_, err = controller.backups.ManualForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			policyID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Backup requested")
}

type resourceRestorePayload struct {
	BackupID     string `json:"backupId"`
	Confirmation string `json:"confirmation"`
}

func (controller Resources) CreateRestore(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	if err == nil && databaseName == "" {
		err = domainPayloadError("database", "Database is required")
	}
	var payload resourceRestorePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	backupID := uuid.Nil
	if err == nil {
		backupID, err = uuid.Parse(payload.BackupID)
		if err != nil {
			err = domainPayloadError("backupId", "Backup is required")
		}
	}
	if err == nil {
		_, err = controller.restore.RequestForResource(
			etx.Request().Context(),
			resourceID,
			databaseName,
			services.DatabaseRestoreInput{
				BackupID: backupID, Confirmation: payload.Confirmation,
				ActorID: cookies.ExtractFromCookieApp(etx).UserID,
			},
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Database restore queued")
}

func (controller Resources) renderShow(
	etx *echo.Context,
	resourceID uuid.UUID,
	option inertia.PageOption,
) error {
	return controller.renderShowSection(etx, resourceID, resourceReturnSection(etx), nil, option)
}

func (controller Resources) renderShowSection(
	etx *echo.Context,
	resourceID uuid.UUID,
	section string,
	enrollment inertia.Props,
	option inertia.PageOption,
) error {
	if err := controller.access.ObserveResource(etx.Request().Context(), resourceID); err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to observe Resource WireGuard device handshakes",
			"resource_id",
			resourceID,
			"error",
			err,
		)
	}
	detail, err := controller.service.Details(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	publications, err := controller.caddy.ResourcePublications(etx.Request().Context(), resourceID)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	dnsZones, err := controller.caddy.ResourceDNSOptions(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	options, err := controller.service.OptionsForEngine(
		etx.Request().Context(),
		detail.Resource.Engine(),
	)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	privateAccess, err := controller.access.Details(
		etx.Request().Context(),
		resourceID,
		cookies.ExtractFromCookieApp(etx).UserID,
	)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	backups := models.ResourceBackupCatalog{}
	if section == "backups" || section == "databases" {
		backups, err = controller.backups.DetailsForResource(etx.Request().Context(), resourceID)
		if err != nil {
			return controller.renderLoadError(etx, err)
		}
	}
	props := inertia.Props{
		"auth":             authProps(etx),
		"resource":         resourceDetailProps(detail, privateAccess),
		"backups":          resourceBackupProps(backups),
		"options":          resourceOptionsProps(options),
		"publications":     publications,
		"dnsZones":         dnsZones,
		"section":          section,
		"selectedDatabase": strings.TrimSpace(etx.QueryParam("database")),
		"flash":            resourceFlashProps(etx),
	}
	if enrollment != nil {
		props["enrollment"] = enrollment
	}
	if option != nil {
		return inertia.Page(etx, "Resources/Show", props, option)
	}
	return inertia.Page(etx, "Resources/Show", props)
}

func (controller Resources) renderEdit(
	etx *echo.Context,
	resourceID uuid.UUID,
	option inertia.PageOption,
) error {
	detail, err := controller.service.Details(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	props := inertia.Props{
		"auth":     authProps(etx),
		"resource": resourceDetailProps(detail, models.ResourcePrivateAccessDetails{}),
		"options":  resourceOptionsProps(options),
		"flash":    resourceFlashProps(etx),
	}
	if option != nil {
		return inertia.Page(etx, "Resources/Edit", props, option)
	}
	return inertia.Page(etx, "Resources/Edit", props)
}

func (controller Resources) renderCreateError(etx *echo.Context, err error) error {
	options, optionsErr := controller.service.Options(etx.Request().Context())
	if optionsErr != nil {
		return controller.renderLoadError(etx, errors.Join(err, optionsErr))
	}
	if validationErrors, ok := validation.As(err); ok {
		return inertia.Page(
			etx,
			"Resources/New",
			inertia.Props{"auth": authProps(etx), "options": resourceOptionsProps(options)},
			inertia.WithValidationErrors(validationErrors.ToMap()),
		)
	}
	return controller.redirectError(etx, routes.ResourceNew.URL(), err)
}

func (controller Resources) finishChildMutation(
	etx *echo.Context,
	resourceID uuid.UUID,
	err error,
	success string,
) error {
	section := resourceReturnSection(etx)
	if err != nil {
		if validationErrors, ok := validation.As(err); ok && resourceID != uuid.Nil {
			if section == "settings" {
				return controller.renderEdit(
					etx,
					resourceID,
					inertia.WithValidationErrors(validationErrors.ToMap()),
				)
			}
			return controller.renderShowSection(
				etx,
				resourceID,
				section,
				nil,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return controller.redirectError(etx, resourceSectionURL(resourceID, section), err)
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashSuccess, success); flashErr != nil {
		return controller.renderLoadError(etx, flashErr)
	}
	return inertia.Redirect(etx, resourceSectionURL(resourceID, section), http.StatusSeeOther)
}

func resourceReturnSection(etx *echo.Context) string {
	section := etx.QueryParam("returnTo")
	if section == "edit" {
		return "settings"
	}
	switch section {
	case "databases", "backups", "endpoints", "credentials", "health", "settings":
		return section
	default:
		return "overview"
	}
}

func resourceSectionURL(resourceID uuid.UUID, section string) string {
	switch section {
	case "databases":
		return routes.ResourceDatabases.URL(resourceID)
	case "backups":
		return routes.ResourceBackups.URL(resourceID)
	case "endpoints":
		return routes.ResourceEndpoints.URL(resourceID)
	case "credentials":
		return routes.ResourceCredentials.URL(resourceID)
	case "health":
		return routes.ResourceHealth.URL(resourceID)
	case "settings":
		return routes.ResourceSettings.URL(resourceID)
	default:
		return routes.ResourceShow.URL(resourceID)
	}
}

func resourceFlashProps(etx *echo.Context) []inertia.Props {
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

func (controller Resources) redirectError(etx *echo.Context, location string, err error) error {
	message := "Resource operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func (controller Resources) renderLoadError(etx *echo.Context, err error) error {
	slog.ErrorContext(etx.Request().Context(), "failed to load Resource page", "error", err)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}

func parseChildIDs(etx *echo.Context, childParam string) (uuid.UUID, uuid.UUID, error) {
	resourceID, resourceErr := uuid.Parse(etx.Param("id"))
	childID, childErr := uuid.Parse(etx.Param(childParam))
	return resourceID, childID, errors.Join(resourceErr, childErr)
}

func parseResourceDatabasePolicyIDs(etx *echo.Context) (uuid.UUID, string, uuid.UUID, error) {
	resourceID, resourceErr := uuid.Parse(etx.Param("id"))
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	var databaseErr error
	if databaseName == "" {
		databaseErr = errors.New("database is required")
	}
	policyID, policyErr := uuid.Parse(etx.Param("backupPolicyID"))
	return resourceID, databaseName, policyID, errors.Join(resourceErr, databaseErr, policyErr)
}

func bindResourceChild[T any](etx *echo.Context, bind func() (T, error)) (uuid.UUID, T, error) {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var input T
	if err == nil {
		input, err = bind()
	}
	return resourceID, input, err
}

func domainPayloadError(field, message string) error {
	return errors.Join(
		models.ErrDomainValidation,
		validation.ValidationErrors{{Field: field, Code: "invalid", Message: message}},
	)
}

func prefixResourceValidation(err error, prefix string) error {
	validationErrors, ok := validation.As(err)
	if !ok {
		return err
	}
	return errors.Join(
		models.ErrDomainValidation,
		validation.WithFieldPrefix(validationErrors, prefix),
	)
}
