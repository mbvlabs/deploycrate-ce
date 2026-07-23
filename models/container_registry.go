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

type ContainerRegistryEntity struct {
	bun.BaseModel `bun:"table:container_registries,alias:container_registries"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	Name          string       `bun:"name"`
	Provider      string       `bun:"provider"`
	Endpoint      string       `bun:"endpoint"`
	CredentialID  uuid.UUID    `bun:"credential_id,type:uuid"`
}

func (e *ContainerRegistryEntity) Validate() error {
	return nil
}

func (cr containerRegistry) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ContainerRegistryEntity, error) {
	var entity ContainerRegistryEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ContainerRegistryEntity{}, err
	}

	return entity, nil
}

type CreateContainerRegistryData struct {
	ArchivedAt   sql.NullTime
	Name         string
	Provider     string
	Endpoint     string
	CredentialID uuid.UUID
}

func (cr containerRegistry) Create(ctx context.Context, db storage.Executor, data CreateContainerRegistryData) (ContainerRegistryEntity, error) {
	entity := ContainerRegistryEntity{
		ID:           uuid.New(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ArchivedAt:   data.ArchivedAt,
		Name:         data.Name,
		Provider:     data.Provider,
		Endpoint:     data.Endpoint,
		CredentialID: data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ContainerRegistryEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ContainerRegistryEntity{}, err
	}

	return entity, nil
}

type UpdateContainerRegistryData struct {
	ID           uuid.UUID
	UpdatedAt    time.Time
	ArchivedAt   sql.NullTime
	Name         string
	Provider     string
	Endpoint     string
	CredentialID uuid.UUID
}

func (cr containerRegistry) Update(ctx context.Context, db storage.Executor, data UpdateContainerRegistryData) (ContainerRegistryEntity, error) {
	entity := ContainerRegistryEntity{
		ID:           data.ID,
		UpdatedAt:    time.Now(),
		ArchivedAt:   data.ArchivedAt,
		Name:         data.Name,
		Provider:     data.Provider,
		Endpoint:     data.Endpoint,
		CredentialID: data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ContainerRegistryEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("archived_at").
		Column("name").
		Column("provider").
		Column("endpoint").
		Column("credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ContainerRegistryEntity{}, err
	}

	return entity, nil
}

func (cr containerRegistry) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ContainerRegistryEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (cr containerRegistry) All(ctx context.Context, db storage.Executor) ([]ContainerRegistryEntity, error) {
	var entities []ContainerRegistryEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedContainerRegistries struct {
	ContainerRegistries []ContainerRegistryEntity
	TotalCount          int64
	Page                int64
	PageSize            int64
	TotalPages          int64
}

func (cr containerRegistry) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedContainerRegistries, error) {
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
		Model(&ContainerRegistryEntity{}).Count(ctx)
	if err != nil {
		return PaginatedContainerRegistries{}, err
	}

	entities := make([]ContainerRegistryEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedContainerRegistries{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedContainerRegistries{
		ContainerRegistries: entities,
		TotalCount:          int64(totalCount),
		Page:                page,
		PageSize:            pageSize,
		TotalPages:          totalPages,
	}, nil
}

func (cr containerRegistry) Upsert(ctx context.Context, db storage.Executor, data CreateContainerRegistryData) (ContainerRegistryEntity, error) {
	entity := ContainerRegistryEntity{
		ID:           uuid.New(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ArchivedAt:   data.ArchivedAt,
		Name:         data.Name,
		Provider:     data.Provider,
		Endpoint:     data.Endpoint,
		CredentialID: data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ContainerRegistryEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("archived_at = excluded.archived_at").
		Set("name = excluded.name").
		Set("provider = excluded.provider").
		Set("endpoint = excluded.endpoint").
		Set("credential_id = excluded.credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ContainerRegistryEntity{}, err
	}

	return entity, nil
}
