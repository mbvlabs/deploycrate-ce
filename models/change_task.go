package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ChangeTaskEntity struct {
	bun.BaseModel       `bun:"table:change_tasks,alias:change_tasks"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ChangeID            uuid.UUID       `bun:"change_id,type:uuid"`
	ParentTaskID        *uuid.UUID      `bun:"parent_task_id,type:uuid"`
	ServerID            *uuid.UUID      `bun:"server_id,type:uuid"`
	EnvironmentTargetID *uuid.UUID      `bun:"environment_target_id,type:uuid"`
	Kind                string          `bun:"kind"`
	SubjectType         string          `bun:"subject_type"`
	SubjectID           uuid.UUID       `bun:"subject_id,type:uuid"`
	IdempotencyKey      string          `bun:"idempotency_key"`
	Input               json.RawMessage `bun:"input,type:jsonb"`
	Status              string          `bun:"status"`
	AttemptCount        int32           `bun:"attempt_count"`
	AvailableAt         time.Time       `bun:"available_at"`
}

func (e *ChangeTaskEntity) Validate() error {
	return nil
}

func (ct changeTask) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ChangeTaskEntity, error) {
	var entity ChangeTaskEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeTaskEntity{}, err
	}

	return entity, nil
}

type CreateChangeTaskData struct {
	ChangeID            uuid.UUID
	ParentTaskID        *uuid.UUID
	ServerID            *uuid.UUID
	EnvironmentTargetID *uuid.UUID
	Kind                string
	SubjectType         string
	SubjectID           uuid.UUID
	IdempotencyKey      string
	Input               json.RawMessage
	Status              string
	AttemptCount        int32
	AvailableAt         time.Time
}

func (ct changeTask) Create(ctx context.Context, db storage.Executor, data CreateChangeTaskData) (ChangeTaskEntity, error) {
	entity := ChangeTaskEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ChangeID:            data.ChangeID,
		ParentTaskID:        data.ParentTaskID,
		ServerID:            data.ServerID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		Kind:                data.Kind,
		SubjectType:         data.SubjectType,
		SubjectID:           data.SubjectID,
		IdempotencyKey:      data.IdempotencyKey,
		Input:               data.Input,
		Status:              data.Status,
		AttemptCount:        data.AttemptCount,
		AvailableAt:         data.AvailableAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeTaskEntity{}, err
	}

	return entity, nil
}

type UpdateChangeTaskData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	ChangeID            uuid.UUID
	ParentTaskID        *uuid.UUID
	ServerID            *uuid.UUID
	EnvironmentTargetID *uuid.UUID
	Kind                string
	SubjectType         string
	SubjectID           uuid.UUID
	IdempotencyKey      string
	Input               json.RawMessage
	Status              string
	AttemptCount        int32
	AvailableAt         time.Time
}

func (ct changeTask) Update(ctx context.Context, db storage.Executor, data UpdateChangeTaskData) (ChangeTaskEntity, error) {
	entity := ChangeTaskEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ChangeID:            data.ChangeID,
		ParentTaskID:        data.ParentTaskID,
		ServerID:            data.ServerID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		Kind:                data.Kind,
		SubjectType:         data.SubjectType,
		SubjectID:           data.SubjectID,
		IdempotencyKey:      data.IdempotencyKey,
		Input:               data.Input,
		Status:              data.Status,
		AttemptCount:        data.AttemptCount,
		AvailableAt:         data.AvailableAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("change_id").
		Column("parent_task_id").
		Column("server_id").
		Column("environment_target_id").
		Column("kind").
		Column("subject_type").
		Column("subject_id").
		Column("idempotency_key").
		Column("input").
		Column("status").
		Column("attempt_count").
		Column("available_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeTaskEntity{}, err
	}

	return entity, nil
}

func (ct changeTask) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ChangeTaskEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ct changeTask) All(ctx context.Context, db storage.Executor) ([]ChangeTaskEntity, error) {
	var entities []ChangeTaskEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeTasks struct {
	ChangeTasks []ChangeTaskEntity
	TotalCount  int64
	Page        int64
	PageSize    int64
	TotalPages  int64
}

func (ct changeTask) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedChangeTasks, error) {
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
		Model(&ChangeTaskEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeTasks{}, err
	}

	entities := make([]ChangeTaskEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeTasks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeTasks{
		ChangeTasks: entities,
		TotalCount:  int64(totalCount),
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	}, nil
}

func (ct changeTask) Upsert(ctx context.Context, db storage.Executor, data CreateChangeTaskData) (ChangeTaskEntity, error) {
	entity := ChangeTaskEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ChangeID:            data.ChangeID,
		ParentTaskID:        data.ParentTaskID,
		ServerID:            data.ServerID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		Kind:                data.Kind,
		SubjectType:         data.SubjectType,
		SubjectID:           data.SubjectID,
		IdempotencyKey:      data.IdempotencyKey,
		Input:               data.Input,
		Status:              data.Status,
		AttemptCount:        data.AttemptCount,
		AvailableAt:         data.AvailableAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeTaskEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("change_id = excluded.change_id").
		Set("parent_task_id = excluded.parent_task_id").
		Set("server_id = excluded.server_id").
		Set("environment_target_id = excluded.environment_target_id").
		Set("kind = excluded.kind").
		Set("subject_type = excluded.subject_type").
		Set("subject_id = excluded.subject_id").
		Set("idempotency_key = excluded.idempotency_key").
		Set("input = excluded.input").
		Set("status = excluded.status").
		Set("attempt_count = excluded.attempt_count").
		Set("available_at = excluded.available_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeTaskEntity{}, err
	}

	return entity, nil
}
