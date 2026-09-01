package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
)

type ApplicationRegistryOption struct {
	ID       uuid.UUID `json:"id"       bun:"id"`
	Name     string    `json:"name"     bun:"name"`
	Endpoint string    `json:"endpoint" bun:"endpoint"`
}

type ApplicationBuildServerOption struct {
	ID           uuid.UUID          `json:"id"           bun:"id"`
	Name         string             `json:"name"         bun:"name"`
	Kind         string             `json:"kind"         bun:"kind"`
	Address      string             `json:"address"      bun:"address"`
	Architecture string             `json:"architecture" bun:"architecture"`
	Buildpacks   []BuildpackRuntime `json:"buildpacks"   bun:"-"`
	Capabilities json.RawMessage    `json:"-"            bun:"capabilities"`
}

type ApplicationEnvironmentRow struct {
	ApplicationID      uuid.UUID `bun:"application_id"`
	ApplicationName    string    `bun:"application_name"`
	ApplicationSlug    string    `bun:"application_slug"`
	EnvironmentID      uuid.UUID `bun:"environment_id"`
	EnvironmentName    string    `bun:"environment_name"`
	EnvironmentKind    string    `bun:"environment_kind"`
	RepositoryFullName string    `bun:"repository_full_name"`
	Reference          string    `bun:"reference"`
	SourceHealthy      bool      `bun:"source_healthy"`
	SourceType         string    `bun:"source_type"`
}

type ApplicationDetails struct {
	ID                           uuid.UUID       `json:"id"                                     bun:"id"`
	Name                         string          `json:"name"                                   bun:"name"`
	Slug                         string          `json:"slug"                                   bun:"slug"`
	EnvironmentID                uuid.UUID       `json:"environmentId"                          bun:"environment_id"`
	EnvironmentName              string          `json:"environmentName"                        bun:"environment_name"`
	EnvironmentSlug              string          `json:"environmentSlug"                        bun:"environment_slug"`
	EnvironmentKind              string          `json:"environmentKind"                        bun:"environment_kind"`
	SetupComplete                bool            `json:"setupComplete"                          bun:"setup_complete"`
	Runtime                      string          `json:"runtime"                                bun:"runtime"`
	EnvironmentSourceID          uuid.UUID       `json:"environmentSourceId"                    bun:"environment_source_id"`
	SourceType                   string          `json:"sourceType"                             bun:"source_type"`
	RepositoryID                 uuid.UUID       `json:"repositoryId"                           bun:"repository_id"`
	RepositoryFullName           string          `json:"repositoryFullName"                     bun:"repository_full_name"`
	RepositoryRemovedAt          sql.NullTime    `json:"repositoryRemovedAt"                    bun:"repository_removed_at"`
	InstallationID               uuid.UUID       `json:"installationId"                         bun:"installation_id"`
	InstallationAccount          string          `json:"installationAccount"                    bun:"installation_account"`
	InstallationSuspendedAt      sql.NullTime    `json:"installationSuspendedAt"                bun:"installation_suspended_at"`
	Reference                    string          `json:"reference"                              bun:"reference"`
	AutoBuild                    bool            `json:"autoBuild"                              bun:"auto_build"`
	ContextPath                  string          `json:"contextPath"                            bun:"context_path"`
	BuilderReference             sql.NullString  `json:"builderReference"                       bun:"builder_reference"`
	BuildpackSettings            json.RawMessage `json:"buildpackSettings"                      bun:"buildpack_settings"`
	ImageRepository              string          `json:"imageRepository"                        bun:"image_repository"`
	RegistryName                 string          `json:"registryName"                           bun:"registry_name"`
	RegistryID                   uuid.UUID       `json:"registryId"                             bun:"registry_id"`
	BuildServerID                uuid.UUID       `json:"buildServerId"                          bun:"build_server_id"`
	BuildServerName              string          `json:"buildServerName"                        bun:"build_server_name"`
	LatestRevision               sql.NullString  `json:"latestRevision"                         bun:"latest_revision"`
	LatestDeliveryStatus         sql.NullString  `json:"latestDeliveryStatus"                   bun:"latest_delivery_status"`
	LatestBuildStatus            sql.NullString  `json:"latestBuildStatus"                      bun:"latest_build_status"`
	CanPromoteToProduction       bool            `json:"canPromoteToProduction"`
	PromotionTargetName          string          `json:"promotionTargetName"`
	LatestSuccessfulDeploymentID *uuid.UUID      `json:"latestSuccessfulDeploymentId,omitempty"`
	LatestSuccessfulReleaseID    *uuid.UUID      `json:"latestSuccessfulReleaseId,omitempty"`
}

type ApplicationDeploymentActivity struct {
	ID              uuid.UUID `json:"id"              bun:"id"`
	EnvironmentID   uuid.UUID `json:"environmentId"   bun:"environment_id"`
	EnvironmentName string    `json:"environmentName" bun:"environment_name"`
	EnvironmentKind string    `json:"environmentKind" bun:"environment_kind"`
	Status          string    `json:"status"          bun:"status"`
	CurrentStep     string    `json:"currentStep"     bun:"current_step"`
	Error           string    `json:"error"           bun:"error"`
	ReleaseID       uuid.UUID `json:"releaseId"       bun:"release_id"`
	SourceRevision  string    `json:"sourceRevision"  bun:"source_revision"`
	CreatedAt       time.Time `json:"createdAt"       bun:"created_at"`
	Active          bool      `json:"active"          bun:"active"`
}

const applicationDeploymentActiveExpression = `EXISTS (
	SELECT 1
	FROM instances AS active_instance
	JOIN caddy_route_backends AS active_backend ON active_backend.instance_id = active_instance.id
	JOIN caddy_routes AS active_route ON active_route.id = active_backend.caddy_route_id
	WHERE active_instance.deployment_id = deployment.id
		AND active_instance.state = 'serving'
		AND active_instance.removed_at IS NULL
		AND active_backend.weight = 100
		AND active_backend.removed_at IS NULL
		AND active_route.environment_target_id = deployment.environment_target_id
		AND active_route.removed_at IS NULL
)`

func (application) EnvironmentRows(
	ctx context.Context,
	db storage.Executor,
) ([]ApplicationEnvironmentRow, error) {
	rows := make([]ApplicationEnvironmentRow, 0)
	err := db.NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.id AS application_id, application.name AS application_name, application.slug AS application_slug").
		ColumnExpr("environment.id AS environment_id, environment.name AS environment_name, environment.kind AS environment_kind").
		ColumnExpr("COALESCE(repository.full_name, source.repository) AS repository_full_name, source.reference").
		ColumnExpr("CASE WHEN source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type").
		ColumnExpr("CASE WHEN source.kind = 'image' THEN registry_resource.id IS NOT NULL AND registry_resource.archived_at IS NULL ELSE (repository.removed_at IS NULL AND installation.archived_at IS NULL AND installation.suspended_at IS NULL) END AS source_healthy").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").
		Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Join("LEFT JOIN resources AS registry_resource ON registry_resource.id = image.registry_resource_id").
		Where("application.archived_at IS NULL").
		Where("application.slug <> ?", SystemApplicationSlug).
		OrderExpr("application.name ASC, CASE environment.kind WHEN 'staging' THEN 0 WHEN 'production' THEN 1 ELSE 2 END, environment.name ASC").
		Scan(ctx, &rows)
	return rows, err
}

func (application) RecentDeploymentActivity(
	ctx context.Context, db storage.Executor, applicationID uuid.UUID,
) ([]ApplicationDeploymentActivity, error) {
	items := make([]ApplicationDeploymentActivity, 0)
	err := db.NewSelect().TableExpr("deployments AS deployment").
		ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.created_at").
		ColumnExpr("environment.id AS environment_id, environment.name AS environment_name, environment.kind AS environment_kind").
		ColumnExpr("COALESCE(release.source_revision, release.version, '') AS source_revision").
		ColumnExpr(applicationDeploymentActiveExpression+" AS active").
		Join("JOIN releases AS release ON release.id = deployment.release_id AND release.registry_resource_id IS NOT NULL").
		Join("JOIN environments AS environment ON environment.id = release.environment_id AND environment.archived_at IS NULL").
		Where("environment.application_id = ?", applicationID).
		OrderExpr("deployment.created_at DESC").Limit(20).Scan(ctx, &items)
	return items, err
}

func (application) Details(
	ctx context.Context, db storage.Executor, applicationID uuid.UUID, environmentID *uuid.UUID,
) (ApplicationDetails, error) {
	var details ApplicationDetails
	query := db.NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.id, application.name, application.slug").
		ColumnExpr("environment.id AS environment_id, environment.name AS environment_name, environment.slug AS environment_slug, environment.kind AS environment_kind").
		ColumnExpr(`EXISTS (SELECT 1 FROM changes AS setup_change JOIN change_state_revisions AS setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result' JOIN environment_state_revisions AS setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = environment.id WHERE setup_change.environment_id = environment.id AND setup_change.kind = 'environment_setup' AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL) AS setup_complete`).
		ColumnExpr("source.id AS environment_source_id, source.reference, source.auto_build").
		ColumnExpr("CASE WHEN source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type").
		ColumnExpr("COALESCE(runtime.runtime, '') AS runtime, repository.id AS repository_id").
		ColumnExpr("COALESCE(repository.full_name, source.repository) AS repository_full_name, repository.removed_at AS repository_removed_at").
		ColumnExpr("installation.id AS installation_id, COALESCE(installation.account_login, '') AS installation_account, installation.suspended_at AS installation_suspended_at").
		ColumnExpr("COALESCE(buildpack.context_path, '') AS context_path, buildpack.builder_reference, COALESCE(buildpack.settings, '{}'::jsonb) AS buildpack_settings").
		ColumnExpr("COALESCE(buildpack.image_repository, source.repository) AS image_repository").
		ColumnExpr("build_server.id AS build_server_id, COALESCE(build_server.name, '') AS build_server_name").
		ColumnExpr("registry_resource.id AS registry_id, registry_resource.name AS registry_name").
		ColumnExpr("(SELECT event.source_revision FROM source_events AS event WHERE event.environment_source_id = source.id ORDER BY event.received_at DESC LIMIT 1) AS latest_revision").
		ColumnExpr("(SELECT delivery.status FROM github_webhook_deliveries AS delivery WHERE delivery.repository_external_id = repository.external_id ORDER BY delivery.received_at DESC LIMIT 1) AS latest_delivery_status").
		ColumnExpr("(SELECT build.status FROM builds AS build WHERE build.environment_source_id = source.id ORDER BY build.created_at DESC LIMIT 1) AS latest_build_status").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").
		Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Join("LEFT JOIN runtime_configurations AS runtime ON runtime.environment_id = environment.id").
		Join("LEFT JOIN servers AS build_server ON build_server.id = buildpack.server_id").
		Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").
		Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id").
		Where("application.id = ?", applicationID).Where("application.archived_at IS NULL").
		Where("application.slug <> ?", SystemApplicationSlug)
	if environmentID != nil {
		query = query.Where("environment.id = ?", *environmentID)
	}
	err := query.Scan(ctx, &details)
	return details, err
}
