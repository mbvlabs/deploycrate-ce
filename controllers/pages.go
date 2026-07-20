package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"

	"github.com/labstack/echo/v5"
)

type Pages struct {
	db         storage.Pool
	insertOnly queue.InsertOnly
}

func NewPages(
	db storage.Pool,
	insertOnly queue.InsertOnly,
) Pages {
	return Pages{db, insertOnly}
}

func (p Pages) RegisterRoutes(r *router.Router) error {
	errs := []error{}

	_, err := r.AddRoute(echo.Route{
		Method:  http.MethodGet,
		Path:    routes.HomePage.Path(),
		Name:    routes.HomePage.Name(),
		Handler: p.Home,
		Middlewares: []echo.MiddlewareFunc{
			middleware.AuthOnly,
		},
	})
	if err != nil {
		errs = append(errs, err)
	}

	_ = r.AddRouteNotFound(middleware.AuthOnly(p.NotFound))

	return errors.Join(errs...)
}

func (p Pages) Home(etx *echo.Context) error {
	appSession := cookies.ExtractFromCookieApp(etx)
	if appSession.Email == "" {
		user, err := models.User.Find(
			etx.Request().Context(),
			p.db.Executor(),
			appSession.UserID,
		)
		if err != nil {
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to load authenticated user",
				"error",
				err,
			)
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}

		if err := cookies.CreateAppSession(etx, user); err != nil {
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to refresh authenticated user session",
				"error",
				err,
			)
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}
		appSession.Email = user.Email
	}

	return inertia.Page(etx, "Home", inertia.Props{
		"auth": inertia.Props{
			"email": appSession.Email,
		},
	})
}

func (p Pages) NotFound(etx *echo.Context) error {
	return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
}
