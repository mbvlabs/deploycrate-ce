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

type APIEnvironmentParams struct {
	EnvironmentID string `param:"environmentID"`
}

var APIEnvironmentDeploymentsCreate = routing.NewRouteWithParams[APIEnvironmentParams](
	"/environments/:environmentID/deployments",
	"api.environments.deployments.create",
	APIPrefix,
)
