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

type EnvironmentSecretEntity struct {
	bun.BaseModel `bun:"table:environment_secrets,alias:environment_secrets"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Key           string       `bun:"key"`
	EncValue      []byte       `bun:"enc_value"`
	Digest        []byte       `bun:"digest"`
	SourceType    string       `bun:"source_type"`
	SourceID      uuid.UUID    `bun:"source_id,type:uuid"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
}

func (e *EnvironmentSecretEntity) Validate() error {
	return nil
}

func (es environmentSecret) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentSecretEntity, error) {
	var entity EnvironmentSecretEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentSecretData struct {
	Key           string
	EncValue      []byte
	Digest        []byte
	SourceType    string
	SourceID      uuid.UUID
	ArchivedAt    sql.NullTime
	EnvironmentID uuid.UUID
}

func (es environmentSecret) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentSecretData) (EnvironmentSecretEntity, error) {
	entity := EnvironmentSecretEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Key:           data.Key,
		EncValue:      data.EncValue,
		Digest:        data.Digest,
		SourceType:    data.SourceType,
		SourceID:      data.SourceID,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSecretEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentSecretData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	Key           string
	EncValue      []byte
	Digest        []byte
	SourceType    string
	SourceID      uuid.UUID
	ArchivedAt    sql.NullTime
	EnvironmentID uuid.UUID
}

func (es environmentSecret) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentSecretData) (EnvironmentSecretEntity, error) {
	entity := EnvironmentSecretEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		Key:           data.Key,
		EncValue:      data.EncValue,
		Digest:        data.Digest,
		SourceType:    data.SourceType,
		SourceID:      data.SourceID,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSecretEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("key").
		Column("enc_value").
		Column("digest").
		Column("source_type").
		Column("source_id").
		Column("archived_at").
		Column("environment_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}

	return entity, nil
}

func (es environmentSecret) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentSecretEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (es environmentSecret) All(ctx context.Context, db storage.Executor) ([]EnvironmentSecretEntity, error) {
	var entities []EnvironmentSecretEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentSecrets struct {
	EnvironmentSecrets []EnvironmentSecretEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (es environmentSecret) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentSecrets, error) {
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
		Model(&EnvironmentSecretEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentSecrets{}, err
	}

	entities := make([]EnvironmentSecretEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentSecrets{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentSecrets{
		EnvironmentSecrets: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (es environmentSecret) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentSecretData) (EnvironmentSecretEntity, error) {
	entity := EnvironmentSecretEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Key:           data.Key,
		EncValue:      data.EncValue,
		Digest:        data.Digest,
		SourceType:    data.SourceType,
		SourceID:      data.SourceID,
		ArchivedAt:    data.ArchivedAt,
		EnvironmentID: data.EnvironmentID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSecretEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("key = excluded.key").
		Set("enc_value = excluded.enc_value").
		Set("digest = excluded.digest").
		Set("source_type = excluded.source_type").
		Set("source_id = excluded.source_id").
		Set("archived_at = excluded.archived_at").
		Set("environment_id = excluded.environment_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}

	return entity, nil
}
