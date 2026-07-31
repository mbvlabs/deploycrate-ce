package routes

import "deploycrate-ce/internal/routing"

const RegistryResourcesPrefix = "/connections/registries"

var RegistryResources = routing.NewSimpleRoute("", "registry-resources.index", RegistryResourcesPrefix)
var RegistryResourceCreate = routing.NewSimpleRoute("", "registry-resources.create", RegistryResourcesPrefix)
var RegistryResourceDestroy = routing.NewRouteWithUUIDID("/:id", "registry-resources.destroy", RegistryResourcesPrefix)
