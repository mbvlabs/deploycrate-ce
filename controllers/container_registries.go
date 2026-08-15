package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type RegistryResources struct {
	service *services.RegistryResources
}

func NewRegistryResources(service *services.RegistryResources) RegistryResources {
	return RegistryResources{service: service}
}

func (controller RegistryResources) RegisterRoutes(r *router.Router) error {
	return errors.Join(
		registerRoutes(r, []routeDefinition{
			{http.MethodGet, routes.RegistryResources, controller.Index},
			{http.MethodPost, routes.RegistryResourceCreate, controller.Create},
			{http.MethodGet, routes.RegistryResourceShow, controller.Show},
			{http.MethodDelete, routes.RegistryResourceDestroy, controller.Destroy},
			{http.MethodPost, routes.RegistryResourceImageDelete, controller.DeleteImage},
		}, middleware.AdminOnly),
		registerRoutes(r, []routeDefinition{
			{http.MethodPost, routes.RegistryResourceCredentials, controller.Credentials},
		}, middleware.AdminOnly, middleware.IPRateLimiter(5, routes.RegistryResources)),
	)
}

func (controller RegistryResources) Credentials(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	resourceID, err := uuidPathParam(etx, "id")
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Managed registry not found"},
		)
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := etx.Bind(
		&payload,
	); err != nil || payload.Password == "" ||
		len(payload.Password) > 4096 {
		return etx.JSON(
			http.StatusUnprocessableEntity,
			map[string]string{"error": "Current password is required"},
		)
	}

	credentials, err := controller.service.RevealManagedCredentials(
		etx.Request().Context(),
		resourceID,
		cookies.ExtractFromCookieApp(etx).UserID,
		payload.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCredentials):
			return etx.JSON(
				http.StatusUnprocessableEntity,
				map[string]string{"error": "Current password is incorrect"},
			)
		case errors.Is(err, services.ErrManagedRegistryUnavailable):
			return etx.JSON(
				http.StatusNotFound,
				map[string]string{"error": "Managed registry not found"},
			)
		default:
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to reveal managed Registry credentials",
				"resource_id",
				resourceID,
				"error",
				err,
			)
			return etx.JSON(
				http.StatusInternalServerError,
				map[string]string{"error": "Registry credentials could not be loaded"},
			)
		}
	}

	return etx.JSON(http.StatusOK, credentials)
}

func (controller RegistryResources) Index(etx *echo.Context) error {
	registries, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(
		etx,
		"Connections/Registries",
		inertia.Props{"auth": authProps(etx), "registries": registries},
	)
}

func (controller RegistryResources) Show(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	registry, err := controller.service.Find(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Registry",
			"resource_id",
			resourceID,
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	repositories := make([]services.RegistryRepositorySummary, 0)
	inventoryError := ""
	if registry.Managed {
		repositories, err = controller.service.Inventory(etx.Request().Context(), resourceID)
		if err != nil {
			slog.WarnContext(
				etx.Request().Context(),
				"failed to load managed Registry inventory",
				"resource_id",
				resourceID,
				"error",
				err,
			)
			inventoryError = "Repository inventory could not be loaded. Confirm that the managed registry is reachable and try again."
		}
	}
	return inertia.Page(etx, "Connections/Registries/Show", inertia.Props{
		"auth": authProps(etx), "registry": registry,
		"repositories": repositories, "inventoryError": inventoryError,
	})
}

func (controller RegistryResources) Create(etx *echo.Context) error {
	var payload struct {
		Name        string `json:"name"`
		Endpoint    string `json:"endpoint"`
		Username    string `json:"username"`
		AccessToken string `json:"accessToken"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	_, err := controller.service.CreateExternal(
		etx.Request().Context(),
		services.ExternalRegistryResourceInput{
			Name:        payload.Name,
			Endpoint:    payload.Endpoint,
			Username:    payload.Username,
			AccessToken: payload.AccessToken,
		},
	)
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			registries, listErr := controller.service.List(etx.Request().Context())
			if listErr == nil {
				return inertia.Page(
					etx,
					"Connections/Registries",
					inertia.Props{"auth": authProps(etx), "registries": registries},
					inertia.WithValidationErrors(validationErrors.ToMap()),
				)
			}
		}
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Registry Resource connected")
	return inertia.Redirect(etx, routes.RegistryResources.URL(), http.StatusSeeOther)
}

func (controller RegistryResources) Destroy(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err == nil {
		err = controller.service.ArchiveExternal(etx.Request().Context(), id)
	}
	if err != nil {
		return controller.redirectWithError(etx, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Registry Resource archived")
	return inertia.Redirect(etx, routes.RegistryResources.URL(), http.StatusSeeOther)
}

func (controller RegistryResources) DeleteImage(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	var payload struct {
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
	}
	if err := etx.Bind(&payload); err != nil ||
		strings.TrimSpace(payload.Repository) == "" || strings.TrimSpace(payload.Tag) == "" {
		return controller.redirectWithErrorTo(
			etx,
			routes.RegistryResourceShow.URL(resourceID),
			errors.New("image repository and tag are required"),
		)
	}
	if err := controller.service.DeleteImage(
		etx.Request().Context(),
		resourceID,
		payload.Repository,
		payload.Tag,
	); err != nil {
		return controller.redirectWithErrorTo(
			etx,
			routes.RegistryResourceShow.URL(resourceID),
			err,
		)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Image deleted")
	return inertia.Redirect(etx, routes.RegistryResourceShow.URL(resourceID), http.StatusSeeOther)
}

func (controller RegistryResources) redirectWithError(etx *echo.Context, err error) error {
	return controller.redirectWithErrorTo(etx, routes.RegistryResources.URL(), err)
}

func (controller RegistryResources) redirectWithErrorTo(
	etx *echo.Context,
	url string,
	err error,
) error {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Registry Resource operation failed"
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, url, http.StatusSeeOther)
}
