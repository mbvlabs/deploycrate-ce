package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"deploycrate-ce/models"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

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
	resourceID, err := uuidPathParam(etx, "id")
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
