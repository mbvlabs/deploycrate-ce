package controllers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/models"
)

func resourceDetailProps(detail models.ResourceDetails, privateAccess models.ResourcePrivateAccessDetails) inertia.Props {
	resource := detail.Resource
	connections := make([]inertia.Props, 0, len(detail.Connections))
	for _, connection := range detail.Connections {
		connections = append(connections, inertia.Props{
			"id": connection.ID, "createdAt": connection.CreatedAt, "updatedAt": connection.UpdatedAt,
			"alias": connection.Alias, "configuration": connection.Configuration,
			"database":      resourceConnectionDatabase(connection.Configuration),
			"environmentId": connection.EnvironmentID, "environmentName": connection.EnvironmentName,
			"environmentKind": connection.EnvironmentKind, "environmentArchived": connection.EnvironmentArchived,
			"applicationName": connection.ApplicationName, "applicationSlug": connection.ApplicationSlug,
			"applicationArchived": connection.ApplicationArchived, "resourceEndpointId": connection.ResourceEndpointID,
			"endpointName": connection.EndpointName, "resourceCredentialId": connection.ResourceCredentialID,
			"credentialName": connection.CredentialName,
		})
	}
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
			"name": credential.Name, "username": nullString(credential.Username),
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
			"containerDetails": installation.ContainerDetails, "canControl": installation.CanControl,
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
	deviceGrants := make([]inertia.Props, 0, len(privateAccess.DeviceGrants))
	privateAccessState := ""
	for _, endpoint := range detail.Endpoints {
		if endpoint.PrivateNetworkID != nil {
			privateAccessState = "configured"
			break
		}
	}
	allApplied := len(privateAccess.DeviceGrants) > 0
	for _, grant := range privateAccess.DeviceGrants {
		deviceGrants = append(deviceGrants, inertia.Props{
			"deviceId": grant.DeviceID, "deviceName": grant.DeviceName, "ownerEmail": grant.OwnerEmail,
			"privateAddress": grant.PrivateAddress, "grantId": grant.GrantID, "grantedAt": grant.GrantedAt,
			"applicationState": grant.ApplicationState, "applicationError": grant.ApplicationError,
			"latestHandshakeAt": grant.LatestHandshakeAt, "observedAt": grant.ObservedAt,
		})
		if grant.ApplicationState == "failed" {
			privateAccessState = "failed"
		}
		if grant.ApplicationState != "applied" {
			allApplied = false
			if privateAccessState != "failed" {
				privateAccessState = "applying"
			}
		}
	}
	if allApplied {
		privateAccessState = "ready"
	}
	availableDevices := make([]inertia.Props, 0, len(privateAccess.AvailableDevices))
	for _, device := range privateAccess.AvailableDevices {
		availableDevices = append(availableDevices, inertia.Props{
			"id": device.ID, "name": device.Name, "privateAddress": device.PrivateAddress,
		})
	}
	return inertia.Props{
		"id": resource.ID, "createdAt": resource.CreatedAt, "updatedAt": resource.UpdatedAt,
		"name": resource.Name, "category": resource.Category, "kind": resource.Kind,
		"databaseName":   resource.DatabaseName,
		"managementMode": resource.ManagementMode.String(), "sharingScope": resource.SharingScope.String(),
		"connectionCount": len(connections), "connections": connections,
		"endpoints": endpoints, "credentials": credentials, "installations": installations,
		"volumes": volumes, "mounts": mounts, "healthChecks": healthChecks,
		"privateAccessState": privateAccessState, "deviceGrants": deviceGrants,
		"availableDevices": availableDevices,
	}
}

func resourceBackupProps(detail models.ResourceBackupDetails) inertia.Props {
	destinations := make([]inertia.Props, 0, len(detail.Destinations))
	for _, destination := range detail.Destinations {
		destinations = append(destinations, inertia.Props{
			"id": destination.ID, "name": destination.Name, "provider": destination.Provider,
			"endpoint": destination.Endpoint, "region": destination.Region, "bucket": destination.Bucket,
			"prefix": destination.Prefix, "verifiedAt": destination.VerifiedAt, "lastUsedAt": destination.LastUsedAt,
		})
	}
	history := make([]inertia.Props, 0, len(detail.History))
	for _, backup := range detail.History {
		history = append(history, inertia.Props{
			"id": backup.ID, "status": backup.Status, "triggerType": backup.TriggerType,
			"scheduledAt": backup.ScheduledAt, "finishedAt": backup.FinishedAt,
			"verifiedAt": backup.VerifiedAt, "sizeBytes": backup.SizeBytes, "error": backup.Error,
		})
	}
	var policy inertia.Props
	if detail.Policy != nil {
		retention, _ := detail.Policy.RetentionPolicy()
		policy = inertia.Props{
			"id": detail.Policy.ID, "schedule": detail.Policy.Schedule,
			"active": detail.Policy.Schedulable(), "nextRunAt": detail.Policy.NextRunAt,
			"backupDestinationId": detail.Policy.BackupDestinationID,
			"keepLast":            retention.KeepLast, "keepDaily": retention.KeepDaily,
			"keepWeekly": retention.KeepWeekly, "keepMonthly": retention.KeepMonthly,
		}
	}
	return inertia.Props{
		"eligibility": inertia.Props{
			"eligible": detail.Eligibility.Eligible, "reason": detail.Eligibility.Reason,
			"installationId": detail.Eligibility.InstallationID,
		},
		"policy": policy, "destinations": destinations, "history": history,
	}
}

func resourceConnectionDatabase(configuration json.RawMessage) string {
	var value struct {
		Database string `json:"database"`
	}
	if json.Unmarshal(configuration, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value.Database)
}

func resourceListProps(items []models.ResourceListItem) []inertia.Props {
	props := make([]inertia.Props, 0, len(items))
	for _, item := range items {
		props = append(props, inertia.Props{
			"id": item.ID, "name": item.Name, "category": item.Category, "kind": item.Kind,
			"databaseName":   item.DatabaseName,
			"managementMode": item.ManagementMode.String(), "sharingScope": item.SharingScope.String(),
			"connectionCount": item.ConnectionCount, "installationCount": item.InstallationCount,
			"endpointCount": item.EndpointCount, "health": item.Health,
		})
	}
	return props
}

func resourceOptionsProps(options models.ResourceFormOptions) inertia.Props {
	kinds := make([]inertia.Props, 0, len(options.Kinds))
	for _, kind := range options.Kinds {
		fields := make([]inertia.Props, 0, len(kind.CredentialFields))
		for _, field := range kind.CredentialFields {
			fields = append(fields, inertia.Props{"name": field.Name, "label": field.Label, "required": field.Required, "secret": field.Secret})
		}
		kinds = append(kinds, inertia.Props{
			"kind": kind.Kind, "label": kind.Label, "category": kind.Category,
			"protocols": kind.Protocols, "endpointRoles": kind.EndpointRoles, "tlsModes": kind.TLSModes,
			"credentialFields": fields, "healthCheckKinds": kind.HealthCheckKinds,
			"defaultPort": kind.DefaultPort, "defaultProtocol": kind.DefaultProtocol, "defaultTlsMode": kind.DefaultTLSMode,
		})
	}
	environments := make([]inertia.Props, 0, len(options.Environments))
	for _, environment := range options.Environments {
		environments = append(environments, inertia.Props{"id": environment.ID, "name": environment.Name, "kind": environment.Kind, "applicationName": environment.ApplicationName})
	}
	servers := make([]inertia.Props, 0, len(options.Servers))
	for _, server := range options.Servers {
		servers = append(servers, inertia.Props{"id": server.ID, "name": server.Name, "address": server.Address})
	}
	networks := make([]inertia.Props, 0, len(options.PrivateNetworks))
	for _, network := range options.PrivateNetworks {
		serverAddresses := make(map[string]string, len(network.ServerAddresses))
		for serverID, address := range network.ServerAddresses {
			serverAddresses[serverID.String()] = address
		}
		networks = append(networks, inertia.Props{"id": network.ID, "name": network.Name, "serverIds": network.ServerIDs, "serverAddresses": serverAddresses})
	}
	credentials := make([]inertia.Props, 0, len(options.RegistryCredentials))
	for _, credential := range options.RegistryCredentials {
		credentials = append(credentials, inertia.Props{"id": credential.ID, "name": credential.Name})
	}
	return inertia.Props{
		"kinds": kinds, "environments": environments, "servers": servers,
		"privateNetworks": networks, "registryCredentials": credentials,
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
