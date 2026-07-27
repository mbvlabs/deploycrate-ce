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

type ChangeReleaseEntity struct {
	bun.BaseModel `bun:"table:change_releases,alias:change_releases"`
	ID            int32     `bun:"id,pk,autoincrement"`
	CreatedAt     time.Time `bun:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at"`
	ChangeID      uuid.UUID `bun:"change_id,type:uuid"`
	ReleaseID     uuid.UUID `bun:"release_id,type:uuid"`
}

func (e *ChangeReleaseEntity) Validate() error {
	return nil
}

func (cr changeRelease) Find(
	ctx context.Context,
	db storage.Executor,
	id int32,
) (ChangeReleaseEntity, error) {
	var entity ChangeReleaseEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeReleaseEntity{}, err
	}

	return entity, nil
}

type CreateChangeReleaseData struct {
	ChangeID  uuid.UUID
	ReleaseID uuid.UUID
}

func (cr changeRelease) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeReleaseData,
) (ChangeReleaseEntity, error) {
	entity := ChangeReleaseEntity{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ChangeID:  data.ChangeID,
		ReleaseID: data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeReleaseEntity{}, err
	}

	return entity, nil
}

type UpdateChangeReleaseData struct {
	ID        int32
	UpdatedAt time.Time
	ChangeID  uuid.UUID
	ReleaseID uuid.UUID
}

func (cr changeRelease) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateChangeReleaseData,
) (ChangeReleaseEntity, error) {
	entity := ChangeReleaseEntity{
		ID:        data.ID,
		UpdatedAt: time.Now(),
		ChangeID:  data.ChangeID,
		ReleaseID: data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("change_id").
		Column("release_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeReleaseEntity{}, err
	}

	return entity, nil
}

func (cr changeRelease) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ChangeReleaseEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (cr changeRelease) All(
	ctx context.Context,
	db storage.Executor,
) ([]ChangeReleaseEntity, error) {
	var entities []ChangeReleaseEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeReleases struct {
	ChangeReleases []ChangeReleaseEntity
	TotalCount     int64
	Page           int64
	PageSize       int64
	TotalPages     int64
}

func (cr changeRelease) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedChangeReleases, error) {
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
		Model(&ChangeReleaseEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeReleases{}, err
	}

	entities := make([]ChangeReleaseEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeReleases{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeReleases{
		ChangeReleases: entities,
		TotalCount:     int64(totalCount),
		Page:           page,
		PageSize:       pageSize,
		TotalPages:     totalPages,
	}, nil
}

func (cr changeRelease) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateChangeReleaseData,
) (ChangeReleaseEntity, error) {
	entity := ChangeReleaseEntity{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ChangeID:  data.ChangeID,
		ReleaseID: data.ReleaseID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("change_id = excluded.change_id").
		Set("release_id = excluded.release_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeReleaseEntity{}, err
	}

	return entity, nil
}
