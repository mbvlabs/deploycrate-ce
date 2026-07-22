package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentSourceEntity struct {
	bun.BaseModel `bun:"table:environment_sources,alias:environment_sources"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	EnvironmentID uuid.UUID       `bun:"environment_id,type:uuid"`
	CredentialID  *uuid.UUID      `bun:"credential_id,type:uuid"`
	Kind          string          `bun:"kind"`
	Provider      string          `bun:"provider"`
	Repository    string          `bun:"repository"`
	Reference     string          `bun:"reference"`
	Settings      json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt    sql.NullTime    `bun:"archived_at"`
}

func (e *EnvironmentSourceEntity) Validate() error {
	return nil
}

func (es environmentSource) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (EnvironmentSourceEntity, error) {
	var entity EnvironmentSourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentSourceEntity{}, err
	}

	return entity, nil
}

type CreateEnvironmentSourceData struct {
	EnvironmentID uuid.UUID
	CredentialID  *uuid.UUID
	Kind          string
	Provider      string
	Repository    string
	Reference     string
	Settings      json.RawMessage
	ArchivedAt    sql.NullTime
}

func (es environmentSource) Create(ctx context.Context, db storage.Executor, data CreateEnvironmentSourceData) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentSourceEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentSourceData struct {
	ID            uuid.UUID
	UpdatedAt     time.Time
	EnvironmentID uuid.UUID
	CredentialID  *uuid.UUID
	Kind          string
	Provider      string
	Repository    string
	Reference     string
	Settings      json.RawMessage
	ArchivedAt    sql.NullTime
}

func (es environmentSource) Update(ctx context.Context, db storage.Executor, data UpdateEnvironmentSourceData) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("environment_id").
		Column("credential_id").
		Column("kind").
		Column("provider").
		Column("repository").
		Column("reference").
		Column("settings").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentSourceEntity{}, err
	}

	return entity, nil
}

func (es environmentSource) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*EnvironmentSourceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (es environmentSource) All(ctx context.Context, db storage.Executor) ([]EnvironmentSourceEntity, error) {
	var entities []EnvironmentSourceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentSources struct {
	EnvironmentSources []EnvironmentSourceEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (es environmentSource) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedEnvironmentSources, error) {
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
		Model(&EnvironmentSourceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentSources{}, err
	}

	entities := make([]EnvironmentSourceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentSources{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentSources{
		EnvironmentSources: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (es environmentSource) Upsert(ctx context.Context, db storage.Executor, data CreateEnvironmentSourceData) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		ArchivedAt:    data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("environment_id = excluded.environment_id").
		Set("credential_id = excluded.credential_id").
		Set("kind = excluded.kind").
		Set("provider = excluded.provider").
		Set("repository = excluded.repository").
		Set("reference = excluded.reference").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentSourceEntity{}, err
	}

	return entity, nil
}
