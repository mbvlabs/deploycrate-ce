package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"go.uber.org/fx"
)

type CaddyClient interface {
	ApplyRoute(context.Context, caddyclients.Route) error
	DeleteRoute(context.Context, string) error
	RouteConfig(context.Context, string) (json.RawMessage, error)
	VerifyRoute(context.Context, string) error
	VerifyPublic(context.Context, string, string) error
}

type CaddyRouteService struct {
	db    storage.Pool
	caddy CaddyClient
}

func NewCaddyRouteService(db storage.Pool, caddy CaddyClient) CaddyRouteService {
	return CaddyRouteService{db: db, caddy: caddy}
}

func StartResourceCaddyReconciler(lifecycle fx.Lifecycle, appCtx context.Context, service CaddyRouteService) {
	lifecycle.Append(fx.Hook{OnStart: func(context.Context) error {
		go service.runResourceRouteReconciler(appCtx)
		return nil
	}})
}

func (service CaddyRouteService) runResourceRouteReconciler(ctx context.Context) {
	service.reconcileResourceRouteCandidates(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.reconcileResourceRouteCandidates(ctx)
		}
	}
}

func (service CaddyRouteService) reconcileResourceRouteCandidates(ctx context.Context) {
	if err := service.ReconcileManagedResourceEndpoints(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, "failed to reconcile Resource Caddy endpoints", "error", err)
	}
}

func (service CaddyRouteService) Reconcile(ctx context.Context, routeID uuid.UUID) (string, error) {
	type backendRow struct {
		Weight int32           `bun:"weight"`
		Ports  json.RawMessage `bun:"ports"`
	}

	route, err := models.CaddyRoute.Find(ctx, service.db.Executor(), routeID)
	if err != nil {
		return "", fmt.Errorf("load Caddy route desired state: %w", err)
	}
	if route.RemovedAt.Valid {
		return "", errors.New("cannot reconcile a removed Caddy route")
	}
	domain, err := models.EnvironmentDomain.Find(
		ctx,
		service.db.Executor(),
		route.EnvironmentDomainID,
	)
	if err != nil {
		return "", fmt.Errorf("load Caddy route domain: %w", err)
	}
	if domain.ArchivedAt.Valid {
		return "", errors.New("cannot reconcile a Caddy route for an archived domain")
	}
	var healthPath string
	err = service.db.Executor().NewSelect().TableExpr("caddy_route_backends AS backend").
		ColumnExpr("COALESCE(process.value ->> 'health_path', runtime.settings ->> 'health_path', '')").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.process_kind = 'web'").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id").
		Join("LEFT JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(deployment.process_configuration) = 'array' THEN deployment.process_configuration ELSE '[]'::jsonb END) AS process(value) ON process.value ->> 'kind' = 'web'").
		Join("LEFT JOIN LATERAL (SELECT settings FROM runtime_configurations WHERE environment_id = ? ORDER BY created_at DESC LIMIT 1) AS runtime ON TRUE", domain.EnvironmentID).
		Where("backend.caddy_route_id = ?", routeID).Where("backend.removed_at IS NULL").OrderExpr("backend.weight DESC, backend.id DESC").Limit(1).
		Scan(ctx, &healthPath)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("load Environment route health path: %w", err)
	}

	var rows []backendRow
	if err := service.db.Executor().NewSelect().
		TableExpr("caddy_route_backends AS backend").
		ColumnExpr("backend.weight AS weight").
		ColumnExpr("instance.ports AS ports").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id").
		Where("backend.caddy_route_id = ?", routeID).
		Where("backend.removed_at IS NULL").
		Where("instance.removed_at IS NULL").
		OrderExpr("backend.id ASC").
		Scan(ctx, &rows); err != nil {
		return "", fmt.Errorf("load Caddy route backends: %w", err)
	}
	if len(rows) == 0 {
		return "", errors.New("Caddy route has no active backends")
	}

	backends := make([]caddyclients.Backend, 0, len(rows))
	for _, row := range rows {
		var ports struct {
			Host string `json:"host"`
			HTTP int    `json:"http"`
		}
		if err := json.Unmarshal(row.Ports, &ports); err != nil {
			return "", fmt.Errorf("decode Caddy backend ports: %w", err)
		}
		if ports.HTTP < 1 || ports.HTTP > 65535 {
			return "", fmt.Errorf("Caddy backend has invalid HTTP port %d", ports.HTTP)
		}
		if ports.Host == "" {
			ports.Host = "127.0.0.1"
		}
		if !validWorkloadBackendAddress(ports.Host) {
			return "", fmt.Errorf("Caddy backend has invalid workload address %q", ports.Host)
		}
		backends = append(backends, caddyclients.Backend{
			Dial: net.JoinHostPort(ports.Host, fmt.Sprint(ports.HTTP)), Weight: int(row.Weight),
		})
	}

	if err := service.caddy.ApplyRoute(ctx, caddyclients.Route{
		ID: route.ExternalID, Domain: domain.Hostname, Backends: backends, HealthPath: healthPath,
	}); err != nil {
		return "", fmt.Errorf("apply Caddy route desired state: %w", err)
	}
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	if _, err := models.CaddyRoute.Update(ctx, service.db.Executor(), models.UpdateCaddyRouteData{
		ID:                  route.ID,
		ExternalID:          route.ExternalID,
		State:               "applied",
		AppliedAt:           now,
		ObservedAt:          now,
		RemovedAt:           route.RemovedAt,
		EnvironmentTargetID: route.EnvironmentTargetID,
		EnvironmentDomainID: route.EnvironmentDomainID,
		ReleaseID:           route.ReleaseID,
	}); err != nil {
		return "", fmt.Errorf("mark Caddy route applied: %w", err)
	}
	return route.ExternalID, nil
}

func (service CaddyRouteService) ReconcileRegistry(
	ctx context.Context,
	externalID, domain, origin, username, passwordHash string,
) error {
	if err := service.caddy.ApplyRoute(ctx, caddyclients.Route{
		ID: externalID, Domain: domain, HealthPath: "/v2/",
		Backends: []caddyclients.Backend{{Dial: origin, Weight: 100}},
		Authentication: &caddyclients.BasicAuthentication{
			Username: username, PasswordHash: passwordHash,
		},
	}); err != nil {
		return fmt.Errorf("apply managed registry Caddy route: %w", err)
	}
	return nil
}

func (service CaddyRouteService) SwitchTraffic(
	ctx context.Context,
	routeID uuid.UUID,
	releaseID uuid.UUID,
	weights map[uuid.UUID]int32,
) error {
	if releaseID == uuid.Nil {
		return errors.New("active release is required for a Caddy traffic switch")
	}
	if len(weights) == 0 {
		return errors.New("Caddy route weights are required")
	}
	total := int32(0)
	for _, weight := range weights {
		if weight < 0 || weight > 100 {
			return fmt.Errorf("Caddy route weight %d must be between 0 and 100", weight)
		}
		total += weight
	}
	if total != 100 {
		return fmt.Errorf("Caddy route weights must total 100, got %d", total)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy weight transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var route models.CaddyRouteEntity
	if err := tx.NewSelect().Model(&route).
		Where("id = ?", routeID).
		Where("removed_at IS NULL").
		For("UPDATE").
		Scan(ctx); err != nil {
		return fmt.Errorf("lock Caddy route for traffic switch: %w", err)
	}
	var backends []models.CaddyRouteBackendEntity
	if err := tx.NewSelect().Model(&backends).
		Where("caddy_route_id = ?", routeID).
		Where("removed_at IS NULL").
		For("UPDATE").
		Scan(ctx); err != nil {
		return fmt.Errorf("lock Caddy route backends: %w", err)
	}
	if len(backends) != len(weights) {
		return errors.New("weights must be supplied for every active Caddy backend")
	}
	for _, backend := range backends {
		weight, ok := weights[backend.InstanceID]
		if !ok {
			return fmt.Errorf("weight is missing for Caddy backend instance %s", backend.InstanceID)
		}
		if _, err := models.CaddyRouteBackend.Update(ctx, tx, models.UpdateCaddyRouteBackendData{
			ID: backend.ID, Weight: weight, RemovedAt: backend.RemovedAt,
			CaddyRouteID: backend.CaddyRouteID, InstanceID: backend.InstanceID,
		}); err != nil {
			return fmt.Errorf("update Caddy backend weight: %w", err)
		}
	}

	if _, err := models.CaddyRoute.Update(ctx, tx, models.UpdateCaddyRouteData{
		ID:                  route.ID,
		ExternalID:          route.ExternalID,
		State:               "pending",
		AppliedAt:           route.AppliedAt,
		ObservedAt:          route.ObservedAt,
		RemovedAt:           route.RemovedAt,
		EnvironmentTargetID: route.EnvironmentTargetID,
		EnvironmentDomainID: route.EnvironmentDomainID,
		ReleaseID:           releaseID,
	}); err != nil {
		return fmt.Errorf("mark Caddy route pending: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy weight transaction: %w", err)
	}
	committed = true
	_, err = service.Reconcile(ctx, routeID)
	return err
}

func (service CaddyRouteService) AddBackend(
	ctx context.Context,
	routeID, instanceID uuid.UUID,
	weight int32,
) error {
	if weight < 0 || weight > 100 {
		return fmt.Errorf("Caddy route weight %d must be between 0 and 100", weight)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy backend transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var route models.CaddyRouteEntity
	if err := tx.NewSelect().Model(&route).
		Where("id = ?", routeID).
		Where("removed_at IS NULL").
		For("UPDATE").
		Scan(ctx); err != nil {
		return fmt.Errorf("lock Caddy route for backend addition: %w", err)
	}
	count, err := tx.NewSelect().Model((*models.CaddyRouteBackendEntity)(nil)).
		Where("caddy_route_id = ?", routeID).
		Where("instance_id = ?", instanceID).
		Where("removed_at IS NULL").
		Count(ctx)
	if err != nil {
		return fmt.Errorf("inspect existing Caddy backend: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("instance %s is already an active Caddy backend", instanceID)
	}
	if _, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{
		Weight: weight, CaddyRouteID: routeID, InstanceID: instanceID,
	}); err != nil {
		return fmt.Errorf("create Caddy route backend: %w", err)
	}
	if err := markCaddyRoutePending(ctx, tx, routeID, uuid.Nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy backend transaction: %w", err)
	}
	committed = true
	_, err = service.Reconcile(ctx, routeID)
	return err
}

func (service CaddyRouteService) RemoveBackend(
	ctx context.Context,
	routeID, instanceID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy backend removal transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var route models.CaddyRouteEntity
	if err := tx.NewSelect().Model(&route).
		Where("id = ?", routeID).
		Where("removed_at IS NULL").
		For("UPDATE").
		Scan(ctx); err != nil {
		return fmt.Errorf("lock Caddy route for backend removal: %w", err)
	}
	var backend models.CaddyRouteBackendEntity
	if err := tx.NewSelect().Model(&backend).
		Where("caddy_route_id = ?", routeID).
		Where("instance_id = ?", instanceID).
		Where("removed_at IS NULL").
		For("UPDATE").
		Scan(ctx); err != nil {
		return fmt.Errorf("load Caddy backend for removal: %w", err)
	}
	activeCount, err := tx.NewSelect().Model((*models.CaddyRouteBackendEntity)(nil)).
		Where("caddy_route_id = ?", routeID).
		Where("removed_at IS NULL").
		Count(ctx)
	if err != nil {
		return fmt.Errorf("count active Caddy backends: %w", err)
	}
	if activeCount < 2 {
		return errors.New("cannot remove the last active Caddy backend")
	}
	backend.RemovedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	if _, err := models.CaddyRouteBackend.Update(ctx, tx, models.UpdateCaddyRouteBackendData{
		ID: backend.ID, Weight: backend.Weight, RemovedAt: backend.RemovedAt,
		CaddyRouteID: backend.CaddyRouteID, InstanceID: backend.InstanceID,
	}); err != nil {
		return fmt.Errorf("remove Caddy route backend: %w", err)
	}
	if err := markCaddyRoutePending(ctx, tx, routeID, uuid.Nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy backend removal: %w", err)
	}
	committed = true
	_, err = service.Reconcile(ctx, routeID)
	return err
}

func markCaddyRoutePending(
	ctx context.Context,
	exec storage.Executor,
	routeID, releaseID uuid.UUID,
) error {
	route, err := models.CaddyRoute.Find(ctx, exec, routeID)
	if err != nil {
		return fmt.Errorf("load Caddy route for pending update: %w", err)
	}
	if releaseID == uuid.Nil {
		releaseID = route.ReleaseID
	}
	if _, err := models.CaddyRoute.Update(ctx, exec, models.UpdateCaddyRouteData{
		ID:                  route.ID,
		ExternalID:          route.ExternalID,
		State:               "pending",
		AppliedAt:           route.AppliedAt,
		ObservedAt:          route.ObservedAt,
		RemovedAt:           route.RemovedAt,
		EnvironmentTargetID: route.EnvironmentTargetID,
		EnvironmentDomainID: route.EnvironmentDomainID,
		ReleaseID:           releaseID,
	}); err != nil {
		return fmt.Errorf("mark Caddy route pending: %w", err)
	}
	return nil
}

type ManagedCaddyRouteBackendInput struct {
	InstanceID uuid.UUID `json:"instanceId"`
	Weight     int32     `json:"weight"`
}

type ManagedCaddyRouteInput struct {
	ExternalID          string                          `json:"externalId"`
	EnvironmentDomainID uuid.UUID                       `json:"environmentDomainId"`
	EnvironmentTargetID uuid.UUID                       `json:"environmentTargetId"`
	ReleaseID           uuid.UUID                       `json:"releaseId"`
	Backends            []ManagedCaddyRouteBackendInput `json:"backends"`
}

type ManagedCaddyRouteBackend struct {
	InstanceID string `json:"instanceId"`
	ExternalID string `json:"externalId"`
	Slot       string `json:"slot"`
	State      string `json:"state"`
	Address    string `json:"address"`
	Weight     int32  `json:"weight"`
}

type ManagedCaddyRoute struct {
	ID                  string                     `json:"id"`
	ExternalID          string                     `json:"externalId"`
	State               string                     `json:"state"`
	Hostname            string                     `json:"hostname"`
	ApplicationName     string                     `json:"applicationName"`
	EnvironmentName     string                     `json:"environmentName"`
	EnvironmentID       string                     `json:"environmentId"`
	EnvironmentDomainID string                     `json:"environmentDomainId"`
	EnvironmentTargetID string                     `json:"environmentTargetId"`
	ReleaseID           string                     `json:"releaseId"`
	ReleaseLabel        string                     `json:"releaseLabel"`
	ServerName          string                     `json:"serverName"`
	HealthPath          string                     `json:"healthPath"`
	AppliedAt           string                     `json:"appliedAt"`
	ObservedAt          string                     `json:"observedAt"`
	Backends            []ManagedCaddyRouteBackend `json:"backends"`
}

type ManagedCaddyDomainOption struct {
	ID              string `json:"id"`
	Hostname        string `json:"hostname"`
	EnvironmentID   string `json:"environmentId"`
	EnvironmentName string `json:"environmentName"`
	ApplicationName string `json:"applicationName"`
}

type ManagedCaddyTargetOption struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	ServerName    string `json:"serverName"`
}

type ManagedCaddyReleaseOption struct {
	ID                string `json:"id"`
	EnvironmentID     string `json:"environmentId"`
	Label             string `json:"label"`
	ArtifactReference string `json:"artifactReference"`
}

type ManagedCaddyInstanceOption struct {
	ID                  string `json:"id"`
	EnvironmentID       string `json:"environmentId"`
	EnvironmentTargetID string `json:"environmentTargetId"`
	ReleaseID           string `json:"releaseId"`
	ExternalID          string `json:"externalId"`
	Slot                string `json:"slot"`
	State               string `json:"state"`
	Address             string `json:"address"`
}

type ManagedCaddyRouteOptions struct {
	Domains   []ManagedCaddyDomainOption   `json:"domains"`
	Targets   []ManagedCaddyTargetOption   `json:"targets"`
	Releases  []ManagedCaddyReleaseOption  `json:"releases"`
	Instances []ManagedCaddyInstanceOption `json:"instances"`
}

type ManagedCaddyRouteSnapshot struct {
	Routes  []ManagedCaddyRoute      `json:"routes"`
	Options ManagedCaddyRouteOptions `json:"options"`
}

type ManagedCaddyRouteDetail struct {
	ExternalID         string                     `json:"externalId"`
	Kind               string                     `json:"kind"`
	Hostname           string                     `json:"hostname"`
	State              string                     `json:"state"`
	LastError          string                     `json:"lastError"`
	Source             string                     `json:"source"`
	Target             string                     `json:"target"`
	HealthPath         string                     `json:"healthPath"`
	AppliedAt          string                     `json:"appliedAt"`
	ObservedAt         string                     `json:"observedAt"`
	Backends           []ManagedCaddyRouteBackend `json:"backends"`
	Configuration      json.RawMessage            `json:"configuration"`
	ConfigurationError string                     `json:"configurationError"`
	EnvironmentRoute   *ManagedCaddyRoute         `json:"environmentRoute,omitempty"`
	ResourceRoute      *ManagedResourceCaddyRoute `json:"resourceRoute,omitempty"`
	Options            ManagedCaddyRouteOptions   `json:"options"`
}

func (service CaddyRouteService) RouteDetail(ctx context.Context, externalID string) (ManagedCaddyRouteDetail, error) {
	externalID = strings.TrimSpace(externalID)
	snapshot, err := service.ManagementSnapshot(ctx)
	if err != nil {
		return ManagedCaddyRouteDetail{}, err
	}
	var detail ManagedCaddyRouteDetail
	for _, route := range snapshot.Routes {
		if route.ExternalID != externalID {
			continue
		}
		detail = ManagedCaddyRouteDetail{
			ExternalID: route.ExternalID, Kind: "environment", Hostname: route.Hostname,
			State: route.State, Source: route.ApplicationName + " / " + route.EnvironmentName,
			Target: route.ServerName + " / " + route.ReleaseLabel, HealthPath: route.HealthPath,
			AppliedAt: route.AppliedAt, ObservedAt: route.ObservedAt, Backends: route.Backends,
			EnvironmentRoute: &route, Options: snapshot.Options,
		}
		break
	}
	if detail.ExternalID == "" {
		resourceRoutes, routeErr := service.ResourceRouteSnapshot(ctx)
		if routeErr != nil {
			return ManagedCaddyRouteDetail{}, routeErr
		}
		for _, route := range resourceRoutes {
			if route.ExternalID != externalID {
				continue
			}
			detail = ManagedCaddyRouteDetail{
				ExternalID: route.ExternalID, Kind: "resource", Hostname: route.Hostname,
				State: route.State, LastError: route.LastError,
				Source: route.ResourceName + " / " + route.EndpointName, Target: route.Origin,
				AppliedAt: route.AppliedAt, ObservedAt: route.ObservedAt,
				Backends:      []ManagedCaddyRouteBackend{{Address: route.Origin, Weight: 100}},
				ResourceRoute: &route,
			}
			break
		}
	}
	if detail.ExternalID == "" {
		return ManagedCaddyRouteDetail{}, models.ErrNotFound
	}
	detail.Configuration, err = service.caddy.RouteConfig(ctx, externalID)
	if err != nil {
		detail.ConfigurationError = err.Error()
	}
	return detail, nil
}

func (service CaddyRouteService) ManagementSnapshot(ctx context.Context) (ManagedCaddyRouteSnapshot, error) {
	type routeRow struct {
		ID                  uuid.UUID    `bun:"id"`
		ExternalID          string       `bun:"external_id"`
		State               string       `bun:"state"`
		Hostname            string       `bun:"hostname"`
		ApplicationName     string       `bun:"application_name"`
		EnvironmentName     string       `bun:"environment_name"`
		EnvironmentID       uuid.UUID    `bun:"environment_id"`
		EnvironmentDomainID uuid.UUID    `bun:"environment_domain_id"`
		EnvironmentTargetID uuid.UUID    `bun:"environment_target_id"`
		ReleaseID           uuid.UUID    `bun:"release_id"`
		ReleaseLabel        string       `bun:"release_label"`
		ServerName          string       `bun:"server_name"`
		HealthPath          string       `bun:"health_path"`
		AppliedAt           sql.NullTime `bun:"applied_at"`
		ObservedAt          sql.NullTime `bun:"observed_at"`
	}
	var routeRows []routeRow
	if err := service.db.Executor().NewSelect().
		TableExpr("caddy_routes AS route").
		ColumnExpr("route.id, route.external_id, route.state, route.environment_domain_id, route.environment_target_id, route.release_id, route.applied_at, route.observed_at").
		ColumnExpr("domain.hostname, environment.id AS environment_id, environment.name AS environment_name, application.name AS application_name").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("COALESCE(NULLIF(release.version, ''), release.artifact_reference) AS release_label").
		ColumnExpr("COALESCE(process.health_path, configuration.settings ->> 'health_path', '/api/health') AS health_path").
		Join("JOIN environment_domains AS domain ON domain.id = route.environment_domain_id").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").
		Join("JOIN servers AS server ON server.id = target.server_id").
		Join("JOIN releases AS release ON release.id = route.release_id").
		Join("LEFT JOIN LATERAL (SELECT process.value ->> 'health_path' AS health_path FROM caddy_route_backends backend JOIN instances instance ON instance.id = backend.instance_id AND instance.process_kind = 'web' JOIN deployments deployment ON deployment.id = instance.deployment_id LEFT JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(deployment.process_configuration) = 'array' THEN deployment.process_configuration ELSE '[]'::jsonb END) AS process(value) ON process.value ->> 'kind' = 'web' WHERE backend.caddy_route_id = route.id AND backend.removed_at IS NULL ORDER BY backend.weight DESC, backend.id DESC LIMIT 1) AS process ON TRUE").
		Join("LEFT JOIN LATERAL (SELECT settings FROM runtime_configurations WHERE environment_id = environment.id ORDER BY created_at DESC LIMIT 1) AS configuration ON TRUE").
		Where("route.removed_at IS NULL OR route.state = 'removal_pending'").
		OrderExpr("domain.hostname ASC").
		Scan(ctx, &routeRows); err != nil {
		return ManagedCaddyRouteSnapshot{}, fmt.Errorf("load managed Caddy routes: %w", err)
	}

	routesByID := make(map[uuid.UUID]int, len(routeRows))
	routes := make([]ManagedCaddyRoute, 0, len(routeRows))
	for _, row := range routeRows {
		routesByID[row.ID] = len(routes)
		routes = append(routes, ManagedCaddyRoute{
			ID: row.ID.String(), ExternalID: row.ExternalID, State: row.State,
			Hostname: row.Hostname, ApplicationName: row.ApplicationName,
			EnvironmentName: row.EnvironmentName, EnvironmentID: row.EnvironmentID.String(),
			EnvironmentDomainID: row.EnvironmentDomainID.String(), EnvironmentTargetID: row.EnvironmentTargetID.String(),
			ReleaseID: row.ReleaseID.String(), ReleaseLabel: row.ReleaseLabel, ServerName: row.ServerName,
			HealthPath: row.HealthPath, AppliedAt: nullableTimeString(row.AppliedAt), ObservedAt: nullableTimeString(row.ObservedAt),
			Backends: make([]ManagedCaddyRouteBackend, 0),
		})
	}

	type backendRow struct {
		CaddyRouteID uuid.UUID       `bun:"caddy_route_id"`
		InstanceID   uuid.UUID       `bun:"instance_id"`
		ExternalID   string          `bun:"external_id"`
		Slot         string          `bun:"slot"`
		State        string          `bun:"state"`
		Ports        json.RawMessage `bun:"ports"`
		Weight       int32           `bun:"weight"`
	}
	var backendRows []backendRow
	if len(routes) != 0 {
		if err := service.db.Executor().NewSelect().
			TableExpr("caddy_route_backends AS backend").
			ColumnExpr("backend.caddy_route_id, backend.instance_id, backend.weight").
			ColumnExpr("instance.external_id, instance.slot, instance.state, instance.ports").
			Join("JOIN instances AS instance ON instance.id = backend.instance_id").
			Where("backend.removed_at IS NULL").
			Where("backend.caddy_route_id IN (?)", service.db.Executor().NewSelect().TableExpr("caddy_routes").Column("id").Where("removed_at IS NULL OR state = 'removal_pending'")).
			OrderExpr("backend.id ASC").
			Scan(ctx, &backendRows); err != nil {
			return ManagedCaddyRouteSnapshot{}, fmt.Errorf("load managed Caddy route backends: %w", err)
		}
	}
	for _, row := range backendRows {
		index, ok := routesByID[row.CaddyRouteID]
		if !ok {
			continue
		}
		routes[index].Backends = append(routes[index].Backends, ManagedCaddyRouteBackend{
			InstanceID: row.InstanceID.String(), ExternalID: row.ExternalID, Slot: row.Slot,
			State: row.State, Address: instanceHTTPAddress(row.Ports), Weight: row.Weight,
		})
	}

	options, err := service.managementOptions(ctx)
	if err != nil {
		return ManagedCaddyRouteSnapshot{}, err
	}
	return ManagedCaddyRouteSnapshot{Routes: routes, Options: options}, nil
}

func (service CaddyRouteService) managementOptions(ctx context.Context) (ManagedCaddyRouteOptions, error) {
	type domainRow struct {
		ID              uuid.UUID `bun:"id"`
		Hostname        string    `bun:"hostname"`
		EnvironmentID   uuid.UUID `bun:"environment_id"`
		EnvironmentName string    `bun:"environment_name"`
		ApplicationName string    `bun:"application_name"`
	}
	var domainRows []domainRow
	if err := service.db.Executor().NewSelect().TableExpr("environment_domains AS domain").
		ColumnExpr("domain.id, domain.hostname, domain.environment_id, environment.name AS environment_name, application.name AS application_name").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("domain.archived_at IS NULL").Where("environment.archived_at IS NULL").Where("application.archived_at IS NULL").
		OrderExpr("application.name, environment.name, domain.hostname").Scan(ctx, &domainRows); err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route domain options: %w", err)
	}

	type targetRow struct {
		ID            uuid.UUID `bun:"id"`
		EnvironmentID uuid.UUID `bun:"environment_id"`
		ServerName    string    `bun:"server_name"`
	}
	var targetRows []targetRow
	if err := service.db.Executor().NewSelect().TableExpr("environment_targets AS target").
		ColumnExpr("target.id, target.environment_id, server.name AS server_name").
		Join("JOIN servers AS server ON server.id = target.server_id").
		Where("target.detached_at IS NULL").Where("server.archived_at IS NULL").OrderExpr("server.name").Scan(ctx, &targetRows); err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route target options: %w", err)
	}

	type releaseRow struct {
		ID                uuid.UUID `bun:"id"`
		EnvironmentID     uuid.UUID `bun:"environment_id"`
		Label             string    `bun:"label"`
		ArtifactReference string    `bun:"artifact_reference"`
	}
	var releaseRows []releaseRow
	if err := service.db.Executor().NewSelect().TableExpr("releases AS release").
		ColumnExpr("release.id, release.environment_id, COALESCE(NULLIF(release.version, ''), release.artifact_reference) AS label, release.artifact_reference").
		OrderExpr("release.created_at DESC").Scan(ctx, &releaseRows); err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route release options: %w", err)
	}

	type instanceRow struct {
		ID                  uuid.UUID       `bun:"id"`
		EnvironmentID       uuid.UUID       `bun:"environment_id"`
		EnvironmentTargetID uuid.UUID       `bun:"environment_target_id"`
		ReleaseID           uuid.UUID       `bun:"release_id"`
		ExternalID          string          `bun:"external_id"`
		Slot                string          `bun:"slot"`
		State               string          `bun:"state"`
		Ports               json.RawMessage `bun:"ports"`
	}
	var instanceRows []instanceRow
	if err := service.db.Executor().NewSelect().TableExpr("instances AS instance").
		ColumnExpr("instance.id, target.environment_id, instance.environment_target_id, instance.release_id, instance.external_id, instance.slot, instance.state, instance.ports").
		Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").
		Where("instance.removed_at IS NULL").Where("instance.state IN ('candidate', 'running', 'serving')").Where("target.detached_at IS NULL").
		OrderExpr("instance.created_at DESC").Scan(ctx, &instanceRows); err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route instance options: %w", err)
	}

	options := ManagedCaddyRouteOptions{
		Domains: make([]ManagedCaddyDomainOption, 0, len(domainRows)), Targets: make([]ManagedCaddyTargetOption, 0, len(targetRows)),
		Releases: make([]ManagedCaddyReleaseOption, 0, len(releaseRows)), Instances: make([]ManagedCaddyInstanceOption, 0, len(instanceRows)),
	}
	for _, row := range domainRows {
		options.Domains = append(options.Domains, ManagedCaddyDomainOption{ID: row.ID.String(), Hostname: row.Hostname, EnvironmentID: row.EnvironmentID.String(), EnvironmentName: row.EnvironmentName, ApplicationName: row.ApplicationName})
	}
	for _, row := range targetRows {
		options.Targets = append(options.Targets, ManagedCaddyTargetOption{ID: row.ID.String(), EnvironmentID: row.EnvironmentID.String(), ServerName: row.ServerName})
	}
	for _, row := range releaseRows {
		options.Releases = append(options.Releases, ManagedCaddyReleaseOption{ID: row.ID.String(), EnvironmentID: row.EnvironmentID.String(), Label: row.Label, ArtifactReference: row.ArtifactReference})
	}
	for _, row := range instanceRows {
		options.Instances = append(options.Instances, ManagedCaddyInstanceOption{ID: row.ID.String(), EnvironmentID: row.EnvironmentID.String(), EnvironmentTargetID: row.EnvironmentTargetID.String(), ReleaseID: row.ReleaseID.String(), ExternalID: row.ExternalID, Slot: row.Slot, State: row.State, Address: instanceHTTPAddress(row.Ports)})
	}
	return options, nil
}

func (service CaddyRouteService) CreateManaged(ctx context.Context, input ManagedCaddyRouteInput) (uuid.UUID, error) {
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin Caddy route creation: %w", err)
	}
	defer tx.Rollback()
	if err := validateManagedCaddyRoute(ctx, tx, uuid.Nil, input); err != nil {
		return uuid.Nil, err
	}
	route, err := models.CaddyRoute.Create(ctx, tx, models.CreateCaddyRouteData{ExternalID: input.ExternalID, State: "pending", EnvironmentTargetID: input.EnvironmentTargetID, EnvironmentDomainID: input.EnvironmentDomainID, ReleaseID: input.ReleaseID})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create Caddy route desired state: %w", err)
	}
	for _, backend := range input.Backends {
		if _, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{Weight: backend.Weight, CaddyRouteID: route.ID, InstanceID: backend.InstanceID}); err != nil {
			return uuid.Nil, fmt.Errorf("create Caddy route backend: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit Caddy route creation: %w", err)
	}
	if _, err := service.Reconcile(ctx, route.ID); err != nil {
		return route.ID, err
	}
	return route.ID, nil
}

func (service CaddyRouteService) UpdateManaged(ctx context.Context, routeID uuid.UUID, input ManagedCaddyRouteInput) error {
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy route update: %w", err)
	}
	defer tx.Rollback()
	var current models.CaddyRouteEntity
	if err := tx.NewSelect().Model(&current).Where("id = ?", routeID).Where("removed_at IS NULL").For("UPDATE").Scan(ctx); err != nil {
		return fmt.Errorf("load Caddy route for update: %w", err)
	}
	if err := validateManagedCaddyRoute(ctx, tx, routeID, input); err != nil {
		return err
	}
	if current.ExternalID != input.ExternalID {
		return errors.New("Caddy route identifiers cannot be changed after creation")
	}
	if _, err := models.CaddyRoute.Update(ctx, tx, models.UpdateCaddyRouteData{ID: routeID, ExternalID: input.ExternalID, State: "pending", AppliedAt: current.AppliedAt, ObservedAt: current.ObservedAt, EnvironmentTargetID: input.EnvironmentTargetID, EnvironmentDomainID: input.EnvironmentDomainID, ReleaseID: input.ReleaseID}); err != nil {
		return fmt.Errorf("update Caddy route desired state: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("caddy_route_backends").Set("removed_at = ?", now).Set("updated_at = ?", now).Where("caddy_route_id = ?", routeID).Where("removed_at IS NULL").Exec(ctx); err != nil {
		return fmt.Errorf("retire previous Caddy route backends: %w", err)
	}
	for _, backend := range input.Backends {
		if _, err := models.CaddyRouteBackend.Create(ctx, tx, models.CreateCaddyRouteBackendData{Weight: backend.Weight, CaddyRouteID: routeID, InstanceID: backend.InstanceID}); err != nil {
			return fmt.Errorf("create updated Caddy route backend: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy route update: %w", err)
	}
	if _, err := service.Reconcile(ctx, routeID); err != nil {
		return err
	}
	return nil
}

func (service CaddyRouteService) DestroyManaged(ctx context.Context, routeID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy route removal: %w", err)
	}
	defer tx.Rollback()
	var route models.CaddyRouteEntity
	if err := tx.NewSelect().Model(&route).Where("id = ?", routeID).Where("state <> 'removed'").For("UPDATE").Scan(ctx); err != nil {
		return fmt.Errorf("load Caddy route for removal: %w", err)
	}
	now := route.RemovedAt
	if !now.Valid {
		now = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		if _, err := models.CaddyRoute.Update(ctx, tx, models.UpdateCaddyRouteData{ID: route.ID, ExternalID: route.ExternalID, State: "removal_pending", AppliedAt: route.AppliedAt, ObservedAt: route.ObservedAt, RemovedAt: now, EnvironmentTargetID: route.EnvironmentTargetID, EnvironmentDomainID: route.EnvironmentDomainID, ReleaseID: route.ReleaseID}); err != nil {
			return fmt.Errorf("mark Caddy route for removal: %w", err)
		}
		if _, err := tx.NewUpdate().TableExpr("caddy_route_backends").Set("removed_at = ?", now.Time).Set("updated_at = ?", now.Time).Where("caddy_route_id = ?", routeID).Where("removed_at IS NULL").Exec(ctx); err != nil {
			return fmt.Errorf("retire Caddy route backends: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy route removal: %w", err)
	}
	if err := service.caddy.DeleteRoute(ctx, route.ExternalID); err != nil {
		return err
	}
	_, err = models.CaddyRoute.Update(ctx, service.db.Executor(), models.UpdateCaddyRouteData{ID: route.ID, ExternalID: route.ExternalID, State: "removed", AppliedAt: route.AppliedAt, ObservedAt: now, RemovedAt: now, EnvironmentTargetID: route.EnvironmentTargetID, EnvironmentDomainID: route.EnvironmentDomainID, ReleaseID: route.ReleaseID})
	return err
}

func validateManagedCaddyRoute(ctx context.Context, exec storage.Executor, routeID uuid.UUID, input ManagedCaddyRouteInput) error {
	if input.ExternalID == "" {
		return errors.New("Caddy route identifier is required")
	}
	if input.EnvironmentDomainID == uuid.Nil || input.EnvironmentTargetID == uuid.Nil || input.ReleaseID == uuid.Nil {
		return errors.New("domain, target, and release are required")
	}
	if len(input.Backends) == 0 {
		return errors.New("at least one Caddy backend is required")
	}
	seen := make(map[uuid.UUID]struct{}, len(input.Backends))
	total := int32(0)
	for _, backend := range input.Backends {
		if backend.InstanceID == uuid.Nil {
			return errors.New("Caddy backend instance is required")
		}
		if _, ok := seen[backend.InstanceID]; ok {
			return fmt.Errorf("instance %s is selected more than once", backend.InstanceID)
		}
		seen[backend.InstanceID] = struct{}{}
		if backend.Weight < 0 || backend.Weight > 100 {
			return fmt.Errorf("Caddy backend weight %d must be between 0 and 100", backend.Weight)
		}
		total += backend.Weight
	}
	if total != 100 {
		return fmt.Errorf("Caddy backend weights must total 100, got %d", total)
	}
	count, err := exec.NewSelect().TableExpr("caddy_routes").Where("external_id = ?", input.ExternalID).Where("removed_at IS NULL").Where("id <> ?", routeID).Count(ctx)
	if err != nil {
		return fmt.Errorf("check Caddy route identifier: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("Caddy route identifier %q is already in use", input.ExternalID)
	}
	count, err = exec.NewSelect().TableExpr("caddy_routes").Where("environment_domain_id = ?", input.EnvironmentDomainID).Where("removed_at IS NULL").Where("id <> ?", routeID).Count(ctx)
	if err != nil {
		return fmt.Errorf("check Caddy route domain: %w", err)
	}
	if count != 0 {
		return errors.New("the selected domain already has an active Caddy route")
	}
	var environmentID uuid.UUID
	if err := exec.NewSelect().TableExpr("environment_domains").Column("environment_id").Where("id = ?", input.EnvironmentDomainID).Where("archived_at IS NULL").Scan(ctx, &environmentID); err != nil {
		return fmt.Errorf("load Caddy route environment: %w", err)
	}
	count, err = exec.NewSelect().TableExpr("environment_targets").Where("id = ?", input.EnvironmentTargetID).Where("environment_id = ?", environmentID).Where("detached_at IS NULL").Count(ctx)
	if err != nil || count != 1 {
		return fmt.Errorf("Caddy target must belong to the selected domain's Environment")
	}
	count, err = exec.NewSelect().TableExpr("releases").Where("id = ?", input.ReleaseID).Where("environment_id = ?", environmentID).Count(ctx)
	if err != nil || count != 1 {
		return fmt.Errorf("Caddy release must belong to the selected domain's Environment")
	}
	for instanceID := range seen {
		count, err = exec.NewSelect().TableExpr("instances AS instance").Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").Where("instance.id = ?", instanceID).Where("instance.environment_target_id = ?", input.EnvironmentTargetID).Where("target.environment_id = ?", environmentID).Where("instance.removed_at IS NULL").Where("instance.state IN ('candidate', 'running', 'serving')").Count(ctx)
		if err != nil || count != 1 {
			return fmt.Errorf("Caddy backend instance %s must be active on the selected target", instanceID)
		}
	}
	return nil
}

func instanceHTTPAddress(ports json.RawMessage) string {
	var value struct {
		Host string `json:"host"`
		HTTP int    `json:"http"`
	}
	if json.Unmarshal(ports, &value) != nil || value.HTTP == 0 {
		return "unavailable"
	}
	if value.Host == "" {
		value.Host = "127.0.0.1"
	}
	if !validWorkloadBackendAddress(value.Host) {
		return "unavailable"
	}
	return net.JoinHostPort(value.Host, fmt.Sprint(value.HTTP))
}

func validWorkloadBackendAddress(value string) bool {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !address.Is4() {
		return false
	}
	return address.IsLoopback() || netip.MustParsePrefix(WireGuardMeshCIDR).Contains(address)
}

func nullableTimeString(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339)
}

func (service CaddyRouteService) Verify(ctx context.Context, externalID string) error {
	return service.caddy.VerifyRoute(ctx, externalID)
}

func (service CaddyRouteService) VerifyPublic(ctx context.Context, domain, healthPath string) error {
	return service.caddy.VerifyPublic(ctx, domain, healthPath)
}

func (service CaddyRouteService) Delete(ctx context.Context, externalID string) error {
	return service.caddy.DeleteRoute(ctx, externalID)
}
