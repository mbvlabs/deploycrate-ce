package routes

import "deploycrate-ce/internal/routing"

const ContainerRegistriesPrefix = "/connections/registries"

var ContainerRegistries = routing.NewSimpleRoute("", "container-registries.index", ContainerRegistriesPrefix)
var ContainerRegistryCreate = routing.NewSimpleRoute("", "container-registries.create", ContainerRegistriesPrefix)
var ContainerRegistryDestroy = routing.NewRouteWithUUIDID("/:id", "container-registries.destroy", ContainerRegistriesPrefix)
