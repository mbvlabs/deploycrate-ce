package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentTargetStateEntity struct {
	bun.BaseModel       `bun:"table:environment_target_states,alias:environment_target_states"`
	ID                  int32           `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ObservedState       json.RawMessage `bun:"observed_state,type:jsonb"`
	State               string          `bun:"state"`
	ObservedAt          sql.NullTime    `bun:"observed_at"`
	EnvironmentTargetID uuid.UUID       `bun:"environment_target_id,type:uuid"`
	DesiredRevisionID   *uuid.UUID      `bun:"desired_revision_id,type:uuid"`
	ApplyingRevisionID  *uuid.UUID      `bun:"applying_revision_id,type:uuid"`
	AppliedRevisionID   *uuid.UUID      `bun:"applied_revision_id,type:uuid"`
}

func (environmentTargetState) SetDesiredRevisionForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID, revisionID uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("environment_target_states AS state").
		Set("desired_revision_id = ?", revisionID).
		Set("state = 'pending'").
		Set("updated_at = ?", at).
		Where("EXISTS (SELECT 1 FROM environment_targets target WHERE target.id = state.environment_target_id AND target.environment_id = ? AND target.detached_at IS NULL)", environmentID).
		Exec(ctx)
	return err
}

func (environmentTargetState) MarkFailedForDeployment(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states AS state").
		Set("state = 'failed'").Set("applying_revision_id = NULL").Set("updated_at = ?", at).
		Where("EXISTS (SELECT 1 FROM deployments deployment WHERE deployment.id = ? AND deployment.environment_target_id = state.environment_target_id)", deploymentID).
		Exec(ctx)
	return err
}

func (environmentTargetState) MarkApplying(
	ctx context.Context,
	db storage.Executor,
	targetID, revisionID uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states").
		Set("state = 'applying'").Set("applying_revision_id = ?", revisionID).
		Set("updated_at = ?", at).Where("environment_target_id = ?", targetID).Exec(ctx)
	return err
}

func (environmentTargetState) MarkFailed(
	ctx context.Context,
	db storage.Executor,
	targetID uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states").
		Set("state = 'failed'").Set("applying_revision_id = NULL").Set("updated_at = ?", at).
		Where("environment_target_id = ?", targetID).Exec(ctx)
	return err
}

func (environmentTargetState) RestoreAppliedAfterDeploymentCancellation(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states AS state").
		Set("desired_revision_id = applied_revision_id").
		Set("applying_revision_id = NULL").
		Set("state = CASE WHEN applied_revision_id IS NULL THEN 'failed' ELSE 'applied' END").
		Set("updated_at = ?", at).
		Where("EXISTS (SELECT 1 FROM deployments deployment WHERE deployment.id = ? AND deployment.environment_target_id = state.environment_target_id)", deploymentID).
		Exec(ctx)
	return err
}

func (environmentTargetState) MarkApplied(
	ctx context.Context,
	db storage.Executor,
	targetID, revisionID uuid.UUID,
	observed json.RawMessage,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states").
		Set("state = 'applied'").Set("observed_state = ?", observed).
		Set("applying_revision_id = NULL").Set("applied_revision_id = ?", revisionID).
		Set("observed_at = ?", at).Set("updated_at = ?", at).
		Where("environment_target_id = ?", targetID).Exec(ctx)
	return err
}

func (e *EnvironmentTargetStateEntity) Validate() error {
	return nil
}

func (ets environmentTargetState) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (EnvironmentTargetStateEntity, error) {
	var entity EnvironmentTargetStateEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentTargetStateData struct {
	ObservedState       json.RawMessage
	State               string
	ObservedAt          sql.NullTime
	EnvironmentTargetID uuid.UUID
	DesiredRevisionID   *uuid.UUID
	ApplyingRevisionID  *uuid.UUID
	AppliedRevisionID   *uuid.UUID
}

func (ets environmentTargetState) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentTargetStateData,
) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetStateEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentTargetStateData struct {
	ID                  int32
	UpdatedAt           time.Time
	ObservedState       json.RawMessage
	State               string
	ObservedAt          sql.NullTime
	EnvironmentTargetID uuid.UUID
	DesiredRevisionID   *uuid.UUID
	ApplyingRevisionID  *uuid.UUID
	AppliedRevisionID   *uuid.UUID
}

func (ets environmentTargetState) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentTargetStateData,
) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetStateEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("observed_state").
		Column("state").
		Column("observed_at").
		Column("environment_target_id").
		Column("desired_revision_id").
		Column("applying_revision_id").
		Column("applied_revision_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}

func (ets environmentTargetState) Destroy(
	ctx context.Context,
	db storage.Executor,
	id int32,
) error {
	_, err := db.NewDelete().
		Model((*EnvironmentTargetStateEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ets environmentTargetState) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentTargetStateEntity, error) {
	var entities []EnvironmentTargetStateEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentTargetStates struct {
	EnvironmentTargetStates []EnvironmentTargetStateEntity
	TotalCount              int64
	Page                    int64
	PageSize                int64
	TotalPages              int64
}

func (ets environmentTargetState) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentTargetStates, error) {
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
		Model(&EnvironmentTargetStateEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentTargetStates{}, err
	}

	entities := make([]EnvironmentTargetStateEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentTargetStates{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentTargetStates{
		EnvironmentTargetStates: entities,
		TotalCount:              int64(totalCount),
		Page:                    page,
		PageSize:                pageSize,
		TotalPages:              totalPages,
	}, nil
}

func (ets environmentTargetState) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentTargetStateData,
) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetStateEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("observed_state = excluded.observed_state").
		Set("state = excluded.state").
		Set("observed_at = excluded.observed_at").
		Set("environment_target_id = excluded.environment_target_id").
		Set("desired_revision_id = excluded.desired_revision_id").
		Set("applying_revision_id = excluded.applying_revision_id").
		Set("applied_revision_id = excluded.applied_revision_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}
