package routes

import "deploycrate-ce/internal/routing"

const ResourceDatabasesPrefix = "/resources/databases"

var ResourceDatabaseNew = routing.NewSimpleRoute("/new", "resources.databases.new", ResourceDatabasesPrefix)
var ResourceDatabaseCreate = routing.NewSimpleRoute("", "resources.databases.create", ResourceDatabasesPrefix)
