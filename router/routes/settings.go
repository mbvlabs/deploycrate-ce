package routes

import "deploycrate-ce/internal/routing"

const SettingsPrefix = "/settings"

var SystemOverview = routing.NewSimpleRoute(
	"/system",
	"system.overview",
	"",
)

var SelfUpdateSettings = routing.NewSimpleRoute(
	"/updates",
	"settings.updates",
	SettingsPrefix,
)

var SelfUpdateSettingsCreate = routing.NewSimpleRoute(
	"/updates",
	"settings.update",
	SettingsPrefix,
)
