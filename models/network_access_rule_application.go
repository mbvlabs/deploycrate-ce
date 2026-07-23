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

type NetworkAccessRuleApplicationEntity struct {
	bun.BaseModel              `bun:"table:network_access_rule_applications,alias:network_access_rule_applications"`
	ID                         uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt                  time.Time       `bun:"created_at"`
	UpdatedAt                  time.Time       `bun:"updated_at"`
	Driver                     string          `bun:"driver"`
	ExternalID                 sql.NullString  `bun:"external_id"`
	Configuration              json.RawMessage `bun:"configuration,type:jsonb"`
	State                      string          `bun:"state"`
	AppliedAt                  sql.NullTime    `bun:"applied_at"`
	ObservedAt                 sql.NullTime    `bun:"observed_at"`
	RemovedAt                  sql.NullTime    `bun:"removed_at"`
	Error                      sql.NullString  `bun:"error"`
	NetworkAccessRuleID        uuid.UUID       `bun:"network_access_rule_id,type:uuid"`
	EnvironmentTargetNetworkID int32           `bun:"environment_target_network_id"`
}

func (e *NetworkAccessRuleApplicationEntity) Validate() error {
	return nil
}

func (nara networkAccessRuleApplication) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (NetworkAccessRuleApplicationEntity, error) {
	var entity NetworkAccessRuleApplicationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return NetworkAccessRuleApplicationEntity{}, err
	}

	return entity, nil
}

type CreateNetworkAccessRuleApplicationData struct {
	Driver                     string
	ExternalID                 sql.NullString
	Configuration              json.RawMessage
	State                      string
	AppliedAt                  sql.NullTime
	ObservedAt                 sql.NullTime
	RemovedAt                  sql.NullTime
	Error                      sql.NullString
	NetworkAccessRuleID        uuid.UUID
	EnvironmentTargetNetworkID int32
}

func (nara networkAccessRuleApplication) Create(ctx context.Context, db storage.Executor, data CreateNetworkAccessRuleApplicationData) (NetworkAccessRuleApplicationEntity, error) {
	entity := NetworkAccessRuleApplicationEntity{
		ID:                         uuid.New(),
		CreatedAt:                  time.Now(),
		UpdatedAt:                  time.Now(),
		Driver:                     data.Driver,
		ExternalID:                 data.ExternalID,
		Configuration:              data.Configuration,
		State:                      data.State,
		AppliedAt:                  data.AppliedAt,
		ObservedAt:                 data.ObservedAt,
		RemovedAt:                  data.RemovedAt,
		Error:                      data.Error,
		NetworkAccessRuleID:        data.NetworkAccessRuleID,
		EnvironmentTargetNetworkID: data.EnvironmentTargetNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return NetworkAccessRuleApplicationEntity{}, err
	}

	return entity, nil
}

type UpdateNetworkAccessRuleApplicationData struct {
	ID                         uuid.UUID
	UpdatedAt                  time.Time
	Driver                     string
	ExternalID                 sql.NullString
	Configuration              json.RawMessage
	State                      string
	AppliedAt                  sql.NullTime
	ObservedAt                 sql.NullTime
	RemovedAt                  sql.NullTime
	Error                      sql.NullString
	NetworkAccessRuleID        uuid.UUID
	EnvironmentTargetNetworkID int32
}

func (nara networkAccessRuleApplication) Update(ctx context.Context, db storage.Executor, data UpdateNetworkAccessRuleApplicationData) (NetworkAccessRuleApplicationEntity, error) {
	entity := NetworkAccessRuleApplicationEntity{
		ID:                         data.ID,
		UpdatedAt:                  time.Now(),
		Driver:                     data.Driver,
		ExternalID:                 data.ExternalID,
		Configuration:              data.Configuration,
		State:                      data.State,
		AppliedAt:                  data.AppliedAt,
		ObservedAt:                 data.ObservedAt,
		RemovedAt:                  data.RemovedAt,
		Error:                      data.Error,
		NetworkAccessRuleID:        data.NetworkAccessRuleID,
		EnvironmentTargetNetworkID: data.EnvironmentTargetNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleApplicationEntity{}, errors.Join(ErrDomainValidation, err)
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
		Column("removed_at").
		Column("error").
		Column("network_access_rule_id").
		Column("environment_target_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return NetworkAccessRuleApplicationEntity{}, err
	}

	return entity, nil
}

func (nara networkAccessRuleApplication) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*NetworkAccessRuleApplicationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (nara networkAccessRuleApplication) All(ctx context.Context, db storage.Executor) ([]NetworkAccessRuleApplicationEntity, error) {
	var entities []NetworkAccessRuleApplicationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedNetworkAccessRuleApplications struct {
	NetworkAccessRuleApplications []NetworkAccessRuleApplicationEntity
	TotalCount                    int64
	Page                          int64
	PageSize                      int64
	TotalPages                    int64
}

func (nara networkAccessRuleApplication) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedNetworkAccessRuleApplications, error) {
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
		Model(&NetworkAccessRuleApplicationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedNetworkAccessRuleApplications{}, err
	}

	entities := make([]NetworkAccessRuleApplicationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedNetworkAccessRuleApplications{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedNetworkAccessRuleApplications{
		NetworkAccessRuleApplications: entities,
		TotalCount:                    int64(totalCount),
		Page:                          page,
		PageSize:                      pageSize,
		TotalPages:                    totalPages,
	}, nil
}

func (nara networkAccessRuleApplication) Upsert(ctx context.Context, db storage.Executor, data CreateNetworkAccessRuleApplicationData) (NetworkAccessRuleApplicationEntity, error) {
	entity := NetworkAccessRuleApplicationEntity{
		ID:                         uuid.New(),
		CreatedAt:                  time.Now(),
		UpdatedAt:                  time.Now(),
		Driver:                     data.Driver,
		ExternalID:                 data.ExternalID,
		Configuration:              data.Configuration,
		State:                      data.State,
		AppliedAt:                  data.AppliedAt,
		ObservedAt:                 data.ObservedAt,
		RemovedAt:                  data.RemovedAt,
		Error:                      data.Error,
		NetworkAccessRuleID:        data.NetworkAccessRuleID,
		EnvironmentTargetNetworkID: data.EnvironmentTargetNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleApplicationEntity{}, errors.Join(ErrDomainValidation, err)
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
		Set("removed_at = excluded.removed_at").
		Set("error = excluded.error").
		Set("network_access_rule_id = excluded.network_access_rule_id").
		Set("environment_target_network_id = excluded.environment_target_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return NetworkAccessRuleApplicationEntity{}, err
	}

	return entity, nil
}
