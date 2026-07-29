package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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
	service *services.ResourceManagement
	access  *services.ResourcePrivateAccess
}

func NewResources(service *services.ResourceManagement, access *services.ResourcePrivateAccess) Resources {
	return Resources{service: service, access: access}
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
		{http.MethodPost, routes.ResourceDeploy, controller.Deploy},
		{http.MethodGet, routes.ResourceEdit, controller.Edit},
		{http.MethodPatch, routes.ResourceUpdate, controller.Update},
		{http.MethodDelete, routes.ResourceDestroy, controller.Destroy},
		{http.MethodPost, routes.ResourceConnectionCreate, controller.CreateConnection},
		{http.MethodPatch, routes.ResourceConnectionUpdate, controller.UpdateConnection},
		{http.MethodDelete, routes.ResourceConnectionDestroy, controller.DestroyConnection},
		{http.MethodPost, routes.ResourceEndpointCreate, controller.CreateEndpoint},
		{http.MethodPatch, routes.ResourceEndpointUpdate, controller.UpdateEndpoint},
		{http.MethodDelete, routes.ResourceEndpointDestroy, controller.DestroyEndpoint},
		{http.MethodPost, routes.ResourcePrivateAccessCreate, controller.EnablePrivateAccess},
		{http.MethodDelete, routes.ResourcePrivateAccessDestroy, controller.DisablePrivateAccess},
		{http.MethodPost, routes.ResourcePrivateAccessDeviceCreate, controller.CreatePrivateAccessDevice},
		{http.MethodDelete, routes.ResourcePrivateAccessDeviceDestroy, controller.DestroyPrivateAccessDevice},
		{http.MethodPost, routes.ResourceCredentialCreate, controller.CreateCredential},
		{http.MethodPatch, routes.ResourceCredentialUpdate, controller.UpdateCredential},
		{http.MethodDelete, routes.ResourceCredentialDestroy, controller.DestroyCredential},
		{http.MethodPost, routes.ResourceInstallationCreate, controller.CreateInstallation},
		{http.MethodPost, routes.ResourceInstallationStart, controller.StartInstallation},
		{http.MethodPost, routes.ResourceInstallationStop, controller.StopInstallation},
		{http.MethodPost, routes.ResourceInstallationRestart, controller.RestartInstallation},
		{http.MethodDelete, routes.ResourceInstallationRemove, controller.RemoveInstallationContainer},
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
		Search: etx.QueryParam("search"), Kind: etx.QueryParam("kind"), Category: etx.QueryParam("category"),
		ManagementMode: etx.QueryParam("managementMode"), SharingScope: etx.QueryParam("sharingScope"),
	}
	items, err := controller.service.List(etx.Request().Context(), filters)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	return inertia.Page(etx, "Resources/Index", inertia.Props{
		"auth": authProps(etx), "resources": resourceListProps(items), "options": resourceOptionsProps(options), "filters": inertia.Props{
			"search": filters.Search, "kind": filters.Kind, "category": filters.Category,
			"managementMode": filters.ManagementMode, "sharingScope": filters.SharingScope,
		},
	})
}

func (controller Resources) New(etx *echo.Context) error {
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	return inertia.Page(etx, "Resources/New", inertia.Props{"auth": authProps(etx), "options": resourceOptionsProps(options), "flash": resourceFlashProps(etx)})
}

type resourcePayload struct {
	Name           string `json:"name"`
	Category       string `json:"category"`
	Kind           string `json:"kind"`
	DatabaseName   string `json:"databaseName"`
	ManagementMode string `json:"managementMode"`
	SharingScope   string `json:"sharingScope"`
}

func (payload resourcePayload) serviceInput() (services.ResourceInput, error) {
	managementMode, err := models.ParseResourceManagementModeEnum(strings.ToLower(strings.TrimSpace(payload.ManagementMode)))
	if err != nil {
		return services.ResourceInput{}, domainPayloadError("managementMode", "management mode is invalid")
	}
	sharingScope, err := models.ParseResourceSharingScopeEnum(strings.ToLower(strings.TrimSpace(payload.SharingScope)))
	if err != nil {
		return services.ResourceInput{}, domainPayloadError("sharingScope", "sharing scope is invalid")
	}
	return services.ResourceInput{
		Name: payload.Name, Category: payload.Category, Kind: payload.Kind,
		DatabaseName:   payload.DatabaseName,
		ManagementMode: managementMode, SharingScope: sharingScope,
	}, nil
}

type resourceConnectionPayload struct {
	EnvironmentID        string          `json:"environmentId"`
	Alias                string          `json:"alias"`
	Configuration        json.RawMessage `json:"configuration"`
	ResourceEndpointID   string          `json:"resourceEndpointId"`
	ResourceCredentialID string          `json:"resourceCredentialId"`
}

func (payload resourceConnectionPayload) serviceInput() (services.ResourceConnectionInput, error) {
	environmentID, err := uuid.Parse(payload.EnvironmentID)
	if err != nil {
		return services.ResourceConnectionInput{}, domainPayloadError("environmentId", "Environment is required")
	}
	endpointID, err := uuid.Parse(payload.ResourceEndpointID)
	if err != nil {
		return services.ResourceConnectionInput{}, domainPayloadError("resourceEndpointId", "endpoint is required")
	}
	credentialID, err := optionalUUID(payload.ResourceCredentialID)
	if err != nil {
		return services.ResourceConnectionInput{}, domainPayloadError("resourceCredentialId", "credential is invalid")
	}
	return services.ResourceConnectionInput{
		EnvironmentID: environmentID, Alias: payload.Alias, Configuration: payload.Configuration,
		ResourceEndpointID: endpointID, ResourceCredentialID: credentialID,
	}, nil
}

func (controller Resources) CreateConnection(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload resourceConnectionPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceConnectionInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.ConnectEnvironment(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Environment connected")
}

func (controller Resources) UpdateConnection(etx *echo.Context) error {
	resourceID, connectionID, err := parseChildIDs(etx, "connectionID")
	var payload resourceConnectionPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceConnectionInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.UpdateEnvironmentConnection(etx.Request().Context(), resourceID, connectionID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Environment connection updated")
}

func (controller Resources) DestroyConnection(etx *echo.Context) error {
	resourceID, connectionID, err := parseChildIDs(etx, "connectionID")
	if err == nil {
		err = controller.service.DisconnectEnvironment(etx.Request().Context(), resourceID, connectionID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Environment disconnected")
}

type resourceCreatePayload struct {
	resourcePayload
	Endpoint     *resourceEndpointPayload     `json:"endpoint"`
	Credential   *resourceCredentialPayload   `json:"credential"`
	Installation *resourceInstallationPayload `json:"installation"`
	Volume       *resourceVolumePayload       `json:"volume"`
	Mount        *resourceMountPayload        `json:"mount"`
	HealthCheck  *resourceHealthCheckPayload  `json:"healthCheck"`
}

func (payload resourceCreatePayload) serviceInput() (services.CreateResourceInput, error) {
	resource, err := payload.resourcePayload.serviceInput()
	if err != nil {
		return services.CreateResourceInput{}, err
	}
	input := services.CreateResourceInput{Resource: resource}
	if payload.Installation != nil {
		value, valueErr := payload.Installation.serviceInput()
		if valueErr != nil {
			return services.CreateResourceInput{}, prefixResourceValidation(valueErr, "installation")
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
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return controller.renderShow(etx, resourceID, nil)
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
			return controller.renderEdit(etx, resourceID, inertia.WithValidationErrors(validationErrors.ToMap()))
		}
		return controller.redirectError(etx, routes.ResourceEdit.URL(resourceID), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource updated")
	return inertia.Redirect(etx, routes.ResourceShow.URL(resourceID), http.StatusSeeOther)
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

type resourceEndpointPayload struct {
	Name                   string          `json:"name"`
	Role                   string          `json:"role"`
	Address                string          `json:"address"`
	Port                   int32           `json:"port"`
	Protocol               string          `json:"protocol"`
	TLSMode                string          `json:"tlsMode"`
	Settings               json.RawMessage `json:"settings"`
	ResourceInstallationID string          `json:"resourceInstallationId"`
	PrivateNetworkID       string          `json:"privateNetworkId"`
}

func (payload resourceEndpointPayload) serviceInput() (services.ResourceEndpointInput, error) {
	installationID, err := optionalUUID(payload.ResourceInstallationID)
	if err != nil {
		return services.ResourceEndpointInput{}, domainPayloadError("resourceInstallationId", "installation is invalid")
	}
	networkID, err := optionalUUID(payload.PrivateNetworkID)
	if err != nil {
		return services.ResourceEndpointInput{}, domainPayloadError("privateNetworkId", "private network is invalid")
	}
	return services.ResourceEndpointInput{
		Name: payload.Name, Role: payload.Role, Address: payload.Address, Port: payload.Port,
		Protocol: payload.Protocol, TLSMode: payload.TLSMode, Settings: payload.Settings,
		ResourceInstallationID: installationID, PrivateNetworkID: networkID,
	}, nil
}

func (controller Resources) CreateEndpoint(etx *echo.Context) error {
	resourceID, input, err := bindResourceChild(etx, func() (services.ResourceEndpointInput, error) {
		var payload resourceEndpointPayload
		if bindErr := etx.Bind(&payload); bindErr != nil {
			return services.ResourceEndpointInput{}, bindErr
		}
		return payload.serviceInput()
	})
	if err == nil {
		_, err = controller.service.CreateEndpoint(etx.Request().Context(), resourceID, input)
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
		_, err = controller.service.UpdateEndpoint(etx.Request().Context(), resourceID, endpointID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Endpoint updated")
}

func (controller Resources) DestroyEndpoint(etx *echo.Context) error {
	resourceID, endpointID, err := parseChildIDs(etx, "endpointID")
	if err == nil {
		err = controller.service.ArchiveEndpoint(etx.Request().Context(), resourceID, endpointID)
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
	return controller.finishChildMutation(etx, resourceID, err, "Resource removed from private network")
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
		result, err = controller.access.EnrollManaged(etx.Request().Context(), resourceID, services.ResourcePrivateAccessEnrollment{
			DeviceID: deviceID, Name: payload.Name, UserID: cookies.ExtractFromCookieApp(etx).UserID,
		})
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
	return controller.renderShowPage(etx, resourceID, enrollment, nil)
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
		_, err = controller.service.RotateCredential(etx.Request().Context(), resourceID, credentialID, input)
	} else if err == nil {
		_, err = controller.service.UpdateCredentialMetadata(etx.Request().Context(), resourceID, credentialID, input)
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
		err = controller.service.ArchiveCredential(etx.Request().Context(), resourceID, credentialID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Credential archived")
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
		return services.ResourceInstallationInput{}, domainPayloadError("serverId", "server is required")
	}
	registryID, err := optionalUUID(payload.RegistryCredentialID)
	if err != nil {
		return services.ResourceInstallationInput{}, domainPayloadError("registryCredentialId", "registry credential is invalid")
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
		err = controller.service.RunInstallation(etx.Request().Context(), resourceID, installationID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container is running")
}

func (controller Resources) StopInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.StopInstallation(etx.Request().Context(), resourceID, installationID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container stopped")
}

func (controller Resources) RestartInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.RestartInstallation(etx.Request().Context(), resourceID, installationID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Container restarted")
}

func (controller Resources) RemoveInstallationContainer(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.RemoveInstallationContainer(etx.Request().Context(), resourceID, installationID)
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
		_, err = controller.service.UpdateInstallation(etx.Request().Context(), resourceID, installationID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Installation updated")
}

func (controller Resources) DestroyInstallation(etx *echo.Context) error {
	resourceID, installationID, err := parseChildIDs(etx, "installationID")
	if err == nil {
		err = controller.service.ArchiveInstallation(etx.Request().Context(), resourceID, installationID)
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
	return services.ResourceVolumeInput{Name: payload.Name, Driver: payload.Driver, Configuration: payload.Configuration, ServerID: serverID}, nil
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
		_, err = controller.service.UpdateVolume(etx.Request().Context(), resourceID, volumeID, input)
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
		return services.ResourceMountInput{}, domainPayloadError("mount", "volume or installation is invalid")
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
	Name                   string          `json:"name"`
	Kind                   string          `json:"kind"`
	Configuration          json.RawMessage `json:"configuration"`
	IntervalSeconds        int32           `json:"intervalSeconds"`
	TimeoutSeconds         int32           `json:"timeoutSeconds"`
	FailureThreshold       int32           `json:"failureThreshold"`
	SuccessThreshold       int32           `json:"successThreshold"`
	Enabled                bool            `json:"enabled"`
	ResourceInstallationID string          `json:"resourceInstallationId"`
	ResourceEndpointID     string          `json:"resourceEndpointId"`
	ResourceCredentialID   string          `json:"resourceCredentialId"`
}

func (payload resourceHealthCheckPayload) serviceInput() (services.ResourceHealthCheckInput, error) {
	installationID, installationErr := optionalUUID(payload.ResourceInstallationID)
	endpointID, endpointErr := optionalUUID(payload.ResourceEndpointID)
	credentialID, credentialErr := optionalUUID(payload.ResourceCredentialID)
	if installationErr != nil || endpointErr != nil || credentialErr != nil {
		return services.ResourceHealthCheckInput{}, domainPayloadError("healthCheck", "installation, endpoint, or credential is invalid")
	}
	input := services.ResourceHealthCheckInput{
		Name: payload.Name, Kind: payload.Kind, Configuration: payload.Configuration,
		IntervalSeconds: payload.IntervalSeconds, TimeoutSeconds: payload.TimeoutSeconds,
		FailureThreshold: payload.FailureThreshold, SuccessThreshold: payload.SuccessThreshold,
		Enabled: payload.Enabled, ResourceEndpointID: endpointID, ResourceCredentialID: credentialID,
	}
	if installationID != nil {
		input.ResourceInstallationID = *installationID
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
		_, err = controller.service.UpdateHealthCheck(etx.Request().Context(), resourceID, healthCheckID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Health check updated")
}

func (controller Resources) DestroyHealthCheck(etx *echo.Context) error {
	resourceID, healthCheckID, err := parseChildIDs(etx, "healthCheckID")
	if err == nil {
		err = controller.service.ArchiveHealthCheck(etx.Request().Context(), resourceID, healthCheckID)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Health check archived")
}

func (controller Resources) renderShow(etx *echo.Context, resourceID uuid.UUID, option inertia.PageOption) error {
	return controller.renderShowPage(etx, resourceID, nil, option)
}

func (controller Resources) renderShowPage(etx *echo.Context, resourceID uuid.UUID, enrollment inertia.Props, option inertia.PageOption) error {
	if err := controller.access.ObserveResource(etx.Request().Context(), resourceID); err != nil {
		slog.WarnContext(etx.Request().Context(), "failed to observe Resource WireGuard device handshakes", "resource_id", resourceID, "error", err)
	}
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
	privateAccess, err := controller.access.Details(etx.Request().Context(), resourceID, cookies.ExtractFromCookieApp(etx).UserID)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	props := inertia.Props{"auth": authProps(etx), "resource": resourceDetailProps(detail, privateAccess), "options": resourceOptionsProps(options), "flash": resourceFlashProps(etx)}
	if enrollment != nil {
		props["enrollment"] = enrollment
	}
	if option != nil {
		return inertia.Page(etx, "Resources/Show", props, option)
	}
	return inertia.Page(etx, "Resources/Show", props)
}

func (controller Resources) renderEdit(etx *echo.Context, resourceID uuid.UUID, option inertia.PageOption) error {
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
	props := inertia.Props{"auth": authProps(etx), "resource": resourceDetailProps(detail, models.ResourcePrivateAccessDetails{}), "options": resourceOptionsProps(options), "flash": resourceFlashProps(etx)}
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
		return inertia.Page(etx, "Resources/New", inertia.Props{"auth": authProps(etx), "options": resourceOptionsProps(options)}, inertia.WithValidationErrors(validationErrors.ToMap()))
	}
	return controller.redirectError(etx, routes.ResourceNew.URL(), err)
}

func (controller Resources) finishChildMutation(etx *echo.Context, resourceID uuid.UUID, err error, success string) error {
	returnToEdit := etx.QueryParam("returnTo") == "edit"
	if err != nil {
		if validationErrors, ok := validation.As(err); ok && resourceID != uuid.Nil {
			if returnToEdit {
				return controller.renderEdit(etx, resourceID, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
			return controller.renderShow(etx, resourceID, inertia.WithValidationErrors(validationErrors.ToMap()))
		}
		location := routes.ResourceShow.URL(resourceID)
		if returnToEdit {
			location = routes.ResourceEdit.URL(resourceID)
		}
		return controller.redirectError(etx, location, err)
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashSuccess, success); flashErr != nil {
		return controller.renderLoadError(etx, flashErr)
	}
	location := routes.ResourceShow.URL(resourceID)
	if returnToEdit {
		location = routes.ResourceEdit.URL(resourceID)
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func resourceFlashProps(etx *echo.Context) []inertia.Props {
	flashes := request.ExtractContext[[]cookies.FlashMessage](etx.Request().Context(), request.SessionFlashesKey)
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

func bindResourceChild[T any](etx *echo.Context, bind func() (T, error)) (uuid.UUID, T, error) {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var input T
	if err == nil {
		input, err = bind()
	}
	return resourceID, input, err
}

func domainPayloadError(field, message string) error {
	return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: field, Code: "invalid", Message: message}})
}

func prefixResourceValidation(err error, prefix string) error {
	validationErrors, ok := validation.As(err)
	if !ok {
		return err
	}
	return errors.Join(models.ErrDomainValidation, validation.WithFieldPrefix(validationErrors, prefix))
}
