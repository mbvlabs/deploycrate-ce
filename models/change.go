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

type ChangeEntity struct {
	bun.BaseModel     `bun:"table:changes,alias:changes"`
	ID                uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time       `bun:"created_at"`
	UpdatedAt         time.Time       `bun:"updated_at"`
	Sequence          int64           `bun:"sequence"`
	Kind              string          `bun:"kind"`
	TriggerType       string          `bun:"trigger_type"`
	ActorType         string          `bun:"actor_type"`
	ActorID           *uuid.UUID      `bun:"actor_id,type:uuid"`
	CauseSystem       sql.NullString  `bun:"cause_system"`
	CauseReference    sql.NullString  `bun:"cause_reference"`
	CorrelationID     uuid.UUID       `bun:"correlation_id,type:uuid"`
	CorrectionContext json.RawMessage `bun:"correction_context,type:jsonb"`
	Summary           string          `bun:"summary"`
	Status            string          `bun:"status"`
	RequestedAt       time.Time       `bun:"requested_at"`
	CommittedAt       sql.NullTime    `bun:"committed_at"`
	StartedAt         sql.NullTime    `bun:"started_at"`
	FinishedAt        sql.NullTime    `bun:"finished_at"`
	CancelledAt       sql.NullTime    `bun:"cancelled_at"`
	Error             sql.NullString  `bun:"error"`
	EnvironmentID     uuid.UUID       `bun:"environment_id,type:uuid"`
	CorrectsChangeID  *uuid.UUID      `bun:"corrects_change_id,type:uuid"`
}

func (e *ChangeEntity) Validate() error {
	return nil
}

func (c change) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ChangeEntity, error) {
	var entity ChangeEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeEntity{}, err
	}

	return entity, nil
}

func (c change) Lock(ctx context.Context, db storage.Executor, id uuid.UUID) (ChangeEntity, error) {
	var entity ChangeEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).For("UPDATE").Scan(ctx)
	return entity, err
}

type CreateChangeData struct {
	Sequence          int64
	Kind              string
	TriggerType       string
	ActorType         string
	ActorID           *uuid.UUID
	CauseSystem       sql.NullString
	CauseReference    sql.NullString
	CorrelationID     uuid.UUID
	CorrectionContext json.RawMessage
	Summary           string
	Status            string
	RequestedAt       time.Time
	CommittedAt       sql.NullTime
	StartedAt         sql.NullTime
	FinishedAt        sql.NullTime
	CancelledAt       sql.NullTime
	Error             sql.NullString
	EnvironmentID     uuid.UUID
	CorrectsChangeID  *uuid.UUID
}

func (c change) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeData,
) (ChangeEntity, error) {
	entity := ChangeEntity{
		ID:                uuid.New(),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Sequence:          data.Sequence,
		Kind:              data.Kind,
		TriggerType:       data.TriggerType,
		ActorType:         data.ActorType,
		ActorID:           data.ActorID,
		CauseSystem:       data.CauseSystem,
		CauseReference:    data.CauseReference,
		CorrelationID:     data.CorrelationID,
		CorrectionContext: data.CorrectionContext,
		Summary:           data.Summary,
		Status:            data.Status,
		RequestedAt:       data.RequestedAt,
		CommittedAt:       data.CommittedAt,
		StartedAt:         data.StartedAt,
		FinishedAt:        data.FinishedAt,
		CancelledAt:       data.CancelledAt,
		Error:             data.Error,
		EnvironmentID:     data.EnvironmentID,
		CorrectsChangeID:  data.CorrectsChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeEntity{}, err
	}

	return entity, nil
}

type UpdateChangeData struct {
	ID                uuid.UUID
	UpdatedAt         time.Time
	Sequence          int64
	Kind              string
	TriggerType       string
	ActorType         string
	ActorID           *uuid.UUID
	CauseSystem       sql.NullString
	CauseReference    sql.NullString
	CorrelationID     uuid.UUID
	CorrectionContext json.RawMessage
	Summary           string
	Status            string
	RequestedAt       time.Time
	CommittedAt       sql.NullTime
	StartedAt         sql.NullTime
	FinishedAt        sql.NullTime
	CancelledAt       sql.NullTime
	Error             sql.NullString
	EnvironmentID     uuid.UUID
	CorrectsChangeID  *uuid.UUID
}

func (c change) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateChangeData,
) (ChangeEntity, error) {
	entity := ChangeEntity{
		ID:                data.ID,
		UpdatedAt:         time.Now(),
		Sequence:          data.Sequence,
		Kind:              data.Kind,
		TriggerType:       data.TriggerType,
		ActorType:         data.ActorType,
		ActorID:           data.ActorID,
		CauseSystem:       data.CauseSystem,
		CauseReference:    data.CauseReference,
		CorrelationID:     data.CorrelationID,
		CorrectionContext: data.CorrectionContext,
		Summary:           data.Summary,
		Status:            data.Status,
		RequestedAt:       data.RequestedAt,
		CommittedAt:       data.CommittedAt,
		StartedAt:         data.StartedAt,
		FinishedAt:        data.FinishedAt,
		CancelledAt:       data.CancelledAt,
		Error:             data.Error,
		EnvironmentID:     data.EnvironmentID,
		CorrectsChangeID:  data.CorrectsChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("sequence").
		Column("kind").
		Column("trigger_type").
		Column("actor_type").
		Column("actor_id").
		Column("cause_system").
		Column("cause_reference").
		Column("correlation_id").
		Column("correction_context").
		Column("summary").
		Column("status").
		Column("requested_at").
		Column("committed_at").
		Column("started_at").
		Column("finished_at").
		Column("cancelled_at").
		Column("error").
		Column("environment_id").
		Column("corrects_change_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeEntity{}, err
	}

	return entity, nil
}

func (c change) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ChangeEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (c change) All(ctx context.Context, db storage.Executor) ([]ChangeEntity, error) {
	var entities []ChangeEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChanges struct {
	Changes    []ChangeEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (c change) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedChanges, error) {
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
		Model(&ChangeEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChanges{}, err
	}

	entities := make([]ChangeEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChanges{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChanges{
		Changes:    entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (c change) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeData,
) (ChangeEntity, error) {
	entity := ChangeEntity{
		ID:                uuid.New(),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Sequence:          data.Sequence,
		Kind:              data.Kind,
		TriggerType:       data.TriggerType,
		ActorType:         data.ActorType,
		ActorID:           data.ActorID,
		CauseSystem:       data.CauseSystem,
		CauseReference:    data.CauseReference,
		CorrelationID:     data.CorrelationID,
		CorrectionContext: data.CorrectionContext,
		Summary:           data.Summary,
		Status:            data.Status,
		RequestedAt:       data.RequestedAt,
		CommittedAt:       data.CommittedAt,
		StartedAt:         data.StartedAt,
		FinishedAt:        data.FinishedAt,
		CancelledAt:       data.CancelledAt,
		Error:             data.Error,
		EnvironmentID:     data.EnvironmentID,
		CorrectsChangeID:  data.CorrectsChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("sequence = excluded.sequence").
		Set("kind = excluded.kind").
		Set("trigger_type = excluded.trigger_type").
		Set("actor_type = excluded.actor_type").
		Set("actor_id = excluded.actor_id").
		Set("cause_system = excluded.cause_system").
		Set("cause_reference = excluded.cause_reference").
		Set("correlation_id = excluded.correlation_id").
		Set("correction_context = excluded.correction_context").
		Set("summary = excluded.summary").
		Set("status = excluded.status").
		Set("requested_at = excluded.requested_at").
		Set("committed_at = excluded.committed_at").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("cancelled_at = excluded.cancelled_at").
		Set("error = excluded.error").
		Set("environment_id = excluded.environment_id").
		Set("corrects_change_id = excluded.corrects_change_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeEntity{}, err
	}

	return entity, nil
}

func (c change) NextSequence(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (int64, error) {
	var sequence int64
	err := db.NewSelect().
		TableExpr("changes").
		ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("environment_id = ?", environmentID).
		Scan(ctx, &sequence)
	return sequence, err
}

func (c change) RecordProgress(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	status string,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("changes").
		Set("status = ?", status).
		Set("started_at = COALESCE(started_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (c change) MarkRunning(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("changes").
		Set("status = ?", "running").
		Set("started_at = COALESCE(started_at, ?)", at).
		Set("finished_at = NULL").
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (c change) MarkCompleted(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("changes").
		Set("status = ?", "completed").
		Set("finished_at = ?", at).
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (c change) MarkFailed(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	operationErr error,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("changes").
		Set("status = ?", "failed").
		Set("finished_at = ?", at).
		Set("error = ?", operationErr.Error()).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (c change) MarkBuildCancelled(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("changes").
		Set("status = CASE WHEN kind = 'environment_setup' THEN 'committed' ELSE 'cancelled' END").
		Set("started_at = CASE WHEN kind = 'environment_setup' THEN NULL ELSE started_at END").
		Set("finished_at = CASE WHEN kind = 'environment_setup' THEN NULL ELSE CAST(? AS TIMESTAMPTZ) END", at).
		Set("cancelled_at = CASE WHEN kind = 'environment_setup' THEN NULL ELSE CAST(? AS TIMESTAMPTZ) END", at).
		Set("error = CASE WHEN kind = 'environment_setup' THEN NULL ELSE 'Build cancelled by user' END").
		Set("updated_at = ?", at).
		Where("id = ?", id).Exec(ctx)
	return err
}

func (c change) ResetBuildForRetry(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("changes").Set("status = 'committed'").
		Set("started_at = NULL").Set("finished_at = NULL").Set("cancelled_at = NULL").
		Set("error = NULL").Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}

func (c change) ResetForRetry(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("changes").Set("status = 'committed'").
		Set("started_at = NULL").Set("finished_at = NULL").Set("cancelled_at = NULL").
		Set("error = NULL").Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}

func (c change) FinishSystemUpdate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	status string,
	failure sql.NullString,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("changes").
		Set("status = ?", status).
		Set("finished_at = ?", at).
		Set("error = ?", failure).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}
