package routes

import "deploycrate-ce/internal/routing"

const ObjectStoragePrefix = "/connections/object-storage"

var ObjectStorage = routing.NewSimpleRoute("", "object-storage.index", ObjectStoragePrefix)
