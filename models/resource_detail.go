package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ResourceListFilters struct {
	Search             string
	Kind               string
	Category           string
	ManagementMode     string
	SharingScope       string
	OwnerEnvironmentID *uuid.UUID
}

type ResourceListItem struct {
	ID                 uuid.UUID `bun:"id" json:"id"`
	Name               string    `bun:"name" json:"name"`
	Category           string    `bun:"category" json:"category"`
	Kind               string    `bun:"kind" json:"kind"`
	ManagementMode     string    `bun:"management_mode" json:"managementMode"`
	SharingScope       string    `bun:"sharing_scope" json:"sharingScope"`
	OwnerEnvironmentID uuid.UUID `bun:"owner_environment_id" json:"ownerEnvironmentId"`
	OwnerEnvironment   string    `bun:"owner_environment" json:"ownerEnvironment"`
	OwnerApplication   string    `bun:"owner_application" json:"ownerApplication"`
	InstallationCount  int       `bun:"installation_count" json:"installationCount"`
	EndpointCount      int       `bun:"endpoint_count" json:"endpointCount"`
	Health             string    `bun:"health" json:"health"`
}

type ResourceInstallationDetail struct {
	ResourceInstallationEntity
	ServerName    string       `bun:"server_name" json:"serverName"`
	ServerAddress string       `bun:"server_address" json:"serverAddress"`
	State         string       `bun:"state" json:"state"`
	ServiceState  string       `bun:"service_state" json:"serviceState"`
	Health        string       `bun:"health" json:"health"`
	HealthReason  string       `bun:"health_reason" json:"healthReason"`
	ObservedAt    sql.NullTime `bun:"observed_at" json:"observedAt"`
}

type ResourceVolumeDetail struct {
	ResourceVolumeEntity
	ServerName string `bun:"server_name" json:"serverName"`
}

type ResourceMountDetail struct {
	ResourceVolumeMountEntity
	VolumeName    string `bun:"volume_name" json:"volumeName"`
	ContainerName string `bun:"container_name" json:"containerName"`
}

type ResourceHealthCheckDetail struct {
	ResourceHealthCheckEntity
	State      string       `bun:"state" json:"state"`
	Message    string       `bun:"message" json:"message"`
	ObservedAt sql.NullTime `bun:"observed_at" json:"observedAt"`
}

type ResourceDetails struct {
	Resource         ResourceEntity               `json:"resource"`
	OwnerEnvironment string                       `json:"ownerEnvironment"`
	OwnerApplication string                       `json:"ownerApplication"`
	IsSystem         bool                         `json:"isSystem"`
	BindingCount     int                          `json:"bindingCount"`
	Endpoints        []ResourceEndpointEntity     `json:"endpoints"`
	Credentials      []ResourceCredentialEntity   `json:"-"`
	Installations    []ResourceInstallationDetail `json:"installations"`
	Volumes          []ResourceVolumeDetail       `json:"volumes"`
	Mounts           []ResourceMountDetail        `json:"mounts"`
	HealthChecks     []ResourceHealthCheckDetail  `json:"healthChecks"`
}

type ResourceCredentialSummary struct {
	ID                     uuid.UUID       `json:"id"`
	CreatedAt              time.Time       `json:"createdAt"`
	UpdatedAt              time.Time       `json:"updatedAt"`
	Name                   string          `json:"name"`
	Role                   string          `json:"role"`
	Username               string          `json:"username"`
	Metadata               json.RawMessage `json:"metadata"`
	HasEncryptedPayload    bool            `json:"hasEncryptedPayload"`
	ResourceInstallationID *uuid.UUID      `json:"resourceInstallationId"`
}

type ResourceEnvironmentOption struct {
	ID              uuid.UUID `bun:"id" json:"id"`
	Name            string    `bun:"name" json:"name"`
	Kind            string    `bun:"kind" json:"kind"`
	ApplicationName string    `bun:"application_name" json:"applicationName"`
}

type ResourceServerOption struct {
	ID            uuid.UUID `bun:"id" json:"id"`
	Name          string    `bun:"name" json:"name"`
	Address       string    `bun:"address" json:"address"`
	EnvironmentID uuid.UUID `bun:"environment_id" json:"environmentId"`
}

type ResourceNetworkOption struct {
	ID            uuid.UUID `bun:"id" json:"id"`
	Name          string    `bun:"name" json:"name"`
	EnvironmentID uuid.UUID `bun:"environment_id" json:"environmentId"`
}

type ResourceRegistryCredentialOption struct {
	ID   uuid.UUID `bun:"id" json:"id"`
	Name string    `bun:"name" json:"name"`
}

type ResourceFormOptions struct {
	Kinds               []ResourceKindDefinition           `json:"kinds"`
	Environments        []ResourceEnvironmentOption        `json:"environments"`
	Servers             []ResourceServerOption             `json:"servers"`
	PrivateNetworks     []ResourceNetworkOption            `json:"privateNetworks"`
	RegistryCredentials []ResourceRegistryCredentialOption `json:"registryCredentials"`
}
