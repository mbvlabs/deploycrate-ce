package routes

import "deploycrate-ce/internal/routing"

const SystemPrefix = "/system"

var SystemOverview = routing.NewSimpleRoute(
	"",
	"system.overview",
	SystemPrefix,
)

var SystemDeployments = routing.NewSimpleRoute(
	"/deployments",
	"system.deployments",
	SystemPrefix,
)

var SystemDatabase = routing.NewSimpleRoute(
	"/database",
	"system.database",
	SystemPrefix,
)

var SystemNetwork = routing.NewSimpleRoute(
	"/network",
	"system.network",
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
