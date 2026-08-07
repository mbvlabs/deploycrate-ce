package routes

import "deploycrate-ce/internal/routing"

const GitHubConnectionsPrefix = "/connections/github"
const GitHubWebhookPath = "/webhooks/github"

var GitHubConnection = routing.NewSimpleRoute("", "github", GitHubConnectionsPrefix)

var GitHubAppSetup = routing.NewSimpleRoute(
	"/app/setup",
	"github.app.setup",
	GitHubConnectionsPrefix,
)

var GitHubAppCallback = routing.NewSimpleRoute(
	"/app/callback",
	"github.app.callback",
	GitHubConnectionsPrefix,
)
var GitHubInstall = routing.NewSimpleRoute("/install", "github.install", GitHubConnectionsPrefix)

var GitHubInstallCallback = routing.NewSimpleRoute(
	"/install/callback",
	"github.install.callback",
	GitHubConnectionsPrefix,
)

var GitHubInstallationShow = routing.NewRouteWithUUIDID(
	"/installations/:id",
	"github.installation.show",
	GitHubConnectionsPrefix,
)

var GitHubInstallationSync = routing.NewRouteWithUUIDID(
	"/installations/:id/sync",
	"github.installation.sync",
	GitHubConnectionsPrefix,
)

var GitHubInstallationVerify = routing.NewRouteWithUUIDID(
	"/installations/:id/verify",
	"github.installation.verify",
	GitHubConnectionsPrefix,
)

var GitHubInstallationDestroy = routing.NewRouteWithUUIDID(
	"/installations/:id",
	"github.installation.destroy",
	GitHubConnectionsPrefix,
)
var GitHubAppDestroy = routing.NewSimpleRoute("/app", "github.app.destroy", GitHubConnectionsPrefix)
var GitHubWebhook = routing.NewSimpleRoute(GitHubWebhookPath, "github.webhook", "")
