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
	EnvironmentTargetID uuid.UUID       `bun:"environment_target_id,type:uuid"`
	DesiredRevisionID   *uuid.UUID      `bun:"desired_revision_id,type:uuid"`
	ApplyingRevisionID  *uuid.UUID      `bun:"applying_revision_id,type:uuid"`
	AppliedRevisionID   *uuid.UUID      `bun:"applied_revision_id,type:uuid"`
	ObservedState       json.RawMessage `bun:"observed_state,type:jsonb"`
	State               string          `bun:"state"`
	ObservedAt          sql.NullTime    `bun:"observed_at"`
}

func (e *EnvironmentTargetStateEntity) Validate() error {
	return nil
}

func (ets environmentTargetState) Find(ctx context.Context, db storage.Executor, id int32) (EnvironmentTargetStateEntity, error) {
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
	EnvironmentTargetID uuid.UUID
	DesiredRevisionID   *uuid.UUID
	ApplyingRevisionID  *uuid.UUID
	AppliedRevisionID   *uuid.UUID
	ObservedState       json.RawMessage
	State               string
	ObservedAt          sql.NullTime
}

func (ets environmentTargetState) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetStateData) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
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
	EnvironmentTargetID uuid.UUID
	DesiredRevisionID   *uuid.UUID
	ApplyingRevisionID  *uuid.UUID
	AppliedRevisionID   *uuid.UUID
	ObservedState       json.RawMessage
	State               string
	ObservedAt          sql.NullTime
}

func (ets environmentTargetState) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentTargetStateData) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetStateEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_target_id").
		Column("desired_revision_id").
		Column("applying_revision_id").
		Column("applied_revision_id").
		Column("observed_state").
		Column("state").
		Column("observed_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}

func (ets environmentTargetState) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*EnvironmentTargetStateEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ets environmentTargetState) All(ctx context.Context, db storage.Executor) ([]EnvironmentTargetStateEntity, error) {
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

func (ets environmentTargetState) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentTargetStates, error) {
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

func (ets environmentTargetState) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetStateData) (EnvironmentTargetStateEntity, error) {
	entity := EnvironmentTargetStateEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		EnvironmentTargetID: data.EnvironmentTargetID,
		DesiredRevisionID:   data.DesiredRevisionID,
		ApplyingRevisionID:  data.ApplyingRevisionID,
		AppliedRevisionID:   data.AppliedRevisionID,
		ObservedState:       data.ObservedState,
		State:               data.State,
		ObservedAt:          data.ObservedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetStateEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_target_id = excluded.environment_target_id").
		Set("desired_revision_id = excluded.desired_revision_id").
		Set("applying_revision_id = excluded.applying_revision_id").
		Set("applied_revision_id = excluded.applied_revision_id").
		Set("observed_state = excluded.observed_state").
		Set("state = excluded.state").
		Set("observed_at = excluded.observed_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetStateEntity{}, err
	}

	return entity, nil
}
