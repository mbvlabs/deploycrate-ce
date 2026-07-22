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

type ResourceBindingCredentialEntity struct {
	bun.BaseModel        `bun:"table:resource_binding_credentials,alias:resource_binding_credentials"`
	ID                   uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time    `bun:"created_at"`
	UpdatedAt            time.Time    `bun:"updated_at"`
	ResourceBindingID    uuid.UUID    `bun:"resource_binding_id,type:uuid"`
	ResourceCredentialID uuid.UUID    `bun:"resource_credential_id,type:uuid"`
	Generation           int32        `bun:"generation"`
	State                string       `bun:"state"`
	ActivatedAt          sql.NullTime `bun:"activated_at"`
	RetiredAt            sql.NullTime `bun:"retired_at"`
}

func (e *ResourceBindingCredentialEntity) Validate() error {
	return nil
}

func (rbc resourceBindingCredential) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceBindingCredentialEntity, error) {
	var entity ResourceBindingCredentialEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceBindingCredentialEntity{}, err
	}

	return entity, nil
}

type CreateResourceBindingCredentialData struct {
	ResourceBindingID    uuid.UUID
	ResourceCredentialID uuid.UUID
	Generation           int32
	State                string
	ActivatedAt          sql.NullTime
	RetiredAt            sql.NullTime
}

func (rbc resourceBindingCredential) Create(ctx context.Context, db storage.Executor, data CreateResourceBindingCredentialData) (ResourceBindingCredentialEntity, error) {
	entity := ResourceBindingCredentialEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ResourceBindingID:    data.ResourceBindingID,
		ResourceCredentialID: data.ResourceCredentialID,
		Generation:           data.Generation,
		State:                data.State,
		ActivatedAt:          data.ActivatedAt,
		RetiredAt:            data.RetiredAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceBindingCredentialEntity{}, err
	}

	return entity, nil
}

type UpdateResourceBindingCredentialData struct {
	ID                   uuid.UUID
	UpdatedAt            time.Time
	ResourceBindingID    uuid.UUID
	ResourceCredentialID uuid.UUID
	Generation           int32
	State                string
	ActivatedAt          sql.NullTime
	RetiredAt            sql.NullTime
}

func (rbc resourceBindingCredential) Update(ctx context.Context, db storage.Executor, data UpdateResourceBindingCredentialData) (ResourceBindingCredentialEntity, error) {
	entity := ResourceBindingCredentialEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		ResourceBindingID:    data.ResourceBindingID,
		ResourceCredentialID: data.ResourceCredentialID,
		Generation:           data.Generation,
		State:                data.State,
		ActivatedAt:          data.ActivatedAt,
		RetiredAt:            data.RetiredAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_binding_id").
		Column("resource_credential_id").
		Column("generation").
		Column("state").
		Column("activated_at").
		Column("retired_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBindingCredentialEntity{}, err
	}

	return entity, nil
}

func (rbc resourceBindingCredential) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceBindingCredentialEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rbc resourceBindingCredential) All(ctx context.Context, db storage.Executor) ([]ResourceBindingCredentialEntity, error) {
	var entities []ResourceBindingCredentialEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceBindingCredentials struct {
	ResourceBindingCredentials []ResourceBindingCredentialEntity
	TotalCount                 int64
	Page                       int64
	PageSize                   int64
	TotalPages                 int64
}

func (rbc resourceBindingCredential) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceBindingCredentials, error) {
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
		Model(&ResourceBindingCredentialEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceBindingCredentials{}, err
	}

	entities := make([]ResourceBindingCredentialEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceBindingCredentials{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceBindingCredentials{
		ResourceBindingCredentials: entities,
		TotalCount:                 int64(totalCount),
		Page:                       page,
		PageSize:                   pageSize,
		TotalPages:                 totalPages,
	}, nil
}

func (rbc resourceBindingCredential) Upsert(ctx context.Context, db storage.Executor, data CreateResourceBindingCredentialData) (ResourceBindingCredentialEntity, error) {
	entity := ResourceBindingCredentialEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ResourceBindingID:    data.ResourceBindingID,
		ResourceCredentialID: data.ResourceCredentialID,
		Generation:           data.Generation,
		State:                data.State,
		ActivatedAt:          data.ActivatedAt,
		RetiredAt:            data.RetiredAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceBindingCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_binding_id = excluded.resource_binding_id").
		Set("resource_credential_id = excluded.resource_credential_id").
		Set("generation = excluded.generation").
		Set("state = excluded.state").
		Set("activated_at = excluded.activated_at").
		Set("retired_at = excluded.retired_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceBindingCredentialEntity{}, err
	}

	return entity, nil
}
