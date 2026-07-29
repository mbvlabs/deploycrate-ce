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
	if e.ID == uuid.Nil || e.DeploymentID == uuid.Nil || e.ReleaseID == uuid.Nil || e.EnvironmentTargetID == uuid.Nil {
		builder.Add("id", "required", "Instance ownership identifiers are required")
	}
	if strings.TrimSpace(e.ExternalID) == "" || strings.TrimSpace(e.ReplicaKey) == "" {
		builder.Add("externalId", "required", "Instance identity is required")
	}
	if !slices.Contains([]string{"queued", "candidate", "running", "serving", "failed", "removed"}, e.State) {
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

type CreateInstanceData struct {
	ExternalID          string
	Slot                string
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
