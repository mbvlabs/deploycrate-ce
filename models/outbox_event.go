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

type OutboxEventEntity struct {
	bun.BaseModel   `bun:"table:outbox_events,alias:outbox_events"`
	ID              uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt       time.Time       `bun:"created_at"`
	UpdatedAt       time.Time       `bun:"updated_at"`
	AggregateType   string          `bun:"aggregate_type"`
	AggregateID     uuid.UUID       `bun:"aggregate_id,type:uuid"`
	EventType       string          `bun:"event_type"`
	SchemaVersion   int32           `bun:"schema_version"`
	CorrelationID   uuid.UUID       `bun:"correlation_id,type:uuid"`
	CausationID     uuid.UUID       `bun:"causation_id,type:uuid"`
	Payload         json.RawMessage `bun:"payload,type:jsonb"`
	OccurredAt      time.Time       `bun:"occurred_at"`
	PublishedAt     sql.NullTime    `bun:"published_at"`
	PublishAttempts int32           `bun:"publish_attempts"`
	LastError       sql.NullString  `bun:"last_error"`
}

func (e *OutboxEventEntity) Validate() error {
	return nil
}

func (oe outboxEvent) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (OutboxEventEntity, error) {
	var entity OutboxEventEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return OutboxEventEntity{}, err
	}

	return entity, nil
}

type CreateOutboxEventData struct {
	AggregateType   string
	AggregateID     uuid.UUID
	EventType       string
	SchemaVersion   int32
	CorrelationID   uuid.UUID
	CausationID     uuid.UUID
	Payload         json.RawMessage
	OccurredAt      time.Time
	PublishedAt     sql.NullTime
	PublishAttempts int32
	LastError       sql.NullString
}

func (oe outboxEvent) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateOutboxEventData,
) (OutboxEventEntity, error) {
	entity := OutboxEventEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		AggregateType:   data.AggregateType,
		AggregateID:     data.AggregateID,
		EventType:       data.EventType,
		SchemaVersion:   data.SchemaVersion,
		CorrelationID:   data.CorrelationID,
		CausationID:     data.CausationID,
		Payload:         data.Payload,
		OccurredAt:      data.OccurredAt,
		PublishedAt:     data.PublishedAt,
		PublishAttempts: data.PublishAttempts,
		LastError:       data.LastError,
	}

	if err := validation.Validate(&entity); err != nil {
		return OutboxEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return OutboxEventEntity{}, err
	}

	return entity, nil
}

type UpdateOutboxEventData struct {
	ID              uuid.UUID
	UpdatedAt       time.Time
	AggregateType   string
	AggregateID     uuid.UUID
	EventType       string
	SchemaVersion   int32
	CorrelationID   uuid.UUID
	CausationID     uuid.UUID
	Payload         json.RawMessage
	OccurredAt      time.Time
	PublishedAt     sql.NullTime
	PublishAttempts int32
	LastError       sql.NullString
}

func (oe outboxEvent) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateOutboxEventData,
) (OutboxEventEntity, error) {
	entity := OutboxEventEntity{
		ID:              data.ID,
		UpdatedAt:       time.Now(),
		AggregateType:   data.AggregateType,
		AggregateID:     data.AggregateID,
		EventType:       data.EventType,
		SchemaVersion:   data.SchemaVersion,
		CorrelationID:   data.CorrelationID,
		CausationID:     data.CausationID,
		Payload:         data.Payload,
		OccurredAt:      data.OccurredAt,
		PublishedAt:     data.PublishedAt,
		PublishAttempts: data.PublishAttempts,
		LastError:       data.LastError,
	}

	if err := validation.Validate(&entity); err != nil {
		return OutboxEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("aggregate_type").
		Column("aggregate_id").
		Column("event_type").
		Column("schema_version").
		Column("correlation_id").
		Column("causation_id").
		Column("payload").
		Column("occurred_at").
		Column("published_at").
		Column("publish_attempts").
		Column("last_error").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return OutboxEventEntity{}, err
	}

	return entity, nil
}

func (oe outboxEvent) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*OutboxEventEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (oe outboxEvent) All(ctx context.Context, db storage.Executor) ([]OutboxEventEntity, error) {
	var entities []OutboxEventEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedOutboxEvents struct {
	OutboxEvents []OutboxEventEntity
	TotalCount   int64
	Page         int64
	PageSize     int64
	TotalPages   int64
}

func (oe outboxEvent) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedOutboxEvents, error) {
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
		Model(&OutboxEventEntity{}).Count(ctx)
	if err != nil {
		return PaginatedOutboxEvents{}, err
	}

	entities := make([]OutboxEventEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedOutboxEvents{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedOutboxEvents{
		OutboxEvents: entities,
		TotalCount:   int64(totalCount),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (oe outboxEvent) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateOutboxEventData,
) (OutboxEventEntity, error) {
	entity := OutboxEventEntity{
		ID:              uuid.New(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		AggregateType:   data.AggregateType,
		AggregateID:     data.AggregateID,
		EventType:       data.EventType,
		SchemaVersion:   data.SchemaVersion,
		CorrelationID:   data.CorrelationID,
		CausationID:     data.CausationID,
		Payload:         data.Payload,
		OccurredAt:      data.OccurredAt,
		PublishedAt:     data.PublishedAt,
		PublishAttempts: data.PublishAttempts,
		LastError:       data.LastError,
	}

	if err := validation.Validate(&entity); err != nil {
		return OutboxEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("aggregate_type = excluded.aggregate_type").
		Set("aggregate_id = excluded.aggregate_id").
		Set("event_type = excluded.event_type").
		Set("schema_version = excluded.schema_version").
		Set("correlation_id = excluded.correlation_id").
		Set("causation_id = excluded.causation_id").
		Set("payload = excluded.payload").
		Set("occurred_at = excluded.occurred_at").
		Set("published_at = excluded.published_at").
		Set("publish_attempts = excluded.publish_attempts").
		Set("last_error = excluded.last_error").
		Returning("*").
		Scan(ctx); err != nil {
		return OutboxEventEntity{}, err
	}

	return entity, nil
}
