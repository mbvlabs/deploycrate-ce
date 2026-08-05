package routes

import "deploycrate-ce/internal/routing"

const CaddyRoutesPrefix = "/infrastructure/caddy"

var CaddyRoutes = routing.NewSimpleRoute("", "caddy-routes.index", CaddyRoutesPrefix)
var CaddyRouteShow = routing.NewRouteWithStringID("/routes/:id", "caddy-routes.show", CaddyRoutesPrefix)
var CaddyRouteCreate = routing.NewSimpleRoute("", "caddy-routes.create", CaddyRoutesPrefix)
var CaddyRouteUpdate = routing.NewRouteWithUUIDID("/:id", "caddy-routes.update", CaddyRoutesPrefix)
var CaddyRouteDestroy = routing.NewRouteWithUUIDID("/:id", "caddy-routes.destroy", CaddyRoutesPrefix)
var CaddyResourceRouteUpdate = routing.NewRouteWithUUIDID("/resource-routes/:id", "caddy-resource-routes.update", CaddyRoutesPrefix)
