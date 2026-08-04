package routes

import "deploycrate-ce/internal/routing"

const DNSConnectionsPrefix = "/connections/dns"

var DnsConnections = routing.NewSimpleRoute("", "dns.connections", DNSConnectionsPrefix)
var DnsConnectionCreate = routing.NewSimpleRoute("", "dns.connections.create", DNSConnectionsPrefix)
var DnsConnectionSync = routing.NewRouteWithUUIDID("/:id/sync", "dns.connections.sync", DNSConnectionsPrefix)
var DnsConnectionTokenUpdate = routing.NewRouteWithUUIDID("/:id/token", "dns.connections.token.update", DNSConnectionsPrefix)
var DnsConnectionDestroy = routing.NewRouteWithUUIDID("/:id", "dns.connections.destroy", DNSConnectionsPrefix)
