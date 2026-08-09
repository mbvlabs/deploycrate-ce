package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

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
type EnvironmentOverviewSource struct {
	ApplicationName  string `bun:"application_name"`
	SourceType       string `bun:"source_type"`
	Repository       string `bun:"repository"`
	Reference        string `bun:"reference"`
	ContextPath      string `bun:"context_path"`
	RegistryName     string `bun:"registry_name"`
	RegistryEndpoint string `bun:"registry_endpoint"`
}
type EnvironmentRuntimeServer struct {
	ID       uuid.UUID `bun:"id"`
	TargetID uuid.UUID `bun:"target_id"`
	Name     string    `bun:"name"`
}
type EnvironmentResourceActivity struct {
	ID     uuid.UUID `json:"id" bun:"id"`
	Alias  string    `json:"alias" bun:"alias"`
	Name   string    `json:"name" bun:"name"`
	Engine string    `json:"engine" bun:"engine"`
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
type EnvironmentReleaseActivity struct {
	ID                uuid.UUID `json:"id" bun:"id"`
	SourceRevision    string    `json:"sourceRevision" bun:"source_revision"`
	ArtifactReference string    `json:"artifactReference" bun:"artifact_reference"`
	CreatedAt         time.Time `json:"createdAt" bun:"created_at"`
}
type EnvironmentDeploymentActivity struct {
	ID          uuid.UUID  `json:"id" bun:"id"`
	Status      string     `json:"status" bun:"status"`
	CurrentStep string     `json:"currentStep" bun:"current_step"`
	Error       string     `json:"error" bun:"error"`
	ReleaseID   uuid.UUID  `json:"releaseId" bun:"release_id"`
	TargetID    uuid.UUID  `json:"targetId" bun:"target_id"`
	TargetName  string     `json:"targetName" bun:"target_name"`
	Attempt     int32      `json:"attempt" bun:"attempt"`
	CreatedAt   time.Time  `json:"createdAt" bun:"created_at"`
	StartedAt   *time.Time `json:"startedAt" bun:"started_at"`
	FinishedAt  *time.Time `json:"finishedAt" bun:"finished_at"`
	Active      bool       `json:"active" bun:"active"`
}
type EnvironmentInstanceActivity struct {
	ID           uuid.UUID       `json:"id" bun:"id"`
	State        string          `json:"state" bun:"state"`
	Slot         string          `json:"slot" bun:"slot"`
	ProcessName  string          `json:"processName" bun:"process_name"`
	ProcessKind  string          `json:"processKind" bun:"process_kind"`
	ReplicaKey   string          `json:"replicaKey" bun:"replica_key"`
	Ports        json.RawMessage `json:"ports" bun:"ports"`
	ReleaseID    uuid.UUID       `json:"releaseId" bun:"release_id"`
	DeploymentID uuid.UUID       `json:"deploymentId" bun:"deployment_id"`
	TargetID     uuid.UUID       `json:"targetId" bun:"target_id"`
	TargetName   string          `json:"targetName" bun:"target_name"`
	ObservedAt   time.Time       `json:"observedAt" bun:"observed_at"`
}
type EnvironmentReleaseCommandActivity struct {
	ID             uuid.UUID       `json:"id" bun:"id"`
	Status         string          `json:"status" bun:"status"`
	Attempt        int32           `json:"attempt" bun:"attempt"`
	ExternalID     string          `json:"externalId" bun:"external_id"`
	ExitCode       *int32          `json:"exitCode" bun:"exit_code"`
	StartedAt      *time.Time      `json:"startedAt" bun:"started_at"`
	FinishedAt     *time.Time      `json:"finishedAt" bun:"finished_at"`
	Error          string          `json:"error" bun:"error"`
	ReleaseID      uuid.UUID       `json:"releaseId" bun:"release_id"`
	TargetID       uuid.UUID       `json:"targetId" bun:"target_id"`
	TargetName     string          `json:"targetName" bun:"target_name"`
	Command        string          `json:"command" bun:"command"`
	Arguments      json.RawMessage `json:"arguments" bun:"arguments"`
	TimeoutSeconds int32           `json:"timeoutSeconds" bun:"timeout_seconds"`
	CreatedAt      time.Time       `json:"createdAt" bun:"created_at"`
}

const environmentDeploymentActiveExpression = `EXISTS (SELECT 1 FROM instances AS active_instance JOIN caddy_route_backends AS active_backend ON active_backend.instance_id = active_instance.id JOIN caddy_routes AS active_route ON active_route.id = active_backend.caddy_route_id WHERE active_instance.deployment_id = deployment.id AND active_instance.state = 'serving' AND active_instance.removed_at IS NULL AND active_backend.weight = 100 AND active_backend.removed_at IS NULL AND active_route.environment_target_id = deployment.environment_target_id AND active_route.removed_at IS NULL)`

type EnvironmentOverviewRows struct {
	Source          EnvironmentOverviewSource
	Domain          string
	RuntimeServers  []EnvironmentRuntimeServer
	Resources       []EnvironmentResourceActivity
	Builds          []EnvironmentBuildActivity
	Releases        []EnvironmentReleaseActivity
	Deployments     []EnvironmentDeploymentActivity
	Instances       []EnvironmentInstanceActivity
	ReleaseCommands []EnvironmentReleaseCommandActivity
}

func (environment) ListCatalog(ctx context.Context, db storage.Executor) ([]EnvironmentListItem, error) {
	rows := make([]EnvironmentListItem, 0)
	err := db.NewSelect().TableExpr("environments AS environment").ColumnExpr("environment.id, environment.name, environment.kind").ColumnExpr("application.id AS application_id, application.name AS application_name").
		ColumnExpr(`EXISTS (SELECT 1 FROM changes AS setup_change JOIN change_state_revisions AS setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result' JOIN environment_state_revisions AS setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = environment.id WHERE setup_change.environment_id = environment.id AND setup_change.kind = 'environment_setup' AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL) AS setup_complete`).
		ColumnExpr("COALESCE((SELECT hostname FROM environment_domains WHERE environment_id = environment.id AND is_primary AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS domain").
		ColumnExpr("COALESCE((SELECT repository FROM environment_sources WHERE environment_id = environment.id AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS repository").
		ColumnExpr("COALESCE((SELECT reference FROM environment_sources WHERE environment_id = environment.id AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1), '') AS reference").
		ColumnExpr("COALESCE((SELECT status FROM builds WHERE environment_id = environment.id ORDER BY created_at DESC LIMIT 1), '') AS latest_build_status").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").Where("environment.archived_at IS NULL").Where("application.slug <> ?", SystemApplicationSlug).OrderExpr("application.name, environment.name").Scan(ctx, &rows)
	return rows, err
}

func (environment) OverviewCatalog(ctx context.Context, db storage.Executor, applicationID, environmentID uuid.UUID, includeRuntime bool) (EnvironmentOverviewRows, error) {
	result := EnvironmentOverviewRows{
		RuntimeServers:  make([]EnvironmentRuntimeServer, 0),
		Resources:       make([]EnvironmentResourceActivity, 0),
		Builds:          make([]EnvironmentBuildActivity, 0),
		Releases:        make([]EnvironmentReleaseActivity, 0),
		Deployments:     make([]EnvironmentDeploymentActivity, 0),
		Instances:       make([]EnvironmentInstanceActivity, 0),
		ReleaseCommands: make([]EnvironmentReleaseCommandActivity, 0),
	}
	err := db.NewSelect().TableExpr("applications AS application").ColumnExpr("application.name AS application_name, environment_source.repository, environment_source.reference, CASE WHEN environment_source.kind = 'image' THEN 'image' ELSE 'buildpacks' END AS source_type, COALESCE(buildpack.context_path, '') AS context_path").ColumnExpr("registry_resource.name AS registry_name").ColumnExpr("CASE WHEN registry_resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), registry_endpoint.address) WHEN registry_endpoint.port IN (80, 443) THEN registry_endpoint.address ELSE registry_endpoint.address || ':' || registry_endpoint.port::text END AS registry_endpoint").
		Join("JOIN environment_sources AS environment_source ON environment_source.environment_id = ? AND environment_source.archived_at IS NULL", environmentID).Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = environment_source.id").Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = environment_source.id").Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id").Join("JOIN resource_endpoints AS registry_endpoint ON registry_endpoint.resource_id = registry.resource_id AND registry_endpoint.role = 'primary' AND registry_endpoint.archived_at IS NULL").Where("application.id = ?", applicationID).Limit(1).Scan(ctx, &result.Source)
	if err != nil {
		return result, err
	}
	_ = db.NewSelect().TableExpr("environment_domains").ColumnExpr("hostname").Where("environment_id = ?", environmentID).Where("is_primary = TRUE").Where("archived_at IS NULL").Limit(1).Scan(ctx, &result.Domain)
	if includeRuntime {
		err = db.NewSelect().TableExpr("environment_targets AS target").ColumnExpr("server.id, target.id AS target_id, server.name").Join("JOIN servers AS server ON server.id = target.server_id").Where("target.environment_id = ?", environmentID).Where("target.detached_at IS NULL").OrderExpr("server.name").Scan(ctx, &result.RuntimeServers)
		if err != nil {
			return result, err
		}
	}
	err = db.NewSelect().TableExpr("environment_resources AS connection").ColumnExpr("connection.id, connection.alias, resource.name, resource.configuration ->> 'engine' AS engine").Join("JOIN resources AS resource ON resource.id = connection.resource_id").Where("connection.environment_id = ?", environmentID).Where("connection.archived_at IS NULL").OrderExpr("connection.alias").Scan(ctx, &result.Resources)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().TableExpr("builds").ColumnExpr("id, source_revision, status, COALESCE(current_step, '') AS current_step, COALESCE(error, '') AS error, created_at, started_at, finished_at").ColumnExpr("COALESCE(build_configuration ->> 'registry_endpoint', '') AS registry_endpoint").ColumnExpr("(SELECT job.id FROM river_job AS job WHERE job.kind = 'build_source' AND job.args ->> 'build_id' = builds.id::text ORDER BY job.id DESC LIMIT 1) AS job_id").ColumnExpr("COALESCE((SELECT job.state::text FROM river_job AS job WHERE job.kind = 'build_source' AND job.args ->> 'build_id' = builds.id::text ORDER BY job.id DESC LIMIT 1), '') AS job_state").Where("environment_id = ?", environmentID).OrderExpr("created_at DESC").Limit(20).Scan(ctx, &result.Builds)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().TableExpr("releases").ColumnExpr("id, COALESCE(source_revision, version, '') AS source_revision, artifact_reference, created_at").Where("environment_id = ?", environmentID).Where("registry_resource_id IS NOT NULL").OrderExpr("created_at DESC").Limit(20).Scan(ctx, &result.Releases)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().TableExpr("deployments AS deployment").ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.environment_target_id AS target_id, server.name AS target_name, deployment.attempt, deployment.created_at, deployment.started_at, deployment.finished_at").ColumnExpr(environmentDeploymentActiveExpression+" AS active").Join("JOIN releases AS release ON release.id = deployment.release_id").Join("JOIN environment_targets AS target ON target.id = deployment.environment_target_id").Join("JOIN servers AS server ON server.id = target.server_id").Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").OrderExpr("deployment.created_at DESC").Limit(30).Scan(ctx, &result.Deployments)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().TableExpr("instances AS instance").ColumnExpr("instance.id, instance.state, instance.slot, instance.process_name, instance.process_kind, instance.replica_key, instance.ports, instance.release_id, instance.deployment_id, instance.environment_target_id AS target_id, server.name AS target_name, instance.observed_at").Join("JOIN releases AS release ON release.id = instance.release_id").Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").Join("JOIN servers AS server ON server.id = target.server_id").Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").Where("instance.removed_at IS NULL").OrderExpr("target.attached_at, instance.process_kind, instance.process_name, instance.replica_key, instance.created_at DESC").Limit(200).Scan(ctx, &result.Instances)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().TableExpr("release_command_executions AS execution").ColumnExpr("execution.id, execution.status, execution.attempt, COALESCE(execution.external_id, '') AS external_id, execution.exit_code, execution.started_at, execution.finished_at, COALESCE(execution.error, '') AS error, execution.release_id, execution.environment_target_id AS target_id, server.name AS target_name, execution.configuration ->> 'command' AS command, execution.configuration -> 'arguments' AS arguments, (execution.configuration ->> 'timeout_seconds')::integer AS timeout_seconds, execution.created_at").Join("JOIN releases AS release ON release.id = execution.release_id").Join("JOIN environment_targets AS target ON target.id = execution.environment_target_id").Join("JOIN servers AS server ON server.id = target.server_id").Where("release.environment_id = ?", environmentID).OrderExpr("execution.created_at DESC").Limit(20).Scan(ctx, &result.ReleaseCommands)
	return result, err
}

type EnvironmentServingInstance struct {
	ID                  uuid.UUID `bun:"id"`
	DeploymentID        uuid.UUID `bun:"deployment_id"`
	EnvironmentTargetID uuid.UUID `bun:"environment_target_id"`
}

func (instance) ServingForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (EnvironmentServingInstance, error) {
	var row EnvironmentServingInstance
	err := db.NewSelect().TableExpr("instances AS instance").ColumnExpr("instance.id, instance.deployment_id, instance.environment_target_id").Join("JOIN releases AS release ON release.id = instance.release_id").Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").Where("instance.process_kind = 'web'").Where("instance.state = 'serving'").Where("instance.removed_at IS NULL").OrderExpr("instance.created_at DESC").Limit(1).Scan(ctx, &row)
	return row, err
}

func (environmentTargetState) ActiveForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]EnvironmentTargetStateEntity, error) {
	rows := make([]EnvironmentTargetStateEntity, 0)
	err := db.NewSelect().Model(&rows).Join("JOIN environment_targets AS target ON target.id = environment_target_states.environment_target_id").Where("target.environment_id = ?", environmentID).Where("target.detached_at IS NULL").OrderExpr("target.attached_at, target.id").Scan(ctx)
	return rows, err
}
func (deployment) LatestFailedRevisionID(ctx context.Context, db storage.Executor, targetID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewSelect().TableExpr("deployments AS deployment").ColumnExpr("association.environment_state_revision_id").Join("JOIN change_state_revisions AS association ON association.change_id = deployment.change_id AND association.role = 'result'").Where("deployment.environment_target_id = ?", targetID).Where("deployment.status = 'failed'").OrderExpr("deployment.finished_at DESC NULLS LAST, deployment.created_at DESC").Limit(1).Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

type EnvironmentEditResourceRow struct {
	ID                   uuid.UUID       `bun:"id"`
	ResourceID           uuid.UUID       `bun:"resource_id"`
	ResourceEndpointID   uuid.UUID       `bun:"resource_endpoint_id"`
	ResourceCredentialID *uuid.UUID      `bun:"resource_credential_id"`
	Alias                string          `bun:"alias"`
	Configuration        json.RawMessage `bun:"configuration"`
	CredentialMetadata   json.RawMessage `bun:"credential_metadata"`
}

func (environmentResource) EditRows(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]EnvironmentEditResourceRow, error) {
	rows := make([]EnvironmentEditResourceRow, 0)
	err := db.NewSelect().TableExpr("environment_resources AS connection").ColumnExpr("connection.id, connection.resource_id, connection.resource_endpoint_id, connection.resource_credential_id, connection.alias, connection.configuration").ColumnExpr("credential.metadata AS credential_metadata").Join("JOIN resource_endpoints AS endpoint ON endpoint.id = connection.resource_endpoint_id AND endpoint.archived_at IS NULL").Join("JOIN resources AS resource ON resource.id = connection.resource_id AND resource.archived_at IS NULL").Join("LEFT JOIN resource_credentials AS credential ON credential.id = connection.resource_credential_id AND credential.archived_at IS NULL").Where("connection.environment_id = ?", environmentID).Where("connection.archived_at IS NULL").Where("resource.environment_attachable = TRUE").OrderExpr("connection.alias").Scan(ctx, &rows)
	return rows, err
}

func (deployment) EnvironmentActivity(ctx context.Context, db storage.Executor, environmentID, deploymentID uuid.UUID) (EnvironmentDeploymentActivity, error) {
	var row EnvironmentDeploymentActivity
	err := db.NewSelect().TableExpr("deployments AS deployment").ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, COALESCE(deployment.error, '') AS error, deployment.release_id, deployment.environment_target_id AS target_id, server.name AS target_name, deployment.attempt, deployment.created_at, deployment.started_at, deployment.finished_at").ColumnExpr(environmentDeploymentActiveExpression+" AS active").Join("JOIN releases AS release ON release.id = deployment.release_id").Join("JOIN environment_targets AS target ON target.id = deployment.environment_target_id").Join("JOIN servers AS server ON server.id = target.server_id").Where("deployment.id = ?", deploymentID).Where("release.environment_id = ?", environmentID).Where("release.registry_resource_id IS NOT NULL").Limit(1).Scan(ctx, &row)
	return row, err
}

type EnvironmentSetupServerOption struct {
	ID      uuid.UUID `json:"id" bun:"id"`
	Name    string    `json:"name" bun:"name"`
	Kind    string    `json:"kind" bun:"kind"`
	Address string    `json:"address" bun:"address"`
}

func (server) EnvironmentSetupOptions(ctx context.Context, db storage.Executor) ([]EnvironmentSetupServerOption, error) {
	rows := make([]EnvironmentSetupServerOption, 0)
	err := db.NewSelect().TableExpr("servers AS server").ColumnExpr("server.id, server.name, server.kind, server.address").Where("server.archived_at IS NULL").Where("server.is_configured = TRUE").Where("server.kind IN ('self_hosted', 'worker')").Where("server.capabilities @> '{\"runtime\":true}'::jsonb").OrderExpr("CASE WHEN server.kind = 'self_hosted' THEN 0 ELSE 1 END, server.name").Scan(ctx, &rows)
	return rows, err
}

func (environmentSource) LatestActiveID(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewSelect().TableExpr("environment_sources").ColumnExpr("id").Where("environment_id = ?", environmentID).Where("archived_at IS NULL").OrderExpr("created_at DESC").Limit(1).Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func (environment) ProductionForApplication(ctx context.Context, db storage.Executor, applicationID uuid.UUID) ([]EnvironmentEntity, error) {
	rows := make([]EnvironmentEntity, 0)
	err := db.NewSelect().Model(&rows).Where("application_id = ?", applicationID).Where("kind = 'production'").Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	return rows, err
}
func (environmentNetwork) ActivePrivateNetworkID(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewSelect().TableExpr("environment_networks").ColumnExpr("private_network_id").Where("environment_id = ?", environmentID).Where("removed_at IS NULL").Limit(1).Scan(ctx, &id)
	return id, err
}
func (releaseCommandExecution) ActiveCountForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("release_command_executions AS execution").Join("JOIN releases AS release ON release.id = execution.release_id").Where("release.environment_id = ?", environmentID).Where("execution.status IN ('queued', 'running')").Count(ctx)
}
func (environment) EnsureSlugAvailableExcluding(ctx context.Context, db storage.Executor, applicationID, environmentID uuid.UUID, slug string) (bool, error) {
	if err := lockUnique(ctx, db, "environment-slug:"+applicationID.String()+":"+slug); err != nil {
		return false, err
	}
	count, err := db.NewSelect().TableExpr("environments").Where("application_id = ?", applicationID).Where("id <> ?", environmentID).Where("slug = ?", slug).Count(ctx)
	return count == 0, err
}
func (environmentDomain) PrimaryForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (EnvironmentDomainEntity, error) {
	var row EnvironmentDomainEntity
	err := db.NewSelect().Model(&row).Where("environment_id = ?", environmentID).Where("is_primary = TRUE").Where("archived_at IS NULL").Limit(1).Scan(ctx)
	return row, err
}
func (environmentResource) ActiveForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]EnvironmentResourceEntity, error) {
	rows := make([]EnvironmentResourceEntity, 0)
	err := db.NewSelect().Model(&rows).Where("environment_id = ?", environmentID).Where("archived_at IS NULL").Scan(ctx)
	return rows, err
}
func (resourceCredential) ArchiveID(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("resource_credentials").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("id = ?", id).Where("archived_at IS NULL").Exec(ctx)
	return err
}
func (environmentTargetState) MarkEnvironmentPending(ctx context.Context, db storage.Executor, environmentID, revisionID uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().TableExpr("environment_target_states AS state").Set("desired_revision_id = ?", revisionID).Set("state = 'pending'").Set("updated_at = ?", at).Where("state.environment_target_id IN (SELECT id FROM environment_targets WHERE environment_id = ? AND detached_at IS NULL)", environmentID).Exec(ctx)
	return err
}

type EnvironmentJobReference struct {
	ID    int64  `bun:"id"`
	State string `bun:"state"`
}

func (job) ForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]EnvironmentJobReference, error) {
	rows := make([]EnvironmentJobReference, 0)
	err := db.NewSelect().TableExpr("river_job AS job").ColumnExpr("job.id, job.state::text AS state").Where(`(job.kind = 'build_source' AND EXISTS (SELECT 1 FROM builds AS build WHERE build.environment_id = ? AND build.id::text = job.args ->> 'build_id')) OR (job.kind = 'deploy_release' AND EXISTS (SELECT 1 FROM deployments AS deployment JOIN environment_targets AS target ON target.id = deployment.environment_target_id WHERE target.environment_id = ? AND deployment.id::text = job.args ->> 'deployment_id')) OR (job.kind = 'release_command' AND EXISTS (SELECT 1 FROM release_command_executions AS execution JOIN releases AS release ON release.id = execution.release_id WHERE release.environment_id = ? AND execution.id::text = job.args ->> 'release_command_execution_id'))`, environmentID, environmentID, environmentID).Scan(ctx, &rows)
	return rows, err
}
func (caddyRoute) ExternalIDsForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]string, error) {
	rows := make([]string, 0)
	err := db.NewSelect().TableExpr("caddy_routes AS route").ColumnExpr("route.external_id").Join("JOIN environment_targets AS target ON target.id = route.environment_target_id").Where("target.environment_id = ?", environmentID).Scan(ctx, &rows)
	return rows, err
}
func (environmentTarget) ServerIDsForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]uuid.UUID, error) {
	rows := make([]uuid.UUID, 0)
	err := db.NewSelect().TableExpr("environment_targets").Column("server_id").Where("environment_id = ?", environmentID).Group("server_id").Scan(ctx, &rows)
	return rows, err
}
func (change) RehomeDurableForEnvironments(ctx context.Context, db storage.Executor, environmentIDs []uuid.UUID, at time.Time) error {
	if len(environmentIDs) == 0 {
		return nil
	}
	var systemID uuid.UUID
	if err := db.NewSelect().TableExpr("environments AS environment").ColumnExpr("environment.id").Join("JOIN applications AS application ON application.id = environment.application_id").Where("application.slug = ?", SystemApplicationSlug).Where("application.archived_at IS NULL").Where("environment.archived_at IS NULL").OrderExpr("environment.created_at").Limit(1).Scan(ctx, &systemID); err != nil {
		return err
	}
	_, err := db.NewUpdate().TableExpr("changes AS candidate").Set("environment_id = ?", systemID).Set("updated_at = ?", at).Where("candidate.environment_id IN (?)", bun.In(environmentIDs)).Where(`EXISTS (SELECT 1 FROM backups AS backup WHERE backup.change_id = candidate.id) OR EXISTS (SELECT 1 FROM resource_restores AS restore WHERE restore.change_id = candidate.id) OR EXISTS (SELECT 1 FROM change_tasks AS task WHERE task.change_id = candidate.id AND (EXISTS (SELECT 1 FROM backups AS backup WHERE backup.change_task_id = task.id) OR EXISTS (SELECT 1 FROM resource_restores AS restore WHERE restore.change_task_id = task.id)))`).Exec(ctx)
	return err
}
func (resourceCredential) DeleteForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) error {
	_, err := db.NewDelete().TableExpr("resource_credentials").Where("metadata ->> 'purpose' = 'application'").Where("metadata ->> 'environment_id' = ?", environmentID.String()).Exec(ctx)
	return err
}
func (application) LockNonSystem(ctx context.Context, db storage.Executor, id uuid.UUID) (ApplicationEntity, error) {
	var row ApplicationEntity
	err := db.NewSelect().Model(&row).Where("id = ?", id).Where("slug <> ?", SystemApplicationSlug).For("UPDATE").Scan(ctx)
	return row, err
}
func (environment) CountForApplication(ctx context.Context, db storage.Executor, applicationID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("environments").Where("application_id = ?", applicationID).Count(ctx)
}
func (environment) LockIDsForApplication(ctx context.Context, db storage.Executor, applicationID uuid.UUID) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0)
	err := db.NewSelect().TableExpr("environments").Column("id").Where("application_id = ?", applicationID).OrderExpr("created_at, id").For("UPDATE").Scan(ctx, &ids)
	return ids, err
}
func (build) CacheServerIDs(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0)
	err := db.NewSelect().TableExpr("builds").ColumnExpr("DISTINCT (build_configuration ->> 'server_id')::uuid AS server_id").Where("environment_id = ?", environmentID).Where("build_configuration ->> 'server_id' IS NOT NULL").Scan(ctx, &ids)
	return ids, err
}
func (buildpackConfiguration) ServerIDForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewSelect().TableExpr("buildpack_configurations AS buildpack").ColumnExpr("buildpack.server_id").Join("JOIN environment_sources AS source ON source.id = buildpack.environment_source_id").Where("source.environment_id = ?", environmentID).Limit(1).Scan(ctx, &id)
	return id, err
}

type EnvironmentRuntimePlacement struct {
	ServerID  uuid.UUID `bun:"server_id"`
	NetworkID uuid.UUID `bun:"network_id"`
}

func (application) SystemRuntimePlacement(ctx context.Context, db storage.Executor) (EnvironmentRuntimePlacement, error) {
	var row EnvironmentRuntimePlacement
	err := db.NewSelect().TableExpr("applications AS application").ColumnExpr("target.server_id, membership.private_network_id AS network_id").Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").Join("JOIN environment_networks AS membership ON membership.environment_id = environment.id AND membership.removed_at IS NULL").Where("application.slug = ?", SystemApplicationSlug).Where("application.archived_at IS NULL").Limit(1).Scan(ctx, &row)
	return row, err
}
func (serverNetwork) ActiveForServerNetwork(ctx context.Context, db storage.Executor, serverID, networkID uuid.UUID) (ServerNetworkEntity, error) {
	var row ServerNetworkEntity
	err := db.NewSelect().Model(&row).Where("server_id = ?", serverID).Where("private_network_id = ?", networkID).Where("removed_at IS NULL").Limit(1).Scan(ctx)
	return row, err
}
func (resourceCredential) ActiveApplicationForEnvironment(ctx context.Context, db storage.Executor, resourceID, environmentID uuid.UUID) (ResourceCredentialEntity, error) {
	var row ResourceCredentialEntity
	err := db.NewSelect().Model(&row).Where("resource_id = ?", resourceID).Where("metadata ->> 'purpose' = 'application'").Where("metadata ->> 'environment_id' = ?", environmentID.String()).Where("archived_at IS NULL").Limit(1).Scan(ctx)
	return row, err
}
func (resource) LockActive(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceEntity, error) {
	var row ResourceEntity
	err := db.NewSelect().Model(&row).Where("id = ?", id).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}

type EnvironmentSetupSource struct {
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

func (environmentSource) SetupSource(ctx context.Context, db storage.Executor, applicationID, environmentID uuid.UUID) (EnvironmentSetupSource, error) {
	var source EnvironmentSetupSource
	err := db.NewSelect().TableExpr("environments AS environment").ColumnExpr("environment.application_id, environment.id AS environment_id, environment.archived_at AS environment_archived_at").ColumnExpr("source.id AS environment_source_id, source.kind, source.reference, source.repository").ColumnExpr("repository.id AS repository_id, installation.id AS installation_id").ColumnExpr("COALESCE(buildpack.context_path, '') AS context_path, buildpack.builder_reference, COALESCE(buildpack.settings, '{}'::jsonb) AS buildpack_settings, COALESCE(buildpack.image_repository, source.repository) AS image_repository, buildpack.server_id AS build_server_id").ColumnExpr("registry_resource.id AS registry_id, registry_credential.id AS registry_credential_id").ColumnExpr("CASE WHEN registry_resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), registry_endpoint.address) WHEN registry_endpoint.port IN (80, 443) THEN registry_endpoint.address ELSE registry_endpoint.address || ':' || registry_endpoint.port::text END AS registry_endpoint").
		Join("JOIN environment_sources AS source ON source.environment_id = environment.id AND source.archived_at IS NULL").Join("LEFT JOIN github_environment_sources AS binding ON binding.environment_source_id = source.id").Join("LEFT JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").Join("LEFT JOIN github_installations AS installation ON installation.id = repository.github_installation_id").Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").Join("JOIN registry_resources AS registry ON registry.resource_id = COALESCE(buildpack.registry_resource_id, image.registry_resource_id)").Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id AND registry_resource.archived_at IS NULL").Join("JOIN resource_endpoints AS registry_endpoint ON registry_endpoint.resource_id = registry.resource_id AND registry_endpoint.role = 'primary' AND registry_endpoint.archived_at IS NULL").Join("JOIN resource_credentials AS registry_credential ON registry_credential.resource_id = registry.resource_id AND registry_credential.archived_at IS NULL").Where("environment.id = ?", environmentID).Where("environment.application_id = ?", applicationID).Limit(1).Scan(ctx, &source)
	return source, err
}
