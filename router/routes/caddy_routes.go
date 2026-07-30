package routes

import "deploycrate-ce/internal/routing"

const CaddyRoutesPrefix = "/system/caddy-routes"

var CaddyRoutes = routing.NewSimpleRoute("", "caddy-routes.index", CaddyRoutesPrefix)
var CaddyRouteCreate = routing.NewSimpleRoute("", "caddy-routes.create", CaddyRoutesPrefix)
var CaddyRouteUpdate = routing.NewRouteWithUUIDID("/:id", "caddy-routes.update", CaddyRoutesPrefix)
var CaddyRouteDestroy = routing.NewRouteWithUUIDID("/:id", "caddy-routes.destroy", CaddyRoutesPrefix)
