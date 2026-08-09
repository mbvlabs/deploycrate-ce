package services

import (
	"context"
	"database/sql"
	buildpacksclient "deploycrate-ce/clients/buildpacks"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"strings"

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

type ApplicationCreationData struct {
	ApplicationName string
	ApplicationSlug string
	Environments    []ApplicationSetupData
}

type ApplicationCreationResult struct {
	Application  models.ApplicationEntity
	Environments []ApplicationSetupResult
}

type preparedApplicationEnvironment struct {
	data       ApplicationSetupData
	repository models.GitHubRepositoryEntity
}

type ApplicationSetupOptions struct {
	Installations []GitHubInstallationSummary     `json:"installations"`
	Repositories  []models.GitHubRepositoryEntity `json:"repositories"`
	Registries    []RegistryResourceOption        `json:"registries"`
	BuildServers  []ApplicationBuildServerOption  `json:"buildServers"`
}

type ApplicationBuildServerOption = models.ApplicationBuildServerOption
type RegistryResourceOption = models.ApplicationRegistryOption

type ApplicationListItem struct {
	ID           uuid.UUID                    `json:"id"`
	Name         string                       `json:"name"`
	Slug         string                       `json:"slug"`
	Environments []ApplicationListEnvironment `json:"environments"`
}

type ApplicationListEnvironment struct {
	ID                 uuid.UUID `json:"id"                 bun:"environment_id"`
	EnvironmentName    string    `json:"environmentName"    bun:"environment_name"`
	EnvironmentKind    string    `json:"environmentKind"    bun:"environment_kind"`
	RepositoryFullName string    `json:"repositoryFullName" bun:"repository_full_name"`
	Reference          string    `json:"reference"          bun:"reference"`
	SourceHealthy      bool      `json:"sourceHealthy"      bun:"source_healthy"`
	SourceType         string    `json:"sourceType"         bun:"source_type"`
}

type ApplicationDetails = models.ApplicationDetails

type ApplicationOverview struct {
	ID           uuid.UUID                       `json:"id"`
	Name         string                          `json:"name"`
	Slug         string                          `json:"slug"`
	Environments []ApplicationDetails            `json:"environments"`
	Deployments  []ApplicationDeploymentActivity `json:"deployments"`
}

type ApplicationDeploymentActivity = models.ApplicationDeploymentActivity

type ApplicationSetup struct {
	db  storage.Pool
	cfg config.Config
}

func NewApplicationSetup(db storage.Pool, cfg config.Config) *ApplicationSetup {
	return &ApplicationSetup{db: db, cfg: cfg}
}

func (service *ApplicationSetup) Create(
	ctx context.Context,
	data ApplicationSetupData,
) (ApplicationSetupResult, error) {
	result, err := service.CreateApplication(ctx, ApplicationCreationData{
		ApplicationName: data.ApplicationName,
		ApplicationSlug: data.ApplicationSlug,
		Environments:    []ApplicationSetupData{data},
	})
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	return result.Environments[0], nil
}

func (service *ApplicationSetup) CreateEnvironment(
	ctx context.Context,
	applicationID uuid.UUID,
	data ApplicationSetupData,
) (ApplicationSetupResult, error) {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil || application.ArchivedAt.Valid || application.IsSystem() {
		return ApplicationSetupResult{}, sql.ErrNoRows
	}
	data.ApplicationName = application.Name
	data.ApplicationSlug = application.Slug
	prepared, err := service.prepareEnvironment(ctx, data)
	if err != nil {
		return ApplicationSetupResult{}, err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	defer tx.Rollback()
	available, err := models.Environment.EnsureSlugAvailable(
		ctx, tx, applicationID, prepared.data.EnvironmentSlug,
	)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	if !available {
		return ApplicationSetupResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment slug is already in use for this Application"),
		)
	}
	result, err := service.createEnvironment(ctx, tx, application, prepared)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApplicationSetupResult{}, err
	}
	return result, nil
}

func (service *ApplicationSetup) DeleteIncompleteEnvironment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	environment, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	)
	if err != nil || environment.ArchivedAt.Valid {
		return errors.New("Environment is unavailable")
	}
	complete, err := models.Environment.SetupComplete(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return err
	}
	if complete {
		return errors.New("completed Environment cannot be discarded as incomplete")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.Environment.DeleteIncompleteSources(ctx, tx, environmentID); err != nil {
		return err
	}
	if err := models.Environment.Destroy(ctx, tx, environmentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ApplicationSetup) CreateApplication(
	ctx context.Context,
	data ApplicationCreationData,
) (ApplicationCreationResult, error) {
	data.ApplicationName = strings.TrimSpace(data.ApplicationName)
	data.ApplicationSlug = slug.Make(strings.TrimSpace(data.ApplicationSlug))
	if data.ApplicationName == "" || data.ApplicationSlug == "" ||
		data.ApplicationSlug == models.SystemApplicationSlug {
		return ApplicationCreationResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("application name and slug are required"),
		)
	}
	if len(data.Environments) == 0 {
		return ApplicationCreationResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("a production Environment is required"),
		)
	}
	prepared := make([]preparedApplicationEnvironment, 0, len(data.Environments))
	kinds := make(map[string]struct{}, len(data.Environments))
	for _, environmentData := range data.Environments {
		environmentData.ApplicationName = data.ApplicationName
		environmentData.ApplicationSlug = data.ApplicationSlug
		preparedEnvironment, err := service.prepareEnvironment(ctx, environmentData)
		if err != nil {
			return ApplicationCreationResult{}, err
		}
		kind := preparedEnvironment.data.EnvironmentKind
		if kind != "staging" && kind != "production" {
			return ApplicationCreationResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("Environment kind must be staging or production"),
			)
		}
		if _, exists := kinds[kind]; exists {
			return ApplicationCreationResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("only one Environment of each kind is allowed"),
			)
		}
		kinds[kind] = struct{}{}
		prepared = append(prepared, preparedEnvironment)
	}
	if _, exists := kinds["production"]; !exists {
		return ApplicationCreationResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("a production Environment is required"),
		)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplicationCreationResult{}, err
	}
	defer tx.Rollback()
	available, err := models.Application.EnsureSlugAvailable(ctx, tx, data.ApplicationSlug, uuid.Nil)
	if err != nil {
		return ApplicationCreationResult{}, err
	}
	if !available {
		return ApplicationCreationResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("application slug is already in use"),
		)
	}
	application, err := models.Application.Create(
		ctx,
		tx,
		models.CreateApplicationData{Name: data.ApplicationName, Slug: data.ApplicationSlug},
	)
	if err != nil {
		return ApplicationCreationResult{}, err
	}
	results := make([]ApplicationSetupResult, 0, len(prepared))
	for _, environment := range prepared {
		result, createErr := service.createEnvironment(ctx, tx, application, environment)
		if createErr != nil {
			return ApplicationCreationResult{}, createErr
		}
		results = append(results, result)
	}
	if err := tx.Commit(); err != nil {
		return ApplicationCreationResult{}, err
	}
	return ApplicationCreationResult{Application: application, Environments: results}, nil
}

func (service *ApplicationSetup) prepareEnvironment(
	ctx context.Context,
	data ApplicationSetupData,
) (preparedApplicationEnvironment, error) {
	data.EnvironmentName = strings.TrimSpace(data.EnvironmentName)
	data.EnvironmentSlug = slug.Make(strings.TrimSpace(data.EnvironmentSlug))
	data.EnvironmentKind = strings.ToLower(strings.TrimSpace(data.EnvironmentKind))
	data.SourceType = strings.ToLower(strings.TrimSpace(data.SourceType))
	if data.SourceType == "" {
		data.SourceType = "buildpacks"
	}
	data.Reference = strings.TrimSpace(data.Reference)
	if data.SourceType == "buildpacks" {
		data.Reference = normalizeGitReference(data.Reference)
	}
	data.ImageRepository = strings.TrimSpace(data.ImageRepository)
	if data.EnvironmentName == "" || data.EnvironmentSlug == "" || data.EnvironmentKind == "" ||
		data.Reference == "" ||
		data.ImageRepository == "" {
		return preparedApplicationEnvironment{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("environment and source identity are required"),
		)
	}
	if data.SourceType != "buildpacks" && data.SourceType != "image" {
		return preparedApplicationEnvironment{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("source type must be Buildpack or Repository"),
		)
	}
	if len(data.BuildpackSettings) == 0 {
		data.BuildpackSettings = models.DefaultBuildpackSettings()
	}
	var repository models.GitHubRepositoryEntity
	if data.SourceType == "buildpacks" {
		var err error
		repository, _, err = service.validateRepository(
			ctx,
			data.GitHubInstallationID,
			data.GitHubRepositoryID,
		)
		if err != nil {
			return preparedApplicationEnvironment{}, err
		}
		settings, settingsErr := models.ParseBuildpackSettings(data.BuildpackSettings)
		if settingsErr != nil {
			return preparedApplicationEnvironment{}, errors.Join(models.ErrDomainValidation, settingsErr)
		}
		server, capabilityErr := models.RequireServerBuildpackCapability(
			ctx, service.db.Executor(), data.BuildServerID, settings.Runtime,
		)
		if capabilityErr != nil {
			return preparedApplicationEnvironment{}, capabilityErr
		}
		if _, profileErr := buildpacksclient.ProfileForArchitecture(
			string(settings.Runtime), server.Architecture.String,
		); profileErr != nil {
			return preparedApplicationEnvironment{}, errors.Join(models.ErrDomainValidation, profileErr)
		}
	}
	if err := validateRegistrySelection(
		ctx,
		service.db.Executor(),
		data.RegistryResourceID,
	); err != nil {
		return preparedApplicationEnvironment{}, err
	}
	return preparedApplicationEnvironment{data: data, repository: repository}, nil
}

func (service *ApplicationSetup) createEnvironment(
	ctx context.Context,
	tx storage.Executor,
	application models.ApplicationEntity,
	prepared preparedApplicationEnvironment,
) (ApplicationSetupResult, error) {
	data := prepared.data
	environment, err := models.Environment.Create(
		ctx,
		tx,
		models.CreateEnvironmentData{
			Name:          data.EnvironmentName,
			Slug:          data.EnvironmentSlug,
			Kind:          data.EnvironmentKind,
			ApplicationID: application.ID,
		},
	)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	sourceData := models.CreateEnvironmentSourceData{
		Kind:       "image",
		Provider:   "registry",
		Repository: data.ImageRepository,
		Reference:  data.Reference,
		Settings: json.RawMessage(
			`{"schema_version":1}`,
		),
		AutoBuild:     false,
		EnvironmentID: environment.ID,
	}
	if data.SourceType == "buildpacks" {
		sourceData.Kind, sourceData.Provider, sourceData.Repository, sourceData.AutoBuild = "git", "github", prepared.repository.FullName, data.AutoBuild
	}
	source, err := models.EnvironmentSource.Create(ctx, tx, sourceData)
	if err != nil {
		return ApplicationSetupResult{}, err
	}
	if data.SourceType == "buildpacks" {
		if _, err := models.GitHubEnvironmentSource.Create(
			ctx,
			tx,
			source.ID,
			prepared.repository.ID,
		); err != nil {
			return ApplicationSetupResult{}, err
		}
		builderReference := sql.NullString{
			String: strings.TrimSpace(data.BuilderReference),
			Valid:  strings.TrimSpace(data.BuilderReference) != "",
		}
		if _, err := models.BuildpackConfiguration.Create(
			ctx,
			tx,
			models.CreateBuildpackConfigurationData{
				ContextPath:         data.ContextPath,
				BuilderReference:    builderReference,
				ImageRepository:     data.ImageRepository,
				Settings:            data.BuildpackSettings,
				EnvironmentSourceID: source.ID,
				RegistryResourceID:  data.RegistryResourceID,
				ServerID:            data.BuildServerID,
			},
		); err != nil {
			return ApplicationSetupResult{}, err
		}
	} else if _, err := models.ImageConfiguration.Create(ctx, tx, models.CreateImageConfigurationData{EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID}); err != nil {
		return ApplicationSetupResult{}, err
	}
	return ApplicationSetupResult{
		Application: application,
		Environment: environment,
		Source:      source,
	}, nil
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
		installations, err := models.GitHubInstallation.ListForApp(
			ctx,
			service.db.Executor(),
			app.ID,
		)
		if err != nil {
			return ApplicationSetupOptions{}, err
		}
		for _, installation := range installations {
			if installation.ArchivedAt.Valid || installation.SuspendedAt.Valid {
				continue
			}
			installationRepositories, loadErr := models.GitHubRepository.ListActive(
				ctx,
				service.db.Executor(),
				installation.ID,
			)
			if loadErr != nil {
				return ApplicationSetupOptions{}, loadErr
			}
			summaries = append(
				summaries,
				GitHubInstallationSummary{
					GitHubInstallationEntity: installation,
					RepositoryCount:          len(installationRepositories),
				},
			)
			repositories = append(repositories, installationRepositories...)
		}
	}

	registries, err := models.RegistryResource.ApplicationOptions(ctx, service.db.Executor())
	if err != nil {
		return ApplicationSetupOptions{}, err
	}

	rows, err := models.Server.ApplicationBuildOptions(ctx, service.db.Executor())
	if err != nil {
		return ApplicationSetupOptions{}, err
	}
	buildServers := make([]ApplicationBuildServerOption, 0, len(rows))
	for _, row := range rows {
		capabilities, parseErr := models.ParseServerCapabilities(row.Capabilities)
		if parseErr != nil {
			continue
		}
		row.Buildpacks = capabilities.Buildpacks.Runtimes
		buildServers = append(buildServers, row)
	}

	return ApplicationSetupOptions{
		Installations: summaries,
		Repositories:  repositories,
		Registries:    registries,
		BuildServers:  buildServers,
	}, nil
}

func (service *ApplicationSetup) List(ctx context.Context) ([]ApplicationListItem, error) {
	rows, err := models.Application.EnvironmentRows(ctx, service.db.Executor())
	if err != nil {
		return nil, err
	}
	items := make([]ApplicationListItem, 0)
	indexes := make(map[uuid.UUID]int)
	for _, row := range rows {
		index, exists := indexes[row.ApplicationID]
		if !exists {
			index = len(items)
			indexes[row.ApplicationID] = index
			items = append(
				items,
				ApplicationListItem{
					ID:           row.ApplicationID,
					Name:         row.ApplicationName,
					Slug:         row.ApplicationSlug,
					Environments: make([]ApplicationListEnvironment, 0, 2),
				},
			)
		}
		items[index].Environments = append(items[index].Environments, ApplicationListEnvironment{
			ID:                 row.EnvironmentID,
			EnvironmentName:    row.EnvironmentName,
			EnvironmentKind:    row.EnvironmentKind,
			RepositoryFullName: row.RepositoryFullName,
			Reference:          row.Reference,
			SourceHealthy:      row.SourceHealthy,
			SourceType:         row.SourceType,
		})
	}
	return items, nil
}

func (service *ApplicationSetup) Details(
	ctx context.Context,
	applicationID uuid.UUID,
) (ApplicationDetails, error) {
	return service.details(ctx, applicationID, nil)
}

func (service *ApplicationSetup) Overview(
	ctx context.Context,
	applicationID uuid.UUID,
) (ApplicationOverview, error) {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil || application.ArchivedAt.Valid ||
		application.Slug == models.SystemApplicationSlug {
		return ApplicationOverview{}, sql.ErrNoRows
	}
	environmentIDs, err := models.Environment.ActiveIDsForApplication(
		ctx, service.db.Executor(), applicationID,
	)
	if err != nil {
		return ApplicationOverview{}, err
	}
	environments := make([]ApplicationDetails, 0, len(environmentIDs))
	for _, environmentID := range environmentIDs {
		details, detailsErr := service.DetailsForEnvironment(ctx, applicationID, environmentID)
		if detailsErr != nil {
			return ApplicationOverview{}, detailsErr
		}
		environments = append(environments, details)
	}
	for i := range environments {
		if !strings.EqualFold(strings.TrimSpace(environments[i].EnvironmentKind), "staging") {
			continue
		}
		canPromote, targetName, latestDeploymentID, latestReleaseID, promotionErr := promotionOverview(
			ctx,
			service.db.Executor(),
			applicationID,
			environments[i].EnvironmentID,
			environments[i].EnvironmentKind,
			environments[i].SetupComplete,
		)
		if promotionErr != nil {
			return ApplicationOverview{}, promotionErr
		}
		environments[i].CanPromoteToProduction = canPromote
		environments[i].PromotionTargetName = targetName
		environments[i].LatestSuccessfulDeploymentID = latestDeploymentID
		environments[i].LatestSuccessfulReleaseID = latestReleaseID
		break
	}
	deployments, err := models.Application.RecentDeploymentActivity(
		ctx, service.db.Executor(), applicationID,
	)
	if err != nil {
		return ApplicationOverview{}, err
	}
	return ApplicationOverview{
		ID: application.ID, Name: application.Name, Slug: application.Slug,
		Environments: environments, Deployments: deployments,
	}, nil
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
	return models.Application.Details(ctx, service.db.Executor(), applicationID, environmentID)
}

func (service *ApplicationSetup) UpdatePresentation(
	ctx context.Context,
	applicationID uuid.UUID,
	name, applicationSlug string,
) error {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil {
		return err
	}
	applicationSlug = slug.Make(applicationSlug)
	if strings.TrimSpace(name) == "" || applicationSlug == "" ||
		applicationSlug == models.SystemApplicationSlug {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("application name and slug are required"),
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	available, err := models.Application.EnsureSlugAvailable(
		ctx, tx, applicationSlug, application.ID,
	)
	if err != nil {
		return err
	}
	if !available {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("application slug is already in use"),
		)
	}
	if _, err := models.Application.Update(
		ctx,
		tx,
		models.UpdateApplicationData{
			ID:         application.ID,
			Name:       strings.TrimSpace(name),
			Slug:       applicationSlug,
			ArchivedAt: application.ArchivedAt,
		},
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ApplicationSetup) UpdateSource(
	ctx context.Context,
	applicationID uuid.UUID,
	data ApplicationSetupData,
) error {
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
	if data.Reference == "" || data.ImageRepository == "" ||
		(data.SourceType != "buildpacks" && data.SourceType != "image") {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("source type, image repository, and reference are required"),
		)
	}
	var repository models.GitHubRepositoryEntity
	if data.SourceType == "buildpacks" {
		var err error
		repository, _, err = service.validateRepository(
			ctx,
			data.GitHubInstallationID,
			data.GitHubRepositoryID,
		)
		if err != nil {
			return err
		}
		data.Reference = normalizeGitReference(data.Reference)
	}
	if err := validateRegistrySelection(
		ctx,
		service.db.Executor(),
		data.RegistryResourceID,
	); err != nil {
		return err
	}
	if len(data.BuildpackSettings) == 0 {
		data.BuildpackSettings = models.DefaultBuildpackSettings()
	}
	if data.SourceType == "buildpacks" {
		requestedSettings, settingsErr := models.ParseBuildpackSettings(data.BuildpackSettings)
		if settingsErr != nil {
			return errors.Join(models.ErrDomainValidation, settingsErr)
		}
		if details.SetupComplete {
			configuredRuntime := models.BuildpackRuntime(details.Runtime)
			if !models.IsSupportedBuildpackRuntime(configuredRuntime) {
				return errors.New("Environment runtime configuration is unavailable")
			}
			if requestedSettings.Runtime != configuredRuntime {
				return errors.Join(
					models.ErrDomainValidation,
					errors.New("Buildpacks runtime cannot change after Environment setup"),
				)
			}
		}
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
	sourceData := models.UpdateEnvironmentSourceData{
		ID:            source.ID,
		ArchivedAt:    source.ArchivedAt,
		Kind:          "image",
		Provider:      "registry",
		Repository:    data.ImageRepository,
		Reference:     data.Reference,
		Settings:      source.Settings,
		AutoBuild:     false,
		EnvironmentID: source.EnvironmentID,
	}
	if data.SourceType == "buildpacks" {
		sourceData.Kind, sourceData.Provider, sourceData.Repository, sourceData.AutoBuild = "git", "github", repository.FullName, data.AutoBuild
	}
	if _, err := models.EnvironmentSource.Update(ctx, tx, sourceData); err != nil {
		return err
	}
	if err := models.EnvironmentSource.DeleteConfiguration(ctx, tx, source.ID); err != nil {
		return err
	}
	if data.SourceType == "buildpacks" {
		if _, err := models.GitHubEnvironmentSource.Create(
			ctx,
			tx,
			source.ID,
			repository.ID,
		); err != nil {
			return err
		}
		_, err := models.BuildpackConfiguration.Create(
			ctx,
			tx,
			models.CreateBuildpackConfigurationData{
				ContextPath: data.ContextPath,
				BuilderReference: sql.NullString{
					String: strings.TrimSpace(data.BuilderReference),
					Valid:  strings.TrimSpace(data.BuilderReference) != "",
				},
				ImageRepository:     data.ImageRepository,
				Settings:            data.BuildpackSettings,
				EnvironmentSourceID: source.ID,
				RegistryResourceID:  data.RegistryResourceID,
				ServerID:            data.BuildServerID,
			},
		)
		if err != nil {
			return err
		}
	} else if _, err := models.ImageConfiguration.Create(ctx, tx, models.CreateImageConfigurationData{EnvironmentSourceID: source.ID, RegistryResourceID: data.RegistryResourceID}); err != nil {
		return err
	}
	return tx.Commit()
}

func validateRegistrySelection(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) error {
	valid, err := models.RegistryResource.ValidApplicationSelection(ctx, db, resourceID)
	if err != nil || !valid {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New(
				"Registry Resource is unavailable or does not have one active endpoint and access credential",
			),
		)
	}
	return nil
}

func (service *ApplicationSetup) validateRepository(
	ctx context.Context,
	installationID, repositoryID uuid.UUID,
) (models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), repositoryID)
	if err != nil || repository.RemovedAt.Valid ||
		repository.GitHubInstallationID != installationID {
		return repository, models.GitHubInstallationEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("GitHub repository is unavailable"),
		)
	}
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), installationID)
	if err != nil || installation.ArchivedAt.Valid || installation.SuspendedAt.Valid {
		return repository, installation, errors.Join(
			models.ErrDomainValidation,
			errors.New("GitHub installation is unavailable"),
		)
	}
	instanceID, err := uuid.Parse(service.cfg.App.InstanceID)
	if err != nil {
		return repository, installation, err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil || app.ID != installation.GitHubAppID {
		return repository, installation, errors.Join(
			models.ErrDomainValidation,
			errors.New("GitHub installation does not belong to the active App"),
		)
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
