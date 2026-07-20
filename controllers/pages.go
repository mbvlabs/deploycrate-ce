package controllers

import (
	"errors"
	"net/http"

	"deploycrate-ce/internal/hypermedia"
	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/views"

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
	})
	if err != nil {
		errs = append(errs, err)
	}

	_, err = r.AddRoute(echo.Route{
		Method:  http.MethodHead,
		Path:    routes.HomePage.Path(),
		Name:    routes.HomePage.Name() + ".head",
		Handler: p.Home,
	})
	if err != nil {
		errs = append(errs, err)
	}

	_ = r.AddRouteNotFound(p.NotFound)

	return errors.Join(errs...)
}

func (p Pages) Home(etx *echo.Context) error {
	return hypermedia.RenderPage(etx, views.Welcome{}.Page())
}

func (p Pages) NotFound(etx *echo.Context) error {
	return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
}
