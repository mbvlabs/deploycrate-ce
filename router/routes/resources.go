package routes

import "deploycrate-ce/internal/routing"

const ResourcesPrefix = "/resources"

var Resources = routing.NewSimpleRoute("", "resources.index", ResourcesPrefix)
var ResourceNew = routing.NewSimpleRoute("/new", "resources.new", ResourcesPrefix)
var ResourceCreate = routing.NewSimpleRoute("", "resources.create", ResourcesPrefix)
var ResourceShow = routing.NewRouteWithUUIDID("/:id", "resources.show", ResourcesPrefix)
var ResourceEdit = routing.NewRouteWithUUIDID("/:id/edit", "resources.edit", ResourcesPrefix)
var ResourceUpdate = routing.NewRouteWithUUIDID("/:id", "resources.update", ResourcesPrefix)
var ResourceDestroy = routing.NewRouteWithUUIDID("/:id", "resources.destroy", ResourcesPrefix)

type ResourceConnectionParams struct {
	ResourceID   string `param:"id"`
	ConnectionID string `param:"connectionID"`
}

var ResourceConnectionCreate = routing.NewRouteWithUUIDID("/:id/connections", "resources.connections.create", ResourcesPrefix)
var ResourceConnectionUpdate = routing.NewRouteWithParams[ResourceConnectionParams]("/:id/connections/:connectionID", "resources.connections.update", ResourcesPrefix)
var ResourceConnectionDestroy = routing.NewRouteWithParams[ResourceConnectionParams]("/:id/connections/:connectionID", "resources.connections.destroy", ResourcesPrefix)

type ResourceEndpointParams struct {
	ResourceID string `param:"id"`
	EndpointID string `param:"endpointID"`
}

var ResourceEndpointCreate = routing.NewRouteWithUUIDID("/:id/endpoints", "resources.endpoints.create", ResourcesPrefix)
var ResourceEndpointUpdate = routing.NewRouteWithParams[ResourceEndpointParams]("/:id/endpoints/:endpointID", "resources.endpoints.update", ResourcesPrefix)
var ResourceEndpointDestroy = routing.NewRouteWithParams[ResourceEndpointParams]("/:id/endpoints/:endpointID", "resources.endpoints.destroy", ResourcesPrefix)

type ResourceCredentialParams struct {
	ResourceID   string `param:"id"`
	CredentialID string `param:"credentialID"`
}

var ResourceCredentialCreate = routing.NewRouteWithUUIDID("/:id/credentials", "resources.credentials.create", ResourcesPrefix)
var ResourceCredentialUpdate = routing.NewRouteWithParams[ResourceCredentialParams]("/:id/credentials/:credentialID", "resources.credentials.update", ResourcesPrefix)
var ResourceCredentialDestroy = routing.NewRouteWithParams[ResourceCredentialParams]("/:id/credentials/:credentialID", "resources.credentials.destroy", ResourcesPrefix)

type ResourceInstallationParams struct {
	ResourceID     string `param:"id"`
	InstallationID string `param:"installationID"`
}

var ResourceInstallationCreate = routing.NewRouteWithUUIDID("/:id/installations", "resources.installations.create", ResourcesPrefix)
var ResourceInstallationUpdate = routing.NewRouteWithParams[ResourceInstallationParams]("/:id/installations/:installationID", "resources.installations.update", ResourcesPrefix)
var ResourceInstallationDestroy = routing.NewRouteWithParams[ResourceInstallationParams]("/:id/installations/:installationID", "resources.installations.destroy", ResourcesPrefix)

type ResourceVolumeParams struct {
	ResourceID string `param:"id"`
	VolumeID   string `param:"volumeID"`
}

var ResourceVolumeCreate = routing.NewRouteWithUUIDID("/:id/volumes", "resources.volumes.create", ResourcesPrefix)
var ResourceVolumeUpdate = routing.NewRouteWithParams[ResourceVolumeParams]("/:id/volumes/:volumeID", "resources.volumes.update", ResourcesPrefix)
var ResourceVolumeDestroy = routing.NewRouteWithParams[ResourceVolumeParams]("/:id/volumes/:volumeID", "resources.volumes.destroy", ResourcesPrefix)

type ResourceMountParams struct {
	ResourceID string `param:"id"`
	MountID    string `param:"mountID"`
}

var ResourceMountCreate = routing.NewRouteWithUUIDID("/:id/mounts", "resources.mounts.create", ResourcesPrefix)
var ResourceMountUpdate = routing.NewRouteWithParams[ResourceMountParams]("/:id/mounts/:mountID", "resources.mounts.update", ResourcesPrefix)
var ResourceMountDestroy = routing.NewRouteWithParams[ResourceMountParams]("/:id/mounts/:mountID", "resources.mounts.destroy", ResourcesPrefix)

type ResourceHealthCheckParams struct {
	ResourceID    string `param:"id"`
	HealthCheckID string `param:"healthCheckID"`
}

var ResourceHealthCheckCreate = routing.NewRouteWithUUIDID("/:id/health-checks", "resources.health-checks.create", ResourcesPrefix)
var ResourceHealthCheckUpdate = routing.NewRouteWithParams[ResourceHealthCheckParams]("/:id/health-checks/:healthCheckID", "resources.health-checks.update", ResourcesPrefix)
var ResourceHealthCheckDestroy = routing.NewRouteWithParams[ResourceHealthCheckParams]("/:id/health-checks/:healthCheckID", "resources.health-checks.destroy", ResourcesPrefix)
