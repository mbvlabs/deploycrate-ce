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

type NetworkAccessRuleEntity struct {
	bun.BaseModel         `bun:"table:network_access_rules,alias:network_access_rules"`
	ID                    uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt             time.Time    `bun:"created_at"`
	UpdatedAt             time.Time    `bun:"updated_at"`
	Protocol              string       `bun:"protocol"`
	DestinationAddress    string       `bun:"destination_address"`
	DestinationPort       int32        `bun:"destination_port"`
	Action                string       `bun:"action"`
	ArchivedAt            sql.NullTime `bun:"archived_at"`
	EnvironmentResourceID uuid.UUID    `bun:"environment_resource_id,type:uuid"`
}

func (e *NetworkAccessRuleEntity) Validate() error {
	return nil
}

func (nar networkAccessRule) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (NetworkAccessRuleEntity, error) {
	var entity NetworkAccessRuleEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return NetworkAccessRuleEntity{}, err
	}

	return entity, nil
}

type CreateNetworkAccessRuleData struct {
	Protocol              string
	DestinationAddress    string
	DestinationPort       int32
	Action                string
	ArchivedAt            sql.NullTime
	EnvironmentResourceID uuid.UUID
}

func (nar networkAccessRule) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateNetworkAccessRuleData,
) (NetworkAccessRuleEntity, error) {
	entity := NetworkAccessRuleEntity{
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Protocol:              data.Protocol,
		DestinationAddress:    data.DestinationAddress,
		DestinationPort:       data.DestinationPort,
		Action:                data.Action,
		ArchivedAt:            data.ArchivedAt,
		EnvironmentResourceID: data.EnvironmentResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return NetworkAccessRuleEntity{}, err
	}

	return entity, nil
}

type UpdateNetworkAccessRuleData struct {
	ID                    uuid.UUID
	UpdatedAt             time.Time
	Protocol              string
	DestinationAddress    string
	DestinationPort       int32
	Action                string
	ArchivedAt            sql.NullTime
	EnvironmentResourceID uuid.UUID
}

func (nar networkAccessRule) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateNetworkAccessRuleData,
) (NetworkAccessRuleEntity, error) {
	entity := NetworkAccessRuleEntity{
		ID:                    data.ID,
		UpdatedAt:             time.Now(),
		Protocol:              data.Protocol,
		DestinationAddress:    data.DestinationAddress,
		DestinationPort:       data.DestinationPort,
		Action:                data.Action,
		ArchivedAt:            data.ArchivedAt,
		EnvironmentResourceID: data.EnvironmentResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("protocol").
		Column("destination_address").
		Column("destination_port").
		Column("action").
		Column("archived_at").
		Column("environment_resource_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return NetworkAccessRuleEntity{}, err
	}

	return entity, nil
}

func (nar networkAccessRule) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*NetworkAccessRuleEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (nar networkAccessRule) All(
	ctx context.Context,
	db storage.Executor,
) ([]NetworkAccessRuleEntity, error) {
	var entities []NetworkAccessRuleEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedNetworkAccessRules struct {
	NetworkAccessRules []NetworkAccessRuleEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (nar networkAccessRule) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedNetworkAccessRules, error) {
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
		Model(&NetworkAccessRuleEntity{}).Count(ctx)
	if err != nil {
		return PaginatedNetworkAccessRules{}, err
	}

	entities := make([]NetworkAccessRuleEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedNetworkAccessRules{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedNetworkAccessRules{
		NetworkAccessRules: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (nar networkAccessRule) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateNetworkAccessRuleData,
) (NetworkAccessRuleEntity, error) {
	entity := NetworkAccessRuleEntity{
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Protocol:              data.Protocol,
		DestinationAddress:    data.DestinationAddress,
		DestinationPort:       data.DestinationPort,
		Action:                data.Action,
		ArchivedAt:            data.ArchivedAt,
		EnvironmentResourceID: data.EnvironmentResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return NetworkAccessRuleEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("protocol = excluded.protocol").
		Set("destination_address = excluded.destination_address").
		Set("destination_port = excluded.destination_port").
		Set("action = excluded.action").
		Set("archived_at = excluded.archived_at").
		Set("environment_resource_id = excluded.environment_resource_id").
		Returning("*").
		Scan(ctx); err != nil {
		return NetworkAccessRuleEntity{}, err
	}

	return entity, nil
}
