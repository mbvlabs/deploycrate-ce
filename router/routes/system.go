package routes

import "deploycrate-ce/internal/routing"

const SystemPrefix = "/system"

var SystemOverview = routing.NewSimpleRoute(
	"",
	"system.overview",
	SystemPrefix,
)

var SystemTelemetry = routing.NewSimpleRoute(
	"/telemetry",
	"system.telemetry",
	SystemPrefix,
)

var SystemTelemetryLogs = routing.NewSimpleRoute(
	"/telemetry/logs",
	"system.telemetry.logs",
	SystemPrefix,
)

var SystemTelemetryTrace = routing.NewRouteWithStringID(
	"/telemetry/traces/:id",
	"system.telemetry.trace",
	SystemPrefix,
)

var SystemDeployments = routing.NewSimpleRoute(
	"/deployments",
	"system.deployments",
	SystemPrefix,
)

var SystemResources = routing.NewSimpleRoute(
	"/resources",
	"system.resources",
	SystemPrefix,
)

var SystemResource = routing.NewRouteWithUUIDID(
	"/resources/:id",
	"system.resource",
	SystemPrefix,
)

var SystemResourceEndpointCreate = routing.NewRouteWithUUIDID(
	"/resources/:id/endpoints",
	"system.resource.endpoint.create",
	SystemPrefix,
)

type SystemResourceCredentialParams struct {
	ResourceID   string `param:"resourceID"`
	CredentialID string `param:"credentialID"`
}

var SystemResourceCredentialReveal = routing.NewRouteWithParams[SystemResourceCredentialParams](
	"/resources/:resourceID/credentials/:credentialID/reveal",
	"system.resource.credential.reveal",
	SystemPrefix,
)

var SystemResourceWireGuardDeviceCreate = routing.NewRouteWithUUIDID(
	"/resources/:id/wireguard-devices",
	"system.resource.wireguard-device.create",
	SystemPrefix,
)

type SystemResourceWireGuardDeviceParams struct {
	ResourceID string `param:"resourceID"`
	DeviceID   string `param:"deviceID"`
}

var SystemResourceWireGuardDeviceDestroy = routing.NewRouteWithParams[SystemResourceWireGuardDeviceParams](
	"/resources/:resourceID/wireguard-devices/:deviceID",
	"system.resource.wireguard-device.destroy",
	SystemPrefix,
)

var SystemUpdate = routing.NewSimpleRoute(
	"/update",
	"system.update",
	SystemPrefix,
)

var SystemUpdateCreate = routing.NewSimpleRoute(
	"/update",
	"system.update.create",
	SystemPrefix,
)

var SystemUpdateStatus = routing.NewSimpleRoute(
	"/update/status",
	"system.update.status",
	SystemPrefix,
)
