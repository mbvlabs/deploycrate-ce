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

type DeploymentEntity struct {
	bun.BaseModel        `bun:"table:deployments,alias:deployments"`
	ID                   uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time       `bun:"created_at"`
	UpdatedAt            time.Time       `bun:"updated_at"`
	Attempt              int32           `bun:"attempt"`
	Strategy             json.RawMessage `bun:"strategy,type:jsonb"`
	ProcessConfiguration json.RawMessage `bun:"process_configuration,type:jsonb"`
	RuntimeConfiguration json.RawMessage `bun:"-"`
	Status               string          `bun:"status"`
	CurrentStep          sql.NullString  `bun:"current_step"`
	StartedAt            sql.NullTime    `bun:"started_at"`
	FinishedAt           sql.NullTime    `bun:"finished_at"`
	Error                sql.NullString  `bun:"error"`
	ChangeID             uuid.UUID       `bun:"change_id,type:uuid"`
	ReleaseID            uuid.UUID       `bun:"release_id,type:uuid"`
	EnvironmentTargetID  uuid.UUID       `bun:"environment_target_id,type:uuid"`
}

func (e *DeploymentEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil || e.ChangeID == uuid.Nil || e.ReleaseID == uuid.Nil || e.EnvironmentTargetID == uuid.Nil {
		builder.Add("id", "required", "Deployment ownership identifiers are required")
	}
	if e.Attempt < 1 {
		builder.Add("attempt", "invalid", "Deployment attempt must be positive")
	}
	if !slices.Contains([]string{"queued", "running", "succeeded", "failed"}, e.Status) {
		builder.Add("status", "invalid", "Deployment status is invalid")
	}
	if !json.Valid(e.Strategy) || !json.Valid(e.ProcessConfiguration) {
		builder.Add("configuration", "invalid", "Deployment configuration must be valid JSON")
	}
	return builder.Err()
}

func (d deployment) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (DeploymentEntity, error) {
	var entity DeploymentEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return DeploymentEntity{}, err
	}

	entity.RuntimeConfiguration = entity.ProcessConfiguration
	return entity, nil
}

func (d deployment) Lock(ctx context.Context, db storage.Executor, id uuid.UUID) (DeploymentEntity, error) {
	var entity DeploymentEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).For("UPDATE").Scan(ctx)
	return entity, err
}

func (d deployment) MarkRunning(ctx context.Context, db storage.Executor, id uuid.UUID, step string, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("deployments").Set("status = 'running'").Set("current_step = ?", step).
		Set("started_at = COALESCE(started_at, ?)", at).Set("finished_at = NULL").Set("error = NULL").
		Set("updated_at = ?", at).Where("id = ?", id).Where("status IN ('queued', 'running')").Exec(ctx)
	return err
}

func (d deployment) MarkFailed(ctx context.Context, db storage.Executor, id uuid.UUID, operationErr error, at time.Time) error {
	message := strings.TrimSpace(operationErr.Error())
	if len(message) > 2048 {
		message = message[:2048]
	}
	_, err := db.NewUpdate().TableExpr("deployments").Set("status = 'failed'").Set("current_step = 'failed'").
		Set("finished_at = ?", at).Set("error = ?", message).Set("updated_at = ?", at).
		Where("id = ?", id).Where("status IN ('queued', 'running')").Exec(ctx)
	return err
}

type CreateDeploymentData struct {
	Attempt              int32
	Strategy             json.RawMessage
	ProcessConfiguration json.RawMessage
	Status               string
	CurrentStep          sql.NullString
	StartedAt            sql.NullTime
	FinishedAt           sql.NullTime
	Error                sql.NullString
	ChangeID             uuid.UUID
	ReleaseID            uuid.UUID
	EnvironmentTargetID  uuid.UUID
}

func (d deployment) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateDeploymentData,
) (DeploymentEntity, error) {
	entity := DeploymentEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Attempt:              data.Attempt,
		Strategy:             data.Strategy,
		ProcessConfiguration: data.ProcessConfiguration,
		Status:               data.Status,
		CurrentStep:          data.CurrentStep,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		Error:                data.Error,
		ChangeID:             data.ChangeID,
		ReleaseID:            data.ReleaseID,
		EnvironmentTargetID:  data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DeploymentEntity{}, err
	}

	return entity, nil
}

type UpdateDeploymentData struct {
	ID                   uuid.UUID
	UpdatedAt            time.Time
	Attempt              int32
	Strategy             json.RawMessage
	ProcessConfiguration json.RawMessage
	Status               string
	CurrentStep          sql.NullString
	StartedAt            sql.NullTime
	FinishedAt           sql.NullTime
	Error                sql.NullString
	ChangeID             uuid.UUID
	ReleaseID            uuid.UUID
	EnvironmentTargetID  uuid.UUID
}

func (d deployment) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateDeploymentData,
) (DeploymentEntity, error) {
	entity := DeploymentEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		Attempt:              data.Attempt,
		Strategy:             data.Strategy,
		ProcessConfiguration: data.ProcessConfiguration,
		Status:               data.Status,
		CurrentStep:          data.CurrentStep,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		Error:                data.Error,
		ChangeID:             data.ChangeID,
		ReleaseID:            data.ReleaseID,
		EnvironmentTargetID:  data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("attempt").
		Column("strategy").
		Column("process_configuration").
		Column("status").
		Column("current_step").
		Column("started_at").
		Column("finished_at").
		Column("error").
		Column("change_id").
		Column("release_id").
		Column("environment_target_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return DeploymentEntity{}, err
	}

	return entity, nil
}

func (d deployment) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*DeploymentEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (d deployment) All(ctx context.Context, db storage.Executor) ([]DeploymentEntity, error) {
	var entities []DeploymentEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedDeployments struct {
	Deployments []DeploymentEntity
	TotalCount  int64
	Page        int64
	PageSize    int64
	TotalPages  int64
}

func (d deployment) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedDeployments, error) {
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
		Model(&DeploymentEntity{}).Count(ctx)
	if err != nil {
		return PaginatedDeployments{}, err
	}

	entities := make([]DeploymentEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedDeployments{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedDeployments{
		Deployments: entities,
		TotalCount:  int64(totalCount),
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	}, nil
}

func (d deployment) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateDeploymentData,
) (DeploymentEntity, error) {
	entity := DeploymentEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Attempt:              data.Attempt,
		Strategy:             data.Strategy,
		ProcessConfiguration: data.ProcessConfiguration,
		Status:               data.Status,
		CurrentStep:          data.CurrentStep,
		StartedAt:            data.StartedAt,
		FinishedAt:           data.FinishedAt,
		Error:                data.Error,
		ChangeID:             data.ChangeID,
		ReleaseID:            data.ReleaseID,
		EnvironmentTargetID:  data.EnvironmentTargetID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("attempt = excluded.attempt").
		Set("strategy = excluded.strategy").
		Set("process_configuration = excluded.process_configuration").
		Set("status = excluded.status").
		Set("current_step = excluded.current_step").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("error = excluded.error").
		Set("change_id = excluded.change_id").
		Set("release_id = excluded.release_id").
		Set("environment_target_id = excluded.environment_target_id").
		Returning("*").
		Scan(ctx); err != nil {
		return DeploymentEntity{}, err
	}

	return entity, nil
}

type SystemUpdateCheckpoint struct {
	ServiceTemplate    string `json:"service_template"`
	ActiveSlot         string `json:"active_slot"`
	TargetSlot         string `json:"target_slot"`
	Phase              string `json:"phase"`
	PreviousSlotTarget string `json:"previous_slot_target"`
	TargetStarted      bool   `json:"target_started"`
	TrafficSwitched    bool   `json:"traffic_switched"`
	BootStateSwitched  bool   `json:"boot_state_switched"`
}

type UnresolvedSystemUpdate struct {
	DeploymentID         uuid.UUID       `bun:"deployment_id"`
	ChangeID             uuid.UUID       `bun:"change_id"`
	ReleaseID            uuid.UUID       `bun:"release_id"`
	Version              string          `bun:"version"`
	ReleasePath          string          `bun:"release_path"`
	InstanceID           uuid.UUID       `bun:"instance_id"`
	BackendID            int32           `bun:"backend_id"`
	InactiveSlot         string          `bun:"inactive_slot"`
	CaddyRouteID         uuid.UUID       `bun:"caddy_route_id"`
	CaddyRouteExternalID string          `bun:"caddy_route_external_id"`
	PreviousInstanceID   uuid.UUID       `bun:"previous_instance_id"`
	PreviousInstanceSlot string          `bun:"previous_instance_slot"`
	PreviousBackendID    int32           `bun:"previous_backend_id"`
	PreviousReleaseID    uuid.UUID       `bun:"previous_release_id"`
	EnvironmentID        uuid.UUID       `bun:"environment_id"`
	EnvironmentTargetID  uuid.UUID       `bun:"environment_target_id"`
	ProcessConfiguration json.RawMessage `bun:"process_configuration"`
	EventSequence        int64           `bun:"event_sequence"`
}

func (d deployment) FindUnresolvedSystemUpdate(
	ctx context.Context,
	db storage.Executor,
) (*UnresolvedSystemUpdate, error) {
	var unresolved UnresolvedSystemUpdate
	err := db.NewSelect().
		TableExpr("deployments AS deployment").
		ColumnExpr("deployment.id AS deployment_id").
		ColumnExpr("deployment.change_id AS change_id").
		ColumnExpr("deployment.release_id AS release_id").
		ColumnExpr("COALESCE(release.version, '') AS version").
		ColumnExpr("release.artifact_reference AS release_path").
		ColumnExpr("instance.id AS instance_id").
		ColumnExpr("backend.id AS backend_id").
		ColumnExpr("instance.slot AS inactive_slot").
		ColumnExpr("route.id AS caddy_route_id").
		ColumnExpr("route.external_id AS caddy_route_external_id").
		ColumnExpr("previous_instance.id AS previous_instance_id").
		ColumnExpr("previous_instance.slot AS previous_instance_slot").
		ColumnExpr("previous_backend.id AS previous_backend_id").
		ColumnExpr("previous_instance.release_id AS previous_release_id").
		ColumnExpr("environment.id AS environment_id").
		ColumnExpr("deployment.environment_target_id AS environment_target_id").
		ColumnExpr("deployment.process_configuration AS process_configuration").
		ColumnExpr("(SELECT COALESCE(MAX(event.sequence), 0) FROM deployment_events AS event WHERE event.deployment_id = deployment.id) AS event_sequence").
		Join("JOIN changes AS change ON change.id = deployment.change_id").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Join("JOIN instances AS instance ON instance.deployment_id = deployment.id").
		Join("JOIN caddy_route_backends AS backend ON backend.instance_id = instance.id").
		Join("JOIN caddy_routes AS route ON route.id = backend.caddy_route_id AND route.removed_at IS NULL").
		Join("JOIN environments AS environment ON environment.id = release.environment_id").
		Join("JOIN caddy_route_backends AS previous_backend ON previous_backend.caddy_route_id = route.id AND previous_backend.id <> backend.id AND previous_backend.removed_at IS NULL").
		Join("JOIN instances AS previous_instance ON previous_instance.id = previous_backend.instance_id AND previous_instance.removed_at IS NULL").
		Where("change.kind = ?", "system_update").
		Where("change.cause_system = ?", "deploycrate-ce").
		Where("deployment.status IN (?)", bun.In([]string{"queued", "running", "in_progress"})).
		OrderExpr("deployment.created_at DESC").
		Limit(1).
		Scan(ctx, &unresolved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &unresolved, nil
}

func (d deployment) SaveSystemUpdateCheckpoint(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	checkpoint SystemUpdateCheckpoint,
) error {
	content, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	_, err = db.NewUpdate().
		TableExpr("deployments").
		Set("process_configuration = ?", json.RawMessage(content)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (d deployment) RecordProgress(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	status, step string,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("deployments").
		Set("status = ?", status).
		Set("current_step = ?", step).
		Set("started_at = COALESCE(started_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (d deployment) FinishSystemUpdate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	status string,
	step string,
	failure sql.NullString,
	checkpoint SystemUpdateCheckpoint,
	at time.Time,
) error {
	content, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	_, err = db.NewUpdate().
		TableExpr("deployments").
		Set("status = ?", status).
		Set("current_step = ?", step).
		Set("finished_at = ?", at).
		Set("error = ?", failure).
		Set("process_configuration = ?", json.RawMessage(content)).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
