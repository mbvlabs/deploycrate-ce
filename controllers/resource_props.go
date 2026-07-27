package controllers

import (
	"database/sql"
	"time"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/models"
)

func resourceDetailProps(detail models.ResourceDetails) inertia.Props {
	resource := detail.Resource
	endpoints := make([]inertia.Props, 0, len(detail.Endpoints))
	for _, endpoint := range detail.Endpoints {
		endpoints = append(endpoints, inertia.Props{
			"id": endpoint.ID, "createdAt": endpoint.CreatedAt, "updatedAt": endpoint.UpdatedAt,
			"name": endpoint.Name, "role": endpoint.Role, "address": endpoint.Address,
			"port": endpoint.Port, "protocol": endpoint.Protocol, "tlsMode": endpoint.TlsMode,
			"settings": endpoint.Settings, "resourceInstallationId": endpoint.ResourceInstallationID,
			"privateNetworkId": endpoint.PrivateNetworkID,
		})
	}
	credentials := make([]inertia.Props, 0, len(detail.Credentials))
	for _, credential := range detail.Credentials {
		credentials = append(credentials, inertia.Props{
			"id": credential.ID, "createdAt": credential.CreatedAt, "updatedAt": credential.UpdatedAt,
			"name": credential.Name, "role": credential.Role, "username": nullString(credential.Username),
			"metadata": credential.Metadata, "hasEncryptedPayload": len(credential.EncPayload) > 0,
			"resourceInstallationId": credential.ResourceInstallationID,
		})
	}
	installations := make([]inertia.Props, 0, len(detail.Installations))
	for _, installation := range detail.Installations {
		installations = append(installations, inertia.Props{
			"id": installation.ID, "createdAt": installation.CreatedAt, "updatedAt": installation.UpdatedAt,
			"imageReference": installation.ImageReference, "imageDigest": nullString(installation.ImageDigest),
			"containerName": installation.ContainerName, "restartPolicy": installation.RestartPolicy,
			"configuration": installation.Configuration, "serverId": installation.ServerID,
			"serverName": installation.ServerName, "serverAddress": installation.ServerAddress,
			"registryCredentialId": installation.RegistryCredentialID, "state": installation.State,
			"serviceState": installation.ServiceState, "health": installation.Health,
			"healthReason": installation.HealthReason, "observedAt": nullTime(installation.ObservedAt),
		})
	}
	volumes := make([]inertia.Props, 0, len(detail.Volumes))
	for _, volume := range detail.Volumes {
		volumes = append(volumes, inertia.Props{
			"id": volume.ID, "createdAt": volume.CreatedAt, "updatedAt": volume.UpdatedAt,
			"name": volume.Name, "driver": volume.Driver, "configuration": volume.Configuration,
			"serverId": volume.ServerID, "serverName": volume.ServerName,
		})
	}
	mounts := make([]inertia.Props, 0, len(detail.Mounts))
	for _, mount := range detail.Mounts {
		mounts = append(mounts, inertia.Props{
			"id": mount.ID, "createdAt": mount.CreatedAt, "updatedAt": mount.UpdatedAt,
			"mountPath": mount.MountPath, "readOnly": mount.ReadOnly,
			"resourceVolumeId": mount.ResourceVolumeID, "resourceInstallationId": mount.ResourceInstallationID,
			"volumeName": mount.VolumeName, "containerName": mount.ContainerName,
		})
	}
	healthChecks := make([]inertia.Props, 0, len(detail.HealthChecks))
	for _, healthCheck := range detail.HealthChecks {
		healthChecks = append(healthChecks, inertia.Props{
			"id": healthCheck.ID, "createdAt": healthCheck.CreatedAt, "updatedAt": healthCheck.UpdatedAt,
			"name": healthCheck.Name, "kind": healthCheck.Kind, "configuration": healthCheck.Configuration,
			"intervalSeconds": healthCheck.IntervalSeconds, "timeoutSeconds": healthCheck.TimeoutSeconds,
			"failureThreshold": healthCheck.FailureThreshold, "successThreshold": healthCheck.SuccessThreshold,
			"enabled": healthCheck.Enabled, "resourceInstallationId": healthCheck.ResourceInstallationID,
			"resourceEndpointId": healthCheck.ResourceEndpointID, "resourceCredentialId": healthCheck.ResourceCredentialID,
			"state": healthCheck.State, "message": healthCheck.Message, "observedAt": nullTime(healthCheck.ObservedAt),
		})
	}
	return inertia.Props{
		"id": resource.ID, "createdAt": resource.CreatedAt, "updatedAt": resource.UpdatedAt,
		"name": resource.Name, "category": resource.Category, "kind": resource.Kind,
		"managementMode": resource.ManagementMode, "sharingScope": resource.SharingScope,
		"ownerEnvironmentId": resource.OwnerEnvironmentID, "ownerEnvironment": detail.OwnerEnvironment,
		"ownerApplication": detail.OwnerApplication, "isSystem": detail.IsSystem, "bindingCount": detail.BindingCount,
		"endpoints": endpoints, "credentials": credentials, "installations": installations,
		"volumes": volumes, "mounts": mounts, "healthChecks": healthChecks,
	}
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
