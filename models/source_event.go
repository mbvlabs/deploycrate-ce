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

type SourceEventEntity struct {
	bun.BaseModel       `bun:"table:source_events,alias:source_events"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ExternalID          string          `bun:"external_id"`
	Kind                string          `bun:"kind"`
	SourceRevision      sql.NullString  `bun:"source_revision"`
	Payload             json.RawMessage `bun:"payload,type:jsonb"`
	ReceivedAt          time.Time       `bun:"received_at"`
	ProcessedAt         sql.NullTime    `bun:"processed_at"`
	Error               sql.NullString  `bun:"error"`
	EnvironmentSourceID uuid.UUID       `bun:"environment_source_id,type:uuid"`
}

func (e *SourceEventEntity) Validate() error {
	return nil
}

func (se sourceEvent) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (SourceEventEntity, error) {
	var entity SourceEventEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return SourceEventEntity{}, err
	}

	return entity, nil
}

type CreateSourceEventData struct {
	ExternalID          string
	Kind                string
	SourceRevision      sql.NullString
	Payload             json.RawMessage
	ReceivedAt          time.Time
	ProcessedAt         sql.NullTime
	Error               sql.NullString
	EnvironmentSourceID uuid.UUID
}

func (se sourceEvent) Create(ctx context.Context, db storage.Executor, data CreateSourceEventData) (SourceEventEntity, error) {
	entity := SourceEventEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Kind:                data.Kind,
		SourceRevision:      data.SourceRevision,
		Payload:             data.Payload,
		ReceivedAt:          data.ReceivedAt,
		ProcessedAt:         data.ProcessedAt,
		Error:               data.Error,
		EnvironmentSourceID: data.EnvironmentSourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return SourceEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return SourceEventEntity{}, err
	}

	return entity, nil
}

type UpdateSourceEventData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	ExternalID          string
	Kind                string
	SourceRevision      sql.NullString
	Payload             json.RawMessage
	ReceivedAt          time.Time
	ProcessedAt         sql.NullTime
	Error               sql.NullString
	EnvironmentSourceID uuid.UUID
}

func (se sourceEvent) Update(ctx context.Context, db storage.Executor, data UpdateSourceEventData) (SourceEventEntity, error) {
	entity := SourceEventEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Kind:                data.Kind,
		SourceRevision:      data.SourceRevision,
		Payload:             data.Payload,
		ReceivedAt:          data.ReceivedAt,
		ProcessedAt:         data.ProcessedAt,
		Error:               data.Error,
		EnvironmentSourceID: data.EnvironmentSourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return SourceEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("external_id").
		Column("kind").
		Column("source_revision").
		Column("payload").
		Column("received_at").
		Column("processed_at").
		Column("error").
		Column("environment_source_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return SourceEventEntity{}, err
	}

	return entity, nil
}

func (se sourceEvent) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*SourceEventEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (se sourceEvent) All(ctx context.Context, db storage.Executor) ([]SourceEventEntity, error) {
	var entities []SourceEventEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedSourceEvents struct {
	SourceEvents []SourceEventEntity
	TotalCount   int64
	Page         int64
	PageSize     int64
	TotalPages   int64
}

func (se sourceEvent) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedSourceEvents, error) {
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
		Model(&SourceEventEntity{}).Count(ctx)
	if err != nil {
		return PaginatedSourceEvents{}, err
	}

	entities := make([]SourceEventEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedSourceEvents{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedSourceEvents{
		SourceEvents: entities,
		TotalCount:   int64(totalCount),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (se sourceEvent) Upsert(ctx context.Context, db storage.Executor, data CreateSourceEventData) (SourceEventEntity, error) {
	entity := SourceEventEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ExternalID:          data.ExternalID,
		Kind:                data.Kind,
		SourceRevision:      data.SourceRevision,
		Payload:             data.Payload,
		ReceivedAt:          data.ReceivedAt,
		ProcessedAt:         data.ProcessedAt,
		Error:               data.Error,
		EnvironmentSourceID: data.EnvironmentSourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return SourceEventEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("external_id = excluded.external_id").
		Set("kind = excluded.kind").
		Set("source_revision = excluded.source_revision").
		Set("payload = excluded.payload").
		Set("received_at = excluded.received_at").
		Set("processed_at = excluded.processed_at").
		Set("error = excluded.error").
		Set("environment_source_id = excluded.environment_source_id").
		Returning("*").
		Scan(ctx); err != nil {
		return SourceEventEntity{}, err
	}

	return entity, nil
}
