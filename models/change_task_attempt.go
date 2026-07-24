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

type ChangeTaskAttemptEntity struct {
	bun.BaseModel   `bun:"table:change_task_attempts,alias:change_task_attempts"`
	ID              uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt       time.Time       `bun:"created_at"`
	UpdatedAt       time.Time       `bun:"updated_at"`
	Attempt         int32           `bun:"attempt"`
	Status          string          `bun:"status"`
	StartedAt       time.Time       `bun:"started_at"`
	LastHeartbeatAt sql.NullTime    `bun:"last_heartbeat_at"`
	FinishedAt      sql.NullTime    `bun:"finished_at"`
	Result          json.RawMessage `bun:"result,type:jsonb"`
	Error           sql.NullString  `bun:"error"`
	ChangeTaskID    uuid.UUID       `bun:"change_task_id,type:uuid"`
}

func (e *ChangeTaskAttemptEntity) Validate() error {
	return nil
}

func (cta changeTaskAttempt) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ChangeTaskAttemptEntity, error) {
	var entity ChangeTaskAttemptEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeTaskAttemptEntity{}, err
	}

	return entity, nil
}

type CreateChangeTaskAttemptData struct {
	Attempt         int32
	Status          string
	StartedAt       time.Time
	LastHeartbeatAt sql.NullTime
	FinishedAt      sql.NullTime
	Result          json.RawMessage
	Error           sql.NullString
	ChangeTaskID    uuid.UUID
}

func (cta changeTaskAttempt) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeTaskAttemptData,
) (ChangeTaskAttemptEntity, error) {
	entity := ChangeTaskAttemptEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Attempt:         data.Attempt,
		Status:          data.Status,
		StartedAt:       data.StartedAt,
		LastHeartbeatAt: data.LastHeartbeatAt,
		FinishedAt:      data.FinishedAt,
		Result:          data.Result,
		Error:           data.Error,
		ChangeTaskID:    data.ChangeTaskID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskAttemptEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeTaskAttemptEntity{}, err
	}

	return entity, nil
}

type UpdateChangeTaskAttemptData struct {
	ID              uuid.UUID
	UpdatedAt       time.Time
	Attempt         int32
	Status          string
	StartedAt       time.Time
	LastHeartbeatAt sql.NullTime
	FinishedAt      sql.NullTime
	Result          json.RawMessage
	Error           sql.NullString
	ChangeTaskID    uuid.UUID
}

func (cta changeTaskAttempt) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateChangeTaskAttemptData,
) (ChangeTaskAttemptEntity, error) {
	entity := ChangeTaskAttemptEntity{
		ID:              data.ID,
		UpdatedAt:       time.Now(),
		Attempt:         data.Attempt,
		Status:          data.Status,
		StartedAt:       data.StartedAt,
		LastHeartbeatAt: data.LastHeartbeatAt,
		FinishedAt:      data.FinishedAt,
		Result:          data.Result,
		Error:           data.Error,
		ChangeTaskID:    data.ChangeTaskID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskAttemptEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("attempt").
		Column("status").
		Column("started_at").
		Column("last_heartbeat_at").
		Column("finished_at").
		Column("result").
		Column("error").
		Column("change_task_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeTaskAttemptEntity{}, err
	}

	return entity, nil
}

func (cta changeTaskAttempt) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ChangeTaskAttemptEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (cta changeTaskAttempt) All(
	ctx context.Context,
	db storage.Executor,
) ([]ChangeTaskAttemptEntity, error) {
	var entities []ChangeTaskAttemptEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeTaskAttempts struct {
	ChangeTaskAttempts []ChangeTaskAttemptEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (cta changeTaskAttempt) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedChangeTaskAttempts, error) {
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
		Model(&ChangeTaskAttemptEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeTaskAttempts{}, err
	}

	entities := make([]ChangeTaskAttemptEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeTaskAttempts{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeTaskAttempts{
		ChangeTaskAttempts: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (cta changeTaskAttempt) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeTaskAttemptData,
) (ChangeTaskAttemptEntity, error) {
	entity := ChangeTaskAttemptEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Attempt:         data.Attempt,
		Status:          data.Status,
		StartedAt:       data.StartedAt,
		LastHeartbeatAt: data.LastHeartbeatAt,
		FinishedAt:      data.FinishedAt,
		Result:          data.Result,
		Error:           data.Error,
		ChangeTaskID:    data.ChangeTaskID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskAttemptEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("attempt = excluded.attempt").
		Set("status = excluded.status").
		Set("started_at = excluded.started_at").
		Set("last_heartbeat_at = excluded.last_heartbeat_at").
		Set("finished_at = excluded.finished_at").
		Set("result = excluded.result").
		Set("error = excluded.error").
		Set("change_task_id = excluded.change_task_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeTaskAttemptEntity{}, err
	}

	return entity, nil
}
