package controllers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

const maxGitHubWebhookBody = 2 << 20

type GitHubConnections struct {
	connection *services.GitHubConnection
	webhook    *services.GitHubWebhook
}

func NewGitHubConnections(connection *services.GitHubConnection, webhook *services.GitHubWebhook) GitHubConnections {
	return GitHubConnections{connection: connection, webhook: webhook}
}

func (controller GitHubConnections) RegisterRoutes(r *router.Router) error {
	admin := []echo.MiddlewareFunc{middleware.AdminOnly}
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler     echo.HandlerFunc
		middlewares []echo.MiddlewareFunc
	}{
		{http.MethodGet, routes.GitHubConnection, controller.Show, admin},
		{http.MethodPost, routes.GitHubAppSetup, controller.StartAppSetup, admin},
		{http.MethodGet, routes.GitHubAppCallback, controller.CompleteAppSetup, admin},
		{http.MethodPost, routes.GitHubInstall, controller.StartInstallation, admin},
		{http.MethodGet, routes.GitHubInstallCallback, controller.CompleteInstallation, admin},
		{http.MethodGet, routes.GitHubInstallationShow, controller.ShowInstallation, admin},
		{http.MethodPost, routes.GitHubInstallationSync, controller.SyncInstallation, admin},
		{http.MethodPost, routes.GitHubInstallationVerify, controller.VerifyInstallation, admin},
		{http.MethodDelete, routes.GitHubInstallationDestroy, controller.DestroyInstallation, admin},
		{http.MethodDelete, routes.GitHubAppDestroy, controller.DestroyApp, admin},
		{http.MethodPost, routes.GitHubWebhook, controller.Webhook, nil},
	}
	errList := make([]error, 0, len(definitions))
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(), Handler: definition.handler, Middlewares: definition.middlewares})
		if err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

func (controller GitHubConnections) Show(etx *echo.Context) error {
	state, err := controller.connection.State(etx.Request().Context())
	if err != nil {
		return controller.renderError(etx, "load GitHub connection", err)
	}
	return inertia.Page(etx, "Connections/GitHub", inertia.Props{"auth": authProps(etx), "connection": state})
}

func (controller GitHubConnections) ShowInstallation(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	detail, err := controller.connection.InstallationDetail(etx.Request().Context(), id)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return controller.renderError(etx, "load GitHub installation", err)
	}
	return inertia.Page(etx, "Connections/GitHub/Show", inertia.Props{"auth": authProps(etx), "connection": detail})
}

func (controller GitHubConnections) StartAppSetup(etx *echo.Context) error {
	var payload struct {
		OwnerType  string `json:"ownerType"`
		OwnerLogin string `json:"ownerLogin"`
		Public     bool   `json:"public"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	start, err := controller.connection.StartManifest(etx.Request().Context(), cookies.ExtractFromCookieApp(etx).UserID, payload.OwnerType, payload.OwnerLogin, payload.Public)
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	return inertia.Page(etx, "Connections/GitHub/ManifestHandoff", inertia.Props{"auth": authProps(etx), "handoff": start})
}

func (controller GitHubConnections) CompleteAppSetup(etx *echo.Context) error {
	_, err := controller.connection.CompleteManifest(etx.Request().Context(), cookies.ExtractFromCookieApp(etx).UserID, etx.QueryParam("state"), etx.QueryParam("code"))
	if err != nil {
		message := "GitHub App setup failed. If GitHub created the App, delete the orphan App in GitHub and restart setup."
		if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}
		slog.ErrorContext(etx.Request().Context(), "failed to complete GitHub App setup", "error", err)
		return inertia.Redirect(etx, routes.GitHubConnection.URL(), http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub App connected")
	return inertia.Redirect(etx, routes.GitHubConnection.URL(), http.StatusSeeOther)
}

func (controller GitHubConnections) StartInstallation(etx *echo.Context) error {
	var payload struct {
		OwnerType  string `json:"ownerType"`
		OwnerLogin string `json:"ownerLogin"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	location, err := controller.connection.StartInstallation(etx.Request().Context(), cookies.ExtractFromCookieApp(etx).UserID, payload.OwnerType, payload.OwnerLogin)
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	return inertia.Location(etx, location)
}

func (controller GitHubConnections) CompleteInstallation(etx *echo.Context) error {
	externalID, err := strconv.ParseInt(etx.QueryParam("installation_id"), 10, 64)
	if err != nil || externalID <= 0 {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), services.ErrGitHubSetupState)
	}
	installation, err := controller.connection.CompleteInstallation(etx.Request().Context(), cookies.ExtractFromCookieApp(etx).UserID, etx.QueryParam("state"), externalID)
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub installation synchronized")
	return inertia.Redirect(etx, routes.GitHubInstallationShow.URL(installation.ID), http.StatusSeeOther)
}

func (controller GitHubConnections) SyncInstallation(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		_, err = controller.connection.Synchronize(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub repositories synchronized")
	return inertia.Redirect(etx, routes.GitHubInstallationShow.URL(id), http.StatusSeeOther)
}

func (controller GitHubConnections) VerifyInstallation(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.connection.Verify(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub installation access verified")
	return inertia.Redirect(etx, routes.GitHubInstallationShow.URL(id), http.StatusSeeOther)
}

func (controller GitHubConnections) DestroyInstallation(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.connection.ArchiveInstallation(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub installation archived locally")
	return inertia.Redirect(etx, routes.GitHubConnection.URL(), http.StatusSeeOther)
}

func (controller GitHubConnections) DestroyApp(etx *echo.Context) error {
	if err := controller.connection.ArchiveApp(etx.Request().Context()); err != nil {
		return controller.redirectWithError(etx, routes.GitHubConnection.URL(), err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "GitHub App archived locally")
	return inertia.Redirect(etx, routes.GitHubConnection.URL(), http.StatusSeeOther)
}

func (controller GitHubConnections) Webhook(etx *echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(etx.Request().Body, maxGitHubWebhookBody+1))
	if err != nil || len(body) > maxGitHubWebhookBody {
		return etx.JSON(http.StatusRequestEntityTooLarge, "webhook payload exceeds the allowed size")
	}
	err = controller.webhook.Process(etx.Request().Context(), etx.Request().Header.Get("X-GitHub-Delivery"), etx.Request().Header.Get("X-GitHub-Event"), etx.Request().Header.Get("X-Hub-Signature-256"), body)
	if errors.Is(err, services.ErrGitHubWebhookSignature) {
		return etx.JSON(http.StatusUnauthorized, "invalid webhook signature")
	}
	if err != nil {
		slog.ErrorContext(etx.Request().Context(), "failed to process GitHub webhook", "delivery_id", etx.Request().Header.Get("X-GitHub-Delivery"), "error", err)
		return etx.JSON(http.StatusInternalServerError, "webhook processing failed")
	}
	return etx.NoContent(http.StatusAccepted)
}

func (controller GitHubConnections) redirectWithError(etx *echo.Context, location string, err error) error {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "GitHub operation failed"
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func (controller GitHubConnections) renderError(etx *echo.Context, operation string, err error) error {
	slog.ErrorContext(etx.Request().Context(), operation, "error", err)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}

func authProps(etx *echo.Context) inertia.Props {
	return inertia.Props{"email": cookies.ExtractFromCookieApp(etx).Email}
}
