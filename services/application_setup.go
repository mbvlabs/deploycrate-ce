package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
)

type ApplicationSetupData struct {
	SourceType           string
	ApplicationName      string
	ApplicationSlug      string
	EnvironmentName      string
	EnvironmentSlug      string
	EnvironmentKind      string
	GitHubInstallationID uuid.UUID
	GitHubRepositoryID   uuid.UUID
	Reference            string
	AutoBuild            bool
	ContextPath          string
	BuilderReference     string
	BuildpackSettings    json.RawMessage
	RegistryResourceID   uuid.UUID
	ImageRepository      string
	BuildServerID        uuid.UUID
}

type ApplicationSetupResult struct {
	Application models.ApplicationEntity
	Environment models.EnvironmentEntity
	Source      models.EnvironmentSourceEntity
}

type ApplicationSetupOptions struct {
	Installations []GitHubInstallationSummary     `json:"installations"`
	Repositories  []models.GitHubRepositoryEntity `json:"repositories"`
	Registries    []RegistryResourceOption        `json:"registries"`
	BuildServers  []ApplicationBuildServerOption  `json:"buildServers"`
}

type ApplicationBuildServerOption struct {
	ID      uuid.UUID `json:"id" bun:"id"`
	Name    string    `json:"name" bun:"name"`
	Kind    string    `json:"kind" bun:"kind"`
	Address string    `json:"address" bun:"address"`
}

type RegistryResourceOption struct {
	ID       uuid.UUID `json:"id" bun:"id"`
	Name     string    `json:"name" bun:"name"`
	Endpoint string    `json:"endpoint" bun:"endpoint"`
}

type ApplicationListItem struct {
	ID                 uuid.UUID `json:"id" bun:"id"`
	Name               string    `json:"name" bun:"name"`
	Slug               string    `json:"slug" bun:"slug"`
	EnvironmentName    string    `json:"environmentName" bun:"environment_name"`
	EnvironmentKind    string    `json:"environmentKind" bun:"environment_kind"`
	RepositoryFullName string    `json:"repositoryFullName" bun:"repository_full_name"`
	Reference          string    `json:"reference" bun:"reference"`
	SourceHealthy      bool      `json:"sourceHealthy" bun:"source_healthy"`
	SourceType         string    `json:"sourceType" bun:"source_type"`
}

type ApplicationDetails struct {
	ID                      uuid.UUID       `json:"id" bun:"id"`
	Name                    string          `json:"name" bun:"name"`
	Slug                    string          `json:"slug" bun:"slug"`
	EnvironmentID           uuid.UUID       `json:"environmentId" bun:"environment_id"`
	EnvironmentName         string          `json:"environmentName" bun:"environment_name"`
	EnvironmentSlug         string          `json:"environmentSlug" bun:"environment_slug"`
	EnvironmentKind         string          `json:"environmentKind" bun:"environment_kind"`
	SetupComplete           bool            `json:"setupComplete" bun:"setup_complete"`
	EnvironmentSourceID     uuid.UUID       `json:"environmentSourceId" bun:"environment_source_id"`
	SourceType              string          `json:"sourceType" bun:"source_type"`
	RepositoryID            uuid.UUID       `json:"repositoryId" bun:"repository_id"`
	RepositoryFullName      string          `json:"repositoryFullName" bun:"repository_full_name"`
	RepositoryRemovedAt     sql.NullTime    `json:"repositoryRemovedAt" bun:"repository_removed_at"`
	InstallationID          uuid.UUID       `json:"installationId" bun:"installation_id"`
	InstallationAccount     string          `json:"installationAccount" bun:"installation_account"`
	InstallationSuspendedAt sql.NullTime    `json:"installationSuspendedAt" bun:"installation_suspended_at"`
	Reference               string          `json:"reference" bun:"reference"`
	AutoBuild               bool            `json:"autoBuild" bun:"auto_build"`
	ContextPath             string          `json:"contextPath" bun:"context_path"`
	BuilderReference        sql.NullString  `json:"builderReference" bun:"builder_reference"`
	BuildpackSettings       json.RawMessage `json:"buildpackSettings" bun:"buildpack_settings"`
	ImageRepository         string          `json:"imageRepository" bun:"image_repository"`
	RegistryName            string          `json:"registryName" bun:"registry_name"`
	RegistryID              uuid.UUID       `json:"registryId" bun:"registry_id"`
	BuildServerID           uuid.UUID       `json:"buildServerId" bun:"build_server_id"`
	BuildServerName         string          `json:"buildServerName" bun:"build_server_name"`
	LatestRevision          sql.NullString  `json:"latestRevision" bun:"latest_revision"`
	LatestDeliveryStatus    sql.NullString  `json:"latestDeliveryStatus" bun:"latest_delivery_status"`
	LatestBuildStatus       sql.NullString  `json:"latestBuildStatus" bun:"latest_build_status"`
}

type ApplicationSetup struct {
	db  storage.Pool
	cfg config.Config
}

func NewApplicationSetup(db storage.Pool, cfg config.Config) *ApplicationSetup {
	return &ApplicationSetup{db: db, cfg: cfg}
}

func (service *ApplicationSetup) Create(ctx context.Context, data ApplicationSetupData) (ApplicationSetupResult, error) {
	data.ApplicationName = strings.TrimSpace(data.ApplicationName)
	data.ApplicationSlug = slug.Make(strings.TrimSpace(data.ApplicationSlug))
	data.EnvironmentName = strings.TrimSpace(data.EnvironmentName)
	data.EnvironmentSlug = slug.Make(strings.TrimSpace(data.EnvironmentSlug))
	data.EnvironmentKind = strings.TrimSpace(data.EnvironmentKind)
	data.SourceType = strings.ToLower(strings.TrimSpace(data.SourceType))
	if data.SourceType == "" {
		data.SourceType = "buildpacks"
	}
	data.Reference = strings.TrimSpace(data.Reference)
	if data.SourceType == "buildpacks" {
		data.Reference = normalizeGitReference(data.Reference)
	}
	data.ImageRepository = strings.TrimSpace(data.ImageRepository)
	if data.ApplicationName == "" || data.ApplicationSlug == "" || data.ApplicationSlug == models.SystemApplicationSlug || data.EnvironmentName == "" || data.EnvironmentSlug == "" || data.EnvironmentKind == "" || data.Reference == "" || data.ImageRepository == "" {
		return ApplicationSetupResult{}, errors.Join(models.ErrDomainValidation, errors.New("application, environment, and source identity are required"))
	}
	if data.SourceType != "buildpacks" && data.SourceType != "image" {
		return ApplicationSetupResult{}, errors.Join(models.ErrDomainValidation, errors.New("source type must be Buildpacks or image"))
	}
	if len(data.BuildpackSettings) == 0 {
		data.BuildpackSettings = models.DefaultBuildpackSettings()
	}
	var repository models.GitHubRepositoryEntity
	if data.SourceType == "buildpacks" {
		var err error
		repository, _, err = service.validateRepository(ctx, data.GitHubInstallationID, data.GitHubRepositoryID)
		if err != nil {
			return ApplicationSetupResult{}, err
		}
	}
	if err := validateRegistrySelection(ctx, service.db.Executor(), data.RegistryResourceID); err != nil {
		return ApplicationSetupResult{}, err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "application-slug:"+data.ApplicationSlug); err != nil {
		return ApplicationSetupResult{}, err
	}
	var activeSlugCount int
	activeSlugCount, err = tx.NewSelect().TableExpr("applications").Where("slug = ?", data.ApplicationSlug).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	if activeSlugCount > 0 {
		return ApplicationSetupResult{}, errors.Join(models.ErrDomainValidation, errors.New("application slug is already in use"))
	}
	application, err := models.Application.Create(ctx, tx, models.CreateApplicationData{Name: data.ApplicationName, Slug: data.ApplicationSlug})
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	environment, err := models.Environment.Create(ctx, tx, models.CreateEnvironmentData{Name: data.EnvironmentName, Slug: data.EnvironmentSlug, Kind: data.EnvironmentKind, ApplicationID: application.ID})
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	sourceData := models.CreateEnvironmentSourceData{
		Kind: "image", Provider: "registry", Repository: data.ImageRepository, Reference: data.Reference,
		Settings: json.RawMessage(`{"schema_version":1}`), AutoBuild: false, EnvironmentID: environment.ID,
	}
	if data.SourceType == "buildpacks" {
		sourceData.Kind, sourceData.Provider, sourceData.Repository, sourceData.AutoBuild = "git", "github", repository.FullName, data.AutoBuild
	}
	source, err := models.EnvironmentSource.Create(ctx, tx, sourceData)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	if data.SourceType == "buildpacks" {
		if _, err := models.GitHubEnvironmentSource.Create(ctx, tx, source.ID, repository.ID); err != nil {
			return ApplicationSetupResult{}, err
		}
		builderReference := sql.NullString{String: strings.TrimSpace(data.BuilderReference), Valid: strings.TrimSpace(data.BuilderReference) != ""}
		if _, err := models.BuildpackConfiguration.Create(ctx, tx, models.CreateBuildpackConfigurationData{ContextPath: data.ContextPath, BuilderReference: builderReference, ImageRepository: data.ImageRepository, Settings: data.BuildpackSettings, EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID, ServerID: data.BuildServerID}); err != nil {
			return ApplicationSetupResult{}, err
		}
	} else if _, err := models.ImageConfiguration.Create(ctx, tx, models.CreateImageConfigurationData{EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID}); err != nil {
		return ApplicationSetupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApplicationSetupResult{}, err
	}
	return ApplicationSetupResult{Application: application, Environment: environment, Source: source}, nil
}

func (service *ApplicationSetup) Options(ctx context.Context) (ApplicationSetupOptions, error) {
	connection := NewGitHubConnection(service.db, service.cfg, nil)
	instanceID, err := connection.instanceID()
	if err != nil {
		return ApplicationSetupOptions{}, err
	}
	app, appErr := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if appErr != nil && !errors.Is(appErr, sql.ErrNoRows) {
		return ApplicationSetupOptions{}, appErr
	}
	summaries := make([]GitHubInstallationSummary, 0)
	repositories := make([]models.GitHubRepositoryEntity, 0)
	if appErr == nil {
		installations, err := models.GitHubInstallation.ListForApp(ctx, service.db.Executor(), app.ID)
		if err != nil {
			return ApplicationSetupOptions{}, err
		}
		for _, installation := range installations {
			if installation.ArchivedAt.Valid || installation.SuspendedAt.Valid {
				continue
			}
			installationRepositories, loadErr := models.GitHubRepository.ListActive(ctx, service.db.Executor(), installation.ID)
			if loadErr != nil {
				return ApplicationSetupOptions{}, loadErr
			}
			summaries = append(summaries, GitHubInstallationSummary{GitHubInstallationEntity: installation, RepositoryCount: len(installationRepositories)})
			repositories = append(repositories, installationRepositories...)
		}
	}
	registries := make([]RegistryResourceOption, 0)
	if err := service.db.Executor().NewSelect().TableExpr("registry_resources AS registry").ColumnExpr("resource.id, resource.name").ColumnExpr("CASE WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").Where("EXISTS (SELECT 1 FROM resource_credentials credential WHERE credential.resource_id = resource.id AND credential.archived_at IS NULL)").OrderExpr("resource.name ASC").Scan(ctx, &registries); err != nil {
		return ApplicationSetupOptions{}, err
	}
	buildServers := make([]ApplicationBuildServerOption, 0)
	if err := service.db.Executor().NewSelect().TableExpr("servers AS server").
		ColumnExpr("server.id, server.name, server.kind, server.address").
		Where("server.archived_at IS NULL").Where("server.is_configured = TRUE").
		Where("server.kind IN ('self_hosted', 'worker')").
		Where("server.capabilities @> '{\"build\":true}'::jsonb").
		OrderExpr("CASE WHEN server.kind = 'self_hosted' THEN 0 ELSE 1 END, server.name").Scan(ctx, &buildServers); err != nil {
		return ApplicationSetupOptions{}, err
	}
	return ApplicationSetupOptions{Installations: summaries, Repositories: repositories, Registries: registries, BuildServers: buildServers}, nil
}

func (service *ApplicationSetup) List(ctx context.Context) ([]ApplicationListItem, error) {
	var items []ApplicationListItem
	err := service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.id").ColumnExpr("application.name").ColumnExpr("application.slug").
		ColumnExpr("environment.name AS environment_name").ColumnExpr("environment.kind AS environment_kind").
		ColumnExpr("COALESCE(repository.full_name, source.repository) AS repository_full_name").ColumnExpr("source.reference").ColumnExpr("CASE WHEN source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type").
		ColumnExpr("CASE WHEN source.kind = 'image' THEN registry_resource.id IS NOT NULL AND registry_resource.archived_at IS NULL ELSE (repository.removed_at IS NULL AND installation.archived_at IS NULL AND installation.suspended_at IS NULL) END AS source_healthy").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").
		Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Join("LEFT JOIN resources AS registry_resource ON registry_resource.id = image.registry_resource_id").
		Where("application.archived_at IS NULL").Where("application.slug <> ?", models.SystemApplicationSlug).OrderExpr("application.name ASC").Scan(ctx, &items)
	return items, err
}

func (service *ApplicationSetup) Details(ctx context.Context, applicationID uuid.UUID) (ApplicationDetails, error) {
	return service.details(ctx, applicationID, nil)
}

func (service *ApplicationSetup) DetailsForEnvironment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (ApplicationDetails, error) {
	return service.details(ctx, applicationID, &environmentID)
}

func (service *ApplicationSetup) details(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID *uuid.UUID,
) (ApplicationDetails, error) {
	var details ApplicationDetails
	query := service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.id").ColumnExpr("application.name").ColumnExpr("application.slug").
		ColumnExpr("environment.id AS environment_id").ColumnExpr("environment.name AS environment_name").ColumnExpr("environment.slug AS environment_slug").ColumnExpr("environment.kind AS environment_kind").
		ColumnExpr(`EXISTS (
			SELECT 1 FROM changes AS setup_change
			JOIN change_state_revisions AS setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result'
			JOIN environment_state_revisions AS setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = environment.id
			WHERE setup_change.environment_id = environment.id AND setup_change.kind = 'environment_setup'
			AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL
		) AS setup_complete`).
		ColumnExpr("source.id AS environment_source_id").ColumnExpr("source.reference").ColumnExpr("source.auto_build").ColumnExpr("CASE WHEN source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type").
		ColumnExpr("repository.id AS repository_id").ColumnExpr("COALESCE(repository.full_name, source.repository) AS repository_full_name").ColumnExpr("repository.removed_at AS repository_removed_at").
		ColumnExpr("installation.id AS installation_id").ColumnExpr("COALESCE(installation.account_login, '') AS installation_account").ColumnExpr("installation.suspended_at AS installation_suspended_at").
		ColumnExpr("COALESCE(buildpack.context_path, '') AS context_path").ColumnExpr("buildpack.builder_reference").ColumnExpr("COALESCE(buildpack.settings, '{}'::jsonb) AS buildpack_settings").ColumnExpr("COALESCE(buildpack.image_repository, source.repository) AS image_repository").
		ColumnExpr("build_server.id AS build_server_id, COALESCE(build_server.name, '') AS build_server_name").
		ColumnExpr("registry_resource.id AS registry_id").ColumnExpr("registry_resource.name AS registry_name").
		ColumnExpr("(SELECT event.source_revision FROM source_events AS event WHERE event.environment_source_id = source.id ORDER BY event.received_at DESC LIMIT 1) AS latest_revision").
		ColumnExpr("(SELECT delivery.status FROM github_webhook_deliveries AS delivery WHERE delivery.repository_external_id = repository.external_id ORDER BY delivery.received_at DESC LIMIT 1) AS latest_delivery_status").
		ColumnExpr("(SELECT build.status FROM builds AS build WHERE build.environment_source_id = source.id ORDER BY build.created_at DESC LIMIT 1) AS latest_build_status").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").Join("LEFT JOIN servers AS build_server ON build_server.id = buildpack.server_id").Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id").
		Where("application.id = ?", applicationID).Where("application.archived_at IS NULL").Where("application.slug <> ?", models.SystemApplicationSlug)
	if environmentID != nil {
		query = query.Where("environment.id = ?", *environmentID)
	}
	err := query.Scan(ctx, &details)
	return details, err
}

func (service *ApplicationSetup) UpdatePresentation(ctx context.Context, applicationID uuid.UUID, name, applicationSlug string) error {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil {
		return err
	}
	applicationSlug = slug.Make(applicationSlug)
	if strings.TrimSpace(name) == "" || applicationSlug == "" || applicationSlug == models.SystemApplicationSlug {
		return errors.Join(models.ErrDomainValidation, errors.New("application name and slug are required"))
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "application-slug:"+applicationSlug); err != nil {
		return err
	}
	count, err := tx.NewSelect().TableExpr("applications").Where("slug = ?", applicationSlug).Where("id <> ?", application.ID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(models.ErrDomainValidation, errors.New("application slug is already in use"))
	}
	if _, err := models.Application.Update(ctx, tx, models.UpdateApplicationData{ID: application.ID, Name: strings.TrimSpace(name), Slug: applicationSlug, ArchivedAt: application.ArchivedAt}); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ApplicationSetup) UpdateSource(ctx context.Context, applicationID uuid.UUID, data ApplicationSetupData) error {
	details, err := service.Details(ctx, applicationID)
	if err != nil {
		return err
	}
	return service.updateSource(ctx, details, data)
}

func (service *ApplicationSetup) UpdateEnvironmentSource(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
	data ApplicationSetupData,
) error {
	details, err := service.DetailsForEnvironment(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	return service.updateSource(ctx, details, data)
}

func (service *ApplicationSetup) updateSource(
	ctx context.Context,
	details ApplicationDetails,
	data ApplicationSetupData,
) error {
	data.SourceType = strings.ToLower(strings.TrimSpace(data.SourceType))
	if data.SourceType == "" {
		data.SourceType = details.SourceType
	}
	data.Reference = strings.TrimSpace(data.Reference)
	data.ImageRepository = strings.TrimSpace(data.ImageRepository)
	if data.Reference == "" || data.ImageRepository == "" || (data.SourceType != "buildpacks" && data.SourceType != "image") {
		return errors.Join(models.ErrDomainValidation, errors.New("source type, image repository, and reference are required"))
	}
	var repository models.GitHubRepositoryEntity
	if data.SourceType == "buildpacks" {
		var err error
		repository, _, err = service.validateRepository(ctx, data.GitHubInstallationID, data.GitHubRepositoryID)
		if err != nil {
			return err
		}
		data.Reference = normalizeGitReference(data.Reference)
	}
	if err := validateRegistrySelection(ctx, service.db.Executor(), data.RegistryResourceID); err != nil {
		return err
	}
	if len(data.BuildpackSettings) == 0 {
		data.BuildpackSettings = models.DefaultBuildpackSettings()
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	source, err := models.EnvironmentSource.Find(ctx, tx, details.EnvironmentSourceID)
	if err != nil {
		return err
	}
	sourceData := models.UpdateEnvironmentSourceData{ID: source.ID, ArchivedAt: source.ArchivedAt, Kind: "image", Provider: "registry", Repository: data.ImageRepository, Reference: data.Reference, Settings: source.Settings, AutoBuild: false, EnvironmentID: source.EnvironmentID}
	if data.SourceType == "buildpacks" {
		sourceData.Kind, sourceData.Provider, sourceData.Repository, sourceData.AutoBuild = "git", "github", repository.FullName, data.AutoBuild
	}
	if _, err := models.EnvironmentSource.Update(ctx, tx, sourceData); err != nil {
		return err
	}
	for _, table := range []string{"github_environment_sources", "buildpack_configurations", "image_configurations"} {
		if _, err := tx.NewDelete().TableExpr(table).Where("environment_source_id = ?", source.ID).Exec(ctx); err != nil {
			return err
		}
	}
	if data.SourceType == "buildpacks" {
		if _, err := models.GitHubEnvironmentSource.Create(ctx, tx, source.ID, repository.ID); err != nil {
			return err
		}
		_, err := models.BuildpackConfiguration.Create(ctx, tx, models.CreateBuildpackConfigurationData{ContextPath: data.ContextPath, BuilderReference: sql.NullString{String: strings.TrimSpace(data.BuilderReference), Valid: strings.TrimSpace(data.BuilderReference) != ""}, ImageRepository: data.ImageRepository, Settings: data.BuildpackSettings, EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID, ServerID: data.BuildServerID})
		if err != nil {
			return err
		}
	} else if _, err := models.ImageConfiguration.Create(ctx, tx, models.CreateImageConfigurationData{EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID}); err != nil {
		return err
	}
	return tx.Commit()
}

func validateRegistrySelection(ctx context.Context, db storage.Executor, resourceID uuid.UUID) error {
	var selection struct {
		Engine          string `bun:"engine"`
		EndpointCount   int    `bun:"endpoint_count"`
		CredentialCount int    `bun:"credential_count"`
	}
	err := db.NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("resource.configuration ->> 'engine' AS engine").
		ColumnExpr("(SELECT count(*) FROM resource_endpoints endpoint WHERE endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL) AS endpoint_count").
		ColumnExpr("(SELECT count(*) FROM resource_credentials credential WHERE credential.resource_id = resource.id AND credential.archived_at IS NULL) AS credential_count").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").
		Where("registry.resource_id = ?", resourceID).Scan(ctx, &selection)
	if err != nil || selection.Engine != "registry" || selection.EndpointCount != 1 || selection.CredentialCount != 1 {
		return errors.Join(models.ErrDomainValidation, errors.New("Registry Resource is unavailable or does not have one active endpoint and access credential"))
	}
	return nil
}

func (service *ApplicationSetup) Archive(ctx context.Context, applicationID uuid.UUID) error {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil {
		return err
	}
	activeBuilds, err := service.db.Executor().NewSelect().TableExpr("builds AS build").Join("JOIN environments AS environment ON environment.id = build.environment_id").Where("environment.application_id = ?", application.ID).Where("build.status IN ('pending', 'running')").Count(ctx)
	if err != nil {
		return err
	}
	if activeBuilds > 0 {
		return errors.New("application has active build work")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("environment_sources AS source").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("source.environment_id IN (SELECT id FROM environments WHERE application_id = ?)", application.ID).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("environments").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("application_id = ?", application.ID).Exec(ctx); err != nil {
		return err
	}
	_, err = models.Application.Update(ctx, tx, models.UpdateApplicationData{ID: application.ID, Name: application.Name, Slug: application.Slug, ArchivedAt: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ApplicationSetup) validateRepository(ctx context.Context, installationID, repositoryID uuid.UUID) (models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), repositoryID)
	if err != nil || repository.RemovedAt.Valid || repository.GitHubInstallationID != installationID {
		return repository, models.GitHubInstallationEntity{}, errors.Join(models.ErrDomainValidation, errors.New("GitHub repository is unavailable"))
	}
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), installationID)
	if err != nil || installation.ArchivedAt.Valid || installation.SuspendedAt.Valid {
		return repository, installation, errors.Join(models.ErrDomainValidation, errors.New("GitHub installation is unavailable"))
	}
	instanceID, err := uuid.Parse(service.cfg.App.InstanceID)
	if err != nil {
		return repository, installation, err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil || app.ID != installation.GitHubAppID {
		return repository, installation, errors.Join(models.ErrDomainValidation, errors.New("GitHub installation does not belong to the active App"))
	}
	return repository, installation, nil
}

func normalizeGitReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "refs/heads/") || strings.HasPrefix(value, "refs/tags/") {
		return value
	}
	if strings.HasPrefix(value, "tags/") {
		return "refs/" + value
	}
	return "refs/heads/" + strings.TrimPrefix(value, "heads/")
}
