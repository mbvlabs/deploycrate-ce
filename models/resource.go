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

type ResourceEntity struct {
	bun.BaseModel      `bun:"table:resources,alias:resources"`
	ID                 uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time    `bun:"created_at"`
	UpdatedAt          time.Time    `bun:"updated_at"`
	Name               string       `bun:"name"`
	Category           string       `bun:"category"`
	Kind               string       `bun:"kind"`
	SharingScope       string       `bun:"sharing_scope"`
	ArchivedAt         sql.NullTime `bun:"archived_at"`
	OwnerEnvironmentID uuid.UUID    `bun:"owner_environment_id,type:uuid"`
}

func (e *ResourceEntity) Validate() error {
	return nil
}

func (r resource) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceEntity, error) {
	var entity ResourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

type CreateResourceData struct {
	Name               string
	Category           string
	Kind               string
	SharingScope       string
	ArchivedAt         sql.NullTime
	OwnerEnvironmentID uuid.UUID
}

func (r resource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		Category:           data.Category,
		Kind:               data.Kind,
		SharingScope:       data.SharingScope,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

type UpdateResourceData struct {
	ID                 uuid.UUID
	UpdatedAt          time.Time
	Name               string
	Category           string
	Kind               string
	SharingScope       string
	ArchivedAt         sql.NullTime
	OwnerEnvironmentID uuid.UUID
}

func (r resource) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                 data.ID,
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		Category:           data.Category,
		Kind:               data.Kind,
		SharingScope:       data.SharingScope,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("category").
		Column("kind").
		Column("sharing_scope").
		Column("archived_at").
		Column("owner_environment_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

func (r resource) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (r resource) All(ctx context.Context, db storage.Executor) ([]ResourceEntity, error) {
	var entities []ResourceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResources struct {
	Resources  []ResourceEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (r resource) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResources, error) {
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
		Model(&ResourceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResources{}, err
	}

	entities := make([]ResourceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResources{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResources{
		Resources:  entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (r resource) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Name:               data.Name,
		Category:           data.Category,
		Kind:               data.Kind,
		SharingScope:       data.SharingScope,
		ArchivedAt:         data.ArchivedAt,
		OwnerEnvironmentID: data.OwnerEnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("category = excluded.category").
		Set("kind = excluded.kind").
		Set("sharing_scope = excluded.sharing_scope").
		Set("archived_at = excluded.archived_at").
		Set("owner_environment_id = excluded.owner_environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}
