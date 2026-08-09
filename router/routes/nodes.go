package routes

import "deploycrate-ce/internal/routing"

const NodesPrefix = "/nodes"

var Nodes = routing.NewSimpleRoute("", "nodes.index", NodesPrefix)
var NodeNew = routing.NewSimpleRoute("/new", "nodes.new", NodesPrefix)
var NodeCreate = routing.NewSimpleRoute("", "nodes.create", NodesPrefix)
var NodeShow = routing.NewRouteWithUUIDID("/:id", "nodes.show", NodesPrefix)
var NodeConfirm = routing.NewRouteWithUUIDID("/:id/confirm", "nodes.confirm", NodesPrefix)
var NodeRetry = routing.NewRouteWithUUIDID("/:id/retry", "nodes.retry", NodesPrefix)
var NodeCapabilities = routing.NewRouteWithUUIDID(
	"/:id/capabilities",
	"nodes.capabilities.update",
	NodesPrefix,
)
