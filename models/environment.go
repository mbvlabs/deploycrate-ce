package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EnvironmentEntity struct {
	bun.BaseModel  `bun:"table:environments,alias:environments"`
	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time      `bun:"created_at"`
	UpdatedAt      time.Time      `bun:"updated_at"`
	Name           string         `bun:"name"`
	Slug           string         `bun:"slug"`
	Kind           string         `bun:"kind"`
	APITokenPrefix sql.NullString `bun:"api_token_prefix"`
	APITokenDigest []byte         `bun:"api_token_digest"`
	ArchivedAt     sql.NullTime   `bun:"archived_at"`
	ApplicationID  uuid.UUID      `bun:"application_id,type:uuid"`
}

func (e *EnvironmentEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Slug = strings.TrimSpace(e.Slug)
	e.Kind = strings.TrimSpace(e.Kind)
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	builder.Required("slug", e.Slug)
	builder.Required("kind", e.Kind)
	if e.ApplicationID == uuid.Nil {
		builder.Add("applicationId", "required", "application is required")
	}
	return builder.Err()
}

func (e environment) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentEntity, error) {
	var entity EnvironmentEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}

func (e environment) FindForApplication(
	ctx context.Context,
	db storage.Executor,
	applicationID, environmentID uuid.UUID,
) (EnvironmentEntity, error) {
	var entity EnvironmentEntity
	if err := db.NewSelect().Model(&entity).
		Where("id = ?", environmentID).
		Where("application_id = ?", applicationID).
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}
	return entity, nil
}

func (e environment) Lock(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentEntity, error) {
	var entity EnvironmentEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		For("UPDATE").
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}
	return entity, nil
}

func (environment) EnsureSlugAvailable(
	ctx context.Context,
	db storage.Executor,
	applicationID uuid.UUID,
	slug string,
) (bool, error) {
	if err := lockUnique(ctx, db, "environment-slug:"+applicationID.String()+":"+slug); err != nil {
		return false, err
	}
	count, err := db.NewSelect().TableExpr("environments").
		Where("application_id = ?", applicationID).
		Where("slug = ?", slug).Where("archived_at IS NULL").Count(ctx)
	return count == 0, err
}

func (environment) ActiveIDsForApplication(
	ctx context.Context,
	db storage.Executor,
	applicationID uuid.UUID,
) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0)
	err := db.NewSelect().TableExpr("environments AS environment").ColumnExpr("environment.id").
		Where("environment.application_id = ?", applicationID).
		Where("environment.archived_at IS NULL").
		OrderExpr("CASE environment.kind WHEN 'staging' THEN 0 WHEN 'production' THEN 1 ELSE 2 END, environment.name").
		Scan(ctx, &ids)
	return ids, err
}

func (environment) DeleteIncompleteSources(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) error {
	for _, table := range []string{"github_environment_sources", "buildpack_configurations", "image_configurations"} {
		if _, err := db.NewDelete().TableExpr(table).
			Where("environment_source_id IN (SELECT id FROM environment_sources WHERE environment_id = ?)", environmentID).
			Exec(ctx); err != nil {
			return err
		}
	}
	_, err := db.NewDelete().TableExpr("environment_sources").
		Where("environment_id = ?", environmentID).Exec(ctx)
	return err
}

func (e environment) SetupComplete(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (bool, error) {
	var complete bool
	err := db.NewSelect().TableExpr("environments AS environment").
		ColumnExpr(`EXISTS (
			SELECT 1
			FROM changes AS setup_change
			JOIN change_state_revisions AS setup_result
				ON setup_result.change_id = setup_change.id AND setup_result.role = 'result'
			JOIN environment_state_revisions AS setup_revision
				ON setup_revision.id = setup_result.environment_state_revision_id
				AND setup_revision.environment_id = environment.id
			WHERE setup_change.environment_id = environment.id
				AND setup_change.kind = 'environment_setup'
				AND setup_change.committed_at IS NOT NULL
				AND setup_change.cancelled_at IS NULL
		)`).
		Where("environment.id = ?", id).
		Scan(ctx, &complete)
	return complete, err
}

type EnvironmentDeployability struct {
	Deployable bool     `json:"deployable"`
	Missing    []string `json:"missing"`
}

func (e environment) Deployability(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDeployability, error) {
	var checks struct {
		EnvironmentActive bool `bun:"environment_active"`
		ApplicationActive bool `bun:"application_active"`
		SetupComplete     bool `bun:"setup_complete"`
		SourceReady       bool `bun:"source_ready"`
		ArtifactReady     bool `bun:"artifact_ready"`
		RuntimeReady      bool `bun:"runtime_ready"`
		TargetReady       bool `bun:"target_ready"`
		DomainReady       bool `bun:"domain_ready"`
		DNSReady          bool `bun:"dns_ready"`
		RevisionReady     bool `bun:"revision_ready"`
		ResourcesReady    bool `bun:"resources_ready"`
	}
	query := `
		SELECT
			e.archived_at IS NULL AS environment_active,
			a.archived_at IS NULL AS application_active,
			EXISTS (
				SELECT 1 FROM changes setup_change
				JOIN change_state_revisions setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result'
				JOIN environment_state_revisions setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = e.id
				WHERE setup_change.environment_id = e.id AND setup_change.kind = 'environment_setup'
				AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL
			) AS setup_complete,
			EXISTS (
				SELECT 1 FROM environment_sources es
				LEFT JOIN github_environment_sources ges ON ges.environment_source_id = es.id
				LEFT JOIN github_repositories gr ON gr.id = ges.github_repository_id AND gr.removed_at IS NULL
				LEFT JOIN github_installations gi ON gi.id = gr.github_installation_id AND gi.archived_at IS NULL AND gi.suspended_at IS NULL
				LEFT JOIN image_configurations ic ON ic.environment_source_id = es.id
				WHERE es.environment_id = e.id AND es.archived_at IS NULL AND (
					(es.kind = 'git' AND es.provider = 'github' AND gr.id IS NOT NULL AND gi.id IS NOT NULL) OR
					(es.kind = 'image' AND es.provider = 'registry' AND ic.id IS NOT NULL)
				)
			) AS source_ready,
			EXISTS (
				SELECT 1 FROM environment_sources es
				LEFT JOIN buildpack_configurations bc ON bc.environment_source_id = es.id
				LEFT JOIN image_configurations ic ON ic.environment_source_id = es.id
				JOIN resources registry ON registry.id = COALESCE(bc.registry_resource_id, ic.registry_resource_id) AND registry.archived_at IS NULL
				WHERE es.environment_id = e.id AND es.archived_at IS NULL
				AND EXISTS (SELECT 1 FROM resource_endpoints endpoint WHERE endpoint.resource_id = registry.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL)
				AND EXISTS (SELECT 1 FROM resource_credentials credential WHERE credential.resource_id = registry.id AND credential.archived_at IS NULL)
			) AS artifact_ready,
			EXISTS (SELECT 1 FROM runtime_configurations rc WHERE rc.environment_id = e.id) AS runtime_ready,
			EXISTS (
				SELECT 1 FROM environment_targets et
				JOIN servers s ON s.id = et.server_id AND s.archived_at IS NULL
				WHERE et.environment_id = e.id AND et.detached_at IS NULL
			) AS target_ready,
			EXISTS (
				SELECT 1 FROM environment_domains ed
				WHERE ed.environment_id = e.id AND ed.is_primary AND ed.archived_at IS NULL
			) AS domain_ready,
			NOT EXISTS (
				SELECT 1
				FROM environment_domains ed
				JOIN environment_dns_bindings binding ON binding.environment_domain_id = ed.id
				WHERE ed.environment_id = e.id AND ed.is_primary AND ed.archived_at IS NULL
				AND binding.archived_at IS NULL
				AND (binding.state <> 'applied' OR binding.applied_generation <> binding.generation)
			) AS dns_ready,
			EXISTS (
				SELECT 1 FROM environment_state_revisions esr
				JOIN changes c ON c.id = esr.change_id AND c.committed_at IS NOT NULL AND c.cancelled_at IS NULL
				WHERE esr.environment_id = e.id
			) AS revision_ready,
			NOT EXISTS (
				SELECT 1 FROM environment_resources er
				LEFT JOIN resources r ON r.id = er.resource_id
				LEFT JOIN resource_endpoints re ON re.id = er.resource_endpoint_id
				LEFT JOIN resource_credentials rc ON rc.id = er.resource_credential_id
				WHERE er.environment_id = e.id AND er.archived_at IS NULL
				AND (r.id IS NULL OR r.archived_at IS NOT NULL OR re.id IS NULL OR re.archived_at IS NOT NULL
					OR (er.resource_credential_id IS NOT NULL AND (rc.id IS NULL OR rc.archived_at IS NOT NULL)))
			) AS resources_ready
		FROM environments e
		JOIN applications a ON a.id = e.application_id
		WHERE e.id = ?`
	if err := db.NewRaw(query, id).Scan(ctx, &checks); err != nil {
		return EnvironmentDeployability{}, err
	}
	missing := make([]string, 0, 10)
	for _, check := range []struct {
		name string
		ok   bool
	}{
		{"environment_active", checks.EnvironmentActive},
		{"application_active", checks.ApplicationActive},
		{"setup_complete", checks.SetupComplete},
		{"deployment_source", checks.SourceReady},
		{"artifact_registry", checks.ArtifactReady},
		{"runtime_configuration", checks.RuntimeReady},
		{"environment_target", checks.TargetReady},
		{"primary_domain", checks.DomainReady},
		{"managed_dns", checks.DNSReady},
		{"committed_state_revision", checks.RevisionReady},
		{"resource_connections", checks.ResourcesReady},
	} {
		if !check.ok {
			missing = append(missing, check.name)
		}
	}
	if checks.RevisionReady {
		revision, err := EnvironmentStateRevision.LatestCommitted(ctx, db, id)
		if err != nil {
			return EnvironmentDeployability{}, fmt.Errorf(
				"load latest Environment revision: %w",
				err,
			)
		}
		if _, err := EnvironmentStateRevision.ResolveSecrets(ctx, db, revision); err != nil {
			missing = append(missing, "revision_secrets")
		}
	}
	return EnvironmentDeployability{Deployable: len(missing) == 0, Missing: missing}, nil
}

type CreateEnvironmentData struct {
	Name           string
	Slug           string
	Kind           string
	APITokenPrefix sql.NullString
	APITokenDigest []byte
	ArchivedAt     sql.NullTime
	ApplicationID  uuid.UUID
}

func (e environment) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentData,
) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Slug:           data.Slug,
		Kind:           data.Kind,
		APITokenPrefix: data.APITokenPrefix,
		APITokenDigest: data.APITokenDigest,
		ArchivedAt:     data.ArchivedAt,
		ApplicationID:  data.ApplicationID,
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
	ID             uuid.UUID
	UpdatedAt      time.Time
	Name           string
	Slug           string
	Kind           string
	APITokenPrefix sql.NullString
	APITokenDigest []byte
	ArchivedAt     sql.NullTime
	ApplicationID  uuid.UUID
}

func (e environment) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentData,
) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Slug:           data.Slug,
		Kind:           data.Kind,
		APITokenPrefix: data.APITokenPrefix,
		APITokenDigest: data.APITokenDigest,
		ArchivedAt:     data.ArchivedAt,
		ApplicationID:  data.ApplicationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("slug").
		Column("kind").
		Column("api_token_prefix").
		Column("api_token_digest").
		Column("archived_at").
		Column("application_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}

func (e environment) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	if err := deleteReleaseCommandExecutionsForEnvironmentTargets(
		ctx,
		db,
		"SELECT id FROM environment_targets WHERE environment_id = ?",
		id,
	); err != nil {
		return err
	}
	_, err := db.NewDelete().
		Model((*EnvironmentEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func deleteReleaseCommandExecutionsForEnvironmentTargets(
	ctx context.Context,
	db storage.Executor,
	targetsSubquery string,
	args ...any,
) error {
	_, err := db.NewDelete().
		TableExpr("release_command_executions").
		Where("environment_target_id IN ("+targetsSubquery+")", args...).
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

func (e environment) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironments, error) {
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

func (e environment) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentData,
) (EnvironmentEntity, error) {
	entity := EnvironmentEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Slug:           data.Slug,
		Kind:           data.Kind,
		APITokenPrefix: data.APITokenPrefix,
		APITokenDigest: data.APITokenDigest,
		ArchivedAt:     data.ArchivedAt,
		ApplicationID:  data.ApplicationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("kind = excluded.kind").
		Set("api_token_prefix = excluded.api_token_prefix").
		Set("api_token_digest = excluded.api_token_digest").
		Set("archived_at = excluded.archived_at").
		Set("application_id = excluded.application_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentEntity{}, err
	}

	return entity, nil
}
