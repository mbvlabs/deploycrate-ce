package controllers

import (
	"encoding/json"
	"time"

	"deploycrate-ce/models"
)

type systemResourceIndexProp struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ResourceType  string `json:"resourceType"`
	Engine        string `json:"engine"`
	OriginAddress string `json:"originAddress"`
	OriginPort    int32  `json:"originPort"`
	Health        string `json:"health"`
}

func systemResourceIndexProps(items []models.SystemResourceIndexItem) []systemResourceIndexProp {
	props := make([]systemResourceIndexProp, 0, len(items))
	for _, item := range items {
		props = append(props, systemResourceIndexProp{
			ID: item.ID, Name: item.Name, ResourceType: item.ResourceType, Engine: item.Engine,
			OriginAddress: item.OriginAddress, OriginPort: item.OriginPort, Health: item.Health,
		})
	}
	return props
}

type systemResourceBindingProp struct {
	ID              string          `json:"id"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Alias           string          `json:"alias"`
	Configuration   json.RawMessage `json:"configuration"`
	EnvironmentID   string          `json:"environmentId"`
	EnvironmentName string          `json:"environmentName"`
	EnvironmentKind string          `json:"environmentKind"`
	EndpointID      string          `json:"endpointId"`
	CredentialID    string          `json:"credentialId"`
}

type systemResourceEndpointProp struct {
	ID               string          `json:"id"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	Name             string          `json:"name"`
	Role             string          `json:"role"`
	Address          string          `json:"address"`
	Port             int32           `json:"port"`
	Protocol         string          `json:"protocol"`
	TLSMode          string          `json:"tlsMode"`
	Settings         json.RawMessage `json:"settings"`
	PrivateNetworkID string          `json:"privateNetworkId"`
}

type systemResourceCredentialProp struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Username            string          `json:"username"`
	Metadata            json.RawMessage `json:"metadata"`
	HasEncryptedPayload bool            `json:"hasEncryptedPayload"`
}

type systemResourceInstallationProp struct {
	ID             string          `json:"id"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	ImageReference string          `json:"imageReference"`
	ImageDigest    string          `json:"imageDigest"`
	ContainerName  string          `json:"containerName"`
	RestartPolicy  string          `json:"restartPolicy"`
	Configuration  json.RawMessage `json:"configuration"`
	ServerID       string          `json:"serverId"`
	ServerName     string          `json:"serverName"`
	ServerAddress  string          `json:"serverAddress"`
	State          string          `json:"state"`
	ServiceState   string          `json:"serviceState"`
	Health         string          `json:"health"`
	HealthReason   string          `json:"healthReason"`
	ObservedAt     *time.Time      `json:"observedAt"`
}

type systemResourceVolumeMountProp struct {
	ID             string `json:"id"`
	MountPath      string `json:"mountPath"`
	ReadOnly       bool   `json:"readOnly"`
	InstallationID string `json:"installationId"`
}

type systemResourceVolumeProp struct {
	ID            string                          `json:"id"`
	Name          string                          `json:"name"`
	Driver        string                          `json:"driver"`
	Configuration json.RawMessage                 `json:"configuration"`
	ServerID      string                          `json:"serverId"`
	ServerName    string                          `json:"serverName"`
	Mounts        []systemResourceVolumeMountProp `json:"mounts"`
}

type systemResourceHealthCheckProp struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Configuration    json.RawMessage `json:"configuration"`
	IntervalSeconds  int32           `json:"intervalSeconds"`
	TimeoutSeconds   int32           `json:"timeoutSeconds"`
	FailureThreshold int32           `json:"failureThreshold"`
	SuccessThreshold int32           `json:"successThreshold"`
	Enabled          bool            `json:"enabled"`
	State            string          `json:"state"`
	Message          string          `json:"message"`
	ObservedAt       *time.Time      `json:"observedAt"`
}

type systemWireGuardDeviceGrantProp struct {
	DeviceID          string     `json:"deviceId"`
	DeviceName        string     `json:"deviceName"`
	OwnerEmail        string     `json:"ownerEmail"`
	PrivateAddress    string     `json:"privateAddress"`
	GrantID           string     `json:"grantId"`
	GrantedAt         time.Time  `json:"grantedAt"`
	ApplicationState  string     `json:"applicationState"`
	ApplicationError  string     `json:"applicationError"`
	LatestHandshakeAt *time.Time `json:"latestHandshakeAt"`
	ObservedAt        *time.Time `json:"observedAt"`
}

type systemPrivateNetworkProp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type systemWireGuardDeviceOptionProp struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PrivateAddress string `json:"privateAddress"`
}

type systemResourceDetailProp struct {
	ID               string                            `json:"id"`
	CreatedAt        time.Time                         `json:"createdAt"`
	UpdatedAt        time.Time                         `json:"updatedAt"`
	Name             string                            `json:"name"`
	ResourceType     string                            `json:"resourceType"`
	Engine           string                            `json:"engine"`
	ServerID         string                            `json:"serverId"`
	Bindings         []systemResourceBindingProp       `json:"bindings"`
	Endpoints        []systemResourceEndpointProp      `json:"endpoints"`
	Credentials      []systemResourceCredentialProp    `json:"credentials"`
	Installations    []systemResourceInstallationProp  `json:"installations"`
	Volumes          []systemResourceVolumeProp        `json:"volumes"`
	HealthChecks     []systemResourceHealthCheckProp   `json:"healthChecks"`
	DeviceGrants     []systemWireGuardDeviceGrantProp  `json:"deviceGrants"`
	PrivateNetworks  []systemPrivateNetworkProp        `json:"privateNetworks"`
	AvailableDevices []systemWireGuardDeviceOptionProp `json:"availableDevices"`
}

func systemResourceDetailProps(detail models.SystemResourceDetail) systemResourceDetailProp {
	prop := systemResourceDetailProp{
		ID:               detail.ID,
		CreatedAt:        detail.CreatedAt,
		UpdatedAt:        detail.UpdatedAt,
		Name:             detail.Name,
		ResourceType:     detail.ResourceType,
		Engine:           detail.Engine,
		ServerID:         detail.ServerID,
		Bindings:         make([]systemResourceBindingProp, 0, len(detail.Bindings)),
		Endpoints:        make([]systemResourceEndpointProp, 0, len(detail.Endpoints)),
		Credentials:      make([]systemResourceCredentialProp, 0, len(detail.Credentials)),
		Installations:    make([]systemResourceInstallationProp, 0, len(detail.Installations)),
		Volumes:          make([]systemResourceVolumeProp, 0, len(detail.Volumes)),
		HealthChecks:     make([]systemResourceHealthCheckProp, 0, len(detail.HealthChecks)),
		DeviceGrants:     make([]systemWireGuardDeviceGrantProp, 0, len(detail.DeviceGrants)),
		PrivateNetworks:  make([]systemPrivateNetworkProp, 0, len(detail.PrivateNetworks)),
		AvailableDevices: make([]systemWireGuardDeviceOptionProp, 0, len(detail.AvailableDevices)),
	}
	for _, item := range detail.Bindings {
		prop.Bindings = append(
			prop.Bindings,
			systemResourceBindingProp{
				ID:              item.ID,
				CreatedAt:       item.CreatedAt,
				UpdatedAt:       item.UpdatedAt,
				Alias:           item.Alias,
				Configuration:   item.Configuration,
				EnvironmentID:   item.EnvironmentID,
				EnvironmentName: item.EnvironmentName,
				EnvironmentKind: item.EnvironmentKind,
				EndpointID:      item.EndpointID,
				CredentialID:    item.CredentialID,
			},
		)
	}
	for _, item := range detail.Endpoints {
		prop.Endpoints = append(
			prop.Endpoints,
			systemResourceEndpointProp{
				ID:               item.ID,
				CreatedAt:        item.CreatedAt,
				UpdatedAt:        item.UpdatedAt,
				Name:             item.Name,
				Role:             item.Role,
				Address:          item.Address,
				Port:             item.Port,
				Protocol:         item.Protocol,
				TLSMode:          item.TLSMode,
				Settings:         item.Settings,
				PrivateNetworkID: item.PrivateNetworkID,
			},
		)
	}
	for _, item := range detail.Credentials {
		prop.Credentials = append(
			prop.Credentials,
			systemResourceCredentialProp{
				ID:                  item.ID,
				Name:                item.Name,
				Username:            item.Username,
				Metadata:            item.Metadata,
				HasEncryptedPayload: item.HasEncryptedPayload,
			},
		)
	}
	for _, item := range detail.Installations {
		prop.Installations = append(
			prop.Installations,
			systemResourceInstallationProp{
				ID:             item.ID,
				CreatedAt:      item.CreatedAt,
				UpdatedAt:      item.UpdatedAt,
				ImageReference: item.ImageReference,
				ImageDigest:    item.ImageDigest,
				ContainerName:  item.ContainerName,
				RestartPolicy:  item.RestartPolicy,
				Configuration:  item.Configuration,
				ServerID:       item.ServerID,
				ServerName:     item.ServerName,
				ServerAddress:  item.ServerAddress,
				State:          item.State,
				ServiceState:   item.ServiceState,
				Health:         item.Health,
				HealthReason:   item.HealthReason,
				ObservedAt:     item.ObservedAt,
			},
		)
	}
	for _, item := range detail.Volumes {
		volume := systemResourceVolumeProp{
			ID:            item.ID,
			Name:          item.Name,
			Driver:        item.Driver,
			Configuration: item.Configuration,
			ServerID:      item.ServerID,
			ServerName:    item.ServerName,
			Mounts:        make([]systemResourceVolumeMountProp, 0, len(item.Mounts)),
		}
		for _, mount := range item.Mounts {
			volume.Mounts = append(
				volume.Mounts,
				systemResourceVolumeMountProp{
					ID:             mount.ID,
					MountPath:      mount.MountPath,
					ReadOnly:       mount.ReadOnly,
					InstallationID: mount.InstallationID,
				},
			)
		}
		prop.Volumes = append(prop.Volumes, volume)
	}
	for _, item := range detail.HealthChecks {
		prop.HealthChecks = append(
			prop.HealthChecks,
			systemResourceHealthCheckProp{
				ID:               item.ID,
				Name:             item.Name,
				Kind:             item.Kind,
				Configuration:    item.Configuration,
				IntervalSeconds:  item.IntervalSeconds,
				TimeoutSeconds:   item.TimeoutSeconds,
				FailureThreshold: item.FailureThreshold,
				SuccessThreshold: item.SuccessThreshold,
				Enabled:          item.Enabled,
				State:            item.State,
				Message:          item.Message,
				ObservedAt:       item.ObservedAt,
			},
		)
	}
	for _, item := range detail.DeviceGrants {
		prop.DeviceGrants = append(
			prop.DeviceGrants,
			systemWireGuardDeviceGrantProp{
				DeviceID:          item.DeviceID,
				DeviceName:        item.DeviceName,
				OwnerEmail:        item.OwnerEmail,
				PrivateAddress:    item.PrivateAddress,
				GrantID:           item.GrantID,
				GrantedAt:         item.GrantedAt,
				ApplicationState:  item.ApplicationState,
				ApplicationError:  item.ApplicationError,
				LatestHandshakeAt: item.LatestHandshakeAt,
				ObservedAt:        item.ObservedAt,
			},
		)
	}
	for _, item := range detail.PrivateNetworks {
		prop.PrivateNetworks = append(
			prop.PrivateNetworks,
			systemPrivateNetworkProp{ID: item.ID, Name: item.Name},
		)
	}
	for _, item := range detail.AvailableDevices {
		prop.AvailableDevices = append(
			prop.AvailableDevices,
			systemWireGuardDeviceOptionProp{
				ID:             item.ID,
				Name:           item.Name,
				PrivateAddress: item.PrivateAddress,
			},
		)
	}
	return prop
}

type systemWireGuardDeviceProp struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	OwnerEmail        string     `json:"ownerEmail"`
	PrivateAddress    string     `json:"privateAddress"`
	ActivatedAt       time.Time  `json:"activatedAt"`
	GrantCount        int32      `json:"grantCount"`
	State             string     `json:"state"`
	LatestHandshakeAt *time.Time `json:"latestHandshakeAt"`
	ObservedAt        *time.Time `json:"observedAt"`
}

func systemWireGuardDeviceProps(
	devices []models.SystemWireGuardDevice,
) []systemWireGuardDeviceProp {
	props := make([]systemWireGuardDeviceProp, 0, len(devices))
	for _, device := range devices {
		props = append(props, systemWireGuardDeviceProp{
			ID:                device.ID,
			Name:              device.Name,
			OwnerEmail:        device.OwnerEmail,
			PrivateAddress:    device.PrivateAddress,
			ActivatedAt:       device.ActivatedAt,
			GrantCount:        device.GrantCount,
			State:             device.State,
			LatestHandshakeAt: device.LatestHandshakeAt,
			ObservedAt:        device.ObservedAt,
		})
	}
	return props
}
