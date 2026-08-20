package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
)

type CaddyRouteDesiredBackend struct {
	Weight int32           `bun:"weight"`
	Ports  json.RawMessage `bun:"ports"`
}

func (caddyRoute) DesiredHealthPath(ctx context.Context, db storage.Executor, routeID, environmentID uuid.UUID) (string, error) {
	var healthPath string
	err := db.NewSelect().TableExpr("caddy_route_backends AS backend").
		ColumnExpr("COALESCE(process.value ->> 'health_path', runtime.settings ->> 'health_path', '')").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.process_kind = 'web'").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id").
		Join("LEFT JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(deployment.process_configuration) = 'array' THEN deployment.process_configuration ELSE '[]'::jsonb END) AS process(value) ON process.value ->> 'kind' = 'web'").
		Join("LEFT JOIN LATERAL (SELECT settings FROM runtime_configurations WHERE environment_id = ? ORDER BY created_at DESC LIMIT 1) AS runtime ON TRUE", environmentID).
		Where("backend.caddy_route_id = ?", routeID).Where("backend.removed_at IS NULL").
		OrderExpr("backend.weight DESC, backend.id DESC").Limit(1).Scan(ctx, &healthPath)
	return healthPath, err
}

func (caddyRouteBackend) DesiredForRoute(ctx context.Context, db storage.Executor, routeID uuid.UUID) ([]CaddyRouteDesiredBackend, error) {
	rows := make([]CaddyRouteDesiredBackend, 0)
	err := db.NewSelect().TableExpr("caddy_route_backends AS backend").
		ColumnExpr("backend.weight AS weight, instance.ports AS ports").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id").
		Where("backend.caddy_route_id = ?", routeID).Where("backend.removed_at IS NULL").
		Where("instance.removed_at IS NULL").OrderExpr("backend.id ASC").Scan(ctx, &rows)
	return rows, err
}

func (caddyRoute) LockActive(ctx context.Context, db storage.Executor, id uuid.UUID) (CaddyRouteEntity, error) {
	var route CaddyRouteEntity
	err := db.NewSelect().Model(&route).Where("id = ?", id).Where("removed_at IS NULL").For("UPDATE").Scan(ctx)
	return route, err
}

func (caddyRoute) LockNotRemoved(ctx context.Context, db storage.Executor, id uuid.UUID) (CaddyRouteEntity, error) {
	var route CaddyRouteEntity
	err := db.NewSelect().Model(&route).Where("id = ?", id).Where("state <> 'removed'").For("UPDATE").Scan(ctx)
	return route, err
}

func (caddyRouteBackend) LockActiveForRoute(ctx context.Context, db storage.Executor, routeID uuid.UUID) ([]CaddyRouteBackendEntity, error) {
	rows := make([]CaddyRouteBackendEntity, 0)
	err := db.NewSelect().Model(&rows).Where("caddy_route_id = ?", routeID).Where("removed_at IS NULL").For("UPDATE").Scan(ctx)
	return rows, err
}

func (caddyRouteBackend) ActiveForRoute(
	ctx context.Context,
	db storage.Executor,
	routeID uuid.UUID,
) ([]CaddyRouteBackendEntity, error) {
	rows := make([]CaddyRouteBackendEntity, 0)
	err := db.NewSelect().Model(&rows).
		Where("caddy_route_id = ?", routeID).
		Where("removed_at IS NULL").
		OrderExpr("created_at, id").
		Scan(ctx)
	return rows, err
}

func (caddyRoute) FindActiveForInstance(
	ctx context.Context,
	db storage.Executor,
	instanceID uuid.UUID,
) (CaddyRouteEntity, error) {
	var route CaddyRouteEntity
	err := db.NewSelect().Model(&route).
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = caddy_routes.id").
		Where("backend.instance_id = ?", instanceID).
		Where("backend.removed_at IS NULL").
		Where("caddy_routes.removed_at IS NULL").
		OrderExpr("caddy_routes.created_at").
		Limit(1).
		Scan(ctx)
	return route, err
}

func (caddyRouteBackend) LockActive(ctx context.Context, db storage.Executor, routeID, instanceID uuid.UUID) (CaddyRouteBackendEntity, error) {
	var backend CaddyRouteBackendEntity
	err := db.NewSelect().Model(&backend).Where("caddy_route_id = ?", routeID).
		Where("instance_id = ?", instanceID).Where("removed_at IS NULL").For("UPDATE").Scan(ctx)
	return backend, err
}

func (caddyRouteBackend) ActiveCount(ctx context.Context, db storage.Executor, routeID uuid.UUID) (int, error) {
	return db.NewSelect().Model((*CaddyRouteBackendEntity)(nil)).Where("caddy_route_id = ?", routeID).
		Where("removed_at IS NULL").Count(ctx)
}

func (caddyRouteBackend) RetireActiveForRoute(ctx context.Context, db storage.Executor, routeID uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("caddy_route_backends").Set("removed_at = ?", at).
		Set("updated_at = ?", at).Where("caddy_route_id = ?", routeID).Where("removed_at IS NULL").Exec(ctx)
	return err
}

type ManagedCaddyRouteRow struct {
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

func (caddyRoute) ManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyRouteRow, error) {
	rows := make([]ManagedCaddyRouteRow, 0)
	err := db.NewSelect().TableExpr("caddy_routes AS route").
		ColumnExpr("route.id, route.external_id, route.state, route.environment_domain_id, route.environment_target_id, route.release_id, route.applied_at, route.observed_at").
		ColumnExpr("domain.hostname, environment.id AS environment_id, environment.name AS environment_name, application.name AS application_name").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("COALESCE(NULLIF(release.version, ''), release.artifact_reference) AS release_label").
		ColumnExpr("COALESCE(process.health_path, configuration.settings ->> 'health_path', '/api/health') AS health_path").
		Join("JOIN environment_domains AS domain ON domain.id = route.environment_domain_id").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").
		Join("JOIN servers AS server ON server.id = target.server_id").Join("JOIN releases AS release ON release.id = route.release_id").
		Join("LEFT JOIN LATERAL (SELECT process.value ->> 'health_path' AS health_path FROM caddy_route_backends backend JOIN instances instance ON instance.id = backend.instance_id AND instance.process_kind = 'web' JOIN deployments deployment ON deployment.id = instance.deployment_id LEFT JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(deployment.process_configuration) = 'array' THEN deployment.process_configuration ELSE '[]'::jsonb END) AS process(value) ON process.value ->> 'kind' = 'web' WHERE backend.caddy_route_id = route.id AND backend.removed_at IS NULL ORDER BY backend.weight DESC, backend.id DESC LIMIT 1) AS process ON TRUE").
		Join("LEFT JOIN LATERAL (SELECT settings FROM runtime_configurations WHERE environment_id = environment.id ORDER BY created_at DESC LIMIT 1) AS configuration ON TRUE").
		Where("route.removed_at IS NULL OR route.state = 'removal_pending'").OrderExpr("domain.hostname ASC").Scan(ctx, &rows)
	return rows, err
}

type ManagedCaddyBackendRow struct {
	CaddyRouteID uuid.UUID       `bun:"caddy_route_id"`
	InstanceID   uuid.UUID       `bun:"instance_id"`
	ExternalID   string          `bun:"external_id"`
	Slot         string          `bun:"slot"`
	State        string          `bun:"state"`
	Ports        json.RawMessage `bun:"ports"`
	Weight       int32           `bun:"weight"`
}

func (caddyRouteBackend) ManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyBackendRow, error) {
	rows := make([]ManagedCaddyBackendRow, 0)
	err := db.NewSelect().TableExpr("caddy_route_backends AS backend").
		ColumnExpr("backend.caddy_route_id, backend.instance_id, backend.weight").
		ColumnExpr("instance.external_id, instance.slot, instance.state, instance.ports").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id").Where("backend.removed_at IS NULL").
		Where("EXISTS (SELECT 1 FROM caddy_routes route WHERE route.id = backend.caddy_route_id AND (route.removed_at IS NULL OR route.state = 'removal_pending'))").
		OrderExpr("backend.id ASC").Scan(ctx, &rows)
	return rows, err
}

type ManagedCaddyDomainRow struct {
	ID              uuid.UUID `bun:"id"`
	Hostname        string    `bun:"hostname"`
	EnvironmentID   uuid.UUID `bun:"environment_id"`
	EnvironmentName string    `bun:"environment_name"`
	ApplicationName string    `bun:"application_name"`
}
type ManagedCaddyTargetRow struct {
	ID            uuid.UUID `bun:"id"`
	EnvironmentID uuid.UUID `bun:"environment_id"`
	ServerName    string    `bun:"server_name"`
}
type ManagedCaddyReleaseRow struct {
	ID                uuid.UUID `bun:"id"`
	EnvironmentID     uuid.UUID `bun:"environment_id"`
	Label             string    `bun:"label"`
	ArtifactReference string    `bun:"artifact_reference"`
}
type ManagedCaddyInstanceRow struct {
	ID                  uuid.UUID       `bun:"id"`
	EnvironmentID       uuid.UUID       `bun:"environment_id"`
	EnvironmentTargetID uuid.UUID       `bun:"environment_target_id"`
	ReleaseID           uuid.UUID       `bun:"release_id"`
	ExternalID          string          `bun:"external_id"`
	Slot                string          `bun:"slot"`
	State               string          `bun:"state"`
	Ports               json.RawMessage `bun:"ports"`
}

func (environmentDomain) CaddyManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyDomainRow, error) {
	rows := make([]ManagedCaddyDomainRow, 0)
	err := db.NewSelect().TableExpr("environment_domains AS domain").ColumnExpr("domain.id, domain.hostname, domain.environment_id, environment.name AS environment_name, application.name AS application_name").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("domain.archived_at IS NULL").Where("environment.archived_at IS NULL").Where("application.archived_at IS NULL").
		OrderExpr("application.name, environment.name, domain.hostname").Scan(ctx, &rows)
	return rows, err
}

func (environmentTarget) CaddyManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyTargetRow, error) {
	rows := make([]ManagedCaddyTargetRow, 0)
	err := db.NewSelect().TableExpr("environment_targets AS target").ColumnExpr("target.id, target.environment_id, server.name AS server_name").
		Join("JOIN servers AS server ON server.id = target.server_id").Where("target.detached_at IS NULL").Where("server.archived_at IS NULL").OrderExpr("server.name").Scan(ctx, &rows)
	return rows, err
}

func (release) CaddyManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyReleaseRow, error) {
	rows := make([]ManagedCaddyReleaseRow, 0)
	err := db.NewSelect().TableExpr("releases AS release").ColumnExpr("release.id, release.environment_id, COALESCE(NULLIF(release.version, ''), release.artifact_reference) AS label, release.artifact_reference").OrderExpr("release.created_at DESC").Scan(ctx, &rows)
	return rows, err
}

func (instance) CaddyManagementRows(ctx context.Context, db storage.Executor) ([]ManagedCaddyInstanceRow, error) {
	rows := make([]ManagedCaddyInstanceRow, 0)
	err := db.NewSelect().TableExpr("instances AS instance").ColumnExpr("instance.id, target.environment_id, instance.environment_target_id, instance.release_id, instance.external_id, instance.slot, instance.state, instance.ports").
		Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").Where("instance.removed_at IS NULL").Where("instance.state IN ('candidate', 'running', 'serving')").
		Where("target.detached_at IS NULL").OrderExpr("instance.created_at DESC").Scan(ctx, &rows)
	return rows, err
}

type CaddyRouteReferenceCheck struct {
	ExternalIDAvailable bool
	DomainAvailable     bool
	EnvironmentID       uuid.UUID
	TargetMatches       bool
	ReleaseMatches      bool
	ActiveInstances     map[uuid.UUID]bool
}

func (caddyRoute) CheckManagedReferences(ctx context.Context, db storage.Executor, routeID uuid.UUID, externalID string, domainID, targetID, releaseID uuid.UUID, instanceIDs []uuid.UUID) (CaddyRouteReferenceCheck, error) {
	check := CaddyRouteReferenceCheck{ActiveInstances: make(map[uuid.UUID]bool, len(instanceIDs))}
	count, err := db.NewSelect().TableExpr("caddy_routes").Where("external_id = ?", externalID).Where("removed_at IS NULL").Where("id <> ?", routeID).Count(ctx)
	if err != nil {
		return check, err
	}
	check.ExternalIDAvailable = count == 0
	count, err = db.NewSelect().TableExpr("caddy_routes").Where("environment_domain_id = ?", domainID).Where("removed_at IS NULL").Where("id <> ?", routeID).Count(ctx)
	if err != nil {
		return check, err
	}
	check.DomainAvailable = count == 0
	if err = db.NewSelect().TableExpr("environment_domains").Column("environment_id").Where("id = ?", domainID).Where("archived_at IS NULL").Scan(ctx, &check.EnvironmentID); err != nil {
		return check, err
	}
	count, err = db.NewSelect().TableExpr("environment_targets").Where("id = ?", targetID).Where("environment_id = ?", check.EnvironmentID).Where("detached_at IS NULL").Count(ctx)
	if err != nil {
		return check, err
	}
	check.TargetMatches = count == 1
	count, err = db.NewSelect().TableExpr("releases").Where("id = ?", releaseID).Where("environment_id = ?", check.EnvironmentID).Count(ctx)
	if err != nil {
		return check, err
	}
	check.ReleaseMatches = count == 1
	for _, instanceID := range instanceIDs {
		count, err = db.NewSelect().TableExpr("instances AS instance").Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").
			Where("instance.id = ?", instanceID).Where("instance.environment_target_id = ?", targetID).Where("target.environment_id = ?", check.EnvironmentID).
			Where("instance.removed_at IS NULL").Where("instance.state IN ('candidate', 'running', 'serving')").Count(ctx)
		if err != nil {
			return check, err
		}
		check.ActiveInstances[instanceID] = count == 1
	}
	return check, nil
}
