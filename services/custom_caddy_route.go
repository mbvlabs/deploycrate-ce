package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type CustomCaddyRouteInput struct {
	ExternalID     string `json:"externalId"`
	Hostname       string `json:"hostname"`
	OriginAddress  string `json:"originAddress"`
	OriginPort     int32  `json:"originPort"`
	OriginProtocol string `json:"originProtocol"`
	OriginTLSMode  string `json:"originTlsMode"`
	HealthPath     string `json:"healthPath"`
}

type ManagedCustomCaddyRoute struct {
	ID             string `json:"id"`
	ExternalID     string `json:"externalId"`
	Hostname       string `json:"hostname"`
	Origin         string `json:"origin"`
	OriginAddress  string `json:"originAddress"`
	OriginPort     int32  `json:"originPort"`
	OriginProtocol string `json:"originProtocol"`
	OriginTLSMode  string `json:"originTlsMode"`
	HealthPath     string `json:"healthPath"`
	State          string `json:"state"`
	LastError      string `json:"lastError"`
	AppliedAt      string `json:"appliedAt"`
	ObservedAt     string `json:"observedAt"`
}

func (service CaddyRouteService) CustomRouteSnapshot(ctx context.Context) ([]ManagedCustomCaddyRoute, error) {
	rows, err := models.CustomCaddyRoute.Active(ctx, service.db.Executor())
	if err != nil {
		return nil, err
	}
	routes := make([]ManagedCustomCaddyRoute, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, managedCustomCaddyRoute(row))
	}
	return routes, nil
}

func (service CaddyRouteService) ReconcileManagedCustomRoutes(ctx context.Context) error {
	rows, err := models.CustomCaddyRoute.Active(ctx, service.db.Executor())
	if err != nil {
		return err
	}
	var result error
	for _, row := range rows {
		if row.State == models.CaddyRouteRemovalPending {
			continue
		}
		result = errors.Join(result, service.reconcileCustom(ctx, row))
	}
	return result
}

func managedCustomCaddyRoute(row models.CustomCaddyRouteEntity) ManagedCustomCaddyRoute {
	protocol := resourceCaddyOriginProtocol(row.OriginProtocol, row.OriginTLSMode)
	return ManagedCustomCaddyRoute{
		ID: row.ID.String(), ExternalID: row.ExternalID, Hostname: row.Hostname,
		Origin:        fmt.Sprintf("%s://%s", protocol, net.JoinHostPort(row.OriginAddress, fmt.Sprint(row.OriginPort))),
		OriginAddress: row.OriginAddress, OriginPort: row.OriginPort,
		OriginProtocol: row.OriginProtocol, OriginTLSMode: row.OriginTLSMode,
		HealthPath: row.HealthPath, State: row.State.String(), LastError: row.LastError.String,
		AppliedAt: nullableTimeString(row.AppliedAt), ObservedAt: nullableTimeString(row.ObservedAt),
	}
}

func (service CaddyRouteService) CreateCustom(ctx context.Context, input CustomCaddyRouteInput) (uuid.UUID, error) {
	entity, err := service.validateCustomRoute(ctx, uuid.Nil, input)
	if err != nil {
		return uuid.Nil, err
	}
	created, err := models.CustomCaddyRoute.Create(ctx, service.db.Executor(), models.SaveCustomCaddyRouteData{
		ExternalID: entity.ExternalID, Hostname: entity.Hostname,
		OriginAddress: entity.OriginAddress, OriginPort: entity.OriginPort,
		OriginProtocol: entity.OriginProtocol, OriginTLSMode: entity.OriginTLSMode,
		HealthPath: entity.HealthPath, State: models.CaddyRoutePending,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, service.reconcileCustom(ctx, created)
}

func (service CaddyRouteService) UpdateCustom(ctx context.Context, id uuid.UUID, input CustomCaddyRouteInput) error {
	current, err := models.CustomCaddyRoute.Find(ctx, service.db.Executor(), id)
	if err != nil || current.RemovedAt.Valid {
		return models.ErrNotFound
	}
	entity, err := service.validateCustomRoute(ctx, id, input)
	if err != nil {
		return err
	}
	if current.ExternalID != entity.ExternalID {
		return errors.New("Caddy route identifiers cannot be changed after creation")
	}
	updated, err := models.CustomCaddyRoute.Update(ctx, service.db.Executor(), models.SaveCustomCaddyRouteData{
		ID: id, ExternalID: entity.ExternalID, Hostname: entity.Hostname,
		OriginAddress: entity.OriginAddress, OriginPort: entity.OriginPort,
		OriginProtocol: entity.OriginProtocol, OriginTLSMode: entity.OriginTLSMode,
		HealthPath: entity.HealthPath, State: models.CaddyRoutePending,
		AppliedAt: current.AppliedAt, ObservedAt: current.ObservedAt,
	})
	if err != nil {
		return err
	}
	return service.reconcileCustom(ctx, updated)
}

func (service CaddyRouteService) validateCustomRoute(ctx context.Context, id uuid.UUID, input CustomCaddyRouteInput) (models.CustomCaddyRouteEntity, error) {
	entity := models.CustomCaddyRouteEntity{
		ID: id, ExternalID: input.ExternalID, Hostname: input.Hostname,
		OriginAddress: input.OriginAddress, OriginPort: input.OriginPort,
		OriginProtocol: input.OriginProtocol, OriginTLSMode: input.OriginTLSMode,
		HealthPath: input.HealthPath, State: models.CaddyRoutePending,
	}
	if err := entity.Validate(); err != nil {
		return models.CustomCaddyRouteEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	externalCount, err := service.db.Executor().NewSelect().TableExpr("custom_caddy_routes").
		Where("external_id = ?", entity.ExternalID).Where("removed_at IS NULL").Where("id <> ?", id).Count(ctx)
	if err != nil {
		return models.CustomCaddyRouteEntity{}, err
	}
	existingRouteCount, err := service.db.Executor().NewSelect().TableExpr("caddy_routes").
		Where("external_id = ?", entity.ExternalID).Where("removed_at IS NULL").Count(ctx)
	if err != nil {
		return models.CustomCaddyRouteEntity{}, err
	}
	if externalCount+existingRouteCount > 0 {
		return models.CustomCaddyRouteEntity{}, domainError("externalId", "unique", "Caddy route identifier is already in use")
	}
	resourceEndpoints, err := models.ResourceEndpoint.ManagedActive(ctx, service.db.Executor(), uuid.Nil)
	if err != nil {
		return models.CustomCaddyRouteEntity{}, err
	}
	for _, endpoint := range resourceEndpoints {
		if resourceCaddyRouteID(endpoint.ID) == entity.ExternalID {
			return models.CustomCaddyRouteEntity{}, domainError("externalId", "unique", "Caddy route identifier is already in use")
		}
	}
	hostnameConflicts, err := models.ResourceEndpoint.CaddyHostnameConflicts(ctx, service.db.Executor(), entity.Hostname, uuid.Nil)
	if err != nil {
		return models.CustomCaddyRouteEntity{}, err
	}
	customHostnameCount, err := service.db.Executor().NewSelect().TableExpr("custom_caddy_routes").
		Where("lower(hostname) = ?", entity.Hostname).Where("removed_at IS NULL").Where("id <> ?", id).Count(ctx)
	if err != nil {
		return models.CustomCaddyRouteEntity{}, err
	}
	if hostnameConflicts+customHostnameCount > 0 {
		return models.CustomCaddyRouteEntity{}, domainError("hostname", "unique", "an active Caddy route already uses this hostname")
	}
	return entity, nil
}

func (service CaddyRouteService) reconcileCustom(ctx context.Context, route models.CustomCaddyRouteEntity) error {
	protocol := resourceCaddyOriginProtocol(route.OriginProtocol, route.OriginTLSMode)
	err := service.caddy.ApplyRoute(ctx, caddyclients.Route{
		ID: route.ExternalID, Domain: route.Hostname,
		Backends:   []caddyclients.Backend{{Dial: net.JoinHostPort(route.OriginAddress, fmt.Sprint(route.OriginPort)), Weight: 100}},
		HealthPath: route.HealthPath, DisableActiveHealthChecks: route.HealthPath == "", UpstreamTLS: protocol == "https",
	})
	now := time.Now().UTC()
	state, lastError, appliedAt := models.CaddyRouteApplied, sql.NullString{}, sql.NullTime{Time: now, Valid: true}
	if err != nil {
		state, lastError, appliedAt = models.CaddyRouteFailed, sql.NullString{String: err.Error(), Valid: true}, route.AppliedAt
	}
	_, updateErr := models.CustomCaddyRoute.Update(ctx, service.db.Executor(), models.SaveCustomCaddyRouteData{
		ID: route.ID, ExternalID: route.ExternalID, Hostname: route.Hostname,
		OriginAddress: route.OriginAddress, OriginPort: route.OriginPort,
		OriginProtocol: route.OriginProtocol, OriginTLSMode: route.OriginTLSMode,
		HealthPath: route.HealthPath, State: state, LastError: lastError,
		AppliedAt: appliedAt, ObservedAt: sql.NullTime{Time: now, Valid: true},
	})
	return errors.Join(err, updateErr)
}

func (service CaddyRouteService) DestroyCustom(ctx context.Context, id uuid.UUID) error {
	route, err := models.CustomCaddyRoute.Find(ctx, service.db.Executor(), id)
	if err != nil || route.RemovedAt.Valid {
		return models.ErrNotFound
	}
	if err := service.caddy.DeleteRoute(ctx, route.ExternalID); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = models.CustomCaddyRoute.Update(ctx, service.db.Executor(), models.SaveCustomCaddyRouteData{
		ID: route.ID, ExternalID: route.ExternalID, Hostname: route.Hostname,
		OriginAddress: route.OriginAddress, OriginPort: route.OriginPort,
		OriginProtocol: route.OriginProtocol, OriginTLSMode: route.OriginTLSMode,
		HealthPath: route.HealthPath, State: models.CaddyRouteRemoved,
		AppliedAt: route.AppliedAt, ObservedAt: sql.NullTime{Time: now, Valid: true}, RemovedAt: sql.NullTime{Time: now, Valid: true},
	})
	return err
}
