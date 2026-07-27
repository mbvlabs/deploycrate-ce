package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentStateRevisionEntity struct {
	bun.BaseModel `bun:"table:environment_state_revisions,alias:environment_state_revisions"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	State         json.RawMessage `bun:"state,type:jsonb"`
	EnvironmentID uuid.UUID       `bun:"environment_id,type:uuid"`
	ChangeID      uuid.UUID       `bun:"change_id,type:uuid"`
}

func (e *EnvironmentStateRevisionEntity) Validate() error {
	return nil
}

func (esr environmentStateRevision) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentStateRevisionEntity, error) {
	var entity EnvironmentStateRevisionEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentStateRevisionData struct {
	State         json.RawMessage
	EnvironmentID uuid.UUID
	ChangeID      uuid.UUID
}

func (esr environmentStateRevision) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentStateRevisionData,
) (EnvironmentStateRevisionEntity, error) {
	entity := EnvironmentStateRevisionEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		State:         data.State,
		EnvironmentID: data.EnvironmentID,
		ChangeID:      data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentStateRevisionData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	State         json.RawMessage
	EnvironmentID uuid.UUID
	ChangeID      uuid.UUID
}

func (esr environmentStateRevision) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentStateRevisionData,
) (EnvironmentStateRevisionEntity, error) {
	entity := EnvironmentStateRevisionEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		State:         data.State,
		EnvironmentID: data.EnvironmentID,
		ChangeID:      data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("state").
		Column("environment_id").
		Column("change_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}

	return entity, nil
}

func (esr environmentStateRevision) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*EnvironmentStateRevisionEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (esr environmentStateRevision) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentStateRevisionEntity, error) {
	var entities []EnvironmentStateRevisionEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentStateRevisions struct {
	EnvironmentStateRevisions []EnvironmentStateRevisionEntity
	TotalCount                int64
	Page                      int64
	PageSize                  int64
	TotalPages                int64
}

func (esr environmentStateRevision) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentStateRevisions, error) {
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
		Model(&EnvironmentStateRevisionEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentStateRevisions{}, err
	}

	entities := make([]EnvironmentStateRevisionEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentStateRevisions{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentStateRevisions{
		EnvironmentStateRevisions: entities,
		TotalCount:                int64(totalCount),
		Page:                      page,
		PageSize:                  pageSize,
		TotalPages:                totalPages,
	}, nil
}

func (esr environmentStateRevision) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentStateRevisionData,
) (EnvironmentStateRevisionEntity, error) {
	entity := EnvironmentStateRevisionEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		State:         data.State,
		EnvironmentID: data.EnvironmentID,
		ChangeID:      data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("state = excluded.state").
		Set("environment_id = excluded.environment_id").
		Set("change_id = excluded.change_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}

	return entity, nil
}
