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
	db           storage.Pool
	health       *services.SystemHealth
	metric       services.MetricRollupService
	logs         *services.SystemLogs
	appTelemetry *services.SystemApplicationTelemetry
	access       *services.ResourcePrivateAccess
	credentials  *services.ResourceCredentials
	backups      *services.DatabaseBackups
	resources    *services.ResourceManagement
	caddy        services.CaddyRouteService
}

func NewSystem(
	db storage.Pool,
	health *services.SystemHealth,
	metric services.MetricRollupService,
	logs *services.SystemLogs,
	appTelemetry *services.SystemApplicationTelemetry,
	access *services.ResourcePrivateAccess,
	credentials *services.ResourceCredentials,
	backups *services.DatabaseBackups,
	resources *services.ResourceManagement,
	caddy services.CaddyRouteService,
) System {
	return System{
		db:           db,
		health:       health,
		metric:       metric,
		logs:         logs,
		appTelemetry: appTelemetry,
		access:       access,
		credentials:  credentials,
		backups:      backups,
		resources:    resources,
		caddy:        caddy,
	}
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
		{method: http.MethodGet, route: routes.SystemTelemetryTrace, handler: s.TelemetryTrace},
		{method: http.MethodGet, route: routes.SystemResources, handler: s.Resources},
		{method: http.MethodGet, route: routes.SystemResource, handler: s.Resource},
		{method: http.MethodGet, route: routes.SystemResourceBackups, handler: s.ResourceBackups},
		{
			method:  http.MethodGet,
			route:   routes.SystemResourceEndpoints,
			handler: s.ResourceEndpoints,
		},
		{
			method:  http.MethodGet,
			route:   routes.SystemResourceCredentials,
			handler: s.ResourceCredentials,
		},
		{method: http.MethodGet, route: routes.SystemResourceHealth, handler: s.ResourceHealth},
		{method: http.MethodGet, route: routes.SystemResourceAccess, handler: s.ResourceAccess},
		{
			method:  http.MethodPost,
			route:   routes.SystemResourceEndpointCreate,
			handler: s.CreateResourceEndpoint,
		},
		{
			method:  http.MethodDelete,
			route:   routes.SystemResourceEndpointDestroy,
			handler: s.DestroyResourceEndpoint,
		},
		{
			method: http.MethodPost,
			route:  routes.SystemResourceCredentialReveal,
			handler: middleware.IPRateLimiter(
				5,
				routes.SystemResources,
			)(
				s.RevealResourceCredential,
			),
		},
		{
			method:  http.MethodPost,
			route:   routes.SystemResourceWireGuardDeviceCreate,
			handler: s.CreateResourceWireGuardDevice,
		},
		{
			method:  http.MethodDelete,
			route:   routes.SystemResourceWireGuardDeviceDestroy,
			handler: s.DestroyResourceWireGuardDevice,
		},
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
	return inertia.Page(etx, "System/Overview", inertia.Props{
		"auth":      s.authProps(etx),
		"system":    overview,
		"resources": resources,
		"health":    s.health.Run(etx.Request().Context()),
		"backups":   backups,
	})
}

func (s System) Telemetry(etx *echo.Context) error {
	telemetryRange := services.ParseTelemetryRange(etx.QueryParam("range"))
	overview, err := models.Application.FindSystemOverview(
		etx.Request().Context(),
		s.db.Executor(),
	)
	if err != nil {
		return s.renderLoadError(etx, "telemetry", err)
	}
	metricData, err := s.metric.SystemTelemetry(
		etx.Request().Context(),
		overview.ServerID,
		telemetryRange,
	)
	if err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE extended telemetry",
			"error",
			err,
		)
	}
	applicationData, err := s.appTelemetry.Snapshot(etx.Request().Context(), telemetryRange)
	if err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE application telemetry",
			"error",
			err,
		)
	}
	collectorEndpoint, err := models.ResourceEndpoint.FindSystemEnvironmentEndpoint(
		etx.Request().Context(),
		s.db.Executor(),
		"opentelemetry",
	)
	if err != nil {
		return s.renderLoadError(etx, "telemetry Resource endpoint", err)
	}
	return inertia.Page(etx, "System/Telemetry", inertia.Props{
		"auth":                 s.authProps(etx),
		"system":               overview,
		"telemetry":            metricData,
		"applicationTelemetry": applicationData,
		"telemetryRange":       telemetryRange,
		"collectorEndpoint":    collectorEndpoint.URL(),
	})
}

func (s System) TelemetryLogs(etx *echo.Context) error {
	snapshot, err := s.logs.Snapshot(
		etx.Request().Context(),
		etx.QueryParam("after"),
		services.ParseTelemetryRange(etx.QueryParam("range")),
		etx.QueryParam("search"),
	)
	if errors.Is(err, services.ErrInvalidSystemLogCursor) ||
		errors.Is(err, services.ErrInvalidSystemLogSearch) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load DeployCrate CE logs",
			"error", err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "DeployCrate CE logs could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, snapshot)
}

func (s System) TelemetryTrace(etx *echo.Context) error {
	spans, err := s.appTelemetry.Trace(etx.Request().Context(), etx.Param("id"))
	if errors.Is(err, services.ErrInvalidTraceID) {
		return etx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to load OpenTelemetry trace",
			"error",
			err,
		)
		return etx.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": "Trace could not be loaded"},
		)
	}
	etx.Response().Header().Set("Cache-Control", "no-store")
	return etx.JSON(http.StatusOK, map[string]any{"spans": spans})
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
	return s.resourceSection(etx, "overview")
}

func (s System) ResourceBackups(etx *echo.Context) error {
	return s.resourceSection(etx, "backups")
}

func (s System) ResourceEndpoints(etx *echo.Context) error {
	return s.resourceSection(etx, "endpoints")
}

func (s System) ResourceCredentials(etx *echo.Context) error {
	return s.resourceSection(etx, "credentials")
}

func (s System) ResourceHealth(etx *echo.Context) error {
	return s.resourceSection(etx, "health")
}

func (s System) ResourceAccess(etx *echo.Context) error {
	return s.resourceSection(etx, "access")
}

func (s System) resourceSection(etx *echo.Context, section string) error {
	resourceID, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return s.renderResource(etx, resourceID, section, nil, nil)
}

func (s System) RevealResourceCredential(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	resourceID, resourceErr := uuid.Parse(etx.Param("resourceID"))
	credentialID, credentialErr := uuid.Parse(etx.Param("credentialID"))
	if errors.Join(resourceErr, credentialErr) != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "System Resource credential not found"},
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

	credential, err := s.credentials.RevealSystem(
		etx.Request().Context(), resourceID, credentialID,
		cookies.ExtractFromCookieApp(etx).UserID, payload.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCredentials):
			return etx.JSON(
				http.StatusUnprocessableEntity,
				map[string]string{"error": "Current password is incorrect"},
			)
		case errors.Is(err, services.ErrResourceCredentialUnavailable):
			return etx.JSON(
				http.StatusNotFound,
				map[string]string{"error": "System Resource credential not found"},
			)
		default:
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to reveal system Resource credential",
				"resource_id",
				resourceID,
				"credential_id",
				credentialID,
				"error",
				err,
			)
			return etx.JSON(
				http.StatusInternalServerError,
				map[string]string{"error": "System Resource credential could not be loaded"},
			)
		}
	}
	return etx.JSON(http.StatusOK, credential)
}

type systemResourceEndpointPayload struct {
	Name             string                                 `json:"name"`
	Role             string                                 `json:"role"`
	Address          string                                 `json:"address"`
	Port             int32                                  `json:"port"`
	Protocol         string                                 `json:"protocol"`
	TLSMode          string                                 `json:"tlsMode"`
	Settings         json.RawMessage                        `json:"settings"`
	PrivateNetworkID string                                 `json:"privateNetworkId"`
	Publication      services.ResourceCaddyPublicationInput `json:"publication"`
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
	if len(payload.Settings) == 0 {
		payload.Settings = json.RawMessage(`{}`)
	}
	endpointInput := services.ResourceEndpointInput{
		Name: payload.Name, Role: payload.Role, Address: payload.Address,
		Port: payload.Port, Protocol: payload.Protocol, TLSMode: payload.TLSMode,
		Settings: payload.Settings, PrivateNetworkID: networkID,
	}
	if err == nil {
		endpointInput, err = s.caddy.PrepareResourcePublication(
			etx.Request().Context(),
			resourceID,
			uuid.Nil,
			endpointInput,
			payload.Publication,
		)
	}
	if err == nil {
		var endpoint models.ResourceEndpointEntity
		err = func() error {
			tx, txErr := s.db.BeginTx(etx.Request().Context(), nil)
			if txErr != nil {
				return txErr
			}
			defer tx.Rollback()
			if endpoint, txErr = models.ResourceEndpoint.CreateForSystemResource(
				etx.Request().Context(),
				tx,
				models.CreateResourceEndpointData{
					Name:             endpointInput.Name,
					Role:             endpointInput.Role,
					Address:          endpointInput.Address,
					Port:             endpointInput.Port,
					Protocol:         endpointInput.Protocol,
					TlsMode:          endpointInput.TLSMode,
					Settings:         endpointInput.Settings,
					ResourceID:       resourceID,
					PrivateNetworkID: endpointInput.PrivateNetworkID,
				},
			); txErr != nil {
				return txErr
			}
			return tx.Commit()
		}()
		if err == nil && payload.Publication.Enabled {
			err = s.caddy.SyncResourcePublication(
				etx.Request().Context(),
				resourceID,
				endpoint.ID,
				payload.Publication,
			)
		}
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return s.renderResource(
				etx,
				resourceID,
				"endpoints",
				nil,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return s.redirectResourceError(etx, resourceID, "endpoints", err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource endpoint added")
	return inertia.Redirect(
		etx,
		routes.SystemResourceEndpoints.URL(resourceID),
		http.StatusSeeOther,
	)
}

func (s System) DestroyResourceEndpoint(etx *echo.Context) error {
	resourceID, resourceErr := uuid.Parse(etx.Param("resourceID"))
	endpointID, endpointErr := uuid.Parse(etx.Param("endpointID"))
	err := errors.Join(resourceErr, endpointErr)
	if err == nil {
		err = s.resources.ArchiveSystemEndpoint(etx.Request().Context(), resourceID, endpointID)
	}
	if err == nil {
		err = s.caddy.RemoveResourcePublication(etx.Request().Context(), resourceID, endpointID)
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return s.renderResource(
				etx,
				resourceID,
				"endpoints",
				nil,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return s.redirectResourceError(etx, resourceID, "endpoints", err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Resource endpoint removed")
	return inertia.Redirect(
		etx,
		routes.SystemResourceEndpoints.URL(resourceID),
		http.StatusSeeOther,
	)
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
		result, err = s.access.Enroll(
			etx.Request().Context(),
			resourceID,
			services.ResourcePrivateAccessEnrollment{
				DeviceID: deviceID,
				Name:     payload.Name,
				UserID:   cookies.ExtractFromCookieApp(etx).UserID,
			},
		)
	}
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return s.renderResource(
				etx,
				resourceID,
				"access",
				nil,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return s.redirectResourceError(etx, resourceID, "access", err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "WireGuard resource access granted")
	etx.Response().Header().Set("Cache-Control", "no-store")
	enrollment := inertia.Props{
		"deviceId": result.DeviceID.String(), "grantId": result.GrantID.String(),
		"clientConfiguration": result.ClientConfiguration,
	}
	return s.renderResource(etx, resourceID, "access", enrollment, nil)
}

func (s System) DestroyResourceWireGuardDevice(etx *echo.Context) error {
	resourceID, resourceErr := uuid.Parse(etx.Param("resourceID"))
	deviceID, deviceErr := uuid.Parse(etx.Param("deviceID"))
	err := errors.Join(resourceErr, deviceErr)
	if err == nil {
		err = s.access.RevokeGrant(etx.Request().Context(), resourceID, deviceID)
	}
	if err != nil {
		return s.redirectResourceError(etx, resourceID, "access", err)
	}
	_ = cookies.AddFlash(etx, cookies.FlashSuccess, "WireGuard resource access revoked")
	return inertia.Redirect(etx, routes.SystemResourceAccess.URL(resourceID), http.StatusSeeOther)
}

func (s System) renderResource(
	etx *echo.Context,
	resourceID uuid.UUID,
	section string,
	enrollment inertia.Props,
	option inertia.PageOption,
) error {
	if err := s.access.ObserveResource(etx.Request().Context(), resourceID); err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to observe WireGuard device handshakes",
			"resource_id",
			resourceID,
			"error",
			err,
		)
	}
	detail, err := models.Application.FindSystemResourceDetail(
		etx.Request().Context(),
		s.db.Executor(),
		resourceID,
		cookies.ExtractFromCookieApp(etx).UserID,
	)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return s.renderLoadError(etx, "resource", err)
	}
	options, err := s.resources.OptionsForEngine(etx.Request().Context(), detail.Engine)
	if err != nil {
		return s.renderLoadError(etx, "resource endpoint options", err)
	}
	publications, err := s.caddy.ResourcePublications(etx.Request().Context(), resourceID)
	if err != nil {
		return s.renderLoadError(etx, "resource publications", err)
	}
	backups := models.ResourceBackupCatalog{}
	if section == "backups" && detail.ResourceType == "database" {
		backups, err = s.backups.DetailsForResource(etx.Request().Context(), resourceID)
		if err != nil {
			return s.renderLoadError(etx, "resource backups", err)
		}
	}
	props := inertia.Props{
		"auth": s.authProps(etx), "resource": systemResourceDetailProps(detail),
		"section": section, "backups": resourceBackupProps(backups),
		"options": resourceOptionsProps(options), "publications": publications,
	}
	if enrollment != nil {
		props["enrollment"] = enrollment
	}
	if option != nil {
		return inertia.Page(etx, "System/Resources/Show", props, option)
	}
	return inertia.Page(etx, "System/Resources/Show", props)
}

func (s System) redirectResourceError(
	etx *echo.Context,
	resourceID uuid.UUID,
	section string,
	err error,
) error {
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
	return inertia.Redirect(etx, systemResourceSectionURL(resourceID, section), http.StatusSeeOther)
}

func systemResourceSectionURL(resourceID uuid.UUID, section string) string {
	switch section {
	case "backups":
		return routes.SystemResourceBackups.URL(resourceID)
	case "endpoints":
		return routes.SystemResourceEndpoints.URL(resourceID)
	case "credentials":
		return routes.SystemResourceCredentials.URL(resourceID)
	case "health":
		return routes.SystemResourceHealth.URL(resourceID)
	case "access":
		return routes.SystemResourceAccess.URL(resourceID)
	default:
		return routes.SystemResource.URL(resourceID)
	}
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
