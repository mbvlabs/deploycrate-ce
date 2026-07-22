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

type ChangeItemEntity struct {
	bun.BaseModel  `bun:"table:change_items,alias:change_items"`
	ID             int32           `bun:"id,pk,autoincrement"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	ChangeID       uuid.UUID       `bun:"change_id,type:uuid"`
	Action         string          `bun:"action"`
	SubjectType    string          `bun:"subject_type"`
	SubjectID      uuid.UUID       `bun:"subject_id,type:uuid"`
	PreviousValue  json.RawMessage `bun:"previous_value,type:jsonb"`
	RequestedValue json.RawMessage `bun:"requested_value,type:jsonb"`
}

func (e *ChangeItemEntity) Validate() error {
	return nil
}

func (ci changeItem) Find(ctx context.Context, db storage.Executor, id int32) (ChangeItemEntity, error) {
	var entity ChangeItemEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ChangeItemEntity{}, err
	}

	return entity, nil
}

type CreateChangeItemData struct {
	ChangeID       uuid.UUID
	Action         string
	SubjectType    string
	SubjectID      uuid.UUID
	PreviousValue  json.RawMessage
	RequestedValue json.RawMessage
}

func (ci changeItem) Create(ctx context.Context, db storage.Executor, data CreateChangeItemData) (ChangeItemEntity, error) {
	entity := ChangeItemEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ChangeID:       data.ChangeID,
		Action:         data.Action,
		SubjectType:    data.SubjectType,
		SubjectID:      data.SubjectID,
		PreviousValue:  data.PreviousValue,
		RequestedValue: data.RequestedValue,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeItemEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ChangeItemEntity{}, err
	}

	return entity, nil
}

type UpdateChangeItemData struct {
	ID             int32
	UpdatedAt      time.Time
	ChangeID       uuid.UUID
	Action         string
	SubjectType    string
	SubjectID      uuid.UUID
	PreviousValue  json.RawMessage
	RequestedValue json.RawMessage
}

func (ci changeItem) Update(ctx context.Context, db storage.Executor, data UpdateChangeItemData) (ChangeItemEntity, error) {
	entity := ChangeItemEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		ChangeID:       data.ChangeID,
		Action:         data.Action,
		SubjectType:    data.SubjectType,
		SubjectID:      data.SubjectID,
		PreviousValue:  data.PreviousValue,
		RequestedValue: data.RequestedValue,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeItemEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("change_id").
		Column("action").
		Column("subject_type").
		Column("subject_id").
		Column("previous_value").
		Column("requested_value").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeItemEntity{}, err
	}

	return entity, nil
}

func (ci changeItem) Destroy(ctx context.Context, db storage.Executor, id int32) error {
	_, err := db.NewDelete().
		Model((*ChangeItemEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ci changeItem) All(ctx context.Context, db storage.Executor) ([]ChangeItemEntity, error) {
	var entities []ChangeItemEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedChangeItems struct {
	ChangeItems []ChangeItemEntity
	TotalCount  int64
	Page        int64
	PageSize    int64
	TotalPages  int64
}

func (ci changeItem) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedChangeItems, error) {
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
		Model(&ChangeItemEntity{}).Count(ctx)
	if err != nil {
		return PaginatedChangeItems{}, err
	}

	entities := make([]ChangeItemEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedChangeItems{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedChangeItems{
		ChangeItems: entities,
		TotalCount:  int64(totalCount),
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	}, nil
}

func (ci changeItem) Upsert(ctx context.Context, db storage.Executor, data CreateChangeItemData) (ChangeItemEntity, error) {
	entity := ChangeItemEntity{
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ChangeID:       data.ChangeID,
		Action:         data.Action,
		SubjectType:    data.SubjectType,
		SubjectID:      data.SubjectID,
		PreviousValue:  data.PreviousValue,
		RequestedValue: data.RequestedValue,
	}

	if err := validation.Validate(&entity); err != nil {
		return ChangeItemEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("change_id = excluded.change_id").
		Set("action = excluded.action").
		Set("subject_type = excluded.subject_type").
		Set("subject_id = excluded.subject_id").
		Set("previous_value = excluded.previous_value").
		Set("requested_value = excluded.requested_value").
		Returning("*").
		Scan(ctx); err != nil {
		return ChangeItemEntity{}, err
	}

	return entity, nil
}
