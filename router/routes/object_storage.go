package routes

import "deploycrate-ce/internal/routing"

const ObjectStoragePrefix = "/connections/object-storage"

var ObjectStorage = routing.NewSimpleRoute("", "object-storage.index", ObjectStoragePrefix)
var ObjectStorageCreate = routing.NewSimpleRoute("", "object-storage.create", ObjectStoragePrefix)
var ObjectStorageShow = routing.NewRouteWithUUIDID("/:id", "object-storage.show", ObjectStoragePrefix)
var ObjectStorageRecovery = routing.NewRouteWithUUIDID("/:id/recovery", "object-storage.recovery", ObjectStoragePrefix)
