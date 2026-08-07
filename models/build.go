package models

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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
	CurrentStep         sql.NullString  `bun:"current_step"`
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
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil || e.EnvironmentID == uuid.Nil || e.EnvironmentSourceID == uuid.Nil ||
		e.ChangeID == uuid.Nil {
		builder.Add("id", "required", "Build ownership identifiers are required")
	}
	if len(strings.TrimSpace(e.SourceRevision)) != 40 {
		builder.Add(
			"sourceRevision",
			"invalid",
			"Build source revision must be an exact commit SHA",
		)
	}
	if e.BuildMethod != "buildpacks" {
		builder.Add("buildMethod", "unsupported", "only Buildpacks builds are supported")
	}
	if !slices.Contains(
		[]string{"pending", "running", "succeeded", "failed", "cancelled"},
		e.Status,
	) {
		builder.Add("status", "invalid", "Build status is invalid")
	}
	if len(e.BuildConfiguration) == 0 || !json.Valid(e.BuildConfiguration) {
		builder.Add("buildConfiguration", "invalid", "Build configuration must be valid JSON")
	}
	if e.Status == "succeeded" &&
		(!e.ArtifactReference.Valid || len(e.ArtifactDigest) != sha256.Size) {
		builder.Add("artifact", "required", "successful Builds require an immutable artifact")
	}
	return builder.Err()
}

func (b build) Lock(ctx context.Context, db storage.Executor, id uuid.UUID) (BuildEntity, error) {
	var entity BuildEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).For("UPDATE").Scan(ctx)
	return entity, err
}

func (b build) MarkRunning(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	result, err := db.NewUpdate().TableExpr("builds").Set("status = 'running'").
		Set("current_step = 'starting'").
		Set("started_at = COALESCE(started_at, ?)", at).
		Set("finished_at = NULL").Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).Where("status IN ('pending', 'running')").Exec(ctx)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("Build cannot transition to running")
	}
	return nil
}

func (b build) MarkProgress(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	step string,
	at time.Time,
) error {
	step = strings.TrimSpace(step)
	if step == "" {
		return errors.New("Build progress step is required")
	}
	result, err := db.NewUpdate().
		TableExpr("builds").
		Set("current_step = ?", step).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = 'running'").
		Exec(ctx)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("Build cannot record progress")
	}
	return nil
}

func (b build) MarkSucceeded(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	reference string,
	digest []byte,
	at time.Time,
) error {
	if !strings.Contains(reference, "@sha256:") || len(digest) != sha256.Size {
		return errors.New("successful Build requires an immutable artifact digest")
	}
	result, err := db.NewUpdate().
		TableExpr("builds").
		Set("status = 'succeeded'").
		Set("artifact_reference = ?", reference).
		Set("artifact_digest = ?", digest).
		Set("current_step = 'completed'").
		Set("finished_at = ?", at).
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = 'running'").
		Exec(ctx)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("Build cannot transition to succeeded")
	}
	return nil
}

func (b build) MarkFailed(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	operationErr error,
	at time.Time,
) error {
	message := strings.TrimSpace(strings.ReplaceAll(operationErr.Error(), "\x00", "�"))
	runes := []rune(message)
	if len(runes) > 2048 {
		message = "[earlier error output truncated]\n" + string(runes[len(runes)-2048:])
	}
	_, err := db.NewUpdate().
		TableExpr("builds").
		Set("status = 'failed'").
		Set("finished_at = ?", at).
		Set("current_step = 'failed'").
		Set("error = ?", message).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN ('pending', 'running')").
		Exec(ctx)
	return err
}

func (b build) MarkCancelled(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	result, err := db.NewUpdate().
		TableExpr("builds").
		Set("status = 'cancelled'").
		Set("finished_at = ?", at).
		Set("current_step = 'cancelled'").
		Set("error = 'Build cancelled by user'").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN ('pending', 'running')").
		Exec(ctx)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("only a pending or running Build can be stopped")
	}
	return nil
}

func (b build) ResetForRetry(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	result, err := db.NewUpdate().
		TableExpr("builds").
		Set("status = 'pending'").
		Set("current_step = 'queued'").
		Set("started_at = NULL").
		Set("finished_at = NULL").
		Set("error = NULL").
		Set("artifact_reference = NULL").
		Set("artifact_digest = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN ('failed', 'cancelled')").
		Exec(ctx)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("only a failed or cancelled Build can be retried")
	}
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
	CurrentStep         sql.NullString
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
		CurrentStep:         data.CurrentStep,
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
	CurrentStep         sql.NullString
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
		CurrentStep:         data.CurrentStep,
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
		Column("current_step").
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
		CurrentStep:         data.CurrentStep,
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
		Set("current_step = excluded.current_step").
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
