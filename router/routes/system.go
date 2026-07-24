package routes

import "deploycrate-ce/internal/routing"

const SystemPrefix = "/system"

var SystemOverview = routing.NewSimpleRoute(
	"",
	"system.overview",
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
