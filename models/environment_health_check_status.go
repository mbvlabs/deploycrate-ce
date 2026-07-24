package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentHealthCheckStatusEntity struct {
	bun.BaseModel       `bun:"table:environment_health_check_statuses,alias:environment_health_check_statuses"`
	ID                  int32          `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time      `bun:"created_at"`
	UpdatedAt           time.Time      `bun:"updated_at"`
	State               string         `bun:"state"`
	StatusCode          sql.NullInt32  `bun:"status_code"`
	DurationMs          int32          `bun:"duration_ms"`
	Error               sql.NullString `bun:"error"`
	ObservedAt          time.Time      `bun:"observed_at"`
	HealthCheckID       uuid.UUID      `bun:"health_check_id,type:uuid"`
	EnvironmentTargetID *uuid.UUID     `bun:"environment_target_id,type:uuid"`
	InstanceID          *uuid.UUID     `bun:"instance_id,type:uuid"`
	ReleaseID           *uuid.UUID     `bun:"release_id,type:uuid"`
}

func (e *EnvironmentHealthCheckStatusEntity) Validate() error {
	return nil
}

func (ehcs environmentHealthCheckStatus) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (EnvironmentHealthCheckStatusEntity, error) {
	var entity EnvironmentHealthCheckStatusEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentHealthCheckStatusData struct {
	State               string
	StatusCode          sql.NullInt32
	DurationMs          int32
	Error               sql.NullString
	ObservedAt          time.Time
	HealthCheckID       uuid.UUID
	EnvironmentTargetID *uuid.UUID
	InstanceID          *uuid.UUID
	ReleaseID           *uuid.UUID
}

func (ehcs environmentHealthCheckStatus) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentHealthCheckStatusData,
) (EnvironmentHealthCheckStatusEntity, error) {
	entity := EnvironmentHealthCheckStatusEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		State:               data.State,
		StatusCode:          data.StatusCode,
		DurationMs:          data.DurationMs,
		Error:               data.Error,
		ObservedAt:          data.ObservedAt,
		HealthCheckID:       data.HealthCheckID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		InstanceID:          data.InstanceID,
		ReleaseID:           data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentHealthCheckStatusData struct {
	ID                  int32
	UpdatedAt           time.Time
	State               string
	StatusCode          sql.NullInt32
	DurationMs          int32
	Error               sql.NullString
	ObservedAt          time.Time
	HealthCheckID       uuid.UUID
	EnvironmentTargetID *uuid.UUID
	InstanceID          *uuid.UUID
	ReleaseID           *uuid.UUID
}

func (ehcs environmentHealthCheckStatus) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentHealthCheckStatusData,
) (EnvironmentHealthCheckStatusEntity, error) {
	entity := EnvironmentHealthCheckStatusEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		State:               data.State,
		StatusCode:          data.StatusCode,
		DurationMs:          data.DurationMs,
		Error:               data.Error,
		ObservedAt:          data.ObservedAt,
		HealthCheckID:       data.HealthCheckID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		InstanceID:          data.InstanceID,
		ReleaseID:           data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("state").
		Column("status_code").
		Column("duration_ms").
		Column("error").
		Column("observed_at").
		Column("health_check_id").
		Column("environment_target_id").
		Column("instance_id").
		Column("release_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, err
	}

	return entity, nil
}

func (ehcs environmentHealthCheckStatus) Destroy(
	ctx context.Context,
	db storage.Executor,
	id int32,
) error {
	_, err := db.NewDelete().
		Model((*EnvironmentHealthCheckStatusEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ehcs environmentHealthCheckStatus) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentHealthCheckStatusEntity, error) {
	var entities []EnvironmentHealthCheckStatusEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentHealthCheckStatuses struct {
	EnvironmentHealthCheckStatuses []EnvironmentHealthCheckStatusEntity
	TotalCount                     int64
	Page                           int64
	PageSize                       int64
	TotalPages                     int64
}

func (ehcs environmentHealthCheckStatus) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentHealthCheckStatuses, error) {
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
		Model(&EnvironmentHealthCheckStatusEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentHealthCheckStatuses{}, err
	}

	entities := make([]EnvironmentHealthCheckStatusEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentHealthCheckStatuses{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentHealthCheckStatuses{
		EnvironmentHealthCheckStatuses: entities,
		TotalCount:                     int64(totalCount),
		Page:                           page,
		PageSize:                       pageSize,
		TotalPages:                     totalPages,
	}, nil
}

func (ehcs environmentHealthCheckStatus) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentHealthCheckStatusData,
) (EnvironmentHealthCheckStatusEntity, error) {
	entity := EnvironmentHealthCheckStatusEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		State:               data.State,
		StatusCode:          data.StatusCode,
		DurationMs:          data.DurationMs,
		Error:               data.Error,
		ObservedAt:          data.ObservedAt,
		HealthCheckID:       data.HealthCheckID,
		EnvironmentTargetID: data.EnvironmentTargetID,
		InstanceID:          data.InstanceID,
		ReleaseID:           data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("state = excluded.state").
		Set("status_code = excluded.status_code").
		Set("duration_ms = excluded.duration_ms").
		Set("error = excluded.error").
		Set("observed_at = excluded.observed_at").
		Set("health_check_id = excluded.health_check_id").
		Set("environment_target_id = excluded.environment_target_id").
		Set("instance_id = excluded.instance_id").
		Set("release_id = excluded.release_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentHealthCheckStatusEntity{}, err
	}

	return entity, nil
}
