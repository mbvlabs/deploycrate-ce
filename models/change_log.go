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

type ChangeLogEntity struct {
	bun.BaseModel       `bun:"table:change_logs,alias:change_logs"`
	ID                  int32           `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	OccurredAt          time.Time       `bun:"occurred_at"`
	Level               string          `bun:"level"`
	Step                sql.NullString  `bun:"step"`
	Message             string          `bun:"message"`
	Metadata            json.RawMessage `bun:"metadata,type:jsonb"`
	ChangeID            uuid.UUID       `bun:"change_id,type:uuid"`
	ChangeTaskID        *uuid.UUID      `bun:"change_task_id,type:uuid"`
	ChangeTaskAttemptID *uuid.UUID      `bun:"change_task_attempt_id,type:uuid"`
}

func (e *ChangeLogEntity) Validate() error {
	return nil
}

func (cl changeLog) Find(ctx context.Context, db storage.Executor, id int32) (ChangeLogEntity, error) {
	var entity ChangeLogEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeLogEntity{}, err
	}

	return entity, nil
}

type CreateChangeLogData struct {
	OccurredAt          time.Time
	Level               string
	Step                sql.NullString
	Message             string
	Metadata            json.RawMessage
	ChangeID            uuid.UUID
	ChangeTaskID        *uuid.UUID
	ChangeTaskAttemptID *uuid.UUID
}

func (cl changeLog) Create(ctx context.Context, db storage.Executor, data CreateChangeLogData) (ChangeLogEntity, error) {
	entity := ChangeLogEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		OccurredAt:          data.OccurredAt,
		Level:               data.Level,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		ChangeID:            data.ChangeID,
		ChangeTaskID:        data.ChangeTaskID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeLogEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeLogEntity{}, err
	}

	return entity, nil
}

type UpdateChangeLogData struct {
	ID                  int32
	UpdatedAt           time.Time
	OccurredAt          time.Time
	Level               string
	Step                sql.NullString
	Message             string
	Metadata            json.RawMessage
	ChangeID            uuid.UUID
	ChangeTaskID        *uuid.UUID
	ChangeTaskAttemptID *uuid.UUID
}

func (cl changeLog) Update(ctx context.Context, db storage.Executor, data UpdateChangeLogData) (ChangeLogEntity, error) {
	entity := ChangeLogEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		OccurredAt:          data.OccurredAt,
		Level:               data.Level,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		ChangeID:            data.ChangeID,
		ChangeTaskID:        data.ChangeTaskID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeLogEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("occurred_at").
		Column("level").
		Column("step").
		Column("message").
		Column("metadata").
		Column("change_id").
		Column("change_task_id").
		Column("change_task_attempt_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeLogEntity{}, err
	}

	return entity, nil
}

func (cl changeLog) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ChangeLogEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (cl changeLog) All(ctx context.Context, db storage.Executor) ([]ChangeLogEntity, error) {
	var entities []ChangeLogEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeLogs struct {
	ChangeLogs []ChangeLogEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (cl changeLog) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedChangeLogs, error) {
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
		Model(&ChangeLogEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeLogs{}, err
	}

	entities := make([]ChangeLogEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeLogs{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeLogs{
		ChangeLogs: entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (cl changeLog) Upsert(ctx context.Context, db storage.Executor, data CreateChangeLogData) (ChangeLogEntity, error) {
	entity := ChangeLogEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		OccurredAt:          data.OccurredAt,
		Level:               data.Level,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		ChangeID:            data.ChangeID,
		ChangeTaskID:        data.ChangeTaskID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeLogEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("occurred_at = excluded.occurred_at").
		Set("level = excluded.level").
		Set("step = excluded.step").
		Set("message = excluded.message").
		Set("metadata = excluded.metadata").
		Set("change_id = excluded.change_id").
		Set("change_task_id = excluded.change_task_id").
		Set("change_task_attempt_id = excluded.change_task_attempt_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeLogEntity{}, err
	}

	return entity, nil
}
