package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ResourceListFilters struct {
	Search         string
	Kind           string
	Category       string
	ManagementMode string
	SharingScope   string
}

type ResourceListItem struct {
	ID                uuid.UUID                  `bun:"id"`
	Name              string                     `bun:"name"`
	Category          string                     `bun:"category"`
	Kind              string                     `bun:"kind"`
	DatabaseName      string                     `bun:"database_name"`
	ManagementMode    ResourceManagementModeEnum `bun:"management_mode"`
	SharingScope      ResourceSharingScopeEnum   `bun:"sharing_scope"`
	ConnectionCount   int                        `bun:"connection_count"`
	InstallationCount int                        `bun:"installation_count"`
	EndpointCount     int                        `bun:"endpoint_count"`
	Health            string                     `bun:"health"`
}

type ResourceInstallationDetail struct {
	ResourceInstallationEntity
	ServerName       string          `bun:"server_name"`
	ServerAddress    string          `bun:"server_address"`
	State            string          `bun:"state"`
	ServiceState     string          `bun:"service_state"`
	Health           string          `bun:"health"`
	HealthReason     string          `bun:"health_reason"`
	ContainerDetails json.RawMessage `bun:"container_details"`
	ObservedAt       sql.NullTime    `bun:"observed_at"`
	CanControl       bool            `bun:"-"`
}

type ResourceVolumeDetail struct {
	ResourceVolumeEntity
	ServerName string `bun:"server_name"`
}

type ResourceMountDetail struct {
	ResourceVolumeMountEntity
	VolumeName    string `bun:"volume_name"`
	ContainerName string `bun:"container_name"`
}

type ResourceHealthCheckDetail struct {
	ResourceHealthCheckEntity
	State      string       `bun:"state"`
	Message    string       `bun:"message"`
	ObservedAt sql.NullTime `bun:"observed_at"`
}

type ResourceDetails struct {
	Resource      ResourceEntity
	Connections   []ResourceConnectionDetail
	Endpoints     []ResourceEndpointEntity
	Credentials   []ResourceCredentialEntity
	Installations []ResourceInstallationDetail
	Volumes       []ResourceVolumeDetail
	Mounts        []ResourceMountDetail
	HealthChecks  []ResourceHealthCheckDetail
}

type ResourcePrivateAccessDetails struct {
	DeviceGrants     []SystemWireGuardDeviceGrant
	AvailableDevices []SystemWireGuardDeviceOption
}

type ResourceConnectionDetail struct {
	EnvironmentResourceEntity
	EnvironmentName     string `bun:"environment_name"`
	EnvironmentKind     string `bun:"environment_kind"`
	EnvironmentArchived bool   `bun:"environment_archived"`
	ApplicationName     string `bun:"application_name"`
	ApplicationSlug     string `bun:"application_slug"`
	ApplicationArchived bool   `bun:"application_archived"`
	EndpointName        string `bun:"endpoint_name"`
	CredentialName      string `bun:"credential_name"`
}

type ResourceCredentialSummary struct {
	ID                     uuid.UUID
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Name                   string
	Username               string
	Metadata               json.RawMessage
	HasEncryptedPayload    bool
	ResourceInstallationID *uuid.UUID
}

type ResourceEnvironmentOption struct {
	ID              uuid.UUID `bun:"id"`
	Name            string    `bun:"name"`
	Kind            string    `bun:"kind"`
	ApplicationName string    `bun:"application_name"`
}

type ResourceServerOption struct {
	ID      uuid.UUID `bun:"id"`
	Name    string    `bun:"name"`
	Address string    `bun:"address"`
}

type ResourceNetworkOption struct {
	ID              uuid.UUID            `bun:"id"`
	Name            string               `bun:"name"`
	ServerIDs       []uuid.UUID          `bun:"-"`
	ServerAddresses map[uuid.UUID]string `bun:"-"`
}

type ResourceRegistryCredentialOption struct {
	ID   uuid.UUID `bun:"id"`
	Name string    `bun:"name"`
}

type ResourceFormOptions struct {
	Kinds               []ResourceKindDefinition
	Environments        []ResourceEnvironmentOption
	Servers             []ResourceServerOption
	PrivateNetworks     []ResourceNetworkOption
	RegistryCredentials []ResourceRegistryCredentialOption
}
