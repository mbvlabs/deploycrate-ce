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

type ObjectStorage struct {
	backups *services.DatabaseBackups
}

func NewObjectStorage(backups *services.DatabaseBackups) ObjectStorage {
	return ObjectStorage{backups: backups}
}

func (controller ObjectStorage) RegisterRoutes(r *router.Router) error {
	return errors.Join(
		registerRoutes(r, []routeDefinition{
			{http.MethodGet, routes.ObjectStorage, controller.Index},
			{http.MethodPost, routes.ObjectStorageCreate, controller.Create},
			{http.MethodGet, routes.ObjectStorageShow, controller.Show},
		}, middleware.AdminOnly),
		registerRoutes(r, []routeDefinition{
			{http.MethodPost, routes.ObjectStorageRecovery, controller.Recovery},
		}, middleware.AdminOnly, middleware.IPRateLimiter(5, routes.ObjectStorage)),
	)
}

func (controller ObjectStorage) Recovery(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	destinationID, err := uuidPathParam(etx, "id")
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Object Storage destination not found"},
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
	recovery, err := controller.backups.RecoveryMaterial(
		etx.Request().
			Context(),
		destinationID,
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
		case errors.Is(err, models.ErrNotFound):
			return etx.JSON(
				http.StatusNotFound,
				map[string]string{"error": "Object Storage destination not found"},
			)
		default:
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to reveal Object Storage recovery material",
				"destination_id",
				destinationID,
				"error",
				err,
			)
			return etx.JSON(
				http.StatusInternalServerError,
				map[string]string{"error": "Recovery material could not be loaded"},
			)
		}
	}
	return etx.JSON(http.StatusOK, recovery)
}

func (controller ObjectStorage) Index(etx *echo.Context) error {
	destinations, err := controller.backups.Destinations(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	items := make([]inertia.Props, 0, len(destinations))
	for _, destination := range destinations {
		items = append(items, objectStorageDestinationProps(destination))
	}
	return inertia.Page(
		etx,
		"Connections/ObjectStorage",
		inertia.Props{"auth": authProps(etx), "destinations": items},
	)
}

func (controller ObjectStorage) Show(etx *echo.Context) error {
	id, err := uuidPathParam(etx, "id")
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	destination, err := controller.backups.Destination(etx.Request().Context(), id)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load Object Storage destination",
			"destination_id",
			id,
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Page(etx, "Connections/ObjectStorage/Show", inertia.Props{
		"auth": authProps(etx), "destination": objectStorageDestinationProps(destination),
	})
}

func (controller ObjectStorage) Create(etx *echo.Context) error {
	var payload struct {
		Name            string `json:"name"`
		Provider        string `json:"provider"`
		Endpoint        string `json:"endpoint"`
		Region          string `json:"region"`
		Bucket          string `json:"bucket"`
		Prefix          string `json:"prefix"`
		ForcePathStyle  bool   `json:"forcePathStyle"`
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
	}
	if err := etx.Bind(&payload); err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	_, err := controller.backups.CreateDestination(
		etx.Request().Context(),
		services.ObjectStorageDestinationInput{
			Name: payload.Name, Provider: payload.Provider, Endpoint: payload.Endpoint,
			Region: payload.Region, Bucket: payload.Bucket, Prefix: payload.Prefix,
			ForcePathStyle: payload.ForcePathStyle, AccessKeyID: payload.AccessKeyID,
			SecretAccessKey: payload.SecretAccessKey,
		},
	)
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			destinations, listErr := controller.backups.Destinations(etx.Request().Context())
			if listErr == nil {
				items := make([]inertia.Props, 0, len(destinations))
				for _, destination := range destinations {
					items = append(items, objectStorageDestinationProps(destination))
				}
				return inertia.Page(etx, "Connections/ObjectStorage", inertia.Props{
					"auth": authProps(etx), "destinations": items,
				}, inertia.WithValidationErrors(validationErrors.ToMap()))
			}
		}
		message := strings.TrimSpace(err.Error())
		if message == "" {
			message = "Object Storage destination could not be created"
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, message)
		return inertia.Redirect(etx, routes.ObjectStorage.URL(), http.StatusSeeOther)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Object Storage destination connected")
	return inertia.Redirect(etx, routes.ObjectStorage.URL(), http.StatusSeeOther)
}

func objectStorageDestinationProps(destination models.BackupDestinationSummary) inertia.Props {
	return inertia.Props{
		"id":             destination.ID,
		"name":           destination.Name,
		"provider":       destination.Provider,
		"endpoint":       destination.Endpoint,
		"region":         destination.Region,
		"bucket":         destination.Bucket,
		"prefix":         destination.Prefix,
		"forcePathStyle": destination.ForcePathStyle,
		"verifiedAt":     destination.VerifiedAt,
		"lastUsedAt":     destination.LastUsedAt,
	}
}
