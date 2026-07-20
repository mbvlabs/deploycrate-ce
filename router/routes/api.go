package routes

import (
	"deploycrate-ce/internal/routing"
)

const APIPrefix = "/api"

var Health = routing.NewSimpleRoute(
	"/health",
	"api.health",
	APIPrefix,
)
