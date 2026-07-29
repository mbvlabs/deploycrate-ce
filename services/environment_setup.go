package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/riverqueue/river/rivertype"
)

var goTargetsPattern = regexp.MustCompile(`^[A-Za-z0-9_./,*-]*$`)

const (
	resourceCredentialProjectionConnectionURL   = "connection_url"
	resourceCredentialProjectionIndividualParts = "individual_parts"
)

type EnvironmentSetup struct {
	db         storage.Pool
	queue      storage.InsertQueue
	jobControl BuildJobControl
	github     *GitHubConnection
	secrets    *EnvironmentSecrets
	resources  *ResourceManagement
	caddy      CaddyRouteService
	container  containerclient.WorkloadClient
}

func NewEnvironmentSetup(
	db storage.Pool,
	queue storage.InsertQueue,
	jobControl BuildJobControl,
	github *GitHubConnection,
	secrets *EnvironmentSecrets,
	resources *ResourceManagement,
	caddy CaddyRouteService,
) *EnvironmentSetup {
	return &EnvironmentSetup{
		db: db, queue: queue, jobControl: jobControl, github: github, secrets: secrets,
		resources: resources, caddy: caddy, container: containerclient.NewWorkload(),
	}
}

type BuildJobControl interface {
	CancelJob(context.Context, int64) error
	CancelJobTx(context.Context, *sql.Tx, int64) error
	DeleteJob(context.Context, int64) error
	DeleteJobTx(context.Context, *sql.Tx, int64) error
	RetryJobTx(context.Context, *sql.Tx, int64) error
}

type EnvironmentSetupSecretInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EnvironmentSetupResourceInput struct {
	ResourceID           uuid.UUID  `json:"resourceId"`
	EndpointID           uuid.UUID  `json:"endpointId"`
	CredentialID         *uuid.UUID `json:"credentialId"`
	Alias                string     `json:"alias"`
	Database             string     `json:"database"`
	CredentialProjection string     `json:"credentialProjection"`
}

type EnvironmentSetupInput struct {
	Hostname      string
	ContainerPort int32
	HealthPath    string
	BPGOTargets   string
	Resources     []EnvironmentSetupResourceInput
	Secrets       []EnvironmentSetupSecretInput
}

type EnvironmentSetupResult struct {
	Environment models.EnvironmentEntity              `json:"environment"`
	Revision    models.EnvironmentStateRevisionEntity `json:"revision"`
	Build       models.BuildEntity                    `json:"build"`
}

type EnvironmentSetupResourceOption struct {
	ID           uuid.UUID  `json:"id" bun:"id"`
	Name         string     `json:"name" bun:"name"`
	Kind         string     `json:"kind" bun:"kind"`
	Database     string     `json:"database" bun:"database_name"`
	EndpointID   uuid.UUID  `json:"endpointId" bun:"endpoint_id"`
	Endpoint     string     `json:"endpoint" bun:"endpoint"`
	CredentialID *uuid.UUID `json:"credentialId" bun:"credential_id"`
	Credential   string     `json:"credential" bun:"credential"`
}

type EnvironmentSetupOptions struct {
	Resources []EnvironmentSetupResourceOption `json:"resources"`
}

type EnvironmentEditConfiguration struct {
	Name          string                          `json:"name"`
	Slug          string                          `json:"slug"`
	Kind          string                          `json:"kind"`
	Hostname      string                          `json:"hostname"`
	ContainerPort int32                           `json:"containerPort"`
	HealthPath    string                          `json:"healthPath"`
	BPGOTargets   string                          `json:"bpGoTargets"`
	Resources     []EnvironmentSetupResourceInput `json:"resources"`
}

type EnvironmentEditData struct {
	Overview      EnvironmentOverview          `json:"overview"`
	Configuration EnvironmentEditConfiguration `json:"configuration"`
}

type EnvironmentEditInput struct {
	Name          string
	Slug          string
	Kind          string
	Hostname      string
	ContainerPort int32
	HealthPath    string
	BPGOTargets   string
	Resources     []EnvironmentSetupResourceInput
}

type EnvironmentOverview struct {
	ApplicationID    uuid.UUID                          `json:"applicationId"`
	ApplicationName  string                             `json:"applicationName"`
	Environment      models.EnvironmentEntity           `json:"environment"`
	SetupComplete    bool                               `json:"setupComplete"`
	Repository       string                             `json:"repository"`
	Reference        string                             `json:"reference"`
	ContextPath      string                             `json:"contextPath"`
	RegistryName     string                             `json:"registryName"`
	RegistryEndpoint string                             `json:"registryEndpoint"`
	Deployability    models.EnvironmentDeployability    `json:"deployability"`
	Secrets          []models.EnvironmentSecretMetadata `json:"secrets"`
	Variables        []EnvironmentVariableActivity      `json:"variables"`
	Domain           string                             `json:"domain"`
	Resources        []EnvironmentResourceActivity      `json:"resources"`
	Builds           []EnvironmentBuildActivity         `json:"builds"`
	Releases         []EnvironmentReleaseActivity       `json:"releases"`
	Deployments      []EnvironmentDeploymentActivity    `json:"deployments"`
	Instances        []EnvironmentInstanceActivity      `json:"instances"`
}

type EnvironmentListItem struct {
	ID                uuid.UUID `json:"id" bun:"id"`
	Name              string    `json:"name" bun:"name"`
	Kind              string    `json:"kind" bun:"kind"`
	ApplicationID     uuid.UUID `json:"applicationId" bun:"application_id"`
	ApplicationName   string    `json:"applicationName" bun:"application_name"`
	SetupComplete     bool      `json:"setupComplete" bun:"setup_complete"`
	Domain            string    `json:"domain" bun:"domain"`
	Repository        string    `json:"repository" bun:"repository"`
	Reference         string    `json:"reference" bun:"reference"`
	LatestBuildStatus string    `json:"latestBuildStatus" bun:"latest_build_status"`
}

func (service *EnvironmentSetup) List(ctx context.Context) ([]EnvironmentListItem, error) {
	environments := make([]EnvironmentListItem, 0)
	err := service.db.Executor().NewSelect().TableExpr("environments AS environment").
		ColumnExpr("environment.id, environment.name, environment.kind").
		ColumnExpr("application.id AS application_id, application.name AS application_name").
		ColumnExpr(`EXISTS (
			SELECT 1 FROM changes AS setup_change
			JOIN change_state_revisions AS setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result'
			JOIN environment_state_revisions AS setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = environment.id
			WHERE setup_change.environment_id = environment.id AND setup_change.kind = 'environment_setup'
			AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL
		) AS setup_complete`).
		ColumnExpr("COALESCE((SELECT hostname FROM environment_domains WHERE environment_id = environment.id AND is_primary AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS domain").
		ColumnExpr("COALESCE((SELECT repository FROM environment_sources WHERE environment_id = environment.id AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS repository").
		ColumnExpr("COALESCE((SELECT reference FROM environment_sources WHERE environment_id = environment.id AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS reference").
		ColumnExpr("COALESCE((SELECT status FROM builds WHERE environment_id = environment.id ORDER BY created_at DESC LIMIT 1), '') AS latest_build_status").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
		Where("environment.archived_at IS NULL").Where("application.slug <> ?", models.SystemApplicationSlug).
		OrderExpr("application.name, environment.name").Scan(ctx, &environments)
	return environments, err
}

type EnvironmentResourceActivity struct {
	ID    uuid.UUID `json:"id" bun:"id"`
	Alias string    `json:"alias" bun:"alias"`
	Name  string    `json:"name" bun:"name"`
	Kind  string    `json:"kind" bun:"kind"`
}

type EnvironmentVariableActivity struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
}

type EnvironmentBuildActivity struct {
	ID               uuid.UUID  `json:"id" bun:"id"`
	SourceRevision   string     `json:"sourceRevision" bun:"source_revision"`
	Status           string     `json:"status" bun:"status"`
	CurrentStep      string     `json:"currentStep" bun:"current_step"`
	Error            string     `json:"error" bun:"error"`
	CreatedAt        time.Time  `json:"createdAt" bun:"created_at"`
	StartedAt        *time.Time `json:"startedAt" bun:"started_at"`
	FinishedAt       *time.Time `json:"finishedAt" bun:"finished_at"`
	RegistryEndpoint string     `json:"registryEndpoint" bun:"registry_endpoint"`
	JobID            *int64     `json:"jobId" bun:"job_id"`
	JobState         string     `json:"jobState" bun:"job_state"`
}

type EnvironmentBuildLogSnapshot struct {
	Build        EnvironmentBuildActivity `json:"build"`
	Logs         []models.BuildLogEntity  `json:"logs"`
	NextSequence int64                    `json:"nextSequence"`
	HasMore      bool                     `json:"hasMore"`
}

type EnvironmentReleaseActivity struct {
	ID                uuid.UUID `json:"id" bun:"id"`
	SourceRevision    string    `json:"sourceRevision" bun:"source_revision"`
	ArtifactReference string    `json:"artifactReference" bun:"artifact_reference"`
	CreatedAt         time.Time `json:"createdAt" bun:"created_at"`
}

type EnvironmentDeploymentActivity struct {
	ID          uuid.UUID `json:"id" bun:"id"`
	Status      string    `json:"status" bun:"status"`
	CurrentStep string    `json:"currentStep" bun:"current_step"`
	Error       string    `json:"error" bun:"error"`
	ReleaseID   uuid.UUID `json:"releaseId" bun:"release_id"`
	CreatedAt   time.Time `json:"createdAt" bun:"created_at"`
}

type EnvironmentDeploymentEventActivity struct {
	ID         uuid.UUID `json:"id"`
	Sequence   int64     `json:"sequence"`
	EventType  string    `json:"eventType"`
	Status     string    `json:"status"`
	Step       string    `json:"step"`
	Message    string    `json:"message"`
	Error      string    `json:"error"`
	OccurredAt time.Time `json:"occurredAt"`
}

type EnvironmentDeploymentEventSnapshot struct {
	Deployment   EnvironmentDeploymentActivity        `json:"deployment"`
	Events       []EnvironmentDeploymentEventActivity `json:"events"`
	NextSequence int64                                `json:"nextSequence"`
	HasMore      bool                                 `json:"hasMore"`
}

type EnvironmentInstanceActivity struct {
	ID         uuid.UUID       `json:"id" bun:"id"`
	State      string          `json:"state" bun:"state"`
	Slot       string          `json:"slot" bun:"slot"`
	Ports      json.RawMessage `json:"ports" bun:"ports"`
	ReleaseID  uuid.UUID       `json:"releaseId" bun:"release_id"`
	ObservedAt time.Time       `json:"observedAt" bun:"observed_at"`
}

func (service *EnvironmentSetup) Overview(ctx context.Context, applicationID, environmentID uuid.UUID) (EnvironmentOverview, error) {
	environment, err := models.Environment.FindForApplication(ctx, service.db.Executor(), applicationID, environmentID)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	var source struct {
		ApplicationName  string `bun:"application_name"`
		Repository       string `bun:"repository"`
		Reference        string `bun:"reference"`
		ContextPath      string `bun:"context_path"`
		RegistryName     string `bun:"registry_name"`
		RegistryEndpoint string `bun:"registry_endpoint"`
	}
	err = service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.name AS application_name, environment_source.repository, environment_source.reference, buildpack.context_path").
		ColumnExpr("registry.name AS registry_name, registry.endpoint AS registry_endpoint").
		Join("JOIN environment_sources AS environment_source ON environment_source.environment_id = ? AND environment_source.archived_at IS NULL", environmentID).
		Join("JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = environment_source.id").
		Join("JOIN container_registries AS registry ON registry.id = buildpack.container_registry_id").
		Where("application.id = ?", applicationID).Limit(1).Scan(ctx, &source)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	deployability, err := models.Environment.Deployability(ctx, service.db.Executor(), environmentID)
	if err != nil && setupComplete {
		return EnvironmentOverview{}, err
	}
	activeSecrets, err := models.EnvironmentSecret.ActiveForEnvironment(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	secretMetadata := make([]models.EnvironmentSecretMetadata, 0, len(activeSecrets))
	for _, secret := range activeSecrets {
		secretMetadata = append(secretMetadata, secret.Sanitized())
	}
	variables := make([]EnvironmentVariableActivity, 0)
	if setupComplete {
		revision, revisionErr := models.EnvironmentStateRevision.LatestCommitted(ctx, service.db.Executor(), environmentID)
		if revisionErr != nil {
			return EnvironmentOverview{}, revisionErr
		}
		state, stateErr := models.ParseEnvironmentDesiredState(revision.State)
		if stateErr != nil {
			return EnvironmentOverview{}, stateErr
		}
		for _, resource := range state.Resources {
			for key, value := range resource.Variables {
				variables = append(variables, EnvironmentVariableActivity{
					Key: key, Value: value, Source: resource.Alias, SourceID: resource.EnvironmentResourceID.String(),
				})
			}
		}
		sort.Slice(variables, func(left, right int) bool { return variables[left].Key < variables[right].Key })
	}
	var domain string
	_ = service.db.Executor().NewSelect().TableExpr("environment_domains").ColumnExpr("hostname").Where("environment_id = ?", environmentID).Where("is_primary = TRUE").Where("archived_at IS NULL").Limit(1).Scan(ctx, &domain)
	resources := make([]EnvironmentResourceActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("environment_resources AS connection").ColumnExpr("connection.id, connection.alias, resource.name, resource.kind").Join("JOIN resources AS resource ON resource.id = connection.resource_id").Where("connection.environment_id = ?", environmentID).Where("connection.archived_at IS NULL").OrderExpr("connection.alias").Scan(ctx, &resources); err != nil {
		return EnvironmentOverview{}, err
	}
	builds := make([]EnvironmentBuildActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("builds").
		ColumnExpr("id, source_revision, status, COALESCE(current_step, '') AS current_step, COALESCE(error, '') AS error, created_at, started_at, finished_at").
		ColumnExpr("COALESCE(build_configuration ->> 'registry_endpoint', '') AS registry_endpoint").
		ColumnExpr("(SELECT job.id FROM river_job AS job WHERE job.kind = 'build_source' AND job.args ->> 'build_id' = builds.id::text ORDER BY job.id DESC LIMIT 1) AS job_id").
		ColumnExpr("COALESCE((SELECT job.state::text FROM river_job AS job WHERE job.kind = 'build_source' AND job.args ->> 'build_id' = builds.id::text ORDER BY job.id DESC LIMIT 1), '') AS job_state").
		Where("environment_id = ?", environmentID).OrderExpr("created_at DESC").Limit(20).Scan(ctx, &builds); err != nil {
		return EnvironmentOverview{}, err
	}
	releases := make([]EnvironmentReleaseActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("releases").ColumnExpr("id, COALESCE(source_revision, '') AS source_revision, artifact_reference, created_at").Where("environment_id = ?", environmentID).Where("build_id IS NOT NULL").OrderExpr("created_at DESC").Limit(20).Scan(ctx, &releases); err != nil {
		return EnvironmentOverview{}, err
	}
	deployments := make([]EnvironmentDeploymentActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("deployments AS deployment").ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.created_at").Join("JOIN releases AS release ON release.id = deployment.release_id").Where("release.environment_id = ?", environmentID).Where("release.build_id IS NOT NULL").OrderExpr("deployment.created_at DESC").Limit(30).Scan(ctx, &deployments); err != nil {
		return EnvironmentOverview{}, err
	}
	instances := make([]EnvironmentInstanceActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("instances AS instance").ColumnExpr("instance.id, instance.state, instance.slot, instance.ports, instance.release_id, instance.observed_at").Join("JOIN releases AS release ON release.id = instance.release_id").Where("release.environment_id = ?", environmentID).Where("release.build_id IS NOT NULL").Where("instance.removed_at IS NULL").OrderExpr("instance.created_at DESC").Limit(30).Scan(ctx, &instances); err != nil {
		return EnvironmentOverview{}, err
	}
	return EnvironmentOverview{
		ApplicationID: applicationID, ApplicationName: source.ApplicationName, Environment: environment, SetupComplete: setupComplete,
		Repository: source.Repository, Reference: source.Reference, ContextPath: source.ContextPath,
		RegistryName: source.RegistryName, RegistryEndpoint: source.RegistryEndpoint,
		Deployability: deployability, Secrets: secretMetadata, Variables: variables, Domain: domain,
		Resources: resources, Builds: builds, Releases: releases, Deployments: deployments, Instances: instances,
	}, nil
}

func (service *EnvironmentSetup) EditData(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (EnvironmentEditData, error) {
	overview, err := service.Overview(ctx, applicationID, environmentID)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	if !overview.SetupComplete {
		return EnvironmentEditData{}, errors.New("Environment setup is incomplete")
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	type resourceRow struct {
		ID                   uuid.UUID       `bun:"id"`
		ResourceID           uuid.UUID       `bun:"resource_id"`
		ResourceEndpointID   uuid.UUID       `bun:"resource_endpoint_id"`
		ResourceCredentialID *uuid.UUID      `bun:"resource_credential_id"`
		Alias                string          `bun:"alias"`
		Configuration        json.RawMessage `bun:"configuration"`
	}
	rows := make([]resourceRow, 0)
	if err := service.db.Executor().NewSelect().TableExpr("environment_resources").
		Column("id", "resource_id", "resource_endpoint_id", "resource_credential_id", "alias", "configuration").
		Where("environment_id = ?", environmentID).Where("archived_at IS NULL").OrderExpr("alias").Scan(ctx, &rows); err != nil {
		return EnvironmentEditData{}, err
	}
	resources := make([]EnvironmentSetupResourceInput, 0, len(rows))
	for _, row := range rows {
		var configuration struct {
			Database             string `json:"database"`
			CredentialProjection string `json:"credential_projection"`
		}
		if json.Unmarshal(row.Configuration, &configuration) != nil {
			return EnvironmentEditData{}, errors.New("Environment Resource configuration is invalid")
		}
		resources = append(resources, EnvironmentSetupResourceInput{
			ResourceID: row.ResourceID, EndpointID: row.ResourceEndpointID,
			CredentialID: row.ResourceCredentialID, Alias: row.Alias, Database: configuration.Database,
			CredentialProjection: configuration.CredentialProjection,
		})
	}
	return EnvironmentEditData{
		Overview: overview,
		Configuration: EnvironmentEditConfiguration{
			Name: overview.Environment.Name, Slug: overview.Environment.Slug, Kind: overview.Environment.Kind,
			Hostname: state.Domain.Hostname, ContainerPort: state.Runtime.ContainerPort,
			HealthPath: state.Runtime.HealthPath, BPGOTargets: state.Runtime.BPGOTargets, Resources: resources,
		},
	}, nil
}

func (service *EnvironmentSetup) BuildLogs(
	ctx context.Context,
	environmentID, buildID uuid.UUID,
	after int64,
) (EnvironmentBuildLogSnapshot, error) {
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return EnvironmentBuildLogSnapshot{}, sql.ErrNoRows
	}
	logs, err := models.BuildLog.ForBuildAfter(ctx, service.db.Executor(), buildID, after, 501)
	if err != nil {
		return EnvironmentBuildLogSnapshot{}, err
	}
	hasMore := len(logs) > 500
	if hasMore {
		logs = logs[:500]
	}
	nextSequence := after
	if len(logs) > 0 {
		nextSequence = logs[len(logs)-1].Sequence
	}
	var startedAt, finishedAt *time.Time
	if build.StartedAt.Valid {
		value := build.StartedAt.Time
		startedAt = &value
	}
	if build.FinishedAt.Valid {
		value := build.FinishedAt.Time
		finishedAt = &value
	}
	var registrySnapshot struct {
		RegistryEndpoint string `json:"registry_endpoint"`
	}
	_ = json.Unmarshal(build.BuildConfiguration, &registrySnapshot)
	job, jobErr := models.Job.FindForBuild(ctx, service.db.Executor(), build.ID)
	var jobID *int64
	var jobState string
	if jobErr == nil {
		jobID = &job.ID
		jobState = job.State
	}
	return EnvironmentBuildLogSnapshot{
		Build: EnvironmentBuildActivity{
			ID: build.ID, SourceRevision: build.SourceRevision, Status: build.Status,
			CurrentStep: build.CurrentStep.String, Error: build.Error.String, CreatedAt: build.CreatedAt,
			StartedAt: startedAt, FinishedAt: finishedAt, RegistryEndpoint: registrySnapshot.RegistryEndpoint,
			JobID: jobID, JobState: jobState,
		},
		Logs: logs, NextSequence: nextSequence, HasMore: hasMore,
	}, nil
}

func (service *EnvironmentSetup) DeploymentEvents(
	ctx context.Context,
	environmentID, deploymentID uuid.UUID,
	after int64,
) (EnvironmentDeploymentEventSnapshot, error) {
	deployment, err := models.Deployment.Find(ctx, service.db.Executor(), deploymentID)
	if err != nil {
		return EnvironmentDeploymentEventSnapshot{}, sql.ErrNoRows
	}
	release, err := models.Release.Find(ctx, service.db.Executor(), deployment.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID || release.BuildID == nil {
		return EnvironmentDeploymentEventSnapshot{}, sql.ErrNoRows
	}
	eventEntities, err := models.DeploymentEvent.ForDeploymentAfter(ctx, service.db.Executor(), deploymentID, after, 501)
	if err != nil {
		return EnvironmentDeploymentEventSnapshot{}, err
	}
	hasMore := len(eventEntities) > 500
	if hasMore {
		eventEntities = eventEntities[:500]
	}
	events := make([]EnvironmentDeploymentEventActivity, 0, len(eventEntities))
	nextSequence := after
	for _, event := range eventEntities {
		events = append(events, EnvironmentDeploymentEventActivity{
			ID: event.ID, Sequence: event.Sequence, EventType: event.EventType,
			Status: event.Status.String, Step: event.Step.String, Message: event.Message,
			Error: event.Error.String, OccurredAt: event.OccurredAt,
		})
		nextSequence = event.Sequence
	}
	return EnvironmentDeploymentEventSnapshot{
		Deployment: EnvironmentDeploymentActivity{
			ID: deployment.ID, Status: deployment.Status, CurrentStep: deployment.CurrentStep.String,
			Error: deployment.Error.String, ReleaseID: deployment.ReleaseID, CreatedAt: deployment.CreatedAt,
		},
		Events: events, NextSequence: nextSequence, HasMore: hasMore,
	}, nil
}

func (service *EnvironmentSetup) Options(ctx context.Context) (EnvironmentSetupOptions, error) {
	options := make([]EnvironmentSetupResourceOption, 0)
	err := service.db.Executor().NewSelect().TableExpr("resources AS resource").
		ColumnExpr("resource.id, resource.name, resource.kind, resource.database_name, endpoint.id AS endpoint_id").
		ColumnExpr("endpoint.address || ':' || endpoint.port::text AS endpoint").
		ColumnExpr("credential.id AS credential_id, COALESCE(credential.name, '') AS credential").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.resource_installation_id IS NULL AND credential.archived_at IS NULL").
		Where("resource.archived_at IS NULL").Where("resource.kind = 'postgresql'").
		OrderExpr("resource.name, endpoint.role, credential.name").Scan(ctx, &options)
	return EnvironmentSetupOptions{Resources: options}, err
}

type environmentSetupSource struct {
	ApplicationID         uuid.UUID    `bun:"application_id"`
	EnvironmentID         uuid.UUID    `bun:"environment_id"`
	EnvironmentSourceID   uuid.UUID    `bun:"environment_source_id"`
	EnvironmentArchivedAt sql.NullTime `bun:"environment_archived_at"`
	SetupComplete         bool
	Reference             string          `bun:"reference"`
	Repository            string          `bun:"repository"`
	RepositoryID          uuid.UUID       `bun:"repository_id"`
	InstallationID        uuid.UUID       `bun:"installation_id"`
	ContextPath           string          `bun:"context_path"`
	BuilderReference      sql.NullString  `bun:"builder_reference"`
	BuildpackSettings     json.RawMessage `bun:"buildpack_settings"`
	ImageRepository       string          `bun:"image_repository"`
	RegistryID            uuid.UUID       `bun:"registry_id"`
	RegistryEndpoint      string          `bun:"registry_endpoint"`
}

type preparedSetupResource struct {
	input        EnvironmentSetupResourceInput
	connectionID uuid.UUID
	resource     models.ResourceEntity
	endpoint     models.ResourceEndpointEntity
	credential   *models.ResourceCredentialEntity
	variables    map[string]string
	secrets      []PreparedEnvironmentSecret
}

func (service *EnvironmentSetup) Complete(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	input EnvironmentSetupInput,
) (EnvironmentSetupResult, error) {
	input.HealthPath = strings.TrimSpace(input.HealthPath)
	input.BPGOTargets = strings.TrimSpace(input.BPGOTargets)
	if err := validateEnvironmentSetupInput(input); err != nil {
		return EnvironmentSetupResult{}, err
	}
	source, repository, installation, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if source.EnvironmentArchivedAt.Valid || source.SetupComplete {
		return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, errors.New("Environment is unavailable or setup is already complete"))
	}
	revisionSHA, err := service.github.ResolveRevision(ctx, installation, repository, source.Reference)
	if err != nil {
		return EnvironmentSetupResult{}, fmt.Errorf("resolve configured GitHub reference: %w", err)
	}
	serverID, networkID, serverNetwork, err := service.controlPlanePlacement(ctx)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	preparedResources, err := service.prepareResources(ctx, environmentID, networkID, input.Resources)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	preparedUserSecrets := make([]PreparedEnvironmentSecret, 0, len(input.Secrets))
	keys := map[string]struct{}{"PORT": {}}
	for _, resource := range preparedResources {
		for _, secret := range resource.secrets {
			keys[secret.Key] = struct{}{}
		}
		for key := range resource.variables {
			keys[key] = struct{}{}
		}
	}
	for index, secret := range input.Secrets {
		key := models.NormalizeEnvironmentSecretKey(secret.Key)
		if _, exists := keys[key]; exists {
			return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("secrets.%d.key", index), Code: "reserved", Message: "secret key conflicts with a platform or Resource-owned key"}})
		}
		prepared, prepareErr := service.secrets.Prepare(environmentID, key, secret.Value, models.EnvironmentSecretSourceUser, userID)
		if prepareErr != nil {
			return EnvironmentSetupResult{}, prepareErr
		}
		keys[key] = struct{}{}
		preparedUserSecrets = append(preparedUserSecrets, prepared)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if environment.ApplicationID != applicationID || setupComplete || environment.ArchivedAt.Valid {
		return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, errors.New("Environment is unavailable or setup is already complete"))
	}
	runtimeSettings, _ := json.Marshal(map[string]any{"schema_version": 1, "health_path": input.HealthPath, "bp_go_targets": input.BPGOTargets})
	if _, err := models.RuntimeConfiguration.Create(ctx, tx, models.CreateRuntimeConfigurationData{
		Runtime: "go", Arguments: json.RawMessage(`[]`), Replicas: 1,
		Ports:          json.RawMessage(fmt.Sprintf(`{"http":%d}`, input.ContainerPort)),
		ResourceLimits: json.RawMessage(`{}`), RestartPolicy: "unless-stopped", Settings: runtimeSettings,
		EnvironmentID: environment.ID,
	}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	target, err := models.EnvironmentTarget.Create(ctx, tx, models.CreateEnvironmentTargetData{AttachedAt: time.Now().UTC(), EnvironmentID: environment.ID, ServerID: serverID})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.EnvironmentNetwork.Create(ctx, tx, models.CreateEnvironmentNetworkData{Role: "primary", EnvironmentID: environment.ID, PrivateNetworkID: networkID}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.EnvironmentTargetNetwork.Create(ctx, tx, models.CreateEnvironmentTargetNetworkData{
		Driver: serverNetwork.Driver, ExternalID: serverNetwork.ExternalID, Configuration: serverNetwork.Configuration,
		State: "applied", AppliedAt: serverNetwork.AppliedAt, ObservedAt: serverNetwork.ObservedAt,
		EnvironmentTargetID: target.ID, PrivateNetworkID: networkID,
	}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	domain, err := models.EnvironmentDomain.Create(ctx, tx, models.CreateEnvironmentDomainData{Hostname: input.Hostname, IsPrimary: true, EnvironmentID: environment.ID})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	resourceStates := make([]models.EnvironmentResourceState, 0, len(preparedResources))
	secretEntities := make([]models.EnvironmentSecretEntity, 0, len(preparedUserSecrets)+len(preparedResources))
	for _, prepared := range preparedResources {
		configuration, _ := json.Marshal(map[string]any{
			"schema_version": 1, "database": prepared.input.Database, "credential_source": "managed",
			"credential_projection": prepared.input.CredentialProjection,
		})
		connection, createErr := models.EnvironmentResource.Create(ctx, tx, models.CreateEnvironmentResourceData{
			ID: prepared.connectionID, Alias: prepared.input.Alias, Configuration: configuration,
			EnvironmentID: environment.ID, ResourceID: prepared.resource.ID, ResourceEndpointID: prepared.endpoint.ID,
			ResourceCredentialID: prepared.input.CredentialID,
		})
		if createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		for _, secret := range prepared.secrets {
			secret.SourceID = connection.ID
			entity, createSecretErr := service.secrets.CreatePrepared(ctx, tx, secret)
			if createSecretErr != nil {
				return EnvironmentSetupResult{}, createSecretErr
			}
			secretEntities = append(secretEntities, entity)
		}
		resourceStates = append(resourceStates, models.EnvironmentResourceState{
			EnvironmentResourceID: connection.ID, ResourceID: prepared.resource.ID, Kind: prepared.resource.Kind,
			EndpointID: prepared.endpoint.ID, CredentialID: prepared.input.CredentialID,
			Alias: strings.ToUpper(prepared.input.Alias), Database: prepared.input.Database, Variables: prepared.variables,
		})
	}
	for _, prepared := range preparedUserSecrets {
		entity, createErr := service.secrets.CreatePrepared(ctx, tx, prepared)
		if createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		secretEntities = append(secretEntities, entity)
	}
	now := time.Now().UTC()
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: 1, Kind: "environment_setup", TriggerType: "user", ActorType: "user", ActorID: &userID,
		CorrelationID: uuid.New(), Summary: "Complete Environment setup", Status: "committed",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environment.ID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	descriptors := make([]models.EnvironmentSecretDescriptor, 0, len(secretEntities))
	for _, secret := range secretEntities {
		descriptors = append(descriptors, models.EnvironmentSecretDescriptorFromEntity(secret))
	}
	state := models.EnvironmentDesiredState{
		SchemaVersion: models.EnvironmentStateSchemaVersion,
		Runtime:       models.EnvironmentRuntimeState{Runtime: "go", ContainerPort: input.ContainerPort, HealthPath: input.HealthPath, BPGOTargets: input.BPGOTargets, Replicas: 1, RestartPolicy: "unless-stopped"},
		Domain:        models.EnvironmentDomainState{ID: domain.ID, Hostname: domain.Hostname, Primary: true},
		Resources:     resourceStates, Secrets: descriptors,
	}
	canonicalState, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, err)
	}
	revision, err := models.EnvironmentStateRevision.Create(ctx, tx, models.CreateEnvironmentStateRevisionData{State: canonicalState, EnvironmentID: environment.ID, ChangeID: change.ID})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.EnvironmentTargetState.Create(ctx, tx, models.CreateEnvironmentTargetStateData{ObservedState: json.RawMessage(`{}`), State: "pending", EnvironmentTargetID: target.ID, DesiredRevisionID: &revision.ID}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	correlationID := uuid.New()
	eventPayload, _ := json.Marshal(map[string]any{"schema_version": 1, "reference": source.Reference, "revision": revisionSHA, "repository": source.Repository, "environment_state_revision_id": revision.ID})
	event, err := models.SourceEvent.Create(ctx, tx, models.CreateSourceEventData{
		ExternalID: "manual:" + correlationID.String(), Kind: "manual_deploy",
		SourceRevision: sql.NullString{String: revisionSHA, Valid: true}, Payload: eventPayload,
		ReceivedAt: now, ProcessedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentSourceID: source.EnvironmentSourceID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
		SchemaVersion: 1, SourceEventID: event.ID, EnvironmentStateRevisionID: revision.ID,
		Repository: source.Repository, Reference: source.Reference, SourceRevision: revisionSHA,
		ContextPath: source.ContextPath, BuilderReference: nullableStringPointer(source.BuilderReference),
		ImageRepository: source.ImageRepository, ContainerRegistryID: source.RegistryID,
		RegistryEndpoint: source.RegistryEndpoint, Settings: source.BuildpackSettings,
		BPGOTargets: input.BPGOTargets,
	})
	if err != nil {
		return EnvironmentSetupResult{}, fmt.Errorf("create Build configuration snapshot: %w", err)
	}
	build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
		SourceRevision: revisionSHA, BuildMethod: "buildpacks", BuildConfiguration: buildConfiguration,
		Status: "pending", CurrentStep: sql.NullString{String: "queued", Valid: true},
		EnvironmentID: environment.ID, EnvironmentSourceID: source.EnvironmentSourceID, ChangeID: change.ID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.BuildSourceArgs{BuildID: build.ID}, jobs.BuildSourceInsertOpts(build.ID)); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSetupResult{}, err
	}
	return EnvironmentSetupResult{Environment: environment, Revision: revision, Build: build}, nil
}

func (service *EnvironmentSetup) QueueManualDeploy(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
) (models.BuildEntity, error) {
	deployability, err := models.Environment.Deployability(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	if !deployability.Deployable {
		return models.BuildEntity{}, errors.Join(models.ErrDomainValidation, fmt.Errorf("Environment is not deployable: %s", strings.Join(deployability.Missing, ", ")))
	}
	source, repository, installation, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	revisionSHA, err := service.github.ResolveRevision(ctx, installation, repository, source.Reference)
	if err != nil {
		return models.BuildEntity{}, err
	}
	stateRevision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(stateRevision.State)
	if err != nil {
		return models.BuildEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BuildEntity{}, err
	}
	defer tx.Rollback()
	if _, err := models.Environment.Lock(ctx, tx, environmentID); err != nil {
		return models.BuildEntity{}, err
	}
	now := time.Now().UTC()
	correlationID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"schema_version": 1, "reference": source.Reference, "revision": revisionSHA, "repository": source.Repository, "environment_state_revision_id": stateRevision.ID})
	event, err := models.SourceEvent.Create(ctx, tx, models.CreateSourceEventData{ExternalID: "manual:" + correlationID.String(), Kind: "manual_deploy", SourceRevision: sql.NullString{String: revisionSHA, Valid: true}, Payload: payload, ReceivedAt: now, ProcessedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentSourceID: source.EnvironmentSourceID})
	if err != nil {
		return models.BuildEntity{}, err
	}
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{Sequence: sequence, Kind: "build", TriggerType: "user", ActorType: "user", ActorID: &userID, CorrelationID: correlationID, Summary: "Deploy current GitHub revision", Status: "committed", RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environmentID})
	if err != nil {
		return models.BuildEntity{}, err
	}
	buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
		SchemaVersion: 1, SourceEventID: event.ID, EnvironmentStateRevisionID: stateRevision.ID,
		Repository: source.Repository, Reference: source.Reference, SourceRevision: revisionSHA,
		ContextPath: source.ContextPath, BuilderReference: nullableStringPointer(source.BuilderReference),
		ImageRepository: source.ImageRepository, ContainerRegistryID: source.RegistryID,
		RegistryEndpoint: source.RegistryEndpoint, Settings: source.BuildpackSettings,
		BPGOTargets: state.Runtime.BPGOTargets,
	})
	if err != nil {
		return models.BuildEntity{}, fmt.Errorf("create Build configuration snapshot: %w", err)
	}
	build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
		SourceRevision: revisionSHA, BuildMethod: "buildpacks", BuildConfiguration: buildConfiguration,
		Status: "pending", CurrentStep: sql.NullString{String: "queued", Valid: true},
		EnvironmentID: environmentID, EnvironmentSourceID: source.EnvironmentSourceID, ChangeID: change.ID,
	})
	if err != nil {
		return models.BuildEntity{}, err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.BuildSourceArgs{BuildID: build.ID}, jobs.BuildSourceInsertOpts(build.ID)); err != nil {
		return models.BuildEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.BuildEntity{}, err
	}
	return build, nil
}

func (service *EnvironmentSetup) QueueReleaseDeployment(
	ctx context.Context,
	applicationID, environmentID, releaseID, userID uuid.UUID,
) (models.DeploymentEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return models.DeploymentEntity{}, errors.New("Environment is unavailable")
	}
	release, err := models.Release.Find(ctx, tx, releaseID)
	if err != nil || release.EnvironmentID != environmentID || release.BuildID == nil {
		return models.DeploymentEntity{}, errors.New("Release does not belong to this Environment")
	}
	build, err := models.Build.Find(ctx, tx, *release.BuildID)
	if err != nil || build.EnvironmentID != environmentID || build.Status != "succeeded" {
		return models.DeploymentEntity{}, errors.New("Release Build is unavailable")
	}
	target, err := models.EnvironmentTarget.ActiveForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	var active models.DeploymentEntity
	err = tx.NewSelect().Model(&active).
		Where("environment_target_id = ?", target.ID).
		Where("status IN ('queued', 'running')").
		OrderExpr("created_at DESC").Limit(1).Scan(ctx)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return models.DeploymentEntity{}, err
		}
		return active, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return models.DeploymentEntity{}, err
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "redeploy", TriggerType: "user", ActorType: "user", ActorID: &userID,
		CauseSystem: sql.NullString{String: "release", Valid: true}, CauseReference: sql.NullString{String: release.ID.String(), Valid: true},
		CorrelationID: uuid.New(), Summary: "Redeploy selected Release", Status: "committed", RequestedAt: now,
		CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environmentID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := tx.NewUpdate().TableExpr("environment_target_states").Set("desired_revision_id = ?", revision.ID).
		Set("updated_at = ?", now).Where("environment_target_id = ?", target.ID).Exec(ctx); err != nil {
		return models.DeploymentEntity{}, err
	}
	runtimeSnapshot, _ := json.Marshal(state.Runtime)
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: 1, Strategy: json.RawMessage(`{"type":"blue_green","replicas":1}`), RuntimeConfiguration: runtimeSnapshot,
		Status: "queued", CurrentStep: sql.NullString{String: "queued", Valid: true}, ChangeID: change.ID,
		ReleaseID: release.ID, EnvironmentTargetID: target.ID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID: "pending:" + deployment.ID.String(), Slot: "candidate", ReplicaKey: "primary", State: "candidate",
		Ports: json.RawMessage(`{}`), ObservedAt: now, DeploymentID: deployment.ID, ReleaseID: release.ID, EnvironmentTargetID: target.ID,
	}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); err != nil {
		return models.DeploymentEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.DeploymentEntity{}, err
	}
	return deployment, nil
}

func (service *EnvironmentSetup) StartBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(ctx, tx, applicationID, environmentID); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	if build.Status != "pending" && build.Status != "running" {
		return errors.New("only a pending or retrying Build can be started")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	if build.Status == "running" && job.State != "scheduled" && job.State != "retryable" && job.State != "pending" {
		return errors.New("Build background job is already running")
	}
	if err := service.jobControl.RetryJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("start Build background job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build start requested by user")
	return nil
}

func (service *EnvironmentSetup) StopBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(ctx, tx, applicationID, environmentID); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	if err := service.jobControl.CancelJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("stop Build background job: %w", err)
	}
	now := time.Now().UTC()
	if err := models.Build.MarkCancelled(ctx, tx, build.ID, now); err != nil {
		return err
	}
	if err := models.Change.MarkBuildCancelled(ctx, tx, build.ChangeID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build cancelled by user")
	return nil
}

func (service *EnvironmentSetup) StopBuildJob(ctx context.Context, jobID int64) error {
	buildID, err := models.Job.BuildID(ctx, service.db.Executor(), jobID)
	if err != nil {
		return err
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil {
		return err
	}
	environment, err := models.Environment.Find(ctx, service.db.Executor(), build.EnvironmentID)
	if err != nil {
		return err
	}
	return service.StopBuild(ctx, environment.ApplicationID, environment.ID, build.ID)
}

func (service *EnvironmentSetup) RetryBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(ctx, tx, applicationID, environmentID); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	now := time.Now().UTC()
	if err := models.Build.ResetForRetry(ctx, tx, build.ID, now); err != nil {
		return err
	}
	if err := models.Change.ResetBuildForRetry(ctx, tx, build.ChangeID, now); err != nil {
		return err
	}
	if err := service.jobControl.RetryJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("retry Build background job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build retry requested by user")
	return nil
}

func (service *EnvironmentSetup) recordBuildAction(ctx context.Context, buildID uuid.UUID, message string) {
	logger, err := newBuildLogger(ctx, service.db, buildID)
	if err == nil {
		err = logger.System(ctx, message)
	}
	if err != nil {
		slog.WarnContext(ctx, "Could not record Build action", "build_id", buildID, "error", err)
	}
}

func (service *EnvironmentSetup) RetryBuildJob(ctx context.Context, jobID int64) error {
	buildID, err := models.Job.BuildID(ctx, service.db.Executor(), jobID)
	if err != nil {
		return err
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil {
		return err
	}
	environment, err := models.Environment.Find(ctx, service.db.Executor(), build.EnvironmentID)
	if err != nil {
		return err
	}
	return service.RetryBuild(ctx, environment.ApplicationID, environment.ID, build.ID)
}

func (service *EnvironmentSetup) UpdateEnvironment(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	input EnvironmentEditInput,
) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = slug.Make(strings.TrimSpace(input.Slug))
	input.Kind = strings.TrimSpace(input.Kind)
	input.HealthPath = strings.TrimSpace(input.HealthPath)
	input.BPGOTargets = strings.TrimSpace(input.BPGOTargets)
	if input.Name == "" || input.Slug == "" || input.Kind == "" {
		return errors.Join(models.ErrDomainValidation, errors.New("Environment name, slug, and kind are required"))
	}
	configuration := EnvironmentSetupInput{
		Hostname: input.Hostname, ContainerPort: input.ContainerPort,
		HealthPath: input.HealthPath, BPGOTargets: input.BPGOTargets, Resources: input.Resources,
	}
	if err := validateEnvironmentSetupInput(configuration); err != nil {
		return err
	}
	var networkID uuid.UUID
	if err := service.db.Executor().NewSelect().TableExpr("environment_networks").
		ColumnExpr("private_network_id").Where("environment_id = ?", environmentID).
		Where("removed_at IS NULL").Limit(1).Scan(ctx, &networkID); err != nil {
		return fmt.Errorf("load Environment network: %w", err)
	}
	preparedResources, err := service.prepareResources(ctx, environmentID, networkID, input.Resources)
	if err != nil {
		return err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return errors.New("Environment is unavailable")
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil || !setupComplete {
		return errors.New("Environment setup is incomplete")
	}
	baseRevision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	baseState, err := models.ParseEnvironmentDesiredState(baseRevision.State)
	if err != nil {
		return err
	}
	buildRequired := baseState.Runtime.BPGOTargets != input.BPGOTargets
	activeBuilds, err := tx.NewSelect().TableExpr("builds").Where("environment_id = ?", environmentID).
		Where("status IN ('pending', 'running')").Count(ctx)
	if err != nil {
		return err
	}
	activeDeployments, err := tx.NewSelect().TableExpr("deployments AS deployment").
		Join("JOIN environment_targets AS target ON target.id = deployment.environment_target_id").
		Where("target.environment_id = ?", environmentID).Where("deployment.status IN ('queued', 'running')").Count(ctx)
	if err != nil {
		return err
	}
	if activeBuilds > 0 || activeDeployments > 0 {
		return errors.New("stop active Build and Deployment work before editing the Environment")
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "environment-slug:"+applicationID.String()+":"+input.Slug); err != nil {
		return err
	}
	duplicateSlugs, err := tx.NewSelect().TableExpr("environments").Where("application_id = ?", applicationID).
		Where("id <> ?", environmentID).Where("slug = ?", input.Slug).Count(ctx)
	if err != nil {
		return err
	}
	if duplicateSlugs > 0 {
		return errors.Join(models.ErrDomainValidation, errors.New("Environment slug is already in use"))
	}
	if _, err := models.Environment.Update(ctx, tx, models.UpdateEnvironmentData{
		ID: environment.ID, Name: input.Name, Slug: input.Slug, Kind: input.Kind,
		WebhookTokenPrefix: environment.WebhookTokenPrefix, WebhookTokenDigest: environment.WebhookTokenDigest,
		ArchivedAt: environment.ArchivedAt, ApplicationID: applicationID,
	}); err != nil {
		return err
	}
	var runtime models.RuntimeConfigurationEntity
	if err := tx.NewSelect().Model(&runtime).Where("environment_id = ?", environmentID).Limit(1).Scan(ctx); err != nil {
		return err
	}
	runtimeSettings, _ := json.Marshal(map[string]any{
		"schema_version": 1, "health_path": input.HealthPath, "bp_go_targets": input.BPGOTargets,
	})
	if _, err := models.RuntimeConfiguration.Update(ctx, tx, models.UpdateRuntimeConfigurationData{
		ID: runtime.ID, Runtime: "go", Command: runtime.Command, Arguments: runtime.Arguments, Replicas: 1,
		Ports: json.RawMessage(fmt.Sprintf(`{"http":%d}`, input.ContainerPort)), ResourceLimits: runtime.ResourceLimits,
		RestartPolicy: "unless-stopped", Settings: runtimeSettings, EnvironmentID: environmentID,
	}); err != nil {
		return err
	}
	var domain models.EnvironmentDomainEntity
	if err := tx.NewSelect().Model(&domain).Where("environment_id = ?", environmentID).
		Where("is_primary = TRUE").Where("archived_at IS NULL").Limit(1).Scan(ctx); err != nil {
		return err
	}
	updatedDomain, err := models.EnvironmentDomain.Update(ctx, tx, models.UpdateEnvironmentDomainData{
		ID: domain.ID, Hostname: input.Hostname, IsPrimary: true,
		ArchivedAt: domain.ArchivedAt, EnvironmentID: environmentID,
	})
	if err != nil {
		return err
	}
	activeSecrets, err := models.EnvironmentSecret.ActiveForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	userSecretDescriptors := make([]models.EnvironmentSecretDescriptor, 0)
	for _, secret := range activeSecrets {
		if secret.SourceType == models.EnvironmentSecretSourceUser {
			userSecretDescriptors = append(userSecretDescriptors, models.EnvironmentSecretDescriptorFromEntity(secret))
			continue
		}
		if err := models.EnvironmentSecret.Archive(ctx, tx, environmentID, secret.ID); err != nil {
			return err
		}
	}
	currentConnections := make([]models.EnvironmentResourceEntity, 0)
	if err := tx.NewSelect().Model(&currentConnections).Where("environment_id = ?", environmentID).
		Where("archived_at IS NULL").Scan(ctx); err != nil {
		return err
	}
	for _, connection := range currentConnections {
		if err := models.EnvironmentResource.Archive(ctx, tx, connection.ID); err != nil {
			return err
		}
	}
	resourceStates := make([]models.EnvironmentResourceState, 0, len(preparedResources))
	secretDescriptors := append([]models.EnvironmentSecretDescriptor{}, userSecretDescriptors...)
	for _, prepared := range preparedResources {
		resourceConfiguration, _ := json.Marshal(map[string]any{
			"schema_version": 1, "database": prepared.input.Database, "credential_source": "managed",
			"credential_projection": prepared.input.CredentialProjection,
		})
		connection, createErr := models.EnvironmentResource.Create(ctx, tx, models.CreateEnvironmentResourceData{
			ID: prepared.connectionID, Alias: prepared.input.Alias, Configuration: resourceConfiguration,
			EnvironmentID: environmentID, ResourceID: prepared.resource.ID, ResourceEndpointID: prepared.endpoint.ID,
			ResourceCredentialID: prepared.input.CredentialID,
		})
		if createErr != nil {
			return createErr
		}
		for _, secret := range prepared.secrets {
			secret.SourceID = connection.ID
			entity, createSecretErr := service.secrets.CreatePrepared(ctx, tx, secret)
			if createSecretErr != nil {
				return createSecretErr
			}
			secretDescriptors = append(secretDescriptors, models.EnvironmentSecretDescriptorFromEntity(entity))
		}
		resourceStates = append(resourceStates, models.EnvironmentResourceState{
			EnvironmentResourceID: connection.ID, ResourceID: prepared.resource.ID, Kind: prepared.resource.Kind,
			EndpointID: prepared.endpoint.ID, CredentialID: prepared.input.CredentialID,
			Alias: prepared.input.Alias, Database: prepared.input.Database, Variables: prepared.variables,
		})
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "environment_update", TriggerType: "user", ActorType: "user", ActorID: &userID,
		CorrelationID: uuid.New(), Summary: "Update Environment configuration", Status: "committed",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environmentID,
	})
	if err != nil {
		return err
	}
	state := models.EnvironmentDesiredState{
		SchemaVersion: models.EnvironmentStateSchemaVersion,
		Runtime: models.EnvironmentRuntimeState{
			Runtime: "go", ContainerPort: input.ContainerPort, HealthPath: input.HealthPath,
			BPGOTargets: input.BPGOTargets, Replicas: 1, RestartPolicy: "unless-stopped",
		},
		Domain:    models.EnvironmentDomainState{ID: updatedDomain.ID, Hostname: updatedDomain.Hostname, Primary: true},
		Resources: resourceStates, Secrets: secretDescriptors,
	}
	canonicalState, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return errors.Join(models.ErrDomainValidation, err)
	}
	revision, err := models.EnvironmentStateRevision.Create(ctx, tx, models.CreateEnvironmentStateRevisionData{
		State: canonicalState, EnvironmentID: environmentID, ChangeID: change.ID,
	})
	if err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{
		Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID,
	}); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("environment_target_states AS state").
		Set("desired_revision_id = ?", revision.ID).Set("state = 'pending'").Set("updated_at = ?", now).
		Where("state.environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", environmentID).
		Exec(ctx); err != nil {
		return err
	}
	if !buildRequired {
		if err := service.secrets.queueRevisionDeployment(ctx, tx, change, revision, state); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !buildRequired {
		return nil
	}
	_, err = service.QueueManualDeploy(ctx, applicationID, environmentID, userID)
	return err
}

func (service *EnvironmentSetup) DeleteEnvironment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	environment, err := models.Environment.FindForApplication(ctx, service.db.Executor(), applicationID, environmentID)
	if err != nil || environment.ArchivedAt.Valid {
		return errors.New("Environment is unavailable")
	}
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil || application.IsSystem() {
		return models.ErrSystemApplicationImmutable
	}

	type jobReference struct {
		ID    int64  `bun:"id"`
		State string `bun:"state"`
	}
	jobsToDelete := make([]jobReference, 0)
	if err := service.db.Executor().NewSelect().TableExpr("river_job AS job").
		ColumnExpr("job.id, job.state::text AS state").
		Where(`
			(job.kind = 'build_source' AND EXISTS (
				SELECT 1 FROM builds AS build
				WHERE build.environment_id = ? AND build.id::text = job.args ->> 'build_id'
			)) OR
			(job.kind = 'deploy_release' AND EXISTS (
				SELECT 1 FROM deployments AS deployment
				JOIN environment_targets AS target ON target.id = deployment.environment_target_id
				WHERE target.environment_id = ? AND deployment.id::text = job.args ->> 'deployment_id'
			)) OR
			(job.kind IN ('backup_schedule', 'backup_retention') AND EXISTS (
				SELECT 1 FROM backup_policies AS policy
				JOIN environment_resources AS binding ON binding.id = policy.environment_resource_id
				WHERE binding.environment_id = ? AND policy.id::text = job.args ->> 'backup_policy_id'
			)) OR
			(job.kind IN ('backup_execute', 'backup_verify') AND EXISTS (
				SELECT 1 FROM backups AS backup
				LEFT JOIN backup_policies AS policy ON policy.id = backup.backup_policy_id
				LEFT JOIN environment_resources AS direct_binding ON direct_binding.id = backup.environment_resource_id
				LEFT JOIN environment_resources AS policy_binding ON policy_binding.id = policy.environment_resource_id
				WHERE (direct_binding.environment_id = ? OR policy_binding.environment_id = ?)
				AND backup.id::text = job.args ->> 'backup_id'
			))`, environmentID, environmentID, environmentID, environmentID, environmentID).
		Scan(ctx, &jobsToDelete); err != nil {
		return fmt.Errorf("load Environment background jobs: %w", err)
	}
	for _, job := range jobsToDelete {
		if job.State == "available" || job.State == "pending" || job.State == "retryable" || job.State == "running" || job.State == "scheduled" {
			if err := service.jobControl.CancelJob(ctx, job.ID); err != nil {
				return fmt.Errorf("cancel Environment background job %d: %w", job.ID, err)
			}
		}
	}

	routeIDs := make([]string, 0)
	if err := service.db.Executor().NewSelect().TableExpr("caddy_routes AS route").
		ColumnExpr("route.external_id").
		Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").
		Where("target.environment_id = ?", environmentID).
		Scan(ctx, &routeIDs); err != nil {
		return fmt.Errorf("load Environment Caddy routes: %w", err)
	}
	for _, routeID := range routeIDs {
		if err := service.caddy.Delete(ctx, routeID); err != nil {
			return fmt.Errorf("delete Environment Caddy route: %w", err)
		}
	}
	if err := service.container.DeleteEnvironment(ctx, environmentID); err != nil {
		return err
	}
	for _, job := range jobsToDelete {
		if err := service.jobControl.DeleteJob(ctx, job.ID); err != nil {
			if errors.Is(err, rivertype.ErrNotFound) {
				continue
			}
			if !errors.Is(err, rivertype.ErrJobRunning) {
				return fmt.Errorf("delete Environment background job %d: %w", job.ID, err)
			}
			if _, hardDeleteErr := service.db.Executor().NewDelete().TableExpr("river_job").Where("id = ?", job.ID).Exec(ctx); hardDeleteErr != nil {
				return fmt.Errorf("hard-delete cancelled Environment background job %d: %w", job.ID, hardDeleteErr)
			}
		}
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	locked, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || locked.ApplicationID != applicationID {
		return errors.New("Environment is unavailable")
	}
	if err := models.Environment.Destroy(ctx, tx, environmentID); err != nil {
		return fmt.Errorf("delete Environment data: %w", err)
	}
	remaining, err := tx.NewSelect().TableExpr("environments").Where("application_id = ?", applicationID).Count(ctx)
	if err != nil {
		return err
	}
	if remaining == 0 {
		if err := models.Application.Destroy(ctx, tx, applicationID); err != nil {
			return fmt.Errorf("delete empty Application: %w", err)
		}
	}
	return tx.Commit()
}

func (service *EnvironmentSetup) RetryDeployment(
	ctx context.Context,
	applicationID, environmentID, deploymentID, userID uuid.UUID,
) (models.DeploymentEntity, error) {
	if _, err := models.Environment.FindForApplication(ctx, service.db.Executor(), applicationID, environmentID); err != nil {
		return models.DeploymentEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	defer tx.Rollback()
	previous, err := models.Deployment.Lock(ctx, tx, deploymentID)
	if err != nil || previous.Status != "failed" {
		return models.DeploymentEntity{}, errors.Join(models.ErrDomainValidation, errors.New("only a failed Deployment can be retried"))
	}
	release, err := models.Release.Find(ctx, tx, previous.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID || release.BuildID == nil {
		return models.DeploymentEntity{}, errors.New("failed Deployment does not belong to this Environment")
	}
	target, err := models.EnvironmentTarget.Find(ctx, tx, previous.EnvironmentTargetID)
	if err != nil || target.EnvironmentID != environmentID || target.DetachedAt.Valid {
		return models.DeploymentEntity{}, errors.New("failed Deployment target is unavailable")
	}
	var revision models.EnvironmentStateRevisionEntity
	if err := tx.NewSelect().Model(&revision).Join("JOIN change_state_revisions AS association ON association.environment_state_revision_id = environment_state_revisions.id").Where("association.change_id = ?", previous.ChangeID).Where("association.role = 'result'").Limit(1).Scan(ctx); err != nil {
		return models.DeploymentEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "deployment_retry", TriggerType: "user", ActorType: "user", ActorID: &userID,
		CauseSystem: sql.NullString{String: "deployment", Valid: true}, CauseReference: sql.NullString{String: previous.ID.String(), Valid: true},
		CorrelationID: uuid.New(), Summary: "Retry failed Deployment", Status: "committed", RequestedAt: now,
		CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environmentID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return models.DeploymentEntity{}, err
	}
	runtimeSnapshot, _ := json.Marshal(state.Runtime)
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: previous.Attempt + 1, Strategy: previous.Strategy, RuntimeConfiguration: runtimeSnapshot,
		Status: "queued", CurrentStep: sql.NullString{String: "queued", Valid: true}, ChangeID: change.ID,
		ReleaseID: release.ID, EnvironmentTargetID: target.ID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID: "pending:" + deployment.ID.String(), Slot: "candidate", ReplicaKey: "primary", State: "candidate",
		Ports: json.RawMessage(`{}`), ObservedAt: now, DeploymentID: deployment.ID, ReleaseID: release.ID, EnvironmentTargetID: target.ID,
	}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); err != nil {
		return models.DeploymentEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.DeploymentEntity{}, err
	}
	return deployment, nil
}

func validateEnvironmentSetupInput(input EnvironmentSetupInput) error {
	builder := validation.NewBuilder()
	if input.ContainerPort < 1 || input.ContainerPort > 65535 {
		builder.Add("containerPort", "range", "container port must be between 1 and 65535")
	}
	if input.HealthPath != "" && (!strings.HasPrefix(input.HealthPath, "/") || strings.ContainsAny(input.HealthPath, " \t\r\n")) {
		builder.Add("healthPath", "format", "health path must be an absolute HTTP path")
	}
	if input.BPGOTargets != "" && (!goTargetsPattern.MatchString(input.BPGOTargets) || strings.Contains(input.BPGOTargets, "..")) {
		builder.Add("bpGoTargets", "format", "BP_GO_TARGETS must contain repository-relative Go targets")
	}
	if len(input.Resources) > 32 || len(input.Secrets) > 128 {
		builder.Add("resources", "max_items", "Environment setup exceeds the supported item count")
	}
	return builder.Err()
}

func (service *EnvironmentSetup) loadSource(ctx context.Context, applicationID, environmentID uuid.UUID) (environmentSetupSource, models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	var source environmentSetupSource
	err := service.db.Executor().NewSelect().TableExpr("environments AS environment").
		ColumnExpr("environment.application_id, environment.id AS environment_id, environment.archived_at AS environment_archived_at").
		ColumnExpr("source.id AS environment_source_id, source.reference, source.repository").
		ColumnExpr("repository.id AS repository_id, installation.id AS installation_id").
		ColumnExpr("buildpack.context_path, buildpack.builder_reference, buildpack.settings AS buildpack_settings, buildpack.image_repository").
		ColumnExpr("registry.id AS registry_id, registry.endpoint AS registry_endpoint").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").
		Join("JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("JOIN container_registries AS registry ON registry.id = buildpack.container_registry_id AND registry.archived_at IS NULL").
		Where("environment.id = ?", environmentID).Where("environment.application_id = ?", applicationID).Limit(1).Scan(ctx, &source)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	source.SetupComplete, err = models.Environment.SetupComplete(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), source.RepositoryID)
	if err != nil {
		return source, repository, models.GitHubInstallationEntity{}, err
	}
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), source.InstallationID)
	return source, repository, installation, err
}

func (service *EnvironmentSetup) controlPlanePlacement(ctx context.Context) (uuid.UUID, uuid.UUID, models.ServerNetworkEntity, error) {
	var row struct{ ServerID, NetworkID uuid.UUID }
	err := service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("target.server_id, membership.private_network_id AS network_id").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN environment_networks AS membership ON membership.environment_id = environment.id AND membership.removed_at IS NULL").
		Where("application.slug = ?", models.SystemApplicationSlug).Where("application.archived_at IS NULL").Limit(1).Scan(ctx, &row)
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, err
	}
	var network models.ServerNetworkEntity
	err = service.db.Executor().NewSelect().Model(&network).Where("server_id = ?", row.ServerID).Where("private_network_id = ?", row.NetworkID).Where("removed_at IS NULL").Limit(1).Scan(ctx)
	return row.ServerID, row.NetworkID, network, err
}

func (service *EnvironmentSetup) prepareResources(ctx context.Context, environmentID, networkID uuid.UUID, inputs []EnvironmentSetupResourceInput) ([]preparedSetupResource, error) {
	prepared := make([]preparedSetupResource, 0, len(inputs))
	aliases := make(map[string]struct{}, len(inputs))
	resources := make(map[uuid.UUID]struct{}, len(inputs))
	for index, input := range inputs {
		input.Alias = strings.ToUpper(strings.TrimSpace(input.Alias))
		input.CredentialProjection = strings.ToLower(strings.TrimSpace(input.CredentialProjection))
		if input.Alias == "" {
			input.Alias = "DATABASE"
		}
		if input.CredentialProjection != resourceCredentialProjectionConnectionURL && input.CredentialProjection != resourceCredentialProjectionIndividualParts {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.credentialProjection", index), Code: "unsupported", Message: "Choose Connection URL or Individual parts"}})
		}
		if _, exists := aliases[input.Alias]; exists {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.alias", index), Code: "duplicate", Message: "Resource alias is already selected"}})
		}
		if _, exists := resources[input.ResourceID]; exists {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.resourceId", index), Code: "duplicate", Message: "Resource is already selected"}})
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), input.ResourceID)
		if err != nil || resource.ArchivedAt.Valid || resource.Kind != "postgresql" {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected PostgreSQL Resource is unavailable"))
		}
		input.Database = resource.DatabaseName
		endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), input.EndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID || (endpoint.PrivateNetworkID != nil && *endpoint.PrivateNetworkID != networkID) {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource endpoint is unavailable from the Environment target"))
		}
		if input.CredentialID == nil {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("PostgreSQL application credential is required"))
		}
		credential, err := models.ResourceCredential.Find(ctx, service.db.Executor(), *input.CredentialID)
		if err != nil || credential.ArchivedAt.Valid || credential.ResourceID != resource.ID || credential.ResourceInstallationID != nil {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource credential is unavailable"))
		}
		values, err := service.resources.credentialSecretValues(credential)
		if err != nil {
			return nil, err
		}
		password := values["password"]
		if password == "" || !credential.Username.Valid {
			return nil, errors.New("selected PostgreSQL application credential is incomplete")
		}
		prefix := input.Alias
		connectionID := uuid.New()
		variables := make(map[string]string)
		secrets := make([]PreparedEnvironmentSecret, 0, 1)
		if input.CredentialProjection == resourceCredentialProjectionConnectionURL {
			uri := &url.URL{Scheme: "postgresql", Host: fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port), Path: "/" + input.Database, User: url.UserPassword(credential.Username.String, password)}
			query := uri.Query()
			query.Set("sslmode", endpoint.TlsMode)
			uri.RawQuery = query.Encode()
			urlSecret, prepareErr := service.secrets.Prepare(environmentID, prefix+"_URL", uri.String(), models.EnvironmentSecretSourceResource, connectionID)
			if prepareErr != nil {
				return nil, prepareErr
			}
			secrets = append(secrets, urlSecret)
		} else {
			variables[prefix+"_HOST"] = endpoint.Address
			variables[prefix+"_PORT"] = fmt.Sprint(endpoint.Port)
			variables[prefix+"_USER"] = credential.Username.String
			variables[prefix+"_TLS_MODE"] = endpoint.TlsMode
			passwordSecret, prepareErr := service.secrets.Prepare(environmentID, prefix+"_PASSWORD", password, models.EnvironmentSecretSourceResource, connectionID)
			if prepareErr != nil {
				return nil, prepareErr
			}
			secrets = append(secrets, passwordSecret)
		}
		prepared = append(prepared, preparedSetupResource{input: input, connectionID: connectionID, resource: resource, endpoint: endpoint, credential: &credential, variables: variables, secrets: secrets})
		aliases[input.Alias] = struct{}{}
		resources[input.ResourceID] = struct{}{}
	}
	return prepared, nil
}
