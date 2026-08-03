package services

import (
	"context"
	"crypto/hmac"
	"database/sql"
	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	buildpacksclient "deploycrate-ce/clients/buildpacks"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"
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
	workloads  *WorkloadExecution
	buildpacks buildpacksclient.Client
	servers    *ServerExecution
	builds     *BuildExecution
	registry   registryclient.Client
	config     config.Config
}

func NewEnvironmentSetup(
	db storage.Pool,
	queue storage.InsertQueue,
	jobControl BuildJobControl,
	github *GitHubConnection,
	secrets *EnvironmentSecrets,
	resources *ResourceManagement,
	caddy CaddyRouteService,
	workloads *WorkloadExecution,
	servers *ServerExecution,
	builds *BuildExecution,
	cfg config.Config,
) *EnvironmentSetup {
	return &EnvironmentSetup{
		db: db, queue: queue, jobControl: jobControl, github: github, secrets: secrets,
		resources: resources, caddy: caddy, workloads: workloads, buildpacks: buildpacksclient.New(), servers: servers,
		builds: builds, registry: registryclient.New(), config: cfg,
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
	ServerID      uuid.UUID
	ServerIDs     []uuid.UUID
	Hostname      string
	ContainerPort int32
	HealthPath    string
	BPGOTargets   string
	Resources     []EnvironmentSetupResourceInput
	Secrets       []EnvironmentSetupSecretInput
	Deploy        bool
}

type EnvironmentSetupResult struct {
	Environment models.EnvironmentEntity              `json:"environment"`
	Revision    models.EnvironmentStateRevisionEntity `json:"revision"`
	Build       models.BuildEntity                    `json:"build"`
	Release     models.ReleaseEntity                  `json:"release"`
	Deployment  models.DeploymentEntity               `json:"deployment"`
}

type EnvironmentSetupResourceOption struct {
	ID                    uuid.UUID  `json:"id" bun:"id"`
	Name                  string     `json:"name" bun:"name"`
	Engine                string     `json:"engine" bun:"engine"`
	Database              string     `json:"database" bun:"database_name"`
	EndpointID            uuid.UUID  `json:"endpointId" bun:"endpoint_id"`
	Endpoint              string     `json:"endpoint" bun:"endpoint"`
	CredentialID          *uuid.UUID `json:"credentialId" bun:"credential_id"`
	Credential            string     `json:"credential" bun:"credential"`
	ServerID              *uuid.UUID `json:"serverId" bun:"server_id"`
	CredentialFields      []string   `json:"credentialFields" bun:"-"`
	SupportsConnectionURL bool       `json:"supportsConnectionUrl" bun:"-"`
}

type EnvironmentSetupServerOption struct {
	ID      uuid.UUID `json:"id" bun:"id"`
	Name    string    `json:"name" bun:"name"`
	Kind    string    `json:"kind" bun:"kind"`
	Address string    `json:"address" bun:"address"`
}

type EnvironmentSetupOptions struct {
	Resources []EnvironmentSetupResourceOption `json:"resources"`
	Servers   []EnvironmentSetupServerOption   `json:"servers"`
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
	ServerIDs     []uuid.UUID                     `json:"serverIds"`
	ServerNames   []string                        `json:"serverNames"`
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
	ApplicationID    uuid.UUID                       `json:"applicationId"`
	ApplicationName  string                          `json:"applicationName"`
	SourceType       string                          `json:"sourceType"`
	Environment      models.EnvironmentEntity        `json:"environment"`
	SetupComplete    bool                            `json:"setupComplete"`
	Repository       string                          `json:"repository"`
	Reference        string                          `json:"reference"`
	ContextPath      string                          `json:"contextPath"`
	RegistryName     string                          `json:"registryName"`
	RegistryEndpoint string                          `json:"registryEndpoint"`
	RuntimeServerIDs []uuid.UUID                     `json:"runtimeServerIds"`
	RuntimeServers   []string                        `json:"runtimeServers"`
	Deployability    models.EnvironmentDeployability `json:"deployability"`
	Secrets          []EnvironmentSecretActivity     `json:"secrets"`
	Variables        []EnvironmentVariableActivity   `json:"variables"`
	Domain           string                          `json:"domain"`
	Resources        []EnvironmentResourceActivity   `json:"resources"`
	Builds           []EnvironmentBuildActivity      `json:"builds"`
	Releases         []EnvironmentReleaseActivity    `json:"releases"`
	Deployments      []EnvironmentDeploymentActivity `json:"deployments"`
	Instances        []EnvironmentInstanceActivity   `json:"instances"`
	APITokenPrefix   string                          `json:"apiTokenPrefix"`
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
	ID     uuid.UUID `json:"id" bun:"id"`
	Alias  string    `json:"alias" bun:"alias"`
	Name   string    `json:"name" bun:"name"`
	Engine string    `json:"engine" bun:"engine"`
}

type EnvironmentVariableActivity struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
}

type EnvironmentSecretActivity struct {
	ID           uuid.UUID `json:"id"`
	Key          string    `json:"key"`
	DigestPrefix string    `json:"digestPrefix"`
	SourceType   string    `json:"sourceType"`
	SourceID     uuid.UUID `json:"sourceId"`
	CreatedAt    time.Time `json:"createdAt"`
	Status       string    `json:"status"`
	Desired      bool      `json:"desired"`
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
	Active      bool      `json:"active" bun:"active"`
}

const environmentDeploymentActiveExpression = `EXISTS (
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
		SourceType       string `bun:"source_type"`
		Repository       string `bun:"repository"`
		Reference        string `bun:"reference"`
		ContextPath      string `bun:"context_path"`
		RegistryName     string `bun:"registry_name"`
		RegistryEndpoint string `bun:"registry_endpoint"`
	}
	err = service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("application.name AS application_name, environment_source.repository, environment_source.reference, CASE WHEN environment_source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type, COALESCE(buildpack.context_path, '') AS context_path").
		ColumnExpr("registry_resource.name AS registry_name").
		ColumnExpr("CASE WHEN registry_endpoint.port IN (80, 443) THEN registry_endpoint.address ELSE registry_endpoint.address || ':' || registry_endpoint.port::text END AS registry_endpoint").
		Join("JOIN environment_sources AS environment_source ON environment_source.environment_id = ? AND environment_source.archived_at IS NULL", environmentID).
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = environment_source.id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = environment_source.id").
		Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").
		Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id").
		Join("JOIN resource_endpoints AS registry_endpoint ON registry_endpoint.resource_id = registry.resource_id AND registry_endpoint.role = 'primary' AND registry_endpoint.archived_at IS NULL").
		Where("application.id = ?", applicationID).Limit(1).Scan(ctx, &source)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	deployability, err := models.Environment.Deployability(ctx, service.db.Executor(), environmentID)
	if err != nil && setupComplete {
		return EnvironmentOverview{}, err
	}
	secretActivity := make([]EnvironmentSecretActivity, 0)
	variables := make([]EnvironmentVariableActivity, 0)
	if setupComplete {
		secretActivity, err = service.environmentSecretActivity(ctx, environmentID)
		if err != nil {
			return EnvironmentOverview{}, err
		}
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
	type runtimeServer struct {
		ID   uuid.UUID `bun:"id"`
		Name string    `bun:"name"`
	}
	runtimeServers := make([]runtimeServer, 0)
	if setupComplete {
		if err := service.db.Executor().NewSelect().TableExpr("environment_targets AS target").
			ColumnExpr("server.id, server.name").
			Join("JOIN servers AS server ON server.id = target.server_id").
			Where("target.environment_id = ?", environmentID).Where("target.detached_at IS NULL").OrderExpr("server.name").Scan(ctx, &runtimeServers); err != nil {
			return EnvironmentOverview{}, err
		}
	}
	runtimeServerIDs := make([]uuid.UUID, 0, len(runtimeServers))
	runtimeServerNames := make([]string, 0, len(runtimeServers))
	for _, server := range runtimeServers {
		runtimeServerIDs = append(runtimeServerIDs, server.ID)
		runtimeServerNames = append(runtimeServerNames, server.Name)
	}
	resources := make([]EnvironmentResourceActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("environment_resources AS connection").ColumnExpr("connection.id, connection.alias, resource.name, resource.configuration ->> 'engine' AS engine").Join("JOIN resources AS resource ON resource.id = connection.resource_id").Where("connection.environment_id = ?", environmentID).Where("connection.archived_at IS NULL").OrderExpr("connection.alias").Scan(ctx, &resources); err != nil {
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
	if err := service.db.Executor().NewSelect().TableExpr("releases").ColumnExpr("id, COALESCE(source_revision, version, '') AS source_revision, artifact_reference, created_at").Where("environment_id = ?", environmentID).Where("registry_resource_id IS NOT NULL").OrderExpr("created_at DESC").Limit(20).Scan(ctx, &releases); err != nil {
		return EnvironmentOverview{}, err
	}
	deployments := make([]EnvironmentDeploymentActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("deployments AS deployment").
		ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.created_at").
		ColumnExpr(environmentDeploymentActiveExpression+" AS active").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").
		OrderExpr("deployment.created_at DESC").Limit(30).Scan(ctx, &deployments); err != nil {
		return EnvironmentOverview{}, err
	}
	instances := make([]EnvironmentInstanceActivity, 0)
	if err := service.db.Executor().NewSelect().TableExpr("instances AS instance").ColumnExpr("instance.id, instance.state, instance.slot, instance.ports, instance.release_id, instance.observed_at").Join("JOIN releases AS release ON release.id = instance.release_id").Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").Where("instance.removed_at IS NULL").OrderExpr("instance.created_at DESC").Limit(30).Scan(ctx, &instances); err != nil {
		return EnvironmentOverview{}, err
	}
	return EnvironmentOverview{
		ApplicationID: applicationID, ApplicationName: source.ApplicationName, Environment: environment, SetupComplete: setupComplete,
		SourceType: source.SourceType, Repository: source.Repository, Reference: source.Reference, ContextPath: source.ContextPath,
		RegistryName: source.RegistryName, RegistryEndpoint: source.RegistryEndpoint,
		RuntimeServerIDs: runtimeServerIDs, RuntimeServers: runtimeServerNames,
		Deployability: deployability, Secrets: secretActivity, Variables: variables, Domain: domain,
		Resources: resources, Builds: builds, Releases: releases, Deployments: deployments, Instances: instances,
		APITokenPrefix: environment.APITokenPrefix.String,
	}, nil
}

func (service *EnvironmentSetup) environmentSecretActivity(ctx context.Context, environmentID uuid.UUID) ([]EnvironmentSecretActivity, error) {
	targetStates := make([]models.EnvironmentTargetStateEntity, 0)
	if err := service.db.Executor().NewSelect().Model(&targetStates).
		Join("JOIN environment_targets AS target ON target.id = environment_target_states.environment_target_id").
		Where("target.environment_id = ?", environmentID).Where("target.detached_at IS NULL").OrderExpr("target.attached_at, target.id").Scan(ctx); err != nil {
		return nil, err
	}
	if len(targetStates) == 0 {
		return nil, errors.New("Environment has no runtime Server targets")
	}
	desired, err := service.environmentRevisionSecrets(ctx, targetStates[0].DesiredRevisionID)
	if err != nil {
		return nil, err
	}
	applyingByTarget := make([]map[string]models.EnvironmentSecretDescriptor, 0, len(targetStates))
	appliedByTarget := make([]map[string]models.EnvironmentSecretDescriptor, 0, len(targetStates))
	appliedUnion := make(map[string]models.EnvironmentSecretDescriptor)
	desiredRevisionFailed := false
	for _, targetState := range targetStates {
		applying, applyingErr := service.environmentRevisionSecrets(ctx, targetState.ApplyingRevisionID)
		if applyingErr != nil {
			return nil, applyingErr
		}
		applied, appliedErr := service.environmentRevisionSecrets(ctx, targetState.AppliedRevisionID)
		if appliedErr != nil {
			return nil, appliedErr
		}
		applyingByTarget = append(applyingByTarget, applying)
		appliedByTarget = append(appliedByTarget, applied)
		maps.Copy(appliedUnion, applied)
		failedRevisionID, failedErr := service.environmentLatestFailedRevisionID(ctx, targetState.EnvironmentTargetID)
		if failedErr != nil {
			return nil, failedErr
		}
		if targetState.State == "failed" && targetState.DesiredRevisionID != nil && failedRevisionID != nil && *targetState.DesiredRevisionID == *failedRevisionID {
			desiredRevisionFailed = true
		}
	}

	activity := make([]EnvironmentSecretActivity, 0, len(desired)+len(appliedUnion))
	for _, descriptor := range desired {
		secret, findErr := models.EnvironmentSecret.FindForEnvironment(ctx, service.db.Executor(), environmentID, descriptor.ID)
		if findErr != nil {
			return nil, findErr
		}
		metadata := secret.Sanitized()
		status := "pending"
		deployedEverywhere := true
		deploying := false
		for index := range targetStates {
			if !sameEnvironmentSecretDescriptor(descriptor, appliedByTarget[index][descriptor.Key]) {
				deployedEverywhere = false
			}
			if sameEnvironmentSecretDescriptor(descriptor, applyingByTarget[index][descriptor.Key]) {
				deploying = true
			}
		}
		if deployedEverywhere {
			status = "deployed"
		} else if desiredRevisionFailed {
			status = "failed"
		} else if deploying {
			status = "deploying"
		}
		activity = append(activity, EnvironmentSecretActivity{
			ID: metadata.ID, Key: metadata.Key, DigestPrefix: metadata.DigestPrefix,
			SourceType: metadata.SourceType, SourceID: metadata.SourceID, CreatedAt: metadata.CreatedAt,
			Status: status, Desired: true,
		})
	}
	for key, descriptor := range appliedUnion {
		if _, stillDesired := desired[key]; stillDesired {
			continue
		}
		secret, findErr := models.EnvironmentSecret.FindForEnvironment(ctx, service.db.Executor(), environmentID, descriptor.ID)
		if findErr != nil {
			return nil, findErr
		}
		metadata := secret.Sanitized()
		activity = append(activity, EnvironmentSecretActivity{
			ID: metadata.ID, Key: metadata.Key, DigestPrefix: metadata.DigestPrefix,
			SourceType: metadata.SourceType, SourceID: metadata.SourceID, CreatedAt: metadata.CreatedAt,
			Status: "pending_removal", Desired: false,
		})
	}
	sort.Slice(activity, func(left, right int) bool { return activity[left].Key < activity[right].Key })
	return activity, nil
}

func (service *EnvironmentSetup) environmentLatestFailedRevisionID(ctx context.Context, targetID uuid.UUID) (*uuid.UUID, error) {
	var revisionID uuid.UUID
	err := service.db.Executor().NewSelect().TableExpr("deployments AS deployment").
		ColumnExpr("association.environment_state_revision_id").
		Join("JOIN change_state_revisions AS association ON association.change_id = deployment.change_id AND association.role = 'result'").
		Where("deployment.environment_target_id = ?", targetID).Where("deployment.status = 'failed'").
		OrderExpr("deployment.finished_at DESC NULLS LAST, deployment.created_at DESC").Limit(1).Scan(ctx, &revisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &revisionID, nil
}

func (service *EnvironmentSetup) environmentRevisionSecrets(
	ctx context.Context,
	revisionID *uuid.UUID,
) (map[string]models.EnvironmentSecretDescriptor, error) {
	secrets := make(map[string]models.EnvironmentSecretDescriptor)
	if revisionID == nil {
		return secrets, nil
	}
	revision, err := models.EnvironmentStateRevision.Find(ctx, service.db.Executor(), *revisionID)
	if err != nil {
		return nil, err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return nil, err
	}
	for _, descriptor := range state.Secrets {
		secrets[descriptor.Key] = descriptor
	}
	return secrets, nil
}

func sameEnvironmentSecretDescriptor(left, right models.EnvironmentSecretDescriptor) bool {
	return left.ID != uuid.Nil && left.ID == right.ID && left.Digest == right.Digest
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
		CredentialMetadata   json.RawMessage `bun:"credential_metadata"`
	}
	rows := make([]resourceRow, 0)
	if err := service.db.Executor().NewSelect().TableExpr("environment_resources AS connection").
		ColumnExpr("connection.id, connection.resource_id, connection.resource_endpoint_id, connection.resource_credential_id, connection.alias, connection.configuration").
		ColumnExpr("credential.metadata AS credential_metadata").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.id = connection.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_credentials AS credential ON credential.id = connection.resource_credential_id AND credential.archived_at IS NULL").
		Where("connection.environment_id = ?", environmentID).Where("connection.archived_at IS NULL").OrderExpr("connection.alias").Scan(ctx, &rows); err != nil {
		return EnvironmentEditData{}, err
	}
	resources := make([]EnvironmentSetupResourceInput, 0, len(rows))
	for _, row := range rows {
		var configuration struct {
			CredentialProjection string `json:"credential_projection"`
		}
		if json.Unmarshal(row.Configuration, &configuration) != nil {
			return EnvironmentEditData{}, errors.New("Environment Resource configuration is invalid")
		}
		database := resourceCredentialMetadataDatabase(row.CredentialMetadata)
		resources = append(resources, EnvironmentSetupResourceInput{
			ResourceID: row.ResourceID, EndpointID: row.ResourceEndpointID,
			CredentialID: row.ResourceCredentialID, Alias: row.Alias, Database: database,
			CredentialProjection: configuration.CredentialProjection,
		})
	}
	return EnvironmentEditData{
		Overview: overview,
		Configuration: EnvironmentEditConfiguration{
			Name: overview.Environment.Name, Slug: overview.Environment.Slug, Kind: overview.Environment.Kind,
			Hostname: state.Domain.Hostname, ContainerPort: state.Runtime.ContainerPort,
			HealthPath: state.Runtime.HealthPath, BPGOTargets: state.Runtime.BPGOTargets, Resources: resources,
			ServerIDs: overview.RuntimeServerIDs, ServerNames: overview.RuntimeServers,
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
	var deployment EnvironmentDeploymentActivity
	err := service.db.Executor().NewSelect().TableExpr("deployments AS deployment").
		ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.created_at").
		ColumnExpr(environmentDeploymentActiveExpression+" AS active").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Where("deployment.id = ?", deploymentID).Where("release.environment_id = ?", environmentID).
		Where("release.registry_resource_id IS NOT NULL").Limit(1).Scan(ctx, &deployment)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvironmentDeploymentEventSnapshot{}, sql.ErrNoRows
	}
	if err != nil {
		return EnvironmentDeploymentEventSnapshot{}, err
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
		Deployment: deployment,
		Events:     events, NextSequence: nextSequence, HasMore: hasMore,
	}, nil
}

func (service *EnvironmentSetup) Options(ctx context.Context) (EnvironmentSetupOptions, error) {
	options := make([]EnvironmentSetupResourceOption, 0)
	if err := service.db.Executor().NewSelect().TableExpr("resources AS resource").
		ColumnExpr("resource.id, resource.name, resource.configuration ->> 'engine' AS engine, COALESCE(credential.metadata ->> 'database', '') AS database_name, endpoint.id AS endpoint_id").
		ColumnExpr("endpoint.address || ':' || endpoint.port::text AS endpoint").
		ColumnExpr("credential.id AS credential_id, COALESCE(credential.name, '') AS credential, installation.server_id AS server_id").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_installations AS installation ON installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Join("LEFT JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.metadata ->> 'purpose' = 'application' AND credential.archived_at IS NULL").
		Where("resource.archived_at IS NULL").
		OrderExpr("resource.name, endpoint.role, credential.name").Scan(ctx, &options); err != nil {
		return EnvironmentSetupOptions{}, err
	}
	selectable := make([]EnvironmentSetupResourceOption, 0, len(options))
	credentialless := make(map[string]struct{})
	for _, option := range options {
		definition, supported := models.FindResourceEngine(option.Engine)
		if !supported || option.Engine == "postgresql" && option.CredentialID == nil {
			continue
		}
		option.CredentialFields = make([]string, 0, len(definition.CredentialFields))
		for _, field := range definition.CredentialFields {
			option.CredentialFields = append(option.CredentialFields, field.Name)
		}
		option.SupportsConnectionURL = option.Engine == "postgresql"
		key := option.ID.String() + ":" + option.EndpointID.String()
		if option.Engine != "postgresql" {
			if _, exists := credentialless[key]; !exists {
				withoutCredential := option
				withoutCredential.Database = ""
				withoutCredential.CredentialID = nil
				withoutCredential.Credential = ""
				withoutCredential.CredentialFields = nil
				selectable = append(selectable, withoutCredential)
				credentialless[key] = struct{}{}
			}
			if option.CredentialID == nil {
				continue
			}
		}
		selectable = append(selectable, option)
	}
	options = selectable
	servers := make([]EnvironmentSetupServerOption, 0)
	if err := service.db.Executor().NewSelect().TableExpr("servers AS server").
		ColumnExpr("server.id, server.name, server.kind, server.address").
		Where("server.archived_at IS NULL").Where("server.is_configured = TRUE").
		Where("server.kind IN ('self_hosted', 'worker')").
		Where("server.capabilities @> '{\"runtime\":true}'::jsonb").
		OrderExpr("CASE WHEN server.kind = 'self_hosted' THEN 0 ELSE 1 END, server.name").Scan(ctx, &servers); err != nil {
		return EnvironmentSetupOptions{}, err
	}
	return EnvironmentSetupOptions{Resources: options, Servers: servers}, nil
}

type environmentSetupSource struct {
	ApplicationID         uuid.UUID    `bun:"application_id"`
	EnvironmentID         uuid.UUID    `bun:"environment_id"`
	EnvironmentSourceID   uuid.UUID    `bun:"environment_source_id"`
	EnvironmentArchivedAt sql.NullTime `bun:"environment_archived_at"`
	SetupComplete         bool
	Kind                  string          `bun:"kind"`
	Reference             string          `bun:"reference"`
	Repository            string          `bun:"repository"`
	RepositoryID          uuid.UUID       `bun:"repository_id"`
	InstallationID        uuid.UUID       `bun:"installation_id"`
	ContextPath           string          `bun:"context_path"`
	BuilderReference      sql.NullString  `bun:"builder_reference"`
	BuildpackSettings     json.RawMessage `bun:"buildpack_settings"`
	ImageRepository       string          `bun:"image_repository"`
	RegistryID            uuid.UUID       `bun:"registry_id"`
	RegistryCredentialID  uuid.UUID       `bun:"registry_credential_id"`
	RegistryEndpoint      string          `bun:"registry_endpoint"`
	BuildServerID         uuid.UUID       `bun:"build_server_id"`
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

type preparedRuntimePlacement struct {
	serverID uuid.UUID
	network  models.ServerNetworkEntity
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
	revisionSHA := ""
	var imageArtifact resolvedImageArtifact
	if input.Deploy {
		if source.Kind == "image" {
			imageArtifact, err = service.resolveImageArtifact(ctx, source, "")
			if err != nil {
				return EnvironmentSetupResult{}, fmt.Errorf("resolve configured image reference: %w", err)
			}
		} else {
			revisionSHA, err = service.github.ResolveRevision(ctx, installation, repository, source.Reference)
			if err != nil {
				return EnvironmentSetupResult{}, fmt.Errorf("resolve configured GitHub reference: %w", err)
			}
		}
	}
	serverIDs := normalizedEnvironmentServerIDs(input)
	placements, networkID, err := service.runtimePlacements(ctx, serverIDs)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	preparedResources, err := service.prepareResources(ctx, environmentID, serverIDs, networkID, input.Resources)
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
	if _, err := models.EnvironmentNetwork.Create(ctx, tx, models.CreateEnvironmentNetworkData{Role: "primary", EnvironmentID: environment.ID, PrivateNetworkID: networkID}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	targets := make([]models.EnvironmentTargetEntity, 0, len(placements))
	for _, placement := range placements {
		target, createErr := models.EnvironmentTarget.Create(ctx, tx, models.CreateEnvironmentTargetData{AttachedAt: time.Now().UTC(), EnvironmentID: environment.ID, ServerID: placement.serverID})
		if createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		if _, createErr := models.EnvironmentTargetNetwork.Create(ctx, tx, models.CreateEnvironmentTargetNetworkData{
			Driver: placement.network.Driver, ExternalID: placement.network.ExternalID, Configuration: placement.network.Configuration,
			State: "applied", AppliedAt: placement.network.AppliedAt, ObservedAt: placement.network.ObservedAt,
			EnvironmentTargetID: target.ID, PrivateNetworkID: networkID,
		}); createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		targets = append(targets, target)
	}
	domain, err := models.EnvironmentDomain.Create(ctx, tx, models.CreateEnvironmentDomainData{Hostname: input.Hostname, IsPrimary: true, EnvironmentID: environment.ID})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	resourceStates := make([]models.EnvironmentResourceState, 0, len(preparedResources))
	secretEntities := make([]models.EnvironmentSecretEntity, 0, len(preparedUserSecrets)+len(preparedResources))
	for _, prepared := range preparedResources {
		credentialSource := "none"
		if prepared.input.CredentialID != nil {
			credentialSource = "managed"
		}
		configuration, _ := json.Marshal(map[string]any{
			"schema_version": 1, "credential_source": credentialSource,
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
			EnvironmentResourceID: connection.ID, ResourceID: prepared.resource.ID, Kind: prepared.resource.Engine(),
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
	for _, target := range targets {
		if _, err := models.EnvironmentTargetState.Create(ctx, tx, models.CreateEnvironmentTargetStateData{ObservedState: json.RawMessage(`{}`), State: "pending", EnvironmentTargetID: target.ID, DesiredRevisionID: &revision.ID}); err != nil {
			return EnvironmentSetupResult{}, err
		}
	}
	if !input.Deploy {
		if err := models.Change.MarkCompleted(ctx, tx, change.ID, now); err != nil {
			return EnvironmentSetupResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return EnvironmentSetupResult{}, err
		}
		return EnvironmentSetupResult{Environment: environment, Revision: revision}, nil
	}
	if source.Kind == "image" {
		if err := models.Change.MarkCompleted(ctx, tx, change.ID, now); err != nil {
			return EnvironmentSetupResult{}, err
		}
		release, deployment, err := service.queueImageDeploymentTx(ctx, tx, source, revision, imageArtifact, "system", nil, "Deploy configured image")
		if err != nil {
			return EnvironmentSetupResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return EnvironmentSetupResult{}, err
		}
		return EnvironmentSetupResult{Environment: environment, Revision: revision, Release: release, Deployment: deployment}, nil
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
		SchemaVersion: 2, SourceEventID: event.ID, EnvironmentStateRevisionID: revision.ID,
		Repository: source.Repository, Reference: source.Reference, SourceRevision: revisionSHA,
		ContextPath: source.ContextPath, BuilderReference: nullableStringPointer(source.BuilderReference),
		ImageRepository: source.ImageRepository, RegistryResourceID: source.RegistryID,
		RegistryCredentialID: source.RegistryCredentialID,
		RegistryEndpoint:     source.RegistryEndpoint, Settings: source.BuildpackSettings,
		BPGOTargets: input.BPGOTargets,
		ServerID:    source.BuildServerID,
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

type SourceDeploymentResult struct {
	Build      *models.BuildEntity      `json:"build,omitempty"`
	Release    *models.ReleaseEntity    `json:"release,omitempty"`
	Deployment *models.DeploymentEntity `json:"deployment,omitempty"`
}

type resolvedImageArtifact struct {
	Version              string
	Reference            string
	Digest               []byte
	RegistryResourceID   uuid.UUID
	RegistryCredentialID uuid.UUID
	RegistryEndpoint     string
}

func (service *EnvironmentSetup) RotateAPIToken(ctx context.Context, applicationID, environmentID uuid.UUID) (string, error) {
	random, err := models.GenerateSecureToken()
	if err != nil {
		return "", err
	}
	token := "dcenv_" + strings.ToLower(random)
	digest := []byte(models.HashForStorage(token, service.config.App.SessionEncryptionKey))
	prefixLength := min(len(token), 13)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return "", errors.New("Environment is unavailable")
	}
	if _, err := models.Environment.Update(ctx, tx, models.UpdateEnvironmentData{
		ID: environment.ID, Name: environment.Name, Slug: environment.Slug, Kind: environment.Kind,
		APITokenPrefix: sql.NullString{String: token[:prefixLength], Valid: true}, APITokenDigest: digest,
		ArchivedAt: environment.ArchivedAt, ApplicationID: environment.ApplicationID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (service *EnvironmentSetup) AuthenticateAPIToken(ctx context.Context, environmentID uuid.UUID, token string) (models.EnvironmentEntity, error) {
	token = strings.TrimSpace(token)
	environment, err := models.Environment.Find(ctx, service.db.Executor(), environmentID)
	if err != nil || environment.ArchivedAt.Valid || !environment.APITokenPrefix.Valid || len(environment.APITokenDigest) == 0 {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	digest := []byte(models.HashForStorage(token, service.config.App.SessionEncryptionKey))
	if !hmac.Equal(environment.APITokenDigest, digest) {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	return environment, nil
}

func (service *EnvironmentSetup) QueueSourceDeployment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (SourceDeploymentResult, error) {
	source, _, _, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if source.Kind != "image" {
		if strings.TrimSpace(reference) != "" {
			return SourceDeploymentResult{}, errors.Join(models.ErrDomainValidation, errors.New("Buildpacks deployments do not accept an image reference override"))
		}
		if actorID == nil {
			return SourceDeploymentResult{}, errors.Join(models.ErrDomainValidation, errors.New("Buildpacks API deployments are not supported"))
		}
		build, err := service.QueueManualDeploy(ctx, applicationID, environmentID, *actorID)
		if err != nil {
			return SourceDeploymentResult{}, err
		}
		return SourceDeploymentResult{Build: &build}, nil
	}
	deployability, err := models.Environment.Deployability(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if !deployability.Deployable {
		return SourceDeploymentResult{}, errors.Join(models.ErrDomainValidation, fmt.Errorf("Environment is not deployable: %s", strings.Join(deployability.Missing, ", ")))
	}
	artifact, err := service.resolveImageArtifact(ctx, source, reference)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return SourceDeploymentResult{}, errors.New("Environment is unavailable")
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	release, deployment, err := service.queueImageDeploymentTx(ctx, tx, source, revision, artifact, triggerType, actorID, "Deploy image "+artifact.Version)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SourceDeploymentResult{}, err
	}
	return SourceDeploymentResult{Release: &release, Deployment: &deployment}, nil
}

func (service *EnvironmentSetup) resolveImageArtifact(ctx context.Context, source environmentSetupSource, override string) (resolvedImageArtifact, error) {
	version := strings.TrimSpace(override)
	if version == "" {
		version = strings.TrimSpace(source.Reference)
	}
	if version == "" || strings.ContainsAny(version, " /\t\r\n") {
		return resolvedImageArtifact{}, errors.Join(models.ErrDomainValidation, errors.New("image tag or digest is invalid"))
	}
	credentials, err := service.builds.RegistryCredentials(ctx, source.RegistryID, source.RegistryCredentialID, source.RegistryEndpoint)
	if err != nil {
		return resolvedImageArtifact{}, fmt.Errorf("load image registry credentials: %w", err)
	}
	separator := ":"
	if strings.HasPrefix(strings.ToLower(version), "sha256:") {
		separator = "@"
	}
	mutableReference := strings.TrimSuffix(source.RegistryEndpoint, "/") + "/" + strings.Trim(source.ImageRepository, "/") + separator + version
	immutableReference, err := service.registry.ResolveRemoteDigest(ctx, credentials, mutableReference)
	if err != nil {
		return resolvedImageArtifact{}, fmt.Errorf("resolve image reference: %w", err)
	}
	digestIndex := strings.LastIndex(immutableReference, "@sha256:")
	if digestIndex < 0 {
		return resolvedImageArtifact{}, errors.New("resolved image reference is not immutable")
	}
	digestText := immutableReference[digestIndex+len("@sha256:"):]
	digest, err := hex.DecodeString(digestText)
	if err != nil || len(digest) != 32 {
		return resolvedImageArtifact{}, errors.New("resolved image digest is invalid")
	}
	return resolvedImageArtifact{
		Version: version, Reference: immutableReference, Digest: digest,
		RegistryResourceID: source.RegistryID, RegistryCredentialID: source.RegistryCredentialID,
		RegistryEndpoint: source.RegistryEndpoint,
	}, nil
}

func (service *EnvironmentSetup) queueImageDeploymentTx(
	ctx context.Context,
	tx bun.Tx,
	source environmentSetupSource,
	revision models.EnvironmentStateRevisionEntity,
	artifact resolvedImageArtifact,
	triggerType string,
	actorID *uuid.UUID,
	summary string,
) (models.ReleaseEntity, models.DeploymentEntity, error) {
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if len(targets) == 0 {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(models.ErrDomainValidation, errors.New("Environment has no runtime Server targets"))
	}
	active, err := tx.NewSelect().TableExpr("deployments").
		Where("environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", source.EnvironmentID).
		Where("status IN ('queued', 'running')").Count(ctx)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if active > 0 {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(models.ErrDomainValidation, errors.New("Environment already has an active Deployment"))
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	actorType := "system"
	if actorID != nil {
		actorType = "user"
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence: sequence, Kind: "deploy", TriggerType: triggerType, ActorType: actorType, ActorID: actorID,
		CauseSystem: sql.NullString{String: "registry_image", Valid: true}, CauseReference: sql.NullString{String: artifact.Reference, Valid: true},
		CorrelationID: uuid.New(), CorrectionContext: json.RawMessage(`{}`), Summary: summary, Status: "committed",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: source.EnvironmentID,
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version: sql.NullString{String: artifact.Version, Valid: true}, ArtifactReference: artifact.Reference, ArtifactDigest: artifact.Digest,
		EnvironmentID: source.EnvironmentID, EnvironmentSourceID: &source.EnvironmentSourceID, CreatedByChangeID: change.ID,
		RegistryResourceID: &artifact.RegistryResourceID, RegistryCredentialID: &artifact.RegistryCredentialID,
		RegistryEndpoint: sql.NullString{String: artifact.RegistryEndpoint, Valid: true},
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(ctx, tx, models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID}); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := tx.NewUpdate().TableExpr("environment_target_states").Set("desired_revision_id = ?", revision.ID).Set("state = 'pending'").Set("updated_at = ?", now).
		Where("environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", source.EnvironmentID).Exec(ctx); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	runtimeSnapshot, _ := json.Marshal(state.Runtime)
	var firstDeployment models.DeploymentEntity
	for index, target := range targets {
		deployment, queueErr := service.queueDeploymentForTargetTx(ctx, tx, change.ID, release.ID, target.ID, runtimeSnapshot, now)
		if queueErr != nil {
			return models.ReleaseEntity{}, models.DeploymentEntity{}, queueErr
		}
		if index == 0 {
			firstDeployment = deployment
		}
	}
	return release, firstDeployment, nil
}

func (service *EnvironmentSetup) queueDeploymentForTargetTx(
	ctx context.Context,
	tx bun.Tx,
	changeID, releaseID, targetID uuid.UUID,
	runtimeSnapshot json.RawMessage,
	now time.Time,
) (models.DeploymentEntity, error) {
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt: 1, Strategy: json.RawMessage(`{"type":"blue_green","replicas":1}`), RuntimeConfiguration: runtimeSnapshot,
		Status: "queued", CurrentStep: sql.NullString{String: "queued", Valid: true}, ChangeID: changeID,
		ReleaseID: releaseID, EnvironmentTargetID: targetID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
		ExternalID: "pending:" + deployment.ID.String(), Slot: "candidate", ReplicaKey: "primary", State: "candidate",
		Ports: json.RawMessage(`{}`), ObservedAt: now, DeploymentID: deployment.ID, ReleaseID: releaseID, EnvironmentTargetID: targetID,
	}); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); err != nil {
		return models.DeploymentEntity{}, err
	}
	return deployment, nil
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
		SchemaVersion: 2, SourceEventID: event.ID, EnvironmentStateRevisionID: stateRevision.ID,
		Repository: source.Repository, Reference: source.Reference, SourceRevision: revisionSHA,
		ContextPath: source.ContextPath, BuilderReference: nullableStringPointer(source.BuilderReference),
		ImageRepository: source.ImageRepository, RegistryResourceID: source.RegistryID,
		RegistryCredentialID: source.RegistryCredentialID,
		RegistryEndpoint:     source.RegistryEndpoint, Settings: source.BuildpackSettings,
		BPGOTargets: state.Runtime.BPGOTargets,
		ServerID:    source.BuildServerID,
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
	if err != nil || release.EnvironmentID != environmentID || release.RegistryResourceID == nil || release.RegistryCredentialID == nil || !release.RegistryEndpoint.Valid {
		return models.DeploymentEntity{}, errors.New("Release does not belong to this Environment")
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, environmentID)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if len(targets) == 0 {
		return models.DeploymentEntity{}, errors.Join(models.ErrDomainValidation, errors.New("Environment has no runtime Server targets"))
	}
	active, err := tx.NewSelect().TableExpr("deployments").
		Where("environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", environmentID).
		Where("status IN ('queued', 'running')").Count(ctx)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if active > 0 {
		return models.DeploymentEntity{}, errors.Join(models.ErrDomainValidation, errors.New("Environment already has an active Deployment"))
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
	if _, err := tx.NewUpdate().TableExpr("environment_target_states").Set("desired_revision_id = ?", revision.ID).Set("state = 'pending'").
		Set("updated_at = ?", now).Where("environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", environmentID).Exec(ctx); err != nil {
		return models.DeploymentEntity{}, err
	}
	runtimeSnapshot, _ := json.Marshal(state.Runtime)
	var firstDeployment models.DeploymentEntity
	for index, target := range targets {
		deployment, queueErr := service.queueDeploymentForTargetTx(ctx, tx, change.ID, release.ID, target.ID, runtimeSnapshot, now)
		if queueErr != nil {
			return models.DeploymentEntity{}, queueErr
		}
		if index == 0 {
			firstDeployment = deployment
		}
	}
	if err := tx.Commit(); err != nil {
		return models.DeploymentEntity{}, err
	}
	return firstDeployment, nil
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
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return err
	}
	serverIDs := make([]uuid.UUID, 0, len(targets))
	for _, target := range targets {
		serverIDs = append(serverIDs, target.ServerID)
	}
	preparedResources, err := service.prepareResources(ctx, environmentID, serverIDs, networkID, input.Resources)
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
		APITokenPrefix: environment.APITokenPrefix, APITokenDigest: environment.APITokenDigest,
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
		credentialSource := "none"
		if prepared.input.CredentialID != nil {
			credentialSource = "managed"
		}
		resourceConfiguration, _ := json.Marshal(map[string]any{
			"schema_version": 1, "credential_source": credentialSource,
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
			EnvironmentResourceID: connection.ID, ResourceID: prepared.resource.ID, Kind: prepared.resource.Engine(),
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
	_, err = service.QueueSourceDeployment(ctx, applicationID, environmentID, &userID, "user", "")
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
	serverIDs := make([]uuid.UUID, 0)
	if err := service.db.Executor().NewSelect().TableExpr("environment_targets").Column("server_id").
		Where("environment_id = ?", environmentID).Group("server_id").Scan(ctx, &serverIDs); err != nil {
		return fmt.Errorf("load Environment workload Servers: %w", err)
	}
	for _, serverID := range serverIDs {
		if err := service.workloads.DeleteEnvironment(ctx, serverID, environmentID); err != nil {
			return err
		}
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
	if err := service.deleteEnvironmentBuildCaches(ctx, environmentID); err != nil {
		return fmt.Errorf("delete Environment Build caches: %w", err)
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

func (service *EnvironmentSetup) deleteEnvironmentBuildCaches(ctx context.Context, environmentID uuid.UUID) error {
	serverIDs := make([]uuid.UUID, 0)
	if err := service.db.Executor().NewSelect().TableExpr("builds").
		ColumnExpr("DISTINCT (build_configuration ->> 'server_id')::uuid AS server_id").
		Where("environment_id = ?", environmentID).Where("build_configuration ->> 'server_id' IS NOT NULL").Scan(ctx, &serverIDs); err != nil {
		return err
	}
	var configuredServerID uuid.UUID
	if err := service.db.Executor().NewSelect().TableExpr("buildpack_configurations AS buildpack").ColumnExpr("buildpack.server_id").
		Join("JOIN environment_sources AS source ON source.id = buildpack.environment_source_id").
		Where("source.environment_id = ?", environmentID).Limit(1).Scan(ctx, &configuredServerID); err == nil {
		serverIDs = append(serverIDs, configuredServerID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	caches, err := buildpacksclient.EnvironmentCacheNames(environmentID)
	if err != nil {
		return err
	}
	seen := make(map[uuid.UUID]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		if serverID == uuid.Nil {
			continue
		}
		if _, exists := seen[serverID]; exists {
			continue
		}
		seen[serverID] = struct{}{}
		target, err := service.servers.Target(ctx, serverID, models.ServerCapabilityBuild)
		if err != nil {
			return err
		}
		if !target.Remote {
			if err := service.buildpacks.DeleteEnvironmentCaches(ctx, environmentID); err != nil {
				return err
			}
			continue
		}
		for _, cache := range []string{caches.Build, caches.Launch} {
			if _, err := service.servers.RunRootCommand(ctx, target, nil, remoteDockerExecutable, "volume", "rm", cache); err != nil {
				message := strings.ToLower(err.Error())
				if strings.Contains(message, "no such volume") || strings.Contains(message, "not found") {
					continue
				}
				return err
			}
		}
	}
	return nil
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
	if err != nil || release.EnvironmentID != environmentID || release.RegistryResourceID == nil || release.RegistryCredentialID == nil || !release.RegistryEndpoint.Valid {
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
		builder.Add("bpGoTargets", "format", "Target must contain repository-relative Go targets")
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
		ColumnExpr("source.id AS environment_source_id, source.kind, source.reference, source.repository").
		ColumnExpr("repository.id AS repository_id, installation.id AS installation_id").
		ColumnExpr("COALESCE(buildpack.context_path, '') AS context_path, buildpack.builder_reference, COALESCE(buildpack.settings, '{}'::jsonb) AS buildpack_settings, COALESCE(buildpack.image_repository, source.repository) AS image_repository, buildpack.server_id AS build_server_id").
		ColumnExpr("registry_resource.id AS registry_id, registry_credential.id AS registry_credential_id").
		ColumnExpr("CASE WHEN registry_endpoint.port IN (80, 443) THEN registry_endpoint.address ELSE registry_endpoint.address || ':' || registry_endpoint.port::text END AS registry_endpoint").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").
		Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").
		Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").
		Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id AND registry_resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS registry_endpoint ON registry_endpoint.resource_id = registry.resource_id AND registry_endpoint.role = 'primary' AND registry_endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS registry_credential ON registry_credential.resource_id = registry.resource_id AND registry_credential.archived_at IS NULL").
		Where("environment.id = ?", environmentID).Where("environment.application_id = ?", applicationID).Limit(1).Scan(ctx, &source)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	source.SetupComplete, err = models.Environment.SetupComplete(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	if source.Kind == "image" {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, nil
	}
	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), source.RepositoryID)
	if err != nil {
		return source, repository, models.GitHubInstallationEntity{}, err
	}
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), source.InstallationID)
	return source, repository, installation, err
}

func normalizedEnvironmentServerIDs(input EnvironmentSetupInput) []uuid.UUID {
	serverIDs := make([]uuid.UUID, 0, len(input.ServerIDs)+1)
	seen := make(map[uuid.UUID]struct{}, len(input.ServerIDs)+1)
	for _, serverID := range input.ServerIDs {
		if serverID == uuid.Nil {
			continue
		}
		if _, exists := seen[serverID]; exists {
			continue
		}
		seen[serverID] = struct{}{}
		serverIDs = append(serverIDs, serverID)
	}
	if input.ServerID != uuid.Nil {
		if _, exists := seen[input.ServerID]; !exists {
			serverIDs = append(serverIDs, input.ServerID)
		}
	}
	return serverIDs
}

func (service *EnvironmentSetup) runtimePlacements(ctx context.Context, serverIDs []uuid.UUID) ([]preparedRuntimePlacement, uuid.UUID, error) {
	if len(serverIDs) == 0 {
		return nil, uuid.Nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "serverIds", Code: "required", Message: "select at least one runtime Server target"}})
	}
	placements := make([]preparedRuntimePlacement, 0, len(serverIDs))
	var networkID uuid.UUID
	for _, serverID := range serverIDs {
		resolvedServerID, resolvedNetworkID, network, err := service.runtimePlacement(ctx, serverID)
		if err != nil {
			return nil, uuid.Nil, err
		}
		if networkID != uuid.Nil && networkID != resolvedNetworkID {
			return nil, uuid.Nil, errors.Join(models.ErrDomainValidation, errors.New("selected runtime Server targets do not share the DeployCrate private network"))
		}
		networkID = resolvedNetworkID
		placements = append(placements, preparedRuntimePlacement{serverID: resolvedServerID, network: network})
	}
	return placements, networkID, nil
}

func (service *EnvironmentSetup) runtimePlacement(ctx context.Context, serverID uuid.UUID) (uuid.UUID, uuid.UUID, models.ServerNetworkEntity, error) {
	server, err := models.Server.Find(ctx, service.db.Executor(), serverID)
	if err != nil || server.ArchivedAt.Valid || !server.IsConfigured || (server.Kind != "self_hosted" && server.Kind != "worker") {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "serverIds", Code: "unavailable", Message: "selected runtime Server is unavailable"}})
	}
	capabilities, err := models.ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Runtime {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "serverIds", Code: "capability", Message: "selected Server does not support application runtimes"}})
	}
	var row struct{ ServerID, NetworkID uuid.UUID }
	err = service.db.Executor().NewSelect().TableExpr("applications AS application").
		ColumnExpr("target.server_id, membership.private_network_id AS network_id").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN environment_networks AS membership ON membership.environment_id = environment.id AND membership.removed_at IS NULL").
		Where("application.slug = ?", models.SystemApplicationSlug).Where("application.archived_at IS NULL").Limit(1).Scan(ctx, &row)
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, err
	}
	var network models.ServerNetworkEntity
	err = service.db.Executor().NewSelect().Model(&network).Where("server_id = ?", serverID).Where("private_network_id = ?", row.NetworkID).Where("removed_at IS NULL").Limit(1).Scan(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "serverIds", Code: "network", Message: "selected Server is not attached to the DeployCrate private network"}})
	}
	return serverID, row.NetworkID, network, nil
}

func (service *EnvironmentSetup) prepareResources(ctx context.Context, environmentID uuid.UUID, serverIDs []uuid.UUID, networkID uuid.UUID, inputs []EnvironmentSetupResourceInput) ([]preparedSetupResource, error) {
	prepared := make([]preparedSetupResource, 0, len(inputs))
	aliases := make(map[string]struct{}, len(inputs))
	resources := make(map[uuid.UUID]struct{}, len(inputs))
	selectedServers := make(map[uuid.UUID]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		selectedServers[serverID] = struct{}{}
	}
	for index, input := range inputs {
		if _, exists := resources[input.ResourceID]; exists {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.resourceId", index), Code: "duplicate", Message: "Resource is already selected"}})
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), input.ResourceID)
		definition, supported := models.FindResourceEngine(resource.Engine())
		if err != nil || resource.ArchivedAt.Valid || !supported {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource is unavailable"))
		}
		input.Alias = strings.ToUpper(strings.TrimSpace(input.Alias))
		input.CredentialProjection = strings.ToLower(strings.TrimSpace(input.CredentialProjection))
		if input.Alias == "" {
			input.Alias = strings.ToUpper(resource.Engine())
		}
		if input.CredentialProjection != resourceCredentialProjectionConnectionURL && input.CredentialProjection != resourceCredentialProjectionIndividualParts {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.credentialProjection", index), Code: "unsupported", Message: "Choose Connection URL or Individual parts"}})
		}
		if input.CredentialProjection == resourceCredentialProjectionConnectionURL && resource.Engine() != "postgresql" {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.credentialProjection", index), Code: "unsupported", Message: "Connection URL is not supported for this Resource"}})
		}
		if _, exists := aliases[input.Alias]; exists {
			return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.alias", index), Code: "duplicate", Message: "Resource alias is already selected"}})
		}
		allowed, err := models.ResourceSelectableByEnvironment(ctx, service.db.Executor(), resource.ID, environmentID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource is not granted to this Environment"))
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), input.EndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID || (endpoint.PrivateNetworkID != nil && *endpoint.PrivateNetworkID != networkID) {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource endpoint is unavailable from the Environment target"))
		}
		var installation models.ResourceInstallationEntity
		installationErr := service.db.Executor().NewSelect().Model(&installation).
			Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").OrderExpr("created_at").Limit(1).Scan(ctx)
		if installationErr == nil {
			if _, selected := selectedServers[installation.ServerID]; !selected {
				return nil, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: fmt.Sprintf("resources.%d.resourceId", index), Code: "placement", Message: "selected managed Resource is not installed on a selected runtime Server target"}})
			}
		} else if !errors.Is(installationErr, sql.ErrNoRows) {
			return nil, installationErr
		}
		if input.CredentialID == nil && resource.Engine() == "postgresql" {
			return nil, errors.Join(models.ErrDomainValidation, errors.New("PostgreSQL application credential is required"))
		}
		prefix := input.Alias
		connectionID := uuid.New()
		variables := map[string]string{
			prefix + "_HOST":     endpoint.Address,
			prefix + "_PORT":     fmt.Sprint(endpoint.Port),
			prefix + "_PROTOCOL": endpoint.Protocol,
			prefix + "_TLS_MODE": endpoint.TlsMode,
		}
		secrets := make([]PreparedEnvironmentSecret, 0, len(definition.CredentialFields))
		var credential *models.ResourceCredentialEntity
		credentialValues := make(map[string]string)
		if input.CredentialID != nil {
			selectedCredential, findErr := models.ResourceCredential.Find(ctx, service.db.Executor(), *input.CredentialID)
			if findErr != nil || selectedCredential.ArchivedAt.Valid || selectedCredential.ResourceID != resource.ID {
				return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource credential is unavailable"))
			}
			if resourceCredentialMetadataPurpose(selectedCredential.Metadata) != "application" {
				return nil, errors.Join(models.ErrDomainValidation, errors.New("Resource administrator credentials cannot be injected into an Environment"))
			}
			input.Database = resourceCredentialMetadataDatabase(selectedCredential.Metadata)
			if input.Database != "" && !resourceHasDatabase(resource, input.Database) {
				return nil, errors.Join(models.ErrDomainValidation, errors.New("selected Resource Database is unavailable"))
			}
			credentialValues, err = service.resources.credentialSecretValues(selectedCredential)
			if err != nil {
				return nil, err
			}
			credential = &selectedCredential
			if selectedCredential.Username.Valid {
				variables[prefix+"_USER"] = selectedCredential.Username.String
			}
		}
		if input.Database != "" {
			variables[prefix+"_DATABASE"] = input.Database
		}
		if input.CredentialProjection == resourceCredentialProjectionConnectionURL {
			password := credentialValues["password"]
			if credential == nil || password == "" || !credential.Username.Valid || input.Database == "" {
				return nil, errors.New("selected PostgreSQL application credential is incomplete")
			}
			variables = make(map[string]string)
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
			for _, field := range definition.CredentialFields {
				value := credentialValues[field.Name]
				if value == "" {
					continue
				}
				secret, prepareErr := service.secrets.Prepare(environmentID, prefix+"_"+models.NormalizeEnvironmentSecretKey(field.Name), value, models.EnvironmentSecretSourceResource, connectionID)
				if prepareErr != nil {
					return nil, prepareErr
				}
				secrets = append(secrets, secret)
			}
		}
		prepared = append(prepared, preparedSetupResource{input: input, connectionID: connectionID, resource: resource, endpoint: endpoint, credential: credential, variables: variables, secrets: secrets})
		aliases[input.Alias] = struct{}{}
		resources[input.ResourceID] = struct{}{}
	}
	return prepared, nil
}
