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

type ApplicationEntity struct {
	bun.BaseModel `bun:"table:applications,alias:applications"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Name          string       `bun:"name"`
	Slug          string       `bun:"slug"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
}

func (e *ApplicationEntity) Validate() error {
	return nil
}

func (a application) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ApplicationEntity, error) {
	var entity ApplicationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

type CreateApplicationData struct {
	Name       string
	Slug       string
	ArchivedAt sql.NullTime
}

func (a application) Create(ctx context.Context, db storage.Executor, data CreateApplicationData) (ApplicationEntity, error) {
	entity := ApplicationEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

type UpdateApplicationData struct {
	ID         uuid.UUID
	UpdatedAt  time.Time
	Name       string
	Slug       string
	ArchivedAt sql.NullTime
}

func (a application) Update(ctx context.Context, db storage.Executor, data UpdateApplicationData) (ApplicationEntity, error) {
	entity := ApplicationEntity{
		ID:         data.ID,
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("slug").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

func (a application) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ApplicationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (a application) All(ctx context.Context, db storage.Executor) ([]ApplicationEntity, error) {
	var entities []ApplicationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedApplications struct {
	Applications []ApplicationEntity
	TotalCount   int64
	Page         int64
	PageSize     int64
	TotalPages   int64
}

func (a application) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedApplications, error) {
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
		Model(&ApplicationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedApplications{}, err
	}

	entities := make([]ApplicationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedApplications{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedApplications{
		Applications: entities,
		TotalCount:   int64(totalCount),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (a application) Upsert(ctx context.Context, db storage.Executor, data CreateApplicationData) (ApplicationEntity, error) {
	entity := ApplicationEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}
