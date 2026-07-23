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

type ResourceHealthCheckStatusEntity struct {
	bun.BaseModel        `bun:"table:resource_health_check_statuses,alias:resource_health_check_statuses"`
	CreatedAt            time.Time       `bun:"created_at"`
	UpdatedAt            time.Time       `bun:"updated_at"`
	State                string          `bun:"state"`
	StatusCode           sql.NullInt32   `bun:"status_code"`
	LatencyMs            sql.NullInt32   `bun:"latency_ms"`
	Message              sql.NullString  `bun:"message"`
	ConsecutiveSuccesses int32           `bun:"consecutive_successes"`
	ConsecutiveFailures  int32           `bun:"consecutive_failures"`
	Details              json.RawMessage `bun:"details,type:jsonb"`
	ObservedAt           time.Time       `bun:"observed_at"`
	ExpiresAt            time.Time       `bun:"expires_at"`
	HealthCheckID        uuid.UUID       `bun:"health_check_id,pk,type:uuid"`
}

func (e *ResourceHealthCheckStatusEntity) Validate() error {
	return nil
}

func (rhcs resourceHealthCheckStatus) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceHealthCheckStatusEntity, error) {
	var entity ResourceHealthCheckStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("health_check_id = ?", id).
		Scan(ctx); err != nil {
		return ResourceHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

type CreateResourceHealthCheckStatusData struct {
	HealthCheckID        uuid.UUID
	State                string
	StatusCode           sql.NullInt32
	LatencyMs            sql.NullInt32
	Message              sql.NullString
	ConsecutiveSuccesses int32
	ConsecutiveFailures  int32
	Details              json.RawMessage
	ObservedAt           time.Time
	ExpiresAt            time.Time
}

func (rhcs resourceHealthCheckStatus) Create(ctx context.Context, db storage.Executor, data CreateResourceHealthCheckStatusData) (ResourceHealthCheckStatusEntity, error) {
	entity := ResourceHealthCheckStatusEntity{
		HealthCheckID:        data.HealthCheckID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		State:                data.State,
		StatusCode:           data.StatusCode,
		LatencyMs:            data.LatencyMs,
		Message:              data.Message,
		ConsecutiveSuccesses: data.ConsecutiveSuccesses,
		ConsecutiveFailures:  data.ConsecutiveFailures,
		Details:              data.Details,
		ObservedAt:           data.ObservedAt,
		ExpiresAt:            data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

type UpdateResourceHealthCheckStatusData struct {
	HealthCheckID        uuid.UUID
	UpdatedAt            time.Time
	State                string
	StatusCode           sql.NullInt32
	LatencyMs            sql.NullInt32
	Message              sql.NullString
	ConsecutiveSuccesses int32
	ConsecutiveFailures  int32
	Details              json.RawMessage
	ObservedAt           time.Time
	ExpiresAt            time.Time
}

func (rhcs resourceHealthCheckStatus) Update(ctx context.Context, db storage.Executor, data UpdateResourceHealthCheckStatusData) (ResourceHealthCheckStatusEntity, error) {
	entity := ResourceHealthCheckStatusEntity{
		HealthCheckID:        data.HealthCheckID,
		UpdatedAt:            time.Now(),
		State:                data.State,
		StatusCode:           data.StatusCode,
		LatencyMs:            data.LatencyMs,
		Message:              data.Message,
		ConsecutiveSuccesses: data.ConsecutiveSuccesses,
		ConsecutiveFailures:  data.ConsecutiveFailures,
		Details:              data.Details,
		ObservedAt:           data.ObservedAt,
		ExpiresAt:            data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("state").
		Column("status_code").
		Column("latency_ms").
		Column("message").
		Column("consecutive_successes").
		Column("consecutive_failures").
		Column("details").
		Column("observed_at").
		Column("expires_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

func (rhcs resourceHealthCheckStatus) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceHealthCheckStatusEntity)(nil)).
		Where("health_check_id = ?", id).
		Exec(ctx)

	return err
}

func (rhcs resourceHealthCheckStatus) All(ctx context.Context, db storage.Executor) ([]ResourceHealthCheckStatusEntity, error) {
	var entities []ResourceHealthCheckStatusEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceHealthCheckStatuses struct {
	ResourceHealthCheckStatuses []ResourceHealthCheckStatusEntity
	TotalCount                  int64
	Page                        int64
	PageSize                    int64
	TotalPages                  int64
}

func (rhcs resourceHealthCheckStatus) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceHealthCheckStatuses, error) {
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
		Model(&ResourceHealthCheckStatusEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceHealthCheckStatuses{}, err
	}

	entities := make([]ResourceHealthCheckStatusEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceHealthCheckStatuses{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceHealthCheckStatuses{
		ResourceHealthCheckStatuses: entities,
		TotalCount:                  int64(totalCount),
		Page:                        page,
		PageSize:                    pageSize,
		TotalPages:                  totalPages,
	}, nil
}

func (rhcs resourceHealthCheckStatus) Upsert(ctx context.Context, db storage.Executor, data CreateResourceHealthCheckStatusData) (ResourceHealthCheckStatusEntity, error) {
	entity := ResourceHealthCheckStatusEntity{
		HealthCheckID:        data.HealthCheckID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		State:                data.State,
		StatusCode:           data.StatusCode,
		LatencyMs:            data.LatencyMs,
		Message:              data.Message,
		ConsecutiveSuccesses: data.ConsecutiveSuccesses,
		ConsecutiveFailures:  data.ConsecutiveFailures,
		Details:              data.Details,
		ObservedAt:           data.ObservedAt,
		ExpiresAt:            data.ExpiresAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (health_check_id) DO UPDATE").
		Set("state = excluded.state").
		Set("status_code = excluded.status_code").
		Set("latency_ms = excluded.latency_ms").
		Set("message = excluded.message").
		Set("consecutive_successes = excluded.consecutive_successes").
		Set("consecutive_failures = excluded.consecutive_failures").
		Set("details = excluded.details").
		Set("observed_at = excluded.observed_at").
		Set("expires_at = excluded.expires_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckStatusEntity{}, err
	}

	return entity, nil
}
