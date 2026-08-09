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

type DeploymentEventEntity struct {
	bun.BaseModel       `bun:"table:deployment_events,alias:deployment_events"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	Sequence            int64           `bun:"sequence"`
	EventType           string          `bun:"event_type"`
	Status              sql.NullString  `bun:"status"`
	Step                sql.NullString  `bun:"step"`
	Message             string          `bun:"message"`
	Metadata            json.RawMessage `bun:"metadata,type:jsonb"`
	Error               sql.NullString  `bun:"error"`
	OccurredAt          time.Time       `bun:"occurred_at"`
	DeploymentID        uuid.UUID       `bun:"deployment_id,type:uuid"`
	ChangeTaskAttemptID *uuid.UUID      `bun:"change_task_attempt_id,type:uuid"`
}

func (deploymentEvent) NextSequence(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
) (int64, error) {
	var sequence int64
	err := db.NewSelect().TableExpr("deployment_events").
		ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("deployment_id = ?", deploymentID).Scan(ctx, &sequence)
	return sequence, err
}

func (e *DeploymentEventEntity) Validate() error {
	return nil
}

func (de deploymentEvent) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (DeploymentEventEntity, error) {
	var entity DeploymentEventEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return DeploymentEventEntity{}, err
	}

	return entity, nil
}

type CreateDeploymentEventData struct {
	Sequence            int64
	EventType           string
	Status              sql.NullString
	Step                sql.NullString
	Message             string
	Metadata            json.RawMessage
	Error               sql.NullString
	OccurredAt          time.Time
	DeploymentID        uuid.UUID
	ChangeTaskAttemptID *uuid.UUID
}

func (de deploymentEvent) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateDeploymentEventData,
) (DeploymentEventEntity, error) {
	entity := DeploymentEventEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Sequence:            data.Sequence,
		EventType:           data.EventType,
		Status:              data.Status,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		Error:               data.Error,
		OccurredAt:          data.OccurredAt,
		DeploymentID:        data.DeploymentID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DeploymentEventEntity{}, err
	}

	return entity, nil
}

type UpdateDeploymentEventData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	Sequence            int64
	EventType           string
	Status              sql.NullString
	Step                sql.NullString
	Message             string
	Metadata            json.RawMessage
	Error               sql.NullString
	OccurredAt          time.Time
	DeploymentID        uuid.UUID
	ChangeTaskAttemptID *uuid.UUID
}

func (de deploymentEvent) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateDeploymentEventData,
) (DeploymentEventEntity, error) {
	entity := DeploymentEventEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		Sequence:            data.Sequence,
		EventType:           data.EventType,
		Status:              data.Status,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		Error:               data.Error,
		OccurredAt:          data.OccurredAt,
		DeploymentID:        data.DeploymentID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("sequence").
		Column("event_type").
		Column("status").
		Column("step").
		Column("message").
		Column("metadata").
		Column("error").
		Column("occurred_at").
		Column("deployment_id").
		Column("change_task_attempt_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return DeploymentEventEntity{}, err
	}

	return entity, nil
}

func (de deploymentEvent) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*DeploymentEventEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (de deploymentEvent) All(
	ctx context.Context,
	db storage.Executor,
) ([]DeploymentEventEntity, error) {
	var entities []DeploymentEventEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

func (de deploymentEvent) ForDeploymentAfter(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
	after int64,
	limit int,
) ([]DeploymentEventEntity, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 501 {
		limit = 501
	}
	events := make([]DeploymentEventEntity, 0, limit)
	err := db.NewSelect().
		Model(&events).
		Where("deployment_id = ?", deploymentID).
		Where("sequence > ?", after).
		OrderExpr("sequence ASC").
		Limit(limit).
		Scan(ctx)
	return events, err
}

type PaginatedDeploymentEvents struct {
	DeploymentEvents []DeploymentEventEntity
	TotalCount       int64
	Page             int64
	PageSize         int64
	TotalPages       int64
}

func (de deploymentEvent) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedDeploymentEvents, error) {
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
		Model(&DeploymentEventEntity{}).Count(ctx)
	if err != nil {
		return PaginatedDeploymentEvents{}, err
	}

	entities := make([]DeploymentEventEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedDeploymentEvents{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedDeploymentEvents{
		DeploymentEvents: entities,
		TotalCount:       int64(totalCount),
		Page:             page,
		PageSize:         pageSize,
		TotalPages:       totalPages,
	}, nil
}

func (de deploymentEvent) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateDeploymentEventData,
) (DeploymentEventEntity, error) {
	entity := DeploymentEventEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Sequence:            data.Sequence,
		EventType:           data.EventType,
		Status:              data.Status,
		Step:                data.Step,
		Message:             data.Message,
		Metadata:            data.Metadata,
		Error:               data.Error,
		OccurredAt:          data.OccurredAt,
		DeploymentID:        data.DeploymentID,
		ChangeTaskAttemptID: data.ChangeTaskAttemptID,
	}

	if err := validation.Validate(&entity); err != nil {
		return DeploymentEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("sequence = excluded.sequence").
		Set("event_type = excluded.event_type").
		Set("status = excluded.status").
		Set("step = excluded.step").
		Set("message = excluded.message").
		Set("metadata = excluded.metadata").
		Set("error = excluded.error").
		Set("occurred_at = excluded.occurred_at").
		Set("deployment_id = excluded.deployment_id").
		Set("change_task_attempt_id = excluded.change_task_attempt_id").
		Returning("*").
		Scan(ctx); err != nil {
		return DeploymentEventEntity{}, err
	}

	return entity, nil
}
