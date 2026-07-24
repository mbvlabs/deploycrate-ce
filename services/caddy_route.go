package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type CaddyClient interface {
	ApplyRoute(context.Context, caddyclients.Route) error
	VerifyRoute(context.Context, string) error
}

type CaddyRouteService struct {
	db    storage.Pool
	caddy CaddyClient
}

func NewCaddyRouteService(db storage.Pool, caddy CaddyClient) CaddyRouteService {
	return CaddyRouteService{db: db, caddy: caddy}
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
			HTTP int `json:"http"`
		}
		if err := json.Unmarshal(row.Ports, &ports); err != nil {
			return "", fmt.Errorf("decode Caddy backend ports: %w", err)
		}
		if ports.HTTP < 1 || ports.HTTP > 65535 {
			return "", fmt.Errorf("Caddy backend has invalid HTTP port %d", ports.HTTP)
		}
		backends = append(backends, caddyclients.Backend{
			Dial: fmt.Sprintf("127.0.0.1:%d", ports.HTTP), Weight: int(row.Weight),
		})
	}

	if err := service.caddy.ApplyRoute(ctx, caddyclients.Route{
		ID: route.ExternalID, Domain: domain.Hostname, Backends: backends,
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

func (service CaddyRouteService) Verify(ctx context.Context, externalID string) error {
	return service.caddy.VerifyRoute(ctx, externalID)
}
