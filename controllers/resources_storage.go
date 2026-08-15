package controllers

import (
	"encoding/json"
	"strings"

	"deploycrate-ce/router/cookies"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
	resourceID, err := uuidPathParam(etx, "id")
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
