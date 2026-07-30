package controllers

import (
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/router"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type ObjectStorage struct {
	backups *services.ResourceBackups
}

func NewObjectStorage(backups *services.ResourceBackups) ObjectStorage {
	return ObjectStorage{backups: backups}
}

func (controller ObjectStorage) RegisterRoutes(r *router.Router) error {
	_, err := r.AddRoute(echo.Route{
		Method: http.MethodGet, Path: routes.ObjectStorage.Path(), Name: routes.ObjectStorage.Name(),
		Handler: controller.Index, Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
	})
	return err
}

func (controller ObjectStorage) Index(etx *echo.Context) error {
	destinations, err := controller.backups.Destinations(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	items := make([]inertia.Props, 0, len(destinations))
	for _, destination := range destinations {
		items = append(items, inertia.Props{
			"id": destination.ID, "name": destination.Name, "provider": destination.Provider,
			"endpoint": destination.Endpoint, "region": destination.Region, "bucket": destination.Bucket,
			"prefix": destination.Prefix, "verifiedAt": destination.VerifiedAt, "lastUsedAt": destination.LastUsedAt,
		})
	}
	return inertia.Page(etx, "Connections/ObjectStorage", inertia.Props{"auth": authProps(etx), "destinations": items})
}
