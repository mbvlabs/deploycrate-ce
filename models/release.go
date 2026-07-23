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

type ReleaseEntity struct {
	bun.BaseModel       `bun:"table:releases,alias:releases"`
	ID                  uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time      `bun:"created_at"`
	UpdatedAt           time.Time      `bun:"updated_at"`
	Version             sql.NullString `bun:"version"`
	SourceRevision      sql.NullString `bun:"source_revision"`
	ArtifactReference   string         `bun:"artifact_reference"`
	ArtifactDigest      []byte         `bun:"artifact_digest"`
	EnvironmentID       uuid.UUID      `bun:"environment_id,type:uuid"`
	EnvironmentSourceID *uuid.UUID     `bun:"environment_source_id,type:uuid"`
	BuildID             *uuid.UUID     `bun:"build_id,type:uuid"`
	CreatedByChangeID   uuid.UUID      `bun:"created_by_change_id,type:uuid"`
}

func (e *ReleaseEntity) Validate() error {
	return nil
}

func (r release) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ReleaseEntity, error) {
	var entity ReleaseEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ReleaseEntity{}, err
	}

	return entity, nil
}

type CreateReleaseData struct {
	Version             sql.NullString
	SourceRevision      sql.NullString
	ArtifactReference   string
	ArtifactDigest      []byte
	EnvironmentID       uuid.UUID
	EnvironmentSourceID *uuid.UUID
	BuildID             *uuid.UUID
	CreatedByChangeID   uuid.UUID
}

func (r release) Create(ctx context.Context, db storage.Executor, data CreateReleaseData) (ReleaseEntity, error) {
	entity := ReleaseEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Version:             data.Version,
		SourceRevision:      data.SourceRevision,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		BuildID:             data.BuildID,
		CreatedByChangeID:   data.CreatedByChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ReleaseEntity{}, err
	}

	return entity, nil
}

type UpdateReleaseData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	Version             sql.NullString
	SourceRevision      sql.NullString
	ArtifactReference   string
	ArtifactDigest      []byte
	EnvironmentID       uuid.UUID
	EnvironmentSourceID *uuid.UUID
	BuildID             *uuid.UUID
	CreatedByChangeID   uuid.UUID
}

func (r release) Update(ctx context.Context, db storage.Executor, data UpdateReleaseData) (ReleaseEntity, error) {
	entity := ReleaseEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		Version:             data.Version,
		SourceRevision:      data.SourceRevision,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		BuildID:             data.BuildID,
		CreatedByChangeID:   data.CreatedByChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("version").
		Column("source_revision").
		Column("artifact_reference").
		Column("artifact_digest").
		Column("environment_id").
		Column("environment_source_id").
		Column("build_id").
		Column("created_by_change_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ReleaseEntity{}, err
	}

	return entity, nil
}

func (r release) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ReleaseEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (r release) All(ctx context.Context, db storage.Executor) ([]ReleaseEntity, error) {
	var entities []ReleaseEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedReleases struct {
	Releases   []ReleaseEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (r release) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedReleases, error) {
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
		Model(&ReleaseEntity{}).Count(ctx)
	if err != nil {
		return PaginatedReleases{}, err
	}

	entities := make([]ReleaseEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedReleases{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedReleases{
		Releases:   entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (r release) Upsert(ctx context.Context, db storage.Executor, data CreateReleaseData) (ReleaseEntity, error) {
	entity := ReleaseEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Version:             data.Version,
		SourceRevision:      data.SourceRevision,
		ArtifactReference:   data.ArtifactReference,
		ArtifactDigest:      data.ArtifactDigest,
		EnvironmentID:       data.EnvironmentID,
		EnvironmentSourceID: data.EnvironmentSourceID,
		BuildID:             data.BuildID,
		CreatedByChangeID:   data.CreatedByChangeID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ReleaseEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("version = excluded.version").
		Set("source_revision = excluded.source_revision").
		Set("artifact_reference = excluded.artifact_reference").
		Set("artifact_digest = excluded.artifact_digest").
		Set("environment_id = excluded.environment_id").
		Set("environment_source_id = excluded.environment_source_id").
		Set("build_id = excluded.build_id").
		Set("created_by_change_id = excluded.created_by_change_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ReleaseEntity{}, err
	}

	return entity, nil
}
