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

type EnvironmentEntity struct {
	bun.BaseModel      `bun:"table:environments,alias:environments"`
	ID                 uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt          time.Time      `bun:"created_at"`
	UpdatedAt          time.Time      `bun:"updated_at"`
	ApplicationID      uuid.UUID      `bun:"application_id,type:uuid"`
	Name               string         `bun:"name"`
	Slug               string         `bun:"slug"`
	Kind               string         `bun:"kind"`
	WebhookTokenPrefix sql.NullString `bun:"webhook_token_prefix"`
	WebhookTokenDigest []byte         `bun:"webhook_token_digest"`
	ArchivedAt         sql.NullTime   `bun:"archived_at"`
}

func (e *EnvironmentEntity) Validate() error {
	return nil
}

func (e environment) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentEntity, error) {
	var entity EnvironmentEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentData struct {
	ApplicationID      uuid.UUID
	Name               string
	Slug               string
	Kind               string
	WebhookTokenPrefix sql.NullString
	WebhookTokenDigest []byte
	ArchivedAt         sql.NullTime
}

func (e environment) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentData) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		ApplicationID:      data.ApplicationID,
		Name:               data.Name,
		Slug:               data.Slug,
		Kind:               data.Kind,
		WebhookTokenPrefix: data.WebhookTokenPrefix,
		WebhookTokenDigest: data.WebhookTokenDigest,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentData struct {
	ID                 uuid.UUID
	UpdatedAt          time.Time
	ApplicationID      uuid.UUID
	Name               string
	Slug               string
	Kind               string
	WebhookTokenPrefix sql.NullString
	WebhookTokenDigest []byte
	ArchivedAt         sql.NullTime
}

func (e environment) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentData) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:                 data.ID,
		UpdatedAt:          time.Now(),
		ApplicationID:      data.ApplicationID,
		Name:               data.Name,
		Slug:               data.Slug,
		Kind:               data.Kind,
		WebhookTokenPrefix: data.WebhookTokenPrefix,
		WebhookTokenDigest: data.WebhookTokenDigest,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("application_id").
		Column("name").
		Column("slug").
		Column("kind").
		Column("webhook_token_prefix").
		Column("webhook_token_digest").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}

func (e environment) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (e environment) All(ctx context.Context, db storage.Executor) ([]EnvironmentEntity, error) {
	var entities []EnvironmentEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironments struct {
	Environments []EnvironmentEntity
	TotalCount   int64
	Page         int64
	PageSize     int64
	TotalPages   int64
}

func (e environment) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironments, error) {
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
		Model(&EnvironmentEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironments{}, err
	}

	entities := make([]EnvironmentEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironments{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironments{
		Environments: entities,
		TotalCount:   int64(totalCount),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (e environment) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentData) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:                 uuid.New(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		ApplicationID:      data.ApplicationID,
		Name:               data.Name,
		Slug:               data.Slug,
		Kind:               data.Kind,
		WebhookTokenPrefix: data.WebhookTokenPrefix,
		WebhookTokenDigest: data.WebhookTokenDigest,
		ArchivedAt:         data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("application_id = excluded.application_id").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("kind = excluded.kind").
		Set("webhook_token_prefix = excluded.webhook_token_prefix").
		Set("webhook_token_digest = excluded.webhook_token_digest").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}
