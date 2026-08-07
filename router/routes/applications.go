package routes

import "deploycrate-ce/internal/routing"

const ApplicationsPrefix = "/applications"

var Applications = routing.NewSimpleRoute("", "applications.index", ApplicationsPrefix)
var ApplicationNew = routing.NewSimpleRoute("/new", "applications.new", ApplicationsPrefix)
var ApplicationCreate = routing.NewSimpleRoute("", "applications.create", ApplicationsPrefix)
var ApplicationShow = routing.NewRouteWithUUIDID("/:id", "applications.show", ApplicationsPrefix)

var ApplicationEdit = routing.NewRouteWithUUIDID(
	"/:id/edit",
	"applications.edit",
	ApplicationsPrefix,
)

var ApplicationUpdate = routing.NewRouteWithUUIDID(
	"/:id",
	"applications.update",
	ApplicationsPrefix,
)

var ApplicationSourceEdit = routing.NewRouteWithUUIDID(
	"/:id/source/edit",
	"applications.source.edit",
	ApplicationsPrefix,
)

var ApplicationSourceUpdate = routing.NewRouteWithUUIDID(
	"/:id/source",
	"applications.source.update",
	ApplicationsPrefix,
)

var ApplicationDestroy = routing.NewRouteWithUUIDID(
	"/:id",
	"applications.destroy",
	ApplicationsPrefix,
)
