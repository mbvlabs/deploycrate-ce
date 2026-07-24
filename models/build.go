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

type BuildEntity struct {
	bun.BaseModel       `bun:"table:builds,alias:builds"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	SourceRevision      string          `bun:"source_revision"`
	BuildMethod         string          `bun:"build_method"`
	BuildConfiguration  json.RawMessage `bun:"build_configuration,type:jsonb"`
	Status              string          `bun:"status"`
	ArtifactReference   sql.NullString  `bun:"artifact_reference"`
	ArtifactDigest      []byte          `bun:"artifact_digest"`
	StartedAt           sql.NullTime    `bun:"started_at"`
	FinishedAt          sql.NullTime    `bun:"finished_at"`
	Error               sql.NullString  `bun:"error"`
	EnvironmentID       uuid.UUID       `bun:"environment_id,type:uuid"`
	EnvironmentSourceID uuid.UUID       `bun:"environment_source_id,type:uuid"`
	ChangeID            uuid.UUID       `bun:"change_id,type:uuid"`
}

func (e *BuildEntity) Validate() error {
	return nil
}

func (b build) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (BuildEntity, error) {
	var entity BuildEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return BuildEntity{}, err
	}

	return entity, nil
}

type CreateBuildData struct {
	SourceRevision      string
	BuildMethod         string
	BuildConfiguration  json.RawMessage
	Status              string
	ArtifactReference   sql.NullString
	ArtifactDigest      []byte
	StartedAt           sql.NullTime
	FinishedAt          sql.NullTime
	Error               sql.NullString
	EnvironmentID       uuid.UUID
	EnvironmentSourceID uuid.UUID
	ChangeID            uuid.UUID
}

func (b build) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBuildData,
) (BuildEntity, error) {
	entity := BuildEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		SourceRevision:      data.SourceRevision,
		BuildMethod:         data.BuildMethod,
		BuildConfiguration:  data.BuildConfiguration,
		Status:              data.Status,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		StartedAt:           data.StartedAt,
		FinishedAt:          data.FinishedAt,
		Error:               data.Error,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ChangeID:            data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BuildEntity{}, err
	}

	return entity, nil
}

type UpdateBuildData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	SourceRevision      string
	BuildMethod         string
	BuildConfiguration  json.RawMessage
	Status              string
	ArtifactReference   sql.NullString
	ArtifactDigest      []byte
	StartedAt           sql.NullTime
	FinishedAt          sql.NullTime
	Error               sql.NullString
	EnvironmentID       uuid.UUID
	EnvironmentSourceID uuid.UUID
	ChangeID            uuid.UUID
}

func (b build) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateBuildData,
) (BuildEntity, error) {
	entity := BuildEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		SourceRevision:      data.SourceRevision,
		BuildMethod:         data.BuildMethod,
		BuildConfiguration:  data.BuildConfiguration,
		Status:              data.Status,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		StartedAt:           data.StartedAt,
		FinishedAt:          data.FinishedAt,
		Error:               data.Error,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ChangeID:            data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("source_revision").
		Column("build_method").
		Column("build_configuration").
		Column("status").
		Column("artifact_reference").
		Column("artifact_digest").
		Column("started_at").
		Column("finished_at").
		Column("error").
		Column("environment_id").
		Column("environment_source_id").
		Column("change_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return BuildEntity{}, err
	}

	return entity, nil
}

func (b build) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*BuildEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (b build) All(ctx context.Context, db storage.Executor) ([]BuildEntity, error) {
	var entities []BuildEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedBuilds struct {
	Builds     []BuildEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (b build) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedBuilds, error) {
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
		Model(&BuildEntity{}).Count(ctx)
	if err != nil {
		return PaginatedBuilds{}, err
	}

	entities := make([]BuildEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedBuilds{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedBuilds{
		Builds:     entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (b build) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateBuildData,
) (BuildEntity, error) {
	entity := BuildEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		SourceRevision:      data.SourceRevision,
		BuildMethod:         data.BuildMethod,
		BuildConfiguration:  data.BuildConfiguration,
		Status:              data.Status,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		StartedAt:           data.StartedAt,
		FinishedAt:          data.FinishedAt,
		Error:               data.Error,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		ChangeID:            data.ChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BuildEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("source_revision = excluded.source_revision").
		Set("build_method = excluded.build_method").
		Set("build_configuration = excluded.build_configuration").
		Set("status = excluded.status").
		Set("artifact_reference = excluded.artifact_reference").
		Set("artifact_digest = excluded.artifact_digest").
		Set("started_at = excluded.started_at").
		Set("finished_at = excluded.finished_at").
		Set("error = excluded.error").
		Set("environment_id = excluded.environment_id").
		Set("environment_source_id = excluded.environment_source_id").
		Set("change_id = excluded.change_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BuildEntity{}, err
	}

	return entity, nil
}
