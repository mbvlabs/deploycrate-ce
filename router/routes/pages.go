package routes

import (
	"deploycrate-ce/internal/routing"
)

var HomePage = routing.NewSimpleRoute(
	"/",
	"pages.home",
	"",
)
