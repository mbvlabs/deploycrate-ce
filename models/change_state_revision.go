package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ChangeStateRevisionEntity struct {
	bun.BaseModel              `bun:"table:change_state_revisions,alias:change_state_revisions"`
	ID                         int32     `bun:"id,pk,autoincrement"`
	CreatedAt                  time.Time `bun:"created_at"`
	UpdatedAt                  time.Time `bun:"updated_at"`
	ChangeID                   uuid.UUID `bun:"change_id,type:uuid"`
	EnvironmentStateRevisionID uuid.UUID `bun:"environment_state_revision_id,type:uuid"`
	Role                       string    `bun:"role"`
}

func (e *ChangeStateRevisionEntity) Validate() error {
	return nil
}

func (csr changeStateRevision) Find(ctx context.Context, db storage.Executor, id int32) (ChangeStateRevisionEntity, error) {
	var entity ChangeStateRevisionEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeStateRevisionEntity{}, err
	}

	return entity, nil
}

type CreateChangeStateRevisionData struct {
	ChangeID                   uuid.UUID
	EnvironmentStateRevisionID uuid.UUID
	Role                       string
}

func (csr changeStateRevision) Create(ctx context.Context, db storage.Executor, data CreateChangeStateRevisionData) (ChangeStateRevisionEntity, error) {
	entity := ChangeStateRevisionEntity{
		CreatedAt:                  time.Now(),
		UpdatedAt:                  time.Now(),
		ChangeID:                   data.ChangeID,
		EnvironmentStateRevisionID: data.EnvironmentStateRevisionID,
		Role:                       data.Role,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeStateRevisionEntity{}, err
	}

	return entity, nil
}

type UpdateChangeStateRevisionData struct {
	ID                         int32
	UpdatedAt                  time.Time
	ChangeID                   uuid.UUID
	EnvironmentStateRevisionID uuid.UUID
	Role                       string
}

func (csr changeStateRevision) Update(ctx context.Context, db storage.Executor, data UpdateChangeStateRevisionData) (ChangeStateRevisionEntity, error) {
	entity := ChangeStateRevisionEntity{
		ID:                         data.ID,
		UpdatedAt:                  time.Now(),
		ChangeID:                   data.ChangeID,
		EnvironmentStateRevisionID: data.EnvironmentStateRevisionID,
		Role:                       data.Role,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("change_id").
		Column("environment_state_revision_id").
		Column("role").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeStateRevisionEntity{}, err
	}

	return entity, nil
}

func (csr changeStateRevision) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ChangeStateRevisionEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (csr changeStateRevision) All(ctx context.Context, db storage.Executor) ([]ChangeStateRevisionEntity, error) {
	var entities []ChangeStateRevisionEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeStateRevisions struct {
	ChangeStateRevisions []ChangeStateRevisionEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (csr changeStateRevision) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedChangeStateRevisions, error) {
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
		Model(&ChangeStateRevisionEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeStateRevisions{}, err
	}

	entities := make([]ChangeStateRevisionEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeStateRevisions{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeStateRevisions{
		ChangeStateRevisions: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (csr changeStateRevision) Upsert(ctx context.Context, db storage.Executor, data CreateChangeStateRevisionData) (ChangeStateRevisionEntity, error) {
	entity := ChangeStateRevisionEntity{
		CreatedAt:                  time.Now(),
		UpdatedAt:                  time.Now(),
		ChangeID:                   data.ChangeID,
		EnvironmentStateRevisionID: data.EnvironmentStateRevisionID,
		Role:                       data.Role,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("change_id = excluded.change_id").
		Set("environment_state_revision_id = excluded.environment_state_revision_id").
		Set("role = excluded.role").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeStateRevisionEntity{}, err
	}

	return entity, nil
}
