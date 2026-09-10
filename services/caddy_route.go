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
	dns   *ResourceDNS
}

func NewCaddyRouteService(db storage.Pool, caddy CaddyClient) CaddyRouteService {
	return CaddyRouteService{db: db, caddy: caddy}
}

func NewCaddyRouteServiceWithDNS(
	db storage.Pool,
	caddy CaddyClient,
	dns *ResourceDNS,
) CaddyRouteService {
	return CaddyRouteService{db: db, caddy: caddy, dns: dns}
}

func StartResourceCaddyReconciler(
	lifecycle fx.Lifecycle,
	appCtx context.Context,
	service CaddyRouteService,
) {
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
	err := errors.Join(
		service.ReconcileManagedResourceEndpoints(ctx),
		service.ReconcileManagedCustomRoutes(ctx),
	)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, "failed to reconcile managed Caddy routes", "error", err)
	}
}

func (service CaddyRouteService) Reconcile(ctx context.Context, routeID uuid.UUID) (string, error) {
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
	environment, err := models.Environment.Find(
		ctx,
		service.db.Executor(),
		domain.EnvironmentID,
	)
	if err != nil {
		return "", fmt.Errorf("load Caddy route Environment: %w", err)
	}
	healthPath, err := models.CaddyRoute.DesiredHealthPath(
		ctx,
		service.db.Executor(),
		routeID,
		domain.EnvironmentID,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("load Environment route health path: %w", err)
	}

	rows, err := models.CaddyRouteBackend.DesiredForRoute(ctx, service.db.Executor(), routeID)
	if err != nil {
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

	applied := caddyclients.Route{
		ID: route.ExternalID, Domain: domain.Hostname, Backends: backends, HealthPath: healthPath,
	}
	if environment.AccessMode == models.EnvironmentAccessBasicAuth {
		applied.Authentication = &caddyclients.BasicAuthentication{
			Username: environment.BasicAuthUsername, PasswordHash: environment.BasicAuthPasswordHash,
		}
	}
	if environment.AccessMode == models.EnvironmentAccessPrivateNetwork {
		applied.PrivateNetworkOnly = true
	}
	if err := service.caddy.ApplyRoute(ctx, applied); err != nil {
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

	route, err := models.CaddyRoute.LockActive(ctx, tx, routeID)
	if err != nil {
		return fmt.Errorf("lock Caddy route for traffic switch: %w", err)
	}
	backends, err := models.CaddyRouteBackend.LockActiveForRoute(ctx, tx, routeID)
	if err != nil {
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

	_, err = models.CaddyRoute.LockActive(ctx, tx, routeID)
	if err != nil {
		return fmt.Errorf("lock Caddy route for backend addition: %w", err)
	}
	exists, err := models.CaddyRouteBackend.ActiveExists(ctx, tx, routeID, instanceID)
	if err != nil {
		return fmt.Errorf("inspect existing Caddy backend: %w", err)
	}
	if exists {
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

	_, err = models.CaddyRoute.LockActive(ctx, tx, routeID)
	if err != nil {
		return fmt.Errorf("lock Caddy route for backend removal: %w", err)
	}
	backend, err := models.CaddyRouteBackend.LockActive(ctx, tx, routeID, instanceID)
	if err != nil {
		return fmt.Errorf("load Caddy backend for removal: %w", err)
	}
	activeCount, err := models.CaddyRouteBackend.ActiveCount(ctx, tx, routeID)
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
	AccessMode          string                     `json:"accessMode"`
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
	AccessMode         string                     `json:"accessMode"`
	AppliedAt          string                     `json:"appliedAt"`
	ObservedAt         string                     `json:"observedAt"`
	Backends           []ManagedCaddyRouteBackend `json:"backends"`
	Configuration      json.RawMessage            `json:"configuration"`
	ConfigurationError string                     `json:"configurationError"`
	EnvironmentRoute   *ManagedCaddyRoute         `json:"environmentRoute,omitempty"`
	ResourceRoute      *ManagedResourceCaddyRoute `json:"resourceRoute,omitempty"`
	CustomRoute        *ManagedCustomCaddyRoute   `json:"customRoute,omitempty"`
	Options            ManagedCaddyRouteOptions   `json:"options"`
}

func (service CaddyRouteService) RouteDetail(
	ctx context.Context,
	externalID string,
) (ManagedCaddyRouteDetail, error) {
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
			AccessMode: route.AccessMode, AppliedAt: route.AppliedAt, ObservedAt: route.ObservedAt,
			Backends: route.Backends, EnvironmentRoute: &route, Options: snapshot.Options,
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
		custom, routeErr := models.CustomCaddyRoute.FindActiveByExternalID(
			ctx,
			service.db.Executor(),
			externalID,
		)
		if routeErr == nil {
			managed := managedCustomCaddyRoute(custom)
			detail = ManagedCaddyRouteDetail{
				ExternalID: managed.ExternalID, Kind: "custom", Hostname: managed.Hostname,
				State: managed.State, LastError: managed.LastError, Source: "Direct route",
				Target: managed.Origin, HealthPath: managed.HealthPath,
				AppliedAt: managed.AppliedAt, ObservedAt: managed.ObservedAt,
				Backends:    []ManagedCaddyRouteBackend{{Address: managed.Origin, Weight: 100}},
				CustomRoute: &managed,
			}
		} else if !errors.Is(routeErr, sql.ErrNoRows) {
			return ManagedCaddyRouteDetail{}, routeErr
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

func (service CaddyRouteService) ManagementSnapshot(
	ctx context.Context,
) (ManagedCaddyRouteSnapshot, error) {
	routeRows, err := models.CaddyRoute.ManagementRows(ctx, service.db.Executor())
	if err != nil {
		return ManagedCaddyRouteSnapshot{}, fmt.Errorf("load managed Caddy routes: %w", err)
	}

	routesByID := make(map[uuid.UUID]int, len(routeRows))
	routes := make([]ManagedCaddyRoute, 0, len(routeRows))
	for _, row := range routeRows {
		routesByID[row.ID] = len(routes)
		routes = append(routes, ManagedCaddyRoute{
			ID:                  row.ID.String(),
			ExternalID:          row.ExternalID,
			State:               row.State,
			Hostname:            row.Hostname,
			ApplicationName:     row.ApplicationName,
			EnvironmentName:     row.EnvironmentName,
			EnvironmentID:       row.EnvironmentID.String(),
			EnvironmentDomainID: row.EnvironmentDomainID.String(),
			EnvironmentTargetID: row.EnvironmentTargetID.String(),
			ReleaseID:           row.ReleaseID.String(),
			ReleaseLabel:        row.ReleaseLabel,
			ServerName:          row.ServerName,
			HealthPath:          row.HealthPath,
			AccessMode:          row.AccessMode,
			AppliedAt:           nullableTimeString(row.AppliedAt),
			ObservedAt:          nullableTimeString(row.ObservedAt),
			Backends:            make([]ManagedCaddyRouteBackend, 0),
		})
	}

	var backendRows []models.ManagedCaddyBackendRow
	if len(routes) != 0 {
		backendRows, err = models.CaddyRouteBackend.ManagementRows(ctx, service.db.Executor())
		if err != nil {
			return ManagedCaddyRouteSnapshot{}, fmt.Errorf(
				"load managed Caddy route backends: %w",
				err,
			)
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

func (service CaddyRouteService) managementOptions(
	ctx context.Context,
) (ManagedCaddyRouteOptions, error) {
	domainRows, err := models.EnvironmentDomain.CaddyManagementRows(ctx, service.db.Executor())
	if err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route domain options: %w", err)
	}

	targetRows, err := models.EnvironmentTarget.CaddyManagementRows(ctx, service.db.Executor())
	if err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route target options: %w", err)
	}

	releaseRows, err := models.Release.CaddyManagementRows(ctx, service.db.Executor())
	if err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route release options: %w", err)
	}

	instanceRows, err := models.Instance.CaddyManagementRows(ctx, service.db.Executor())
	if err != nil {
		return ManagedCaddyRouteOptions{}, fmt.Errorf("load Caddy route instance options: %w", err)
	}

	options := ManagedCaddyRouteOptions{
		Domains: make(
			[]ManagedCaddyDomainOption,
			0,
			len(domainRows),
		),
		Targets: make([]ManagedCaddyTargetOption, 0, len(targetRows)),
		Releases: make(
			[]ManagedCaddyReleaseOption,
			0,
			len(releaseRows),
		),
		Instances: make([]ManagedCaddyInstanceOption, 0, len(instanceRows)),
	}
	for _, row := range domainRows {
		options.Domains = append(
			options.Domains,
			ManagedCaddyDomainOption{
				ID:              row.ID.String(),
				Hostname:        row.Hostname,
				EnvironmentID:   row.EnvironmentID.String(),
				EnvironmentName: row.EnvironmentName,
				ApplicationName: row.ApplicationName,
			},
		)
	}
	for _, row := range targetRows {
		options.Targets = append(
			options.Targets,
			ManagedCaddyTargetOption{
				ID:            row.ID.String(),
				EnvironmentID: row.EnvironmentID.String(),
				ServerName:    row.ServerName,
			},
		)
	}
	for _, row := range releaseRows {
		options.Releases = append(
			options.Releases,
			ManagedCaddyReleaseOption{
				ID:                row.ID.String(),
				EnvironmentID:     row.EnvironmentID.String(),
				Label:             row.Label,
				ArtifactReference: row.ArtifactReference,
			},
		)
	}
	for _, row := range instanceRows {
		options.Instances = append(
			options.Instances,
			ManagedCaddyInstanceOption{
				ID:                  row.ID.String(),
				EnvironmentID:       row.EnvironmentID.String(),
				EnvironmentTargetID: row.EnvironmentTargetID.String(),
				ReleaseID:           row.ReleaseID.String(),
				ExternalID:          row.ExternalID,
				Slot:                row.Slot,
				State:               row.State,
				Address:             instanceHTTPAddress(row.Ports),
			},
		)
	}
	return options, nil
}

func (service CaddyRouteService) CreateManaged(
	ctx context.Context,
	input ManagedCaddyRouteInput,
) (uuid.UUID, error) {
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin Caddy route creation: %w", err)
	}
	defer tx.Rollback()
	if err := validateManagedCaddyRoute(ctx, tx, uuid.Nil, input); err != nil {
		return uuid.Nil, err
	}
	route, err := models.CaddyRoute.Create(
		ctx,
		tx,
		models.CreateCaddyRouteData{
			ExternalID:          input.ExternalID,
			State:               "pending",
			EnvironmentTargetID: input.EnvironmentTargetID,
			EnvironmentDomainID: input.EnvironmentDomainID,
			ReleaseID:           input.ReleaseID,
		},
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create Caddy route desired state: %w", err)
	}
	for _, backend := range input.Backends {
		if _, err := models.CaddyRouteBackend.Create(
			ctx,
			tx,
			models.CreateCaddyRouteBackendData{
				Weight:       backend.Weight,
				CaddyRouteID: route.ID,
				InstanceID:   backend.InstanceID,
			},
		); err != nil {
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

func (service CaddyRouteService) UpdateManaged(
	ctx context.Context,
	routeID uuid.UUID,
	input ManagedCaddyRouteInput,
) error {
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Caddy route update: %w", err)
	}
	defer tx.Rollback()
	current, err := models.CaddyRoute.LockActive(ctx, tx, routeID)
	if err != nil {
		return fmt.Errorf("load Caddy route for update: %w", err)
	}
	if err := validateManagedCaddyRoute(ctx, tx, routeID, input); err != nil {
		return err
	}
	if current.ExternalID != input.ExternalID {
		return errors.New("Caddy route identifiers cannot be changed after creation")
	}
	if _, err := models.CaddyRoute.Update(
		ctx,
		tx,
		models.UpdateCaddyRouteData{
			ID:                  routeID,
			ExternalID:          input.ExternalID,
			State:               "pending",
			AppliedAt:           current.AppliedAt,
			ObservedAt:          current.ObservedAt,
			EnvironmentTargetID: input.EnvironmentTargetID,
			EnvironmentDomainID: input.EnvironmentDomainID,
			ReleaseID:           input.ReleaseID,
		},
	); err != nil {
		return fmt.Errorf("update Caddy route desired state: %w", err)
	}
	now := time.Now().UTC()
	if err := models.CaddyRouteBackend.RetireActiveForRoute(ctx, tx, routeID, now); err != nil {
		return fmt.Errorf("retire previous Caddy route backends: %w", err)
	}
	for _, backend := range input.Backends {
		if _, err := models.CaddyRouteBackend.Create(
			ctx,
			tx,
			models.CreateCaddyRouteBackendData{
				Weight:       backend.Weight,
				CaddyRouteID: routeID,
				InstanceID:   backend.InstanceID,
			},
		); err != nil {
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
	route, err := models.CaddyRoute.LockNotRemoved(ctx, tx, routeID)
	if err != nil {
		return fmt.Errorf("load Caddy route for removal: %w", err)
	}
	now := route.RemovedAt
	if !now.Valid {
		now = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		if _, err := models.CaddyRoute.Update(
			ctx,
			tx,
			models.UpdateCaddyRouteData{
				ID:                  route.ID,
				ExternalID:          route.ExternalID,
				State:               "removal_pending",
				AppliedAt:           route.AppliedAt,
				ObservedAt:          route.ObservedAt,
				RemovedAt:           now,
				EnvironmentTargetID: route.EnvironmentTargetID,
				EnvironmentDomainID: route.EnvironmentDomainID,
				ReleaseID:           route.ReleaseID,
			},
		); err != nil {
			return fmt.Errorf("mark Caddy route for removal: %w", err)
		}
		if err := models.CaddyRouteBackend.RetireActiveForRoute(
			ctx,
			tx,
			routeID,
			now.Time,
		); err != nil {
			return fmt.Errorf("retire Caddy route backends: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Caddy route removal: %w", err)
	}
	if err := service.caddy.DeleteRoute(ctx, route.ExternalID); err != nil {
		return err
	}
	_, err = models.CaddyRoute.Update(
		ctx,
		service.db.Executor(),
		models.UpdateCaddyRouteData{
			ID:                  route.ID,
			ExternalID:          route.ExternalID,
			State:               "removed",
			AppliedAt:           route.AppliedAt,
			ObservedAt:          now,
			RemovedAt:           now,
			EnvironmentTargetID: route.EnvironmentTargetID,
			EnvironmentDomainID: route.EnvironmentDomainID,
			ReleaseID:           route.ReleaseID,
		},
	)
	return err
}

func validateManagedCaddyRoute(
	ctx context.Context,
	exec storage.Executor,
	routeID uuid.UUID,
	input ManagedCaddyRouteInput,
) error {
	if input.ExternalID == "" {
		return errors.New("Caddy route identifier is required")
	}
	if input.EnvironmentDomainID == uuid.Nil || input.EnvironmentTargetID == uuid.Nil ||
		input.ReleaseID == uuid.Nil {
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
	instanceIDs := make([]uuid.UUID, 0, len(seen))
	for instanceID := range seen {
		instanceIDs = append(instanceIDs, instanceID)
	}
	check, err := models.CaddyRoute.CheckManagedReferences(
		ctx, exec, routeID, input.ExternalID, input.EnvironmentDomainID,
		input.EnvironmentTargetID, input.ReleaseID, instanceIDs,
	)
	if err != nil {
		return fmt.Errorf("check Caddy route references: %w", err)
	}
	if !check.ExternalIDAvailable {
		return fmt.Errorf("Caddy route identifier %q is already in use", input.ExternalID)
	}
	customExternalIDs, err := exec.NewSelect().TableExpr("custom_caddy_routes").
		Where("external_id = ?", input.ExternalID).Where("removed_at IS NULL").Count(ctx)
	if err != nil {
		return fmt.Errorf("check custom Caddy route identifiers: %w", err)
	}
	if customExternalIDs > 0 {
		return fmt.Errorf("Caddy route identifier %q is already in use", input.ExternalID)
	}
	if !check.DomainAvailable {
		return errors.New("the selected domain already has an active Caddy route")
	}
	if !check.TargetMatches {
		return fmt.Errorf("Caddy target must belong to the selected domain's Environment")
	}
	if !check.ReleaseMatches {
		return fmt.Errorf("Caddy release must belong to the selected domain's Environment")
	}
	for instanceID := range seen {
		if !check.ActiveInstances[instanceID] {
			return fmt.Errorf(
				"Caddy backend instance %s must be active on the selected target",
				instanceID,
			)
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

func (service CaddyRouteService) VerifyPublic(
	ctx context.Context,
	domain, healthPath string,
) error {
	return service.caddy.VerifyPublic(ctx, domain, healthPath)
}

func (service CaddyRouteService) Delete(ctx context.Context, externalID string) error {
	return service.caddy.DeleteRoute(ctx, externalID)
}
