package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type System struct {
	db     storage.Pool
	health *services.SystemHealth
	metric services.MetricRollupService
	logs   *services.SystemLogs
	access *services.ResourcePrivateAccess
}

func NewSystem(
	db storage.Pool,
	health *services.SystemHealth,
	metric services.MetricRollupService,
	logs *services.SystemLogs,
	access *services.ResourcePrivateAccess,
) System {
	return System{db: db, health: health, metric: metric, logs: logs, access: access}
}

func (s System) RegisterRoutes(r *router.Router) error {
	routesToRegister := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{method: http.MethodGet, route: routes.SystemOverview, handler: s.Overview},
		{method: http.MethodGet, route: routes.SystemTelemetry, handler: s.Telemetry},
		{method: http.MethodGet, route: routes.SystemTelemetryLogs, handler: s.TelemetryLogs},
		{method: http.MethodGet, route: routes.SystemDeployments, handler: s.Deployments},
		{method: http.MethodGet, route: routes.SystemResources, handler: s.Resources},
		{method: http.MethodGet, route: routes.SystemResource, handler: s.Resource},
		{method: http.MethodPost, route: routes.SystemResourceEndpointCreate, handler: s.CreateResourceEndpoint},
		{method: http.MethodPost, route: routes.SystemResourceWireGuardDeviceCreate, handler: s.CreateResourceWireGuardDevice},
		{method: http.MethodDelete, route: routes.SystemResourceWireGuardDeviceDestroy, handler: s.DestroyResourceWireGuardDevice},
	}

	errList := make([]error, 0, len(routesToRegister))
	for _, registered := range routesToRegister {
		middlewares := []echo.MiddlewareFunc{middleware.AuthOnly}
		if registered.method != http.MethodGet {
			middlewares = []echo.MiddlewareFunc{middleware.AdminOnly}
		}
		_, err := r.AddRoute(echo.Route{
			Method:      registered.method,
			Path:        registered.route.Path(),
			Name:        registered.route.Name(),
			Handler:     registered.handler,
			Middlewares: middlewares,
		})
		if err != nil {
			errList = append(errList, err)
		}
	}

	return errors.Join(errList...)
}

func (s System) Overview(etx *echo.Context) error {
	overview, err := models.Application.FindSystemOverview(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "overview", err)
	}
	resources, err := models.Application.FindSystemResources(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "resources", err)
	}
	backups, err := models.Backup.FindSystemHealth(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "backup health", err)
	}
	telemetry, err := s.metric.HostTelemetry(etx.Request().Context(), overview.ServerID)
	if err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE system telemetry",
			"error",
			err,
		)
	}

	return inertia.Page(etx, "System/Overview", inertia.Props{
		"auth":      s.authProps(etx),
		"system":    overview,
		"resources": resources,
		"health":    s.health.Run(etx.Request().Context()),
		"backups":   backups,
		"telemetry": telemetry,
	})
}

func (s System) Telemetry(etx *echo.Context) error {
	overview, err := models.Application.FindSystemOverview(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "telemetry", err)
	}
	metricData, err := s.metric.SystemTelemetry(etx.Request().Context(), overview.ServerID)
	if err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE extended telemetry",
			"error",
			err,
		)
	}
	return inertia.Page(etx, "System/Telemetry", inertia.Props{
		"auth":      s.authProps(etx),
		"system":    overview,
		"telemetry": metricData,
	})
}

func (s System) TelemetryLogs(etx *echo.Context) error {
	snapshot, err := s.logs.Snapshot(etx.Request().Context(), etx.QueryParam("after"))
	if errors.Is(err, services.ErrInvalidSystemLogCursor) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE logs",
			"error", err,
		)
		return etx.JSON(http.StatusInternalServerError, map[string]string{"error": "DeployCrate CE logs could not be loaded"})
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (s System) Deployments(etx *echo.Context) error {
	deployments, err := models.Application.FindSystemDeployments(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "deployments", err)
	}

	return inertia.Page(etx, "System/Deployments", inertia.Props{
		"auth":        s.authProps(etx),
		"deployments": deployments,
	})
}

func (s System) Resources(etx *echo.Context) error {
	resources, err := models.Application.FindSystemResourceIndex(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "resources", err)
	}

	return inertia.Page(etx, "System/Resources/Index", inertia.Props{
		"auth":      s.authProps(etx),
		"resources": systemResourceIndexProps(resources),
	})
}

func (s System) Resource(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return s.renderResource(etx, resourceID, nil, nil)
}

type systemResourceEndpointPayload struct {
	Name             string          `json:"name"`
	Role             string          `json:"role"`
	Address          string          `json:"address"`
	Port             int32           `json:"port"`
	Protocol         string          `json:"protocol"`
	TLSMode          string          `json:"tlsMode"`
	Settings         json.RawMessage `json:"settings"`
	PrivateNetworkID string          `json:"privateNetworkId"`
}

func (s System) CreateResourceEndpoint(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload systemResourceEndpointPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var networkID *uuid.UUID
	if err == nil {
		networkID, err = optionalUUID(payload.PrivateNetworkID)
	}
	if err == nil {
		if len(payload.Settings) == 0 {
			payload.Settings = json.RawMessage(`{}`)
		}
		err = func() error {
			tx, txErr := s.db.BeginTx(etx.Request().Context(), nil)
			if txErr != nil {
				return txErr
			}
			defer tx.Rollback()
			if _, txErr = models.ResourceEndpoint.CreateForSystemResource(
				etx.Request().Context(),
				tx,
				models.CreateResourceEndpointData{
					Name: payload.Name, Role: payload.Role, Address: payload.Address,
					Port: payload.Port, Protocol: payload.Protocol, TlsMode: payload.TLSMode,
					Settings: payload.Settings, ResourceID: resourceID, PrivateNetworkID: networkID,
				},
			); txErr != nil {
				return txErr
			}
			return tx.Commit()
		}()
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return s.renderResource(etx, resourceID, nil, inertia.WithValidationErrors(validationErrors.ToMap()))
		}
		return s.redirectResourceError(etx, resourceID, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource endpoint added")
	return inertia.Redirect(etx, routes.SystemResource.URL(resourceID), http.StatusSeeOther)
}

type systemResourceWireGuardPayload struct {
	Name     string `json:"name"`
	DeviceID string `json:"deviceId"`
}

func (s System) CreateResourceWireGuardDevice(etx *echo.Context) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	var payload systemResourceWireGuardPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	deviceID := uuid.Nil
	if err == nil && strings.TrimSpace(payload.DeviceID) != "" {
		deviceID, err = uuid.Parse(payload.DeviceID)
	}
	var result services.ResourcePrivateAccessResult
	if err == nil {
		result, err = s.access.Enroll(etx.Request().Context(), resourceID, services.ResourcePrivateAccessEnrollment{
			DeviceID: deviceID, Name: payload.Name, UserID: cookies.ExtractFromCookieApp(etx).UserID,
		})
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return s.renderResource(etx, resourceID, nil, inertia.WithValidationErrors(validationErrors.ToMap()))
		}
		return s.redirectResourceError(etx, resourceID, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "WireGuard resource access granted")
	etx.Response().Header().Set("Cache-Control", "no-store")
	enrollment := inertia.Props{
		"deviceId": result.DeviceID.String(), "grantId": result.GrantID.String(),
		"clientConfiguration": result.ClientConfiguration,
	}
	return s.renderResource(etx, resourceID, enrollment, nil)
}

func (s System) DestroyResourceWireGuardDevice(etx *echo.Context) error {
	resourceID, resourceErr := uuid.Parse(etx.Param("resourceID"))
	deviceID, deviceErr := uuid.Parse(etx.Param("deviceID"))
	err := errors.Join(resourceErr, deviceErr)
	if err == nil {
		err = s.access.RevokeGrant(etx.Request().Context(), resourceID, deviceID)
	}
	if err != nil {
		return s.redirectResourceError(etx, resourceID, err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "WireGuard resource access revoked")
	return inertia.Redirect(etx, routes.SystemResource.URL(resourceID), http.StatusSeeOther)
}

func (s System) renderResource(etx *echo.Context, resourceID uuid.UUID, enrollment inertia.Props, option inertia.PageOption) error {
	if err := s.access.ObserveResource(etx.Request().Context(), resourceID); err != nil {
		slog.WarnContext(etx.Request().Context(), "failed to observe WireGuard device handshakes", "resource_id", resourceID, "error", err)
	}
	detail, err := models.Application.FindSystemResourceDetail(etx.Request().Context(), s.db.Executor(), resourceID, cookies.ExtractFromCookieApp(etx).UserID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return s.renderLoadError(etx, "resource", err)
	}
	props := inertia.Props{"auth": s.authProps(etx), "resource": systemResourceDetailProps(detail)}
	if enrollment != nil {
		props["enrollment"] = enrollment
	}
	if option != nil {
		return inertia.Page(etx, "System/Resources/Show", props, option)
	}
	return inertia.Page(etx, "System/Resources/Show", props)
}

func (s System) redirectResourceError(etx *echo.Context, resourceID uuid.UUID, err error) error {
	message := "Resource operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	if resourceID == uuid.Nil {
		return inertia.Redirect(etx, routes.SystemResources.URL(), http.StatusSeeOther)
	}
	return inertia.Redirect(etx, routes.SystemResource.URL(resourceID), http.StatusSeeOther)
}

func optionalUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s System) authProps(etx *echo.Context) inertia.Props {
	return inertia.Props{"email": cookies.ExtractFromCookieApp(etx).Email}
}

func (s System) renderLoadError(etx *echo.Context, page string, err error) error {
	slog.ErrorContext(
		etx.Request().Context(),
		"failed to load DeployCrate CE system page",
		"page",
		page,
		"error",
		err,
	)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}
