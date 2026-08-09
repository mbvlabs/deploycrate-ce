package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type InstanceEntity struct {
	bun.BaseModel       `bun:"table:instances,alias:instances"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ExternalID          string          `bun:"external_id"`
	Slot                string          `bun:"slot"`
	ProcessName         string          `bun:"process_name"                    json:"processName"`
	ProcessKind         string          `bun:"process_kind"                    json:"processKind"`
	ReplicaKey          string          `bun:"replica_key"`
	State               string          `bun:"state"`
	Ports               json.RawMessage `bun:"ports,type:jsonb"`
	ObservedAt          time.Time       `bun:"observed_at"`
	RemovedAt           sql.NullTime    `bun:"removed_at"`
	DeploymentID        uuid.UUID       `bun:"deployment_id,type:uuid"`
	ReleaseID           uuid.UUID       `bun:"release_id,type:uuid"`
	EnvironmentTargetID uuid.UUID       `bun:"environment_target_id,type:uuid"`
}

func (e *InstanceEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil || e.DeploymentID == uuid.Nil || e.ReleaseID == uuid.Nil ||
		e.EnvironmentTargetID == uuid.Nil {
		builder.Add("id", "required", "Instance ownership identifiers are required")
	}
	if strings.TrimSpace(e.ExternalID) == "" || strings.TrimSpace(e.ReplicaKey) == "" ||
		!environmentProcessNamePattern.MatchString(e.ProcessName) ||
		!slices.Contains([]string{EnvironmentProcessWeb, EnvironmentProcessWorker}, e.ProcessKind) {
		builder.Add("externalId", "required", "Instance identity is required")
	}
	if !slices.Contains(
		[]string{"queued", "candidate", "running", "serving", "failed", "removed"},
		e.State,
	) {
		builder.Add("state", "invalid", "Instance state is invalid")
	}
	if len(e.Ports) == 0 || !json.Valid(e.Ports) {
		builder.Add("ports", "invalid", "Instance ports must be valid JSON")
	}
	return builder.Err()
}

func (i instance) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (InstanceEntity, error) {
	var entity InstanceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return InstanceEntity{}, err
	}

	return entity, nil
}

func (instance) ActiveForDeployment(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
) ([]InstanceEntity, error) {
	items := make([]InstanceEntity, 0)
	err := db.NewSelect().Model(&items).
		Where("deployment_id = ?", deploymentID).
		Where("removed_at IS NULL").
		OrderExpr("process_kind, process_name, replica_key").Scan(ctx)
	return items, err
}

func (instance) MarkNonServingFailedForDeployment(
	ctx context.Context, db storage.Executor, deploymentID uuid.UUID, at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("instances").
		Set("state = 'failed'").Set("updated_at = ?", at).
		Where("deployment_id = ?", deploymentID).Where("state <> 'serving'").Exec(ctx)
	return err
}

func (instance) MarkState(
	ctx context.Context, db storage.Executor, id uuid.UUID, state string, at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("instances").
		Set("state = ?", state).Set("updated_at = ?", at).
		Where("id = ?", id).Exec(ctx)
	return err
}

func (instance) MarkRemoved(
	ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("instances").
		Set("state = 'removed'").Set("removed_at = ?", at).Set("updated_at = ?", at).
		Where("id = ?", id).Exec(ctx)
	return err
}

func (instance) ActiveForDeployments(
	ctx context.Context,
	db storage.Executor,
	deploymentIDs []uuid.UUID,
) ([]InstanceEntity, error) {
	items := make([]InstanceEntity, 0)
	if len(deploymentIDs) == 0 {
		return items, nil
	}
	err := db.NewSelect().Model(&items).
		Where("deployment_id IN (?)", bun.In(deploymentIDs)).Where("removed_at IS NULL").
		OrderExpr("created_at, process_kind, process_name, replica_key").Scan(ctx)
	return items, err
}

func (instance) PreviousForRoute(
	ctx context.Context,
	db storage.Executor,
	routeID, excludeInstanceID uuid.UUID,
) ([]InstanceEntity, error) {
	items := make([]InstanceEntity, 0)
	err := db.NewSelect().Model(&items).
		Join("JOIN caddy_route_backends AS backend ON backend.instance_id = instances.id").
		Where("backend.caddy_route_id = ?", routeID).
		Where("backend.removed_at IS NULL").Where("instances.id <> ?", excludeInstanceID).
		Where("instances.removed_at IS NULL").OrderExpr("instances.created_at DESC").Scan(ctx)
	return items, err
}

func (instance) ServingForReconciliation(
	ctx context.Context,
	db storage.Executor,
) ([]InstanceEntity, error) {
	items := make([]InstanceEntity, 0)
	err := db.NewSelect().Model(&items).
		Join("JOIN deployments AS deployment ON deployment.id = instances.deployment_id AND deployment.status = 'succeeded'").
		Join("JOIN releases AS release ON release.id = instances.release_id").
		Join("JOIN environments AS environment ON environment.id = release.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("application.slug <> ?", SystemApplicationSlug).
		Where("EXISTS (SELECT 1 FROM instances web JOIN caddy_route_backends backend ON backend.instance_id = web.id AND backend.removed_at IS NULL AND backend.weight = 100 JOIN caddy_routes route ON route.id = backend.caddy_route_id AND route.removed_at IS NULL WHERE web.deployment_id = instances.deployment_id AND web.process_kind = 'web' AND route.environment_target_id = instances.environment_target_id)").
		Where("instances.state = 'serving'").Where("instances.removed_at IS NULL").
		OrderExpr("instances.created_at").Scan(ctx)
	return items, err
}

func (instance) ObserveRuntime(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	externalID string,
	state *string,
	ports json.RawMessage,
	at time.Time,
) error {
	query := db.NewUpdate().TableExpr("instances").
		Set("external_id = ?", externalID).
		Set("ports = ?", ports).
		Set("observed_at = ?", at).
		Set("updated_at = ?", at)
	if state != nil {
		query = query.Set("state = ?", *state)
	}
	_, err := query.Where("id = ?", id).Exec(ctx)
	return err
}

type UnroutedWorkloadInstance struct {
	InstanceID   uuid.UUID `bun:"instance_id"`
	DeploymentID uuid.UUID `bun:"deployment_id"`
	ServerID     uuid.UUID `bun:"server_id"`
}

func (instance) UnroutedWorkloads(
	ctx context.Context,
	db storage.Executor,
	environmentTargetID uuid.UUID,
) ([]UnroutedWorkloadInstance, error) {
	rows := make([]UnroutedWorkloadInstance, 0)
	query := db.NewSelect().TableExpr("instances AS instance").
		ColumnExpr("instance.id AS instance_id, instance.deployment_id AS deployment_id, target.server_id AS server_id").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id AND deployment.status NOT IN ('queued', 'running')").
		Join("JOIN releases AS release ON release.id = instance.release_id").
		Join("JOIN environments AS environment ON environment.id = release.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").
		Where("application.slug <> ?", SystemApplicationSlug).
		Where("instance.removed_at IS NULL").
		Where(`NOT EXISTS (
			SELECT 1 FROM caddy_route_backends AS own_backend
			JOIN caddy_routes AS own_route ON own_route.id = own_backend.caddy_route_id AND own_route.removed_at IS NULL
			WHERE own_backend.instance_id = instance.id AND own_backend.removed_at IS NULL
		)`).
		Where(`(deployment.status = 'failed' OR (NOT EXISTS (
			SELECT 1 FROM caddy_routes own_active_route
			JOIN caddy_route_backends own_active_backend ON own_active_backend.caddy_route_id = own_active_route.id
			JOIN instances own_active_web ON own_active_web.id = own_active_backend.instance_id
			WHERE own_active_web.deployment_id = instance.deployment_id AND own_active_web.process_kind = 'web'
			AND own_active_route.removed_at IS NULL AND own_active_backend.removed_at IS NULL AND own_active_backend.weight = 100
		) AND EXISTS (
			SELECT 1 FROM caddy_routes AS active_route
			JOIN caddy_route_backends AS active_backend ON active_backend.caddy_route_id = active_route.id
			JOIN instances AS active_instance ON active_instance.id = active_backend.instance_id AND active_instance.removed_at IS NULL
			WHERE active_route.environment_target_id = instance.environment_target_id AND active_route.removed_at IS NULL
			AND active_backend.removed_at IS NULL AND active_backend.weight = 100 AND active_instance.id <> instance.id
		)))`)
	if environmentTargetID != uuid.Nil {
		query = query.Where("instance.environment_target_id = ?", environmentTargetID)
	}
	err := query.Scan(ctx, &rows)
	return rows, err
}

type CreateInstanceData struct {
	ExternalID          string
	Slot                string
	ProcessName         string
	ProcessKind         string
	ReplicaKey          string
	State               string
	Ports               json.RawMessage
	ObservedAt          time.Time
	RemovedAt           sql.NullTime
	DeploymentID        uuid.UUID
	ReleaseID           uuid.UUID
	EnvironmentTargetID uuid.UUID
}

func (i instance) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateInstanceData,
) (InstanceEntity, error) {
	entity := InstanceEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Slot:                data.Slot,
		ProcessName:         data.ProcessName,
		ProcessKind:         data.ProcessKind,
		ReplicaKey:          data.ReplicaKey,
		State:               data.State,
		Ports:               data.Ports,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
		DeploymentID:        data.DeploymentID,
		ReleaseID:           data.ReleaseID,
		EnvironmentTargetID: data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return InstanceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return InstanceEntity{}, errors.New("Instance creation requires a transaction")
	}
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"deployment-instance:"+entity.DeploymentID.String(),
	); err != nil {
		return InstanceEntity{}, err
	}
	count, err := db.NewSelect().
		Model((*InstanceEntity)(nil)).
		Where("deployment_id = ?", entity.DeploymentID).
		Where("process_name = ?", entity.ProcessName).
		Where("replica_key = ?", entity.ReplicaKey).
		Count(ctx)
	if err != nil {
		return InstanceEntity{}, err
	}
	if count != 0 {
		return InstanceEntity{}, errors.Join(
			ErrDomainValidation,
			errors.New("Instance replica identity must be unique within its Deployment process"),
		)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return InstanceEntity{}, err
	}

	return entity, nil
}

type UpdateInstanceData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	ExternalID          string
	Slot                string
	ProcessName         string
	ProcessKind         string
	ReplicaKey          string
	State               string
	Ports               json.RawMessage
	ObservedAt          time.Time
	RemovedAt           sql.NullTime
	DeploymentID        uuid.UUID
	ReleaseID           uuid.UUID
	EnvironmentTargetID uuid.UUID
}

func (i instance) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateInstanceData,
) (InstanceEntity, error) {
	entity := InstanceEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Slot:                data.Slot,
		ProcessName:         data.ProcessName,
		ProcessKind:         data.ProcessKind,
		ReplicaKey:          data.ReplicaKey,
		State:               data.State,
		Ports:               data.Ports,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
		DeploymentID:        data.DeploymentID,
		ReleaseID:           data.ReleaseID,
		EnvironmentTargetID: data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return InstanceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("external_id").
		Column("slot").
		Column("process_name").
		Column("process_kind").
		Column("replica_key").
		Column("state").
		Column("ports").
		Column("observed_at").
		Column("removed_at").
		Column("deployment_id").
		Column("release_id").
		Column("environment_target_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return InstanceEntity{}, err
	}

	return entity, nil
}

func (i instance) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*InstanceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (i instance) All(ctx context.Context, db storage.Executor) ([]InstanceEntity, error) {
	var entities []InstanceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedInstances struct {
	Instances  []InstanceEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (i instance) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedInstances, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	totalCount, err := db.NewSelect().
		Model(&InstanceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedInstances{}, err
	}

	entities := make([]InstanceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedInstances{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedInstances{
		Instances:  entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (i instance) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateInstanceData,
) (InstanceEntity, error) {
	entity := InstanceEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Slot:                data.Slot,
		ProcessName:         data.ProcessName,
		ProcessKind:         data.ProcessKind,
		ReplicaKey:          data.ReplicaKey,
		State:               data.State,
		Ports:               data.Ports,
		ObservedAt:          data.ObservedAt,
		RemovedAt:           data.RemovedAt,
		DeploymentID:        data.DeploymentID,
		ReleaseID:           data.ReleaseID,
		EnvironmentTargetID: data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return InstanceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("external_id = excluded.external_id").
		Set("slot = excluded.slot").
		Set("process_name = excluded.process_name").
		Set("process_kind = excluded.process_kind").
		Set("replica_key = excluded.replica_key").
		Set("state = excluded.state").
		Set("ports = excluded.ports").
		Set("observed_at = excluded.observed_at").
		Set("removed_at = excluded.removed_at").
		Set("deployment_id = excluded.deployment_id").
		Set("release_id = excluded.release_id").
		Set("environment_target_id = excluded.environment_target_id").
		Returning("*").
		Scan(ctx); err != nil {
		return InstanceEntity{}, err
	}

	return entity, nil
}

func (i instance) ObserveState(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	state string,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("instances").
		Set("state = ?", state).
		Set("observed_at = ?", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (i instance) FinishSystemUpdate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	state string,
	removed bool,
	at time.Time,
) error {
	query := db.NewUpdate().
		TableExpr("instances").
		Set("state = ?", state).
		Set("observed_at = ?", at).
		Set("updated_at = ?", at)
	if removed {
		query = query.Set("removed_at = ?", at)
	}
	_, err := query.Where("id = ?", id).Exec(ctx)
	return err
}
