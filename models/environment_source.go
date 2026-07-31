package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentSourceEntity struct {
	bun.BaseModel `bun:"table:environment_sources,alias:environment_sources"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	ArchivedAt    sql.NullTime    `bun:"archived_at"`
	Kind          string          `bun:"kind"`
	Provider      string          `bun:"provider"`
	Repository    string          `bun:"repository"`
	Reference     string          `bun:"reference"`
	Settings      json.RawMessage `bun:"settings,type:jsonb"`
	AutoBuild     bool            `bun:"auto_build"`
	EnvironmentID uuid.UUID       `bun:"environment_id,type:uuid"`
	CredentialID  *uuid.UUID      `bun:"credential_id,type:uuid"`
}

func (e *EnvironmentSourceEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil {
		builder.Add("id", "required", "source ID is required")
	}
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environment_id", "required", "environment is required")
	}
	if strings.TrimSpace(e.Kind) == "" || strings.TrimSpace(e.Provider) == "" {
		builder.Add("provider", "required", "source kind and provider are required")
	}
	if e.Provider == "github" && e.Kind != "git" {
		builder.Add("kind", "invalid", "GitHub sources must use the git kind")
	}
	if strings.TrimSpace(e.Repository) == "" || strings.TrimSpace(e.Reference) == "" {
		builder.Add("repository", "required", "repository and reference are required")
	}
	if len(e.Settings) == 0 || !json.Valid(e.Settings) {
		builder.Add("settings", "invalid", "source settings must be valid JSON")
	}
	return builder.Err()
}

func (es environmentSource) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentSourceEntity, error) {
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
	ArchivedAt    sql.NullTime
	Kind          string
	Provider      string
	Repository    string
	Reference     string
	Settings      json.RawMessage
	AutoBuild     bool
	EnvironmentID uuid.UUID
	CredentialID  *uuid.UUID
}

func (es environmentSource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentSourceData,
) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ArchivedAt:    data.ArchivedAt,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		AutoBuild:     data.AutoBuild,
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
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
	ArchivedAt    sql.NullTime
	Kind          string
	Provider      string
	Repository    string
	Reference     string
	Settings      json.RawMessage
	AutoBuild     bool
	EnvironmentID uuid.UUID
	CredentialID  *uuid.UUID
}

func (es environmentSource) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentSourceData,
) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            data.ID,
		UpdatedAt:     time.Now(),
		ArchivedAt:    data.ArchivedAt,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		AutoBuild:     data.AutoBuild,
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("archived_at").
		Column("kind").
		Column("provider").
		Column("repository").
		Column("reference").
		Column("settings").
		Column("auto_build").
		Column("environment_id").
		Column("credential_id").
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

func (es environmentSource) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentSourceEntity, error) {
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

func (es environmentSource) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentSources, error) {
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

func (es environmentSource) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentSourceData,
) (EnvironmentSourceEntity, error) {
	entity := EnvironmentSourceEntity{
		ID:            uuid.New(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ArchivedAt:    data.ArchivedAt,
		Kind:          data.Kind,
		Provider:      data.Provider,
		Repository:    data.Repository,
		Reference:     data.Reference,
		Settings:      data.Settings,
		AutoBuild:     data.AutoBuild,
		EnvironmentID: data.EnvironmentID,
		CredentialID:  data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSourceEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("archived_at = excluded.archived_at").
		Set("kind = excluded.kind").
		Set("provider = excluded.provider").
		Set("repository = excluded.repository").
		Set("reference = excluded.reference").
		Set("settings = excluded.settings").
		Set("auto_build = excluded.auto_build").
		Set("environment_id = excluded.environment_id").
		Set("credential_id = excluded.credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentSourceEntity{}, err
	}

	return entity, nil
}
