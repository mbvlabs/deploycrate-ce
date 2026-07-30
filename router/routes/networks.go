package routes

import "deploycrate-ce/internal/routing"

const NetworksPrefix = "/networks"

var Networks = routing.NewSimpleRoute("", "networks.index", NetworksPrefix)

var NetworkWireGuardDeviceDestroy = routing.NewRouteWithUUIDID(
	"/wireguard-devices/:id",
	"networks.wireguard-device.destroy",
	NetworksPrefix,
)
