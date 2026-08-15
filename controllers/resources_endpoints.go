package controllers

import (
	"encoding/json"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/models"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
