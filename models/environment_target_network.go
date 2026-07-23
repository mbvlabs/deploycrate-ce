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

type EnvironmentTargetNetworkEntity struct {
	bun.BaseModel       `bun:"table:environment_target_networks,alias:environment_target_networks"`
	ID                  int32           `bun:"id,pk,autoincrement"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	Driver              string          `bun:"driver"`
	ExternalID          sql.NullString  `bun:"external_id"`
	Configuration       json.RawMessage `bun:"configuration,type:jsonb"`
	State               string          `bun:"state"`
	AppliedAt           sql.NullTime    `bun:"applied_at"`
	ObservedAt          sql.NullTime    `bun:"observed_at"`
	Error               sql.NullString  `bun:"error"`
	RemovedAt           sql.NullTime    `bun:"removed_at"`
	EnvironmentTargetID uuid.UUID       `bun:"environment_target_id,type:uuid"`
	PrivateNetworkID    uuid.UUID       `bun:"private_network_id,type:uuid"`
}

func (e *EnvironmentTargetNetworkEntity) Validate() error {
	return nil
}

func (etn environmentTargetNetwork) Find(ctx context.Context, db storage.Executor, id int32) (EnvironmentTargetNetworkEntity, error) {
	var entity EnvironmentTargetNetworkEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentTargetNetworkEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentTargetNetworkData struct {
	Driver              string
	ExternalID          sql.NullString
	Configuration       json.RawMessage
	State               string
	AppliedAt           sql.NullTime
	ObservedAt          sql.NullTime
	Error               sql.NullString
	RemovedAt           sql.NullTime
	EnvironmentTargetID uuid.UUID
	PrivateNetworkID    uuid.UUID
}

func (etn environmentTargetNetwork) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetNetworkData) (EnvironmentTargetNetworkEntity, error) {
	entity := EnvironmentTargetNetworkEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Driver:              data.Driver,
		ExternalID:          data.ExternalID,
		Configuration:       data.Configuration,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		Error:               data.Error,
		RemovedAt:           data.RemovedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		PrivateNetworkID:    data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentTargetNetworkEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentTargetNetworkData struct {
	ID                  int32
	UpdatedAt           time.Time
	Driver              string
	ExternalID          sql.NullString
	Configuration       json.RawMessage
	State               string
	AppliedAt           sql.NullTime
	ObservedAt          sql.NullTime
	Error               sql.NullString
	RemovedAt           sql.NullTime
	EnvironmentTargetID uuid.UUID
	PrivateNetworkID    uuid.UUID
}

func (etn environmentTargetNetwork) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentTargetNetworkData) (EnvironmentTargetNetworkEntity, error) {
	entity := EnvironmentTargetNetworkEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		Driver:              data.Driver,
		ExternalID:          data.ExternalID,
		Configuration:       data.Configuration,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		Error:               data.Error,
		RemovedAt:           data.RemovedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		PrivateNetworkID:    data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("driver").
		Column("external_id").
		Column("configuration").
		Column("state").
		Column("applied_at").
		Column("observed_at").
		Column("error").
		Column("removed_at").
		Column("environment_target_id").
		Column("private_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetNetworkEntity{}, err
	}

	return entity, nil
}

func (etn environmentTargetNetwork) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*EnvironmentTargetNetworkEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (etn environmentTargetNetwork) All(ctx context.Context, db storage.Executor) ([]EnvironmentTargetNetworkEntity, error) {
	var entities []EnvironmentTargetNetworkEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentTargetNetworks struct {
	EnvironmentTargetNetworks []EnvironmentTargetNetworkEntity
	TotalCount                int64
	Page                      int64
	PageSize                  int64
	TotalPages                int64
}

func (etn environmentTargetNetwork) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentTargetNetworks, error) {
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
		Model(&EnvironmentTargetNetworkEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentTargetNetworks{}, err
	}

	entities := make([]EnvironmentTargetNetworkEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentTargetNetworks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentTargetNetworks{
		EnvironmentTargetNetworks: entities,
		TotalCount:                int64(totalCount),
		Page:                      page,
		PageSize:                  pageSize,
		TotalPages:                totalPages,
	}, nil
}

func (etn environmentTargetNetwork) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentTargetNetworkData) (EnvironmentTargetNetworkEntity, error) {
	entity := EnvironmentTargetNetworkEntity{
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Driver:              data.Driver,
		ExternalID:          data.ExternalID,
		Configuration:       data.Configuration,
		State:               data.State,
		AppliedAt:           data.AppliedAt,
		ObservedAt:          data.ObservedAt,
		Error:               data.Error,
		RemovedAt:           data.RemovedAt,
		EnvironmentTargetID: data.EnvironmentTargetID,
		PrivateNetworkID:    data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentTargetNetworkEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("driver = excluded.driver").
		Set("external_id = excluded.external_id").
		Set("configuration = excluded.configuration").
		Set("state = excluded.state").
		Set("applied_at = excluded.applied_at").
		Set("observed_at = excluded.observed_at").
		Set("error = excluded.error").
		Set("removed_at = excluded.removed_at").
		Set("environment_target_id = excluded.environment_target_id").
		Set("private_network_id = excluded.private_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentTargetNetworkEntity{}, err
	}

	return entity, nil
}
