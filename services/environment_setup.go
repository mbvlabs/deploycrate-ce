package services

import (
	"bytes"
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
	"path"
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

var goTargetsPattern = regexp.MustCompile(`^[A-Za-z0-9_./,:*-]*$`)

const (
	resourceCredentialProjectionConnectionURL   = "connection_url"
	resourceCredentialProjectionIndividualParts = "individual_parts"
)

type environmentResourceConfiguration struct {
	SchemaVersion           int               `json:"schema_version"`
	CredentialSource        string            `json:"credential_source"`
	CredentialProjection    string            `json:"credential_projection"`
	EnvironmentKeyOverrides map[string]string `json:"environment_key_overrides,omitempty"`
	EnvironmentKeys         map[string]string `json:"environment_keys"`
}

func encodeEnvironmentResourceConfiguration(
	projection string,
	credentialID *uuid.UUID,
	credentialSource string,
	environmentKeys, environmentKeyOverrides map[string]string,
) (json.RawMessage, error) {
	credentialSource = strings.ToLower(strings.TrimSpace(credentialSource))
	if credentialSource == "" && credentialID != nil {
		credentialSource = "managed"
	} else if credentialSource == "" {
		credentialSource = "none"
	}
	return json.Marshal(environmentResourceConfiguration{
		SchemaVersion: 1, CredentialSource: credentialSource,
		CredentialProjection: projection, EnvironmentKeys: environmentKeys,
		EnvironmentKeyOverrides: environmentKeyOverrides,
	})
}

func parseEnvironmentResourceConfiguration(
	raw json.RawMessage,
) (environmentResourceConfiguration, error) {
	var configuration environmentResourceConfiguration
	if json.Unmarshal(raw, &configuration) != nil || configuration.CredentialProjection == "" {
		return environmentResourceConfiguration{}, errors.New(
			"Environment Resource configuration is invalid",
		)
	}
	return configuration, nil
}

type EnvironmentSetup struct {
	db          storage.Pool
	queue       storage.InsertQueue
	jobControl  BuildJobControl
	github      *GitHubConnection
	secrets     *EnvironmentSecrets
	resources   *ResourceManagement
	dns         *EnvironmentDNS
	caddy       CaddyRouteService
	workloads   *WorkloadExecution
	buildpacks  buildpacksclient.Client
	servers     *ServerExecution
	builds      *BuildExecution
	registry    registryclient.Client
	telemetry   *TelemetryIdentity
	releases    *ReleaseDeployment
	deployments *DeploymentExecution
	config      config.Config
}

func NewEnvironmentSetup(
	db storage.Pool,
	queue storage.InsertQueue,
	jobControl BuildJobControl,
	github *GitHubConnection,
	secrets *EnvironmentSecrets,
	resources *ResourceManagement,
	dns *EnvironmentDNS,
	caddy CaddyRouteService,
	workloads *WorkloadExecution,
	servers *ServerExecution,
	builds *BuildExecution,
	telemetry *TelemetryIdentity,
	releases *ReleaseDeployment,
	deployments *DeploymentExecution,
	cfg config.Config,
) *EnvironmentSetup {
	return &EnvironmentSetup{
		db:          db,
		queue:       queue,
		jobControl:  jobControl,
		github:      github,
		secrets:     secrets,
		resources:   resources,
		dns:         dns,
		caddy:       caddy,
		workloads:   workloads,
		buildpacks:  buildpacksclient.New(),
		servers:     servers,
		builds:      builds,
		registry:    registryclient.New(),
		telemetry:   telemetry,
		releases:    releases,
		deployments: deployments,
		config:      cfg,
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
	Processes     []models.EnvironmentProcessInput
	Resources     []EnvironmentSetupResourceInput
	Secrets       []EnvironmentSetupSecretInput
	Deploy        bool
	DNS           EnvironmentDNSInput
}

type EnvironmentSetupResult struct {
	Environment        models.EnvironmentEntity              `json:"environment"`
	Revision           models.EnvironmentStateRevisionEntity `json:"revision"`
	Build              models.BuildEntity                    `json:"build"`
	Release            models.ReleaseEntity                  `json:"release"`
	Deployment         models.DeploymentEntity               `json:"deployment"`
	DeploymentDeferred bool                                  `json:"deploymentDeferred"`
}

type EnvironmentSetupResourceOption struct {
	ID                    uuid.UUID         `json:"id"                    bun:"id"`
	Name                  string            `json:"name"                  bun:"name"`
	Engine                string            `json:"engine"                bun:"engine"`
	Database              string            `json:"database"              bun:"database_name"`
	EndpointID            uuid.UUID         `json:"endpointId"            bun:"endpoint_id"`
	Endpoint              string            `json:"endpoint"              bun:"endpoint"`
	CredentialID          *uuid.UUID        `json:"credentialId"          bun:"credential_id"`
	Credential            string            `json:"credential"            bun:"credential"`
	ServerID              *uuid.UUID        `json:"serverId"              bun:"server_id"`
	ResourceConfiguration json.RawMessage   `json:"-"                     bun:"resource_configuration"`
	CredentialFields      []string          `json:"credentialFields"      bun:"-"`
	EnvironmentKeys       map[string]string `json:"environmentKeys"       bun:"-"`
	SupportsConnectionURL bool              `json:"supportsConnectionUrl" bun:"-"`
}

type EnvironmentSetupServerOption = models.EnvironmentSetupServerOption

type EnvironmentSetupOptions struct {
	Resources []EnvironmentSetupResourceOption `json:"resources"`
	Servers   []EnvironmentSetupServerOption   `json:"servers"`
	DNSZones  []EnvironmentDNSOption           `json:"dnsZones"`
}

type EnvironmentEditConfiguration struct {
	Runtime       models.BuildpackRuntime          `json:"runtime"`
	Name          string                           `json:"name"`
	Slug          string                           `json:"slug"`
	Kind          string                           `json:"kind"`
	Hostname      string                           `json:"hostname"`
	ContainerPort int32                            `json:"containerPort"`
	HealthPath    string                           `json:"healthPath"`
	Processes     []models.EnvironmentProcessInput `json:"processes"`
	Resources     []EnvironmentSetupResourceInput  `json:"resources"`
	ServerIDs     []uuid.UUID                      `json:"serverIds"`
	ServerNames   []string                         `json:"serverNames"`
	DNSMode       string                           `json:"dnsMode"`
	DNSZoneID     *uuid.UUID                       `json:"dnsZoneId"`
}

type EnvironmentEditData struct {
	Overview      EnvironmentOverview          `json:"overview"`
	Configuration EnvironmentEditConfiguration `json:"configuration"`
}

type EnvironmentEditInput struct {
	Name          string
	Slug          string
	Kind          string
	ServerIDs     []uuid.UUID
	Hostname      string
	ContainerPort int32
	HealthPath    string
	Processes     []models.EnvironmentProcessInput
	Resources     []EnvironmentSetupResourceInput
	DNS           EnvironmentDNSInput
}

type EnvironmentOverview struct {
	ApplicationID                uuid.UUID                           `json:"applicationId"`
	ApplicationName              string                              `json:"applicationName"`
	SourceType                   string                              `json:"sourceType"`
	Environment                  models.EnvironmentEntity            `json:"environment"`
	SetupComplete                bool                                `json:"setupComplete"`
	Repository                   string                              `json:"repository"`
	Reference                    string                              `json:"reference"`
	ContextPath                  string                              `json:"contextPath"`
	RegistryName                 string                              `json:"registryName"`
	RegistryEndpoint             string                              `json:"registryEndpoint"`
	RuntimeServerIDs             []uuid.UUID                         `json:"runtimeServerIds"`
	RuntimeTargetIDs             []uuid.UUID                         `json:"runtimeTargetIds"`
	RuntimeServers               []string                            `json:"runtimeServers"`
	Deployability                models.EnvironmentDeployability     `json:"deployability"`
	Secrets                      []EnvironmentSecretActivity         `json:"secrets"`
	Variables                    []EnvironmentVariableActivity       `json:"variables"`
	Domain                       string                              `json:"domain"`
	Resources                    []EnvironmentResourceActivity       `json:"resources"`
	Builds                       []EnvironmentBuildActivity          `json:"builds"`
	Releases                     []EnvironmentReleaseActivity        `json:"releases"`
	Deployments                  []EnvironmentDeploymentActivity     `json:"deployments"`
	Instances                    []EnvironmentInstanceActivity       `json:"instances"`
	Processes                    []models.EnvironmentProcessState    `json:"processes"`
	ReleaseCommands              []EnvironmentReleaseCommandActivity `json:"releaseCommands"`
	APITokenPrefix               string                              `json:"apiTokenPrefix"`
	DNS                          EnvironmentDNSStatus                `json:"dns"`
	CanPromoteToProduction       bool                                `json:"canPromoteToProduction"`
	PromotionTargetName          string                              `json:"promotionTargetName"`
	LatestSuccessfulDeploymentID *uuid.UUID                          `json:"latestSuccessfulDeploymentId,omitempty"`
	LatestSuccessfulReleaseID    *uuid.UUID                          `json:"latestSuccessfulReleaseId,omitempty"`
}

type EnvironmentResourceActivity = models.EnvironmentResourceActivity

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

type EnvironmentBuildActivity = models.EnvironmentBuildActivity

type EnvironmentBuildLogSnapshot struct {
	Build        EnvironmentBuildActivity `json:"build"`
	Logs         []models.BuildLogEntity  `json:"logs"`
	NextSequence int64                    `json:"nextSequence"`
	HasMore      bool                     `json:"hasMore"`
}

type EnvironmentReleaseActivity = models.EnvironmentReleaseActivity

type EnvironmentDeploymentActivity = models.EnvironmentDeploymentActivity

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

type EnvironmentInstanceActivity = models.EnvironmentInstanceActivity

type EnvironmentReleaseCommandActivity = models.EnvironmentReleaseCommandActivity

func (service *EnvironmentSetup) Overview(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (EnvironmentOverview, error) {
	environment, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	overviewRows, err := models.Environment.OverviewCatalog(
		ctx, service.db.Executor(), applicationID, environmentID, setupComplete,
	)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	source := overviewRows.Source
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil && setupComplete {
		return EnvironmentOverview{}, err
	}
	secretActivity := make([]EnvironmentSecretActivity, 0)
	variables := make([]EnvironmentVariableActivity, 0)
	processes := make([]models.EnvironmentProcessState, 0)
	if setupComplete {
		secretActivity, err = service.environmentSecretActivity(ctx, environmentID)
		if err != nil {
			return EnvironmentOverview{}, err
		}
		revision, revisionErr := models.EnvironmentStateRevision.LatestCommitted(
			ctx,
			service.db.Executor(),
			environmentID,
		)
		if revisionErr != nil {
			return EnvironmentOverview{}, revisionErr
		}
		state, stateErr := models.ParseEnvironmentDesiredState(revision.State)
		if stateErr != nil {
			return EnvironmentOverview{}, stateErr
		}
		processes = state.Processes
		for _, resource := range state.Resources {
			for key, value := range resource.Variables {
				variables = append(variables, EnvironmentVariableActivity{
					Key:      key,
					Value:    value,
					Source:   resource.Alias,
					SourceID: resource.EnvironmentResourceID.String(),
				})
			}
		}
		sort.Slice(
			variables,
			func(left, right int) bool { return variables[left].Key < variables[right].Key },
		)
	}
	domain := overviewRows.Domain
	dnsStatus, err := service.dns.Status(ctx, environmentID)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	runtimeServers := overviewRows.RuntimeServers
	runtimeServerIDs := make([]uuid.UUID, 0, len(runtimeServers))
	runtimeTargetIDs := make([]uuid.UUID, 0, len(runtimeServers))
	runtimeServerNames := make([]string, 0, len(runtimeServers))
	for _, server := range runtimeServers {
		runtimeServerIDs = append(runtimeServerIDs, server.ID)
		runtimeTargetIDs = append(runtimeTargetIDs, server.TargetID)
		runtimeServerNames = append(runtimeServerNames, server.Name)
	}
	resources := overviewRows.Resources
	builds := overviewRows.Builds
	releases := overviewRows.Releases
	deployments := overviewRows.Deployments
	instances := overviewRows.Instances
	releaseCommands := overviewRows.ReleaseCommands
	canPromote, promotionTargetName, latestSuccessfulDeploymentID, latestSuccessfulReleaseID, err := promotionOverview(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
		environment.Kind,
		setupComplete,
	)
	if err != nil {
		return EnvironmentOverview{}, err
	}
	return EnvironmentOverview{
		ApplicationID:    applicationID,
		ApplicationName:  source.ApplicationName,
		Environment:      environment,
		SetupComplete:    setupComplete,
		SourceType:       source.SourceType,
		Repository:       source.Repository,
		Reference:        source.Reference,
		ContextPath:      source.ContextPath,
		RegistryName:     source.RegistryName,
		RegistryEndpoint: source.RegistryEndpoint,
		RuntimeServerIDs: runtimeServerIDs,
		RuntimeTargetIDs: runtimeTargetIDs,
		RuntimeServers:   runtimeServerNames,
		Deployability:    deployability,
		Secrets:          secretActivity,
		Variables:        variables,
		Domain:           domain,
		Resources:        resources,
		Builds:           builds,
		Releases:         releases,
		Deployments:      deployments,
		Instances:        instances,
		Processes:        processes,
		ReleaseCommands:  releaseCommands,
		APITokenPrefix:   environment.APITokenPrefix.String,
		DNS:              dnsStatus,

		CanPromoteToProduction:       canPromote,
		PromotionTargetName:          promotionTargetName,
		LatestSuccessfulDeploymentID: latestSuccessfulDeploymentID,
		LatestSuccessfulReleaseID:    latestSuccessfulReleaseID,
	}, nil
}

type EnvironmentServingContainer struct {
	InstanceID   uuid.UUID `json:"instanceId"`
	DeploymentID uuid.UUID `json:"deploymentId"`
	TargetID     uuid.UUID `json:"targetId"`
	ServerID     uuid.UUID `json:"serverId"`
	Exists       bool      `json:"exists"`
	Running      bool      `json:"running"`
}

func (service *EnvironmentSetup) ServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (EnvironmentServingContainer, error) {
	container := EnvironmentServingContainer{}
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return container, err
	}
	instance, err := models.Instance.ServingForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return container, nil
	}
	if err != nil {
		return container, err
	}
	target, err := models.EnvironmentTarget.Find(
		ctx,
		service.db.Executor(),
		instance.EnvironmentTargetID,
	)
	if err != nil {
		return container, err
	}
	container.InstanceID = instance.ID
	container.DeploymentID = instance.DeploymentID
	container.TargetID = instance.EnvironmentTargetID
	container.ServerID = target.ServerID
	state, err := service.workloads.Find(
		ctx,
		target.ServerID,
		instance.DeploymentID,
		instance.ID,
	)
	if err != nil {
		return container, err
	}
	container.Exists = state.Exists
	container.Running = state.Exists && state.Running
	return container, nil
}

func (service *EnvironmentSetup) RestartServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	container, err := service.ServingContainer(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	if container.InstanceID == uuid.Nil || !container.Exists {
		return errors.New("no serving container is available to restart")
	}
	_, err = service.workloads.Restart(
		ctx,
		container.ServerID,
		container.DeploymentID,
		container.InstanceID,
	)
	return err
}

func (service *EnvironmentSetup) StartServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	container, err := service.ServingContainer(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	if container.InstanceID == uuid.Nil || !container.Exists {
		return errors.New("no serving container is available to start")
	}
	if container.Running {
		return nil
	}
	_, err = service.workloads.Start(
		ctx,
		container.ServerID,
		container.DeploymentID,
		container.InstanceID,
	)
	return err
}

func (service *EnvironmentSetup) environmentSecretActivity(
	ctx context.Context,
	environmentID uuid.UUID,
) ([]EnvironmentSecretActivity, error) {
	targetStates, err := models.EnvironmentTargetState.ActiveForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
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
		applying, applyingErr := service.environmentRevisionSecrets(
			ctx,
			targetState.ApplyingRevisionID,
		)
		if applyingErr != nil {
			return nil, applyingErr
		}
		applied, appliedErr := service.environmentRevisionSecrets(
			ctx,
			targetState.AppliedRevisionID,
		)
		if appliedErr != nil {
			return nil, appliedErr
		}
		applyingByTarget = append(applyingByTarget, applying)
		appliedByTarget = append(appliedByTarget, applied)
		maps.Copy(appliedUnion, applied)
		failedRevisionID, failedErr := service.environmentLatestFailedRevisionID(
			ctx,
			targetState.EnvironmentTargetID,
		)
		if failedErr != nil {
			return nil, failedErr
		}
		if targetState.State == "failed" && targetState.DesiredRevisionID != nil &&
			failedRevisionID != nil &&
			*targetState.DesiredRevisionID == *failedRevisionID {
			desiredRevisionFailed = true
		}
	}

	activity := make([]EnvironmentSecretActivity, 0, len(desired)+len(appliedUnion))
	for _, descriptor := range desired {
		secret, findErr := models.EnvironmentSecret.FindForEnvironment(
			ctx,
			service.db.Executor(),
			environmentID,
			descriptor.ID,
		)
		if findErr != nil {
			return nil, findErr
		}
		metadata := secret.Sanitized()
		status := "pending"
		deployedEverywhere := true
		deploying := false
		for index := range targetStates {
			if !sameEnvironmentSecretDescriptor(
				descriptor,
				appliedByTarget[index][descriptor.Key],
			) {
				deployedEverywhere = false
			}
			if sameEnvironmentSecretDescriptor(
				descriptor,
				applyingByTarget[index][descriptor.Key],
			) {
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
			ID:           metadata.ID,
			Key:          metadata.Key,
			DigestPrefix: metadata.DigestPrefix,
			SourceType:   metadata.SourceType,
			SourceID:     metadata.SourceID,
			CreatedAt:    metadata.CreatedAt,
			Status:       status,
			Desired:      true,
		})
	}
	for key, descriptor := range appliedUnion {
		if _, stillDesired := desired[key]; stillDesired {
			continue
		}
		secret, findErr := models.EnvironmentSecret.FindForEnvironment(
			ctx,
			service.db.Executor(),
			environmentID,
			descriptor.ID,
		)
		if findErr != nil {
			return nil, findErr
		}
		metadata := secret.Sanitized()
		activity = append(activity, EnvironmentSecretActivity{
			ID:           metadata.ID,
			Key:          metadata.Key,
			DigestPrefix: metadata.DigestPrefix,
			SourceType:   metadata.SourceType,
			SourceID:     metadata.SourceID,
			CreatedAt:    metadata.CreatedAt,
			Status:       "pending_removal",
			Desired:      false,
		})
	}
	sort.Slice(
		activity,
		func(left, right int) bool { return activity[left].Key < activity[right].Key },
	)
	return activity, nil
}

func (service *EnvironmentSetup) environmentLatestFailedRevisionID(
	ctx context.Context,
	targetID uuid.UUID,
) (*uuid.UUID, error) {
	return models.Deployment.LatestFailedRevisionID(ctx, service.db.Executor(), targetID)
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
	return right.ID != uuid.Nil && left.Key == right.Key && left.Digest == right.Digest
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
	revision, err := models.EnvironmentStateRevision.LatestCommitted(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	rows, err := models.EnvironmentResource.EditRows(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return EnvironmentEditData{}, err
	}
	resources := make([]EnvironmentSetupResourceInput, 0, len(rows))
	for _, row := range rows {
		var configuration struct {
			CredentialProjection string `json:"credential_projection"`
		}
		if json.Unmarshal(row.Configuration, &configuration) != nil {
			return EnvironmentEditData{}, errors.New(
				"environment Resource configuration is invalid",
			)
		}
		database := resourceCredentialMetadataDatabase(row.CredentialMetadata)
		resources = append(resources, EnvironmentSetupResourceInput{
			ResourceID: row.ResourceID, EndpointID: row.ResourceEndpointID,
			CredentialID: row.ResourceCredentialID, Alias: row.Alias, Database: database,
			CredentialProjection: configuration.CredentialProjection,
		})
	}
	processes := processInputsFromState(state.Processes)
	processTargets := make(map[string]string, len(state.Runtime.BPGOTargets))
	for _, target := range state.Runtime.BPGOTargets {
		processTargets[target.Process] = target.Target
	}
	for index := range processes {
		if target, exists := processTargets[processes[index].Name]; exists {
			processes[index].Target = &target
		}
	}
	web, _ := state.WebProcess()
	return EnvironmentEditData{
		Overview: overview,
		Configuration: EnvironmentEditConfiguration{
			Runtime:       models.BuildpackRuntime(state.Runtime.Runtime),
			Name:          overview.Environment.Name,
			Slug:          overview.Environment.Slug,
			Kind:          overview.Environment.Kind,
			Hostname:      state.Domain.Hostname,
			ContainerPort: web.ContainerPort,
			HealthPath:    web.HealthPath,
			Processes:     processes,
			Resources:     resources,
			ServerIDs:     overview.RuntimeServerIDs,
			ServerNames:   overview.RuntimeServers,
			DNSMode:       overview.DNS.Mode,
			DNSZoneID:     overview.DNS.ZoneID,
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
			ID:               build.ID,
			SourceRevision:   build.SourceRevision,
			Status:           build.Status,
			CurrentStep:      build.CurrentStep.String,
			Error:            build.Error.String,
			CreatedAt:        build.CreatedAt,
			StartedAt:        startedAt,
			FinishedAt:       finishedAt,
			RegistryEndpoint: registrySnapshot.RegistryEndpoint,
			JobID:            jobID,
			JobState:         jobState,
		},
		Logs: logs, NextSequence: nextSequence, HasMore: hasMore,
	}, nil
}

func (service *EnvironmentSetup) DeploymentEvents(
	ctx context.Context,
	environmentID, deploymentID uuid.UUID,
	after int64,
) (EnvironmentDeploymentEventSnapshot, error) {
	deployment, err := models.Deployment.EnvironmentActivity(
		ctx,
		service.db.Executor(),
		environmentID,
		deploymentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvironmentDeploymentEventSnapshot{}, sql.ErrNoRows
	}
	if err != nil {
		return EnvironmentDeploymentEventSnapshot{}, err
	}
	eventEntities, err := models.DeploymentEvent.ForDeploymentAfter(
		ctx,
		service.db.Executor(),
		deploymentID,
		after,
		501,
	)
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
	attachable, err := models.Resource.AllAttachable(ctx, service.db.Executor())
	if err != nil {
		return EnvironmentSetupOptions{}, err
	}

	options := make([]EnvironmentSetupResourceOption, 0, len(attachable))
	credentialless := make(map[string]struct{})
	for _, resource := range attachable {
		definition, supported := models.FindResourceEngine(resource.Engine)
		if !supported || resource.Engine == "postgresql" && resource.CredentialID == nil {
			continue
		}
		option := EnvironmentSetupResourceOption{
			ID:                    resource.ID,
			Name:                  resource.Name,
			Engine:                resource.Engine,
			Database:              resource.Database,
			EndpointID:            resource.EndpointID,
			Endpoint:              resource.Endpoint,
			CredentialID:          resource.CredentialID,
			Credential:            resource.Credential,
			ServerID:              resource.ServerID,
			ResourceConfiguration: resource.Configuration,
			EnvironmentKeys: (models.ResourceEntity{
				Configuration: resource.Configuration,
			}).EnvironmentKeys(),
			SupportsConnectionURL: resource.Engine == "postgresql",
		}
		option.CredentialFields = make([]string, 0, len(definition.CredentialFields))
		for _, field := range definition.CredentialFields {
			option.CredentialFields = append(option.CredentialFields, field.Name)
		}
		key := option.ID.String() + ":" + option.EndpointID.String()
		if option.Engine != "postgresql" {
			if _, exists := credentialless[key]; !exists {
				withoutCredential := option
				withoutCredential.Database = ""
				withoutCredential.CredentialID = nil
				withoutCredential.Credential = ""
				withoutCredential.CredentialFields = nil
				options = append(options, withoutCredential)
				credentialless[key] = struct{}{}
			}
			if option.CredentialID == nil {
				continue
			}
		}
		options = append(options, option)
	}

	servers, err := models.Server.EnvironmentSetupOptions(ctx, service.db.Executor())
	if err != nil {
		return EnvironmentSetupOptions{}, err
	}

	dnsZones, err := service.dns.Options(ctx)
	if err != nil {
		return EnvironmentSetupOptions{}, err
	}

	return EnvironmentSetupOptions{Resources: options, Servers: servers, DNSZones: dnsZones}, nil
}

type environmentSetupSource = models.EnvironmentSetupSource

type preparedSetupResource struct {
	input                   EnvironmentSetupResourceInput
	connectionID            uuid.UUID
	resource                models.ResourceEntity
	endpoint                models.ResourceEndpointEntity
	credential              *models.ResourceCredentialEntity
	credentialInput         *ResourceCredentialInput
	environmentKeys         map[string]string
	environmentKeyOverrides map[string]string
	secrets                 []PreparedEnvironmentSecret
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
	source, repository, installation, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if source.EnvironmentArchivedAt.Valid || source.SetupComplete {
		return EnvironmentSetupResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment is unavailable or setup is already complete"),
		)
	}
	runtime := models.BuildpackRuntimeGo
	if source.Kind != "image" {
		settings, settingsErr := models.ParseBuildpackSettings(source.BuildpackSettings)
		if settingsErr != nil {
			return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, settingsErr)
		}
		runtime = settings.Runtime
	}
	processes := normalizedProcessFormation(input.Processes, input.ContainerPort, input.HealthPath)
	if runtime == models.BuildpackRuntimeGo {
		processes = deriveGoProcessCommands(processes)
	}
	input.Processes = processes
	goTargets := goProcessTargetsFromProcesses(processes)
	if err := validateEnvironmentSetupInput(input, runtime); err != nil {
		return EnvironmentSetupResult{}, err
	}
	managedDNS := strings.EqualFold(strings.TrimSpace(input.DNS.Mode), DNSModeCloudflare)
	deployNow := input.Deploy && !managedDNS
	revisionSHA := ""
	var imageArtifact resolvedImageArtifact
	if deployNow {
		if source.Kind == "image" {
			imageArtifact, err = service.resolveImageArtifact(ctx, source, "")
			if err != nil {
				return EnvironmentSetupResult{}, fmt.Errorf(
					"resolve configured image reference: %w",
					err,
				)
			}
		} else {
			revisionSHA, err = service.github.ResolveRevision(
				ctx,
				installation,
				repository,
				source.Reference,
			)
			if err != nil {
				return EnvironmentSetupResult{}, fmt.Errorf(
					"resolve configured GitHub reference: %w",
					err,
				)
			}
		}
	}
	serverIDs := normalizedEnvironmentServerIDs(input)
	placements, networkID, err := service.runtimePlacements(ctx, serverIDs)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	preparedResources, err := service.prepareResources(
		ctx,
		environmentID,
		serverIDs,
		networkID,
		input.Resources,
	)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	preparedUserSecrets := make([]PreparedEnvironmentSecret, 0, len(input.Secrets))
	keys := map[string]struct{}{"PORT": {}}
	for _, resource := range preparedResources {
		for _, secret := range resource.secrets {
			if _, exists := keys[secret.Key]; exists {
				return EnvironmentSetupResult{}, errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   "resources",
							Code:    "duplicate",
							Message: "Resource-managed Environment secret keys must be unique",
						},
					},
				)
			}
			keys[secret.Key] = struct{}{}
		}
	}
	for index, secret := range input.Secrets {
		key := models.NormalizeEnvironmentSecretKey(secret.Key)
		if _, exists := keys[key]; exists {
			return EnvironmentSetupResult{}, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("secrets.%d.key", index),
						Code:    "reserved",
						Message: "secret key conflicts with a platform or Resource-owned key",
					},
				},
			)
		}
		prepared, prepareErr := service.secrets.Prepare(
			environmentID,
			key,
			secret.Value,
			models.EnvironmentSecretSourceUser,
			userID,
		)
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
	if err := lockPreparedSetupResources(ctx, tx, preparedResources); err != nil {
		return EnvironmentSetupResult{}, err
	}
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if environment.ApplicationID != applicationID || setupComplete || environment.ArchivedAt.Valid {
		return EnvironmentSetupResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment is unavailable or setup is already complete"),
		)
	}
	runtimeSettings, _ := json.Marshal(map[string]any{
		"schema_version": 4, "bp_go_targets": goTargets,
	})
	if _, err := models.RuntimeConfiguration.Create(ctx, tx, models.CreateRuntimeConfigurationData{
		Runtime:        string(runtime),
		ResourceLimits: json.RawMessage(`{}`),
		RestartPolicy:  "unless-stopped",
		Settings:       runtimeSettings,
		EnvironmentID:  environment.ID,
	}); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.EnvironmentProcess.ReplaceActive(
		ctx,
		tx,
		environment.ID,
		processes,
	); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.EnvironmentNetwork.Create(
		ctx,
		tx,
		models.CreateEnvironmentNetworkData{
			Role:             "primary",
			EnvironmentID:    environment.ID,
			PrivateNetworkID: networkID,
		},
	); err != nil {
		return EnvironmentSetupResult{}, err
	}
	targets := make([]models.EnvironmentTargetEntity, 0, len(placements))
	for _, placement := range placements {
		target, createErr := models.EnvironmentTarget.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetData{
				AttachedAt:    time.Now().UTC(),
				EnvironmentID: environment.ID,
				ServerID:      placement.serverID,
			},
		)
		if createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		if _, createErr := models.EnvironmentTargetNetwork.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetNetworkData{
				Driver:              placement.network.Driver,
				ExternalID:          placement.network.ExternalID,
				Configuration:       placement.network.Configuration,
				State:               "applied",
				AppliedAt:           placement.network.AppliedAt,
				ObservedAt:          placement.network.ObservedAt,
				EnvironmentTargetID: target.ID,
				PrivateNetworkID:    networkID,
			},
		); createErr != nil {
			return EnvironmentSetupResult{}, createErr
		}
		targets = append(targets, target)
	}
	domain, err := models.EnvironmentDomain.Create(
		ctx,
		tx,
		models.CreateEnvironmentDomainData{
			Hostname:      input.Hostname,
			IsPrimary:     true,
			EnvironmentID: environment.ID,
		},
	)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	dnsResult, err := service.dns.ConfigureTx(
		ctx,
		tx,
		domain,
		input.DNS,
		true,
		input.Deploy,
		input.Deploy,
		&userID,
	)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	resourceStates := make([]models.EnvironmentResourceState, 0, len(preparedResources))
	secretEntities := make(
		[]models.EnvironmentSecretEntity,
		0,
		len(preparedUserSecrets)+len(preparedResources),
	)
	for _, prepared := range preparedResources {
		if prepared.credentialInput != nil {
			createdCredential, createCredentialErr := service.resources.createCredential(
				ctx,
				tx,
				prepared.resource,
				*prepared.credentialInput,
			)
			if createCredentialErr != nil {
				return EnvironmentSetupResult{}, createCredentialErr
			}
			prepared.credential = &createdCredential
			prepared.input.CredentialID = &createdCredential.ID
		}
		configuration, configurationErr := encodeEnvironmentResourceConfiguration(
			prepared.input.CredentialProjection, prepared.input.CredentialID,
			preparedCredentialSource(prepared),
			prepared.environmentKeys, prepared.environmentKeyOverrides,
		)
		if configurationErr != nil {
			return EnvironmentSetupResult{}, configurationErr
		}
		connection, createErr := models.EnvironmentResource.Create(
			ctx,
			tx,
			models.CreateEnvironmentResourceData{
				ID:                   prepared.connectionID,
				Alias:                prepared.input.Alias,
				Configuration:        configuration,
				EnvironmentID:        environment.ID,
				ResourceID:           prepared.resource.ID,
				ResourceEndpointID:   prepared.endpoint.ID,
				ResourceCredentialID: prepared.input.CredentialID,
			},
		)
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
			EnvironmentResourceID: connection.ID,
			ResourceID:            prepared.resource.ID,
			Kind:                  prepared.resource.Engine(),
			EndpointID:            prepared.endpoint.ID,
			CredentialID:          prepared.input.CredentialID,
			Alias: strings.ToUpper(
				prepared.input.Alias,
			),
			Database:        prepared.input.Database,
			EnvironmentKeys: prepared.environmentKeys,
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
		Sequence:      1,
		Kind:          "environment_setup",
		TriggerType:   "user",
		ActorType:     "user",
		ActorID:       &userID,
		CorrelationID: uuid.New(),
		Summary:       "Complete Environment setup",
		Status:        "committed",
		RequestedAt:   now,
		CommittedAt:   sql.NullTime{Time: now, Valid: true},
		EnvironmentID: environment.ID,
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
		Runtime: models.EnvironmentRuntimeState{
			Runtime:       string(runtime),
			BPGOTargets:   goTargets,
			RestartPolicy: "unless-stopped",
		},
		Processes: processStatesFromInputs(processes),
		Domain: models.EnvironmentDomainState{
			ID:       domain.ID,
			Hostname: domain.Hostname,
			Primary:  true,
		},
		Resources: resourceStates,
		Secrets:   descriptors,
	}
	canonicalState, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return EnvironmentSetupResult{}, errors.Join(models.ErrDomainValidation, err)
	}
	revision, err := models.EnvironmentStateRevision.Create(
		ctx,
		tx,
		models.CreateEnvironmentStateRevisionData{
			State:         canonicalState,
			EnvironmentID: environment.ID,
			ChangeID:      change.ID,
		},
	)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return EnvironmentSetupResult{}, err
	}
	for _, target := range targets {
		if _, err := models.EnvironmentTargetState.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetStateData{
				ObservedState:       json.RawMessage(`{}`),
				State:               "pending",
				EnvironmentTargetID: target.ID,
				DesiredRevisionID:   &revision.ID,
			},
		); err != nil {
			return EnvironmentSetupResult{}, err
		}
	}
	if !deployNow {
		if err := models.Change.MarkCompleted(ctx, tx, change.ID, now); err != nil {
			return EnvironmentSetupResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return EnvironmentSetupResult{}, err
		}
		return EnvironmentSetupResult{
			Environment:        environment,
			Revision:           revision,
			DeploymentDeferred: dnsResult.DeploymentDeferred,
		}, nil
	}
	if source.Kind == "image" {
		if err := models.Change.MarkCompleted(ctx, tx, change.ID, now); err != nil {
			return EnvironmentSetupResult{}, err
		}
		release, deployment, err := service.queueImageDeploymentTx(
			ctx,
			tx,
			source,
			revision,
			imageArtifact,
			"system",
			nil,
			"Deploy configured image",
		)
		if err != nil {
			return EnvironmentSetupResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return EnvironmentSetupResult{}, err
		}
		return EnvironmentSetupResult{
			Environment: environment,
			Revision:    revision,
			Release:     release,
			Deployment:  deployment,
		}, nil
	}
	correlationID := uuid.New()
	eventPayload, _ := json.Marshal(
		map[string]any{
			"schema_version":                1,
			"reference":                     source.Reference,
			"revision":                      revisionSHA,
			"repository":                    source.Repository,
			"environment_state_revision_id": revision.ID,
		},
	)
	event, err := models.SourceEvent.Create(ctx, tx, models.CreateSourceEventData{
		ExternalID:          "manual:" + correlationID.String(),
		Kind:                "manual_deploy",
		SourceRevision:      sql.NullString{String: revisionSHA, Valid: true},
		Payload:             eventPayload,
		ReceivedAt:          now,
		ProcessedAt:         sql.NullTime{Time: now, Valid: true},
		EnvironmentSourceID: source.EnvironmentSourceID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
		SchemaVersion:              2,
		SourceEventID:              event.ID,
		EnvironmentStateRevisionID: revision.ID,
		Repository:                 source.Repository,
		Reference:                  source.Reference,
		SourceRevision:             revisionSHA,
		ContextPath:                source.ContextPath,
		BuilderReference:           nullableStringPointer(source.BuilderReference),
		ImageRepository:            source.ImageRepository,
		RegistryResourceID:         source.RegistryID,
		RegistryCredentialID:       source.RegistryCredentialID,
		RegistryEndpoint:           source.RegistryEndpoint,
		Settings:                   source.BuildpackSettings,
		BPGOTargets:                models.FlattenGoProcessTargets(goTargets),
		ServerID:                   source.BuildServerID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, fmt.Errorf("create Build configuration snapshot: %w", err)
	}
	build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
		SourceRevision:      revisionSHA,
		BuildMethod:         "buildpacks",
		BuildConfiguration:  buildConfiguration,
		Status:              "pending",
		CurrentStep:         sql.NullString{String: "queued", Valid: true},
		EnvironmentID:       environment.ID,
		EnvironmentSourceID: source.EnvironmentSourceID,
		ChangeID:            change.ID,
	})
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BuildSourceArgs{BuildID: build.ID},
		jobs.BuildSourceInsertOpts(build.ID),
	); err != nil {
		return EnvironmentSetupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSetupResult{}, err
	}
	return EnvironmentSetupResult{Environment: environment, Revision: revision, Build: build}, nil
}

type SourceDeploymentResult struct {
	Build              *models.BuildEntity      `json:"build,omitempty"`
	Release            *models.ReleaseEntity    `json:"release,omitempty"`
	Deployment         *models.DeploymentEntity `json:"deployment,omitempty"`
	DeploymentDeferred bool                     `json:"deploymentDeferred"`
}

type ReleaseDeploymentResult struct {
	Deployment         models.DeploymentEntity
	DeploymentDeferred bool
}

type PromotionResult struct {
	SourceDeployment models.DeploymentEntity
	SourceRelease    models.ReleaseEntity
	Release          models.ReleaseEntity
	Deployment       models.DeploymentEntity
	Deferred         bool
}

type resolvedImageArtifact struct {
	Version              string
	Reference            string
	Digest               []byte
	RegistryResourceID   uuid.UUID
	RegistryCredentialID uuid.UUID
	RegistryEndpoint     string
}

func (service *EnvironmentSetup) RotateAPIToken(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (string, error) {
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
		ID:   environment.ID,
		Name: environment.Name,
		Slug: environment.Slug,
		Kind: environment.Kind,
		APITokenPrefix: sql.NullString{
			String: token[:prefixLength],
			Valid:  true,
		},
		APITokenDigest: digest,
		ArchivedAt:     environment.ArchivedAt,
		ApplicationID:  environment.ApplicationID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (service *EnvironmentSetup) AuthenticateAPIToken(
	ctx context.Context,
	environmentID uuid.UUID,
	token string,
) (models.EnvironmentEntity, error) {
	token = strings.TrimSpace(token)
	environment, err := models.Environment.Find(ctx, service.db.Executor(), environmentID)
	if err != nil || environment.ArchivedAt.Valid || !environment.APITokenPrefix.Valid ||
		len(environment.APITokenDigest) == 0 {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	digest := []byte(models.HashForStorage(token, service.config.App.SessionEncryptionKey))
	if !hmac.Equal(environment.APITokenDigest, digest) {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	return environment, nil
}

func (service *EnvironmentSetup) RequestSourceDeployment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (SourceDeploymentResult, error) {
	source, _, _, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if source.Kind != "image" && strings.TrimSpace(reference) != "" {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Buildpacks deployments do not accept an image reference override"),
		)
	}
	if source.Kind != "image" && actorID == nil {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Buildpacks API deployments are not supported"),
		)
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	for _, missing := range deployability.Missing {
		if missing != "managed_dns" {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf("Environment is not deployable: %s", strings.Join(blocking, ", ")),
		)
	}
	deferred, err := service.dns.PrepareDeployment(
		ctx,
		environmentID,
		actorID,
		triggerType,
		reference,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if deferred {
		return SourceDeploymentResult{DeploymentDeferred: true}, nil
	}
	return service.QueueSourceDeployment(
		ctx,
		applicationID,
		environmentID,
		actorID,
		triggerType,
		reference,
	)
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
			return SourceDeploymentResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("Buildpacks deployments do not accept an image reference override"),
			)
		}
		if actorID == nil {
			return SourceDeploymentResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("Buildpacks API deployments are not supported"),
			)
		}
		build, err := service.QueueManualDeploy(ctx, applicationID, environmentID, *actorID)
		if err != nil {
			return SourceDeploymentResult{}, err
		}
		return SourceDeploymentResult{Build: &build}, nil
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if !deployability.Deployable {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(deployability.Missing, ", "),
			),
		)
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
	release, deployment, err := service.queueImageDeploymentTx(
		ctx,
		tx,
		source,
		revision,
		artifact,
		triggerType,
		actorID,
		"Deploy image "+artifact.Version,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SourceDeploymentResult{}, err
	}
	return SourceDeploymentResult{Release: &release, Deployment: &deployment}, nil
}

func (service *EnvironmentSetup) resolveImageArtifact(
	ctx context.Context,
	source environmentSetupSource,
	override string,
) (resolvedImageArtifact, error) {
	version := strings.TrimSpace(override)
	if version == "" {
		version = strings.TrimSpace(source.Reference)
	}
	if version == "" || strings.ContainsAny(version, " /\t\r\n") {
		return resolvedImageArtifact{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("image tag or digest is invalid"),
		)
	}
	credentials, err := service.builds.RegistryCredentials(
		ctx,
		source.RegistryID,
		source.RegistryCredentialID,
		source.RegistryEndpoint,
	)
	if err != nil {
		return resolvedImageArtifact{}, fmt.Errorf("load image registry credentials: %w", err)
	}
	separator := ":"
	if strings.HasPrefix(strings.ToLower(version), "sha256:") {
		separator = "@"
	}
	mutableReference := strings.TrimSuffix(
		source.RegistryEndpoint,
		"/",
	) + "/" + strings.Trim(
		source.ImageRepository,
		"/",
	) + separator + version
	immutableReference, err := service.registry.ResolveRemoteDigest(
		ctx,
		credentials,
		mutableReference,
	)
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
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment has no runtime Server targets"),
		)
	}
	active, err := models.Deployment.ActiveCountForEnvironment(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if active > 0 {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment already has an active Deployment"),
		)
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
		Sequence:    sequence,
		Kind:        "deploy",
		TriggerType: triggerType,
		ActorType:   actorType,
		ActorID:     actorID,
		CauseSystem: sql.NullString{
			String: "registry_image",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: artifact.Reference, Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           summary,
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     source.EnvironmentID,
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version: sql.NullString{
			String: artifact.Version,
			Valid:  true,
		},
		ArtifactReference:    artifact.Reference,
		ArtifactDigest:       artifact.Digest,
		EnvironmentID:        source.EnvironmentID,
		EnvironmentSourceID:  &source.EnvironmentSourceID,
		CreatedByChangeID:    change.ID,
		RegistryResourceID:   &artifact.RegistryResourceID,
		RegistryCredentialID: &artifact.RegistryCredentialID,
		RegistryEndpoint:     sql.NullString{String: artifact.RegistryEndpoint, Valid: true},
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	return release, result.Deployment, nil
}

func (service *EnvironmentSetup) QueueManualDeploy(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
) (models.BuildEntity, error) {
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	if !deployability.Deployable {
		return models.BuildEntity{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(deployability.Missing, ", "),
			),
		)
	}
	source, repository, installation, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	revisionSHA, err := service.github.ResolveRevision(
		ctx,
		installation,
		repository,
		source.Reference,
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	stateRevision, err := models.EnvironmentStateRevision.LatestCommitted(
		ctx,
		service.db.Executor(),
		environmentID,
	)
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
	payload, _ := json.Marshal(
		map[string]any{
			"schema_version":                1,
			"reference":                     source.Reference,
			"revision":                      revisionSHA,
			"repository":                    source.Repository,
			"environment_state_revision_id": stateRevision.ID,
		},
	)
	event, err := models.SourceEvent.Create(
		ctx,
		tx,
		models.CreateSourceEventData{
			ExternalID:          "manual:" + correlationID.String(),
			Kind:                "manual_deploy",
			SourceRevision:      sql.NullString{String: revisionSHA, Valid: true},
			Payload:             payload,
			ReceivedAt:          now,
			ProcessedAt:         sql.NullTime{Time: now, Valid: true},
			EnvironmentSourceID: source.EnvironmentSourceID,
		},
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	change, err := models.Change.Create(
		ctx,
		tx,
		models.CreateChangeData{
			Sequence:      sequence,
			Kind:          "build",
			TriggerType:   "user",
			ActorType:     "user",
			ActorID:       &userID,
			CorrelationID: correlationID,
			Summary:       "Deploy current GitHub revision",
			Status:        "committed",
			RequestedAt:   now,
			CommittedAt:   sql.NullTime{Time: now, Valid: true},
			EnvironmentID: environmentID,
		},
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
		SchemaVersion:              2,
		SourceEventID:              event.ID,
		EnvironmentStateRevisionID: stateRevision.ID,
		Repository:                 source.Repository,
		Reference:                  source.Reference,
		SourceRevision:             revisionSHA,
		ContextPath:                source.ContextPath,
		BuilderReference:           nullableStringPointer(source.BuilderReference),
		ImageRepository:            source.ImageRepository,
		RegistryResourceID:         source.RegistryID,
		RegistryCredentialID:       source.RegistryCredentialID,
		RegistryEndpoint:           source.RegistryEndpoint,
		Settings:                   source.BuildpackSettings,
		BPGOTargets:                models.FlattenGoProcessTargets(state.Runtime.BPGOTargets),
		ServerID:                   source.BuildServerID,
	})
	if err != nil {
		return models.BuildEntity{}, fmt.Errorf("create Build configuration snapshot: %w", err)
	}
	build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
		SourceRevision:      revisionSHA,
		BuildMethod:         "buildpacks",
		BuildConfiguration:  buildConfiguration,
		Status:              "pending",
		CurrentStep:         sql.NullString{String: "queued", Valid: true},
		EnvironmentID:       environmentID,
		EnvironmentSourceID: source.EnvironmentSourceID,
		ChangeID:            change.ID,
	})
	if err != nil {
		return models.BuildEntity{}, err
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BuildSourceArgs{BuildID: build.ID},
		jobs.BuildSourceInsertOpts(build.ID),
	); err != nil {
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
) (ReleaseDeploymentResult, error) {
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	dnsPending := false
	for _, missing := range deployability.Missing {
		if missing == "managed_dns" {
			dnsPending = true
		} else {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(blocking, ", "),
			),
		)
	}
	if dnsPending {
		deferred, err := service.dns.PrepareDeployment(
			ctx,
			environmentID,
			&userID,
			ReleaseRedeployTriggerType,
			releaseID.String(),
		)
		if err != nil {
			return ReleaseDeploymentResult{}, err
		}
		if deferred {
			return ReleaseDeploymentResult{DeploymentDeferred: true}, nil
		}
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return ReleaseDeploymentResult{}, errors.New("Environment is unavailable")
	}
	release, err := models.Release.Find(ctx, tx, releaseID)
	if err != nil || release.EnvironmentID != environmentID || release.RegistryResourceID == nil ||
		release.RegistryCredentialID == nil ||
		!release.RegistryEndpoint.Valid {
		return ReleaseDeploymentResult{}, errors.New("Release does not belong to this Environment")
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if len(targets) == 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment has no runtime Server targets"),
		)
	}
	active, err := models.Deployment.ActiveCountForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if active > 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment already has an active Deployment"),
		)
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "redeploy",
		TriggerType: "user",
		ActorType:   "user",
		ActorID:     &userID,
		CauseSystem: sql.NullString{
			String: "release",
			Valid:  true,
		},
		CauseReference: sql.NullString{String: release.ID.String(), Valid: true},
		CorrelationID:  uuid.New(),
		Summary:        "Redeploy selected Release",
		Status:         "committed",
		RequestedAt:    now,
		CommittedAt:    sql.NullTime{Time: now, Valid: true},
		EnvironmentID:  environmentID,
	})
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return ReleaseDeploymentResult{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReleaseDeploymentResult{}, err
	}
	return ReleaseDeploymentResult{Deployment: result.Deployment}, nil
}

func (service *EnvironmentSetup) PromoteToProduction(
	ctx context.Context,
	applicationID, stagingEnvironmentID, userID uuid.UUID,
) (PromotionResult, error) {
	staging, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		stagingEnvironmentID,
	)
	if err != nil || staging.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Environment is unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(staging.Kind), "staging") {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Only a staging Environment can be promoted to production"),
		)
	}
	production, err := productionEnvironmentForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	sourceDeployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		service.db.Executor(),
		staging.ID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromotionResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("No successful staging deployment is available to promote"),
			)
		}
		return PromotionResult{}, err
	}
	sourceRelease, err := models.Release.Find(
		ctx,
		service.db.Executor(),
		sourceDeployment.ReleaseID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	if !promotableRelease(sourceRelease, staging.ID) {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Latest successful staging deployment has no complete immutable artifact"),
		)
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		production.ID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	dnsPending := false
	for _, missing := range deployability.Missing {
		if missing == "managed_dns" {
			dnsPending = true
		} else {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Production environment is not deployable: %s",
				strings.Join(blocking, ", "),
			),
		)
	}
	if dnsPending {
		deferred, err := service.dns.PrepareDeployment(
			ctx,
			production.ID,
			&userID,
			ReleasePromoteTriggerType,
			staging.ID.String(),
		)
		if err != nil {
			return PromotionResult{}, err
		}
		if deferred {
			return PromotionResult{Deferred: true}, nil
		}
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return PromotionResult{}, err
	}
	defer tx.Rollback()
	staging, err = models.Environment.Lock(ctx, tx, staging.ID)
	if err != nil || staging.ApplicationID != applicationID || staging.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Environment is unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(staging.Kind), "staging") {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Only a staging Environment can be promoted to production"),
		)
	}
	production, err = models.Environment.Lock(ctx, tx, production.ID)
	if err != nil || production.ApplicationID != applicationID || production.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Production environment is unavailable")
	}
	sourceDeployment, err = models.Deployment.LatestSucceededForEnvironment(
		ctx,
		tx,
		staging.ID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromotionResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("No successful staging deployment is available to promote"),
			)
		}
		return PromotionResult{}, err
	}
	sourceRelease, err = models.Release.Find(ctx, tx, sourceDeployment.ReleaseID)
	if err != nil {
		return PromotionResult{}, err
	}
	if !promotableRelease(sourceRelease, staging.ID) {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Latest successful staging deployment has no complete immutable artifact"),
		)
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	productionSourceID, err := models.EnvironmentSource.LatestActiveID(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "deploy",
		TriggerType: "user",
		ActorType:   "user",
		ActorID:     &userID,
		CauseSystem: sql.NullString{
			String: "promotion",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: sourceRelease.ID.String(), Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           "Promote staging release to production",
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     production.ID,
	})
	if err != nil {
		return PromotionResult{}, err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version:              sourceRelease.Version,
		SourceRevision:       sourceRelease.SourceRevision,
		ArtifactReference:    sourceRelease.ArtifactReference,
		ArtifactDigest:       sourceRelease.ArtifactDigest,
		EnvironmentID:        production.ID,
		EnvironmentSourceID:  productionSourceID,
		BuildID:              sourceRelease.BuildID,
		CreatedByChangeID:    change.ID,
		RegistryResourceID:   sourceRelease.RegistryResourceID,
		RegistryCredentialID: sourceRelease.RegistryCredentialID,
		RegistryEndpoint:     sourceRelease.RegistryEndpoint,
	})
	if err != nil {
		return PromotionResult{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return PromotionResult{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return PromotionResult{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return PromotionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PromotionResult{}, err
	}
	return PromotionResult{
		SourceDeployment: sourceDeployment,
		SourceRelease:    sourceRelease,
		Release:          release,
		Deployment:       result.Deployment,
	}, nil
}

func promotableRelease(release models.ReleaseEntity, environmentID uuid.UUID) bool {
	return release.EnvironmentID == environmentID &&
		strings.TrimSpace(release.ArtifactReference) != "" &&
		len(release.ArtifactDigest) > 0 &&
		release.RegistryResourceID != nil &&
		release.RegistryCredentialID != nil &&
		release.RegistryEndpoint.Valid
}

func productionEnvironmentForApplication(
	ctx context.Context,
	exec storage.Executor,
	applicationID uuid.UUID,
) (models.EnvironmentEntity, error) {
	environments, err := models.Environment.ProductionForApplication(ctx, exec, applicationID)
	if err != nil {
		return models.EnvironmentEntity{}, err
	}
	if len(environments) == 0 {
		return models.EnvironmentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("No production Environment is available to promote to"),
		)
	}
	if len(environments) > 1 {
		return models.EnvironmentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New(
				"Multiple production Environments exist; promotion target selection is not supported",
			),
		)
	}
	return environments[0], nil
}

func promotionOverview(
	ctx context.Context,
	exec storage.Executor,
	applicationID, environmentID uuid.UUID,
	kind string,
	setupComplete bool,
) (canPromote bool, targetName string, latestDeploymentID, latestReleaseID *uuid.UUID, err error) {
	if !strings.EqualFold(strings.TrimSpace(kind), "staging") || !setupComplete {
		return false, "", nil, nil, nil
	}
	production, err := productionEnvironmentForApplication(ctx, exec, applicationID)
	if err != nil {
		return false, "", nil, nil, nil
	}
	deployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		exec,
		environmentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, production.Name, nil, nil, nil
	}
	if err != nil {
		return false, "", nil, nil, err
	}
	sourceRelease, err := models.Release.Find(ctx, exec, deployment.ReleaseID)
	if err != nil {
		return false, "", nil, nil, err
	}
	productionDeployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		exec,
		production.ID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return true, production.Name, &deployment.ID, &deployment.ReleaseID, nil
	}
	if err != nil {
		return false, "", nil, nil, err
	}
	productionRelease, err := models.Release.Find(ctx, exec, productionDeployment.ReleaseID)
	if err != nil {
		return false, "", nil, nil, err
	}
	return !sameArtifact(
		sourceRelease,
		productionRelease,
	), production.Name, &deployment.ID, &deployment.ReleaseID, nil
}

func sameArtifact(a, b models.ReleaseEntity) bool {
	if len(a.ArtifactDigest) > 0 && len(b.ArtifactDigest) > 0 {
		return bytes.Equal(a.ArtifactDigest, b.ArtifactDigest)
	}
	referenceA := strings.TrimSpace(a.ArtifactReference)
	referenceB := strings.TrimSpace(b.ArtifactReference)
	return referenceA != "" && referenceA == referenceB
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
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	if build.Status != "pending" && build.Status != "running" {
		return errors.New("only a pending or retrying Build can be started")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	if build.Status == "running" && job.State != "scheduled" && job.State != "retryable" &&
		job.State != "pending" {
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
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
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
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
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

func (service *EnvironmentSetup) recordBuildAction(
	ctx context.Context,
	buildID uuid.UUID,
	message string,
) {
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
	currentRuntime, err := models.RuntimeConfiguration.FindForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return err
	}
	runtime := models.BuildpackRuntime(currentRuntime.Runtime)
	processes := normalizedProcessFormation(input.Processes, input.ContainerPort, input.HealthPath)
	if runtime == models.BuildpackRuntimeGo {
		processes = deriveGoProcessCommands(processes)
	}
	input.Processes = processes
	goTargets := goProcessTargetsFromProcesses(processes)
	if input.Name == "" || input.Slug == "" || input.Kind == "" {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment name, slug, and kind are required"),
		)
	}
	configuration := EnvironmentSetupInput{
		ServerIDs:     input.ServerIDs,
		Hostname:      input.Hostname,
		ContainerPort: input.ContainerPort,
		HealthPath:    input.HealthPath,
		Processes:     processes,
		Resources:     input.Resources,
		DNS:           input.DNS,
	}
	if err := validateEnvironmentSetupInput(configuration, runtime); err != nil {
		return err
	}
	serverIDs := normalizedEnvironmentServerIDs(configuration)
	placements, selectedNetworkID, err := service.runtimePlacements(ctx, serverIDs)
	if err != nil {
		return err
	}
	networkID, err := models.EnvironmentNetwork.ActivePrivateNetworkID(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return fmt.Errorf("load Environment network: %w", err)
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return err
	}
	if selectedNetworkID != networkID {
		return errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{{
				Field:   "serverIds",
				Code:    "network",
				Message: "selected runtime Server targets must use the Environment private network",
			}},
		)
	}
	selectedServers := make(map[uuid.UUID]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		selectedServers[serverID] = struct{}{}
	}
	for _, target := range targets {
		if _, selected := selectedServers[target.ServerID]; !selected {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{{
					Field:   "serverIds",
					Code:    "removal_unsupported",
					Message: "existing runtime Server targets cannot be removed yet",
				}},
			)
		}
	}
	preparedResources, err := service.prepareResources(
		ctx,
		environmentID,
		serverIDs,
		networkID,
		input.Resources,
	)
	if err != nil {
		return err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockPreparedSetupResources(ctx, tx, preparedResources); err != nil {
		return err
	}
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return errors.New("Environment is unavailable")
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil || !setupComplete {
		return errors.New("Environment setup is incomplete")
	}
	activeBuilds, err := models.Build.ActiveCountForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	activeDeployments, err := models.Deployment.ActiveCountForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	activeReleaseCommands, err := models.ReleaseCommandExecution.ActiveCountForEnvironment(
		ctx,
		tx,
		environmentID,
	)
	if err != nil {
		return err
	}
	if activeBuilds > 0 || activeDeployments > 0 || activeReleaseCommands > 0 {
		return errors.New(
			"stop active Build, release command, and Deployment work before editing the Environment",
		)
	}
	currentTargets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	currentServers := make(map[uuid.UUID]struct{}, len(currentTargets))
	for _, target := range currentTargets {
		currentServers[target.ServerID] = struct{}{}
		if _, selected := selectedServers[target.ServerID]; !selected {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{{
					Field:   "serverIds",
					Code:    "removal_unsupported",
					Message: "existing runtime Server targets cannot be removed yet",
				}},
			)
		}
	}
	placementsByServer := make(map[uuid.UUID]preparedRuntimePlacement, len(placements))
	for _, placement := range placements {
		placementsByServer[placement.serverID] = placement
	}
	newTargets := make([]models.EnvironmentTargetEntity, 0, len(serverIDs)-len(currentTargets))
	for _, serverID := range serverIDs {
		if _, exists := currentServers[serverID]; exists {
			continue
		}
		placement := placementsByServer[serverID]
		target, createErr := models.EnvironmentTarget.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetData{
				AttachedAt:    time.Now().UTC(),
				EnvironmentID: environmentID,
				ServerID:      serverID,
			},
		)
		if createErr != nil {
			return createErr
		}
		if _, createErr := models.EnvironmentTargetNetwork.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetNetworkData{
				Driver:              placement.network.Driver,
				ExternalID:          placement.network.ExternalID,
				Configuration:       placement.network.Configuration,
				State:               "applied",
				AppliedAt:           placement.network.AppliedAt,
				ObservedAt:          placement.network.ObservedAt,
				EnvironmentTargetID: target.ID,
				PrivateNetworkID:    networkID,
			},
		); createErr != nil {
			return createErr
		}
		newTargets = append(newTargets, target)
	}
	available, err := models.Environment.EnsureSlugAvailableExcluding(
		ctx, tx, applicationID, environmentID, input.Slug,
	)
	if err != nil {
		return err
	}
	if !available {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment slug is already in use"),
		)
	}
	if _, err := models.Environment.Update(ctx, tx, models.UpdateEnvironmentData{
		ID: environment.ID, Name: input.Name, Slug: input.Slug, Kind: input.Kind,
		APITokenPrefix: environment.APITokenPrefix, APITokenDigest: environment.APITokenDigest,
		ArchivedAt: environment.ArchivedAt, ApplicationID: applicationID,
	}); err != nil {
		return err
	}
	persistedRuntime, err := models.RuntimeConfiguration.FindForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	runtimeSettings, _ := json.Marshal(map[string]any{
		"schema_version": 4, "bp_go_targets": goTargets,
	})
	if _, err := models.RuntimeConfiguration.Update(ctx, tx, models.UpdateRuntimeConfigurationData{
		ID:             persistedRuntime.ID,
		Runtime:        string(runtime),
		ResourceLimits: persistedRuntime.ResourceLimits,
		RestartPolicy:  "unless-stopped",
		Settings:       runtimeSettings,
		EnvironmentID:  environmentID,
	}); err != nil {
		return err
	}
	if _, err := models.EnvironmentProcess.ReplaceActive(
		ctx,
		tx,
		environmentID,
		processes,
	); err != nil {
		return err
	}
	domain, err := models.EnvironmentDomain.PrimaryForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	updatedDomain, err := models.EnvironmentDomain.Update(
		ctx,
		tx,
		models.UpdateEnvironmentDomainData{
			ID: domain.ID, Hostname: input.Hostname, IsPrimary: true,
			ArchivedAt: domain.ArchivedAt, EnvironmentID: environmentID,
		},
	)
	if err != nil {
		return err
	}
	_, err = service.dns.ConfigureTx(
		ctx, tx, updatedDomain, input.DNS,
		domain.Hostname != updatedDomain.Hostname || len(newTargets) > 0, false, false, nil,
	)
	if err != nil {
		return err
	}
	activeSecrets, err := models.EnvironmentSecret.ActiveForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	keys := map[string]struct{}{"PORT": {}}
	for _, resource := range preparedResources {
		for _, secret := range resource.secrets {
			if _, exists := keys[secret.Key]; exists {
				return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{
					Field: "resources", Code: "duplicate",
					Message: "Resource-managed Environment secret keys must be unique",
				}})
			}
			keys[secret.Key] = struct{}{}
		}
	}
	for _, secret := range activeSecrets {
		if secret.SourceType != models.EnvironmentSecretSourceUser {
			continue
		}
		if _, exists := keys[secret.Key]; exists {
			return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
				{
					Field:   "resources",
					Code:    "duplicate",
					Message: "Resource-managed key " + secret.Key + " conflicts with an Environment secret",
				},
			})
		}
		keys[secret.Key] = struct{}{}
	}
	userSecretDescriptors := make([]models.EnvironmentSecretDescriptor, 0)
	for _, secret := range activeSecrets {
		if secret.SourceType == models.EnvironmentSecretSourceUser {
			userSecretDescriptors = append(
				userSecretDescriptors,
				models.EnvironmentSecretDescriptorFromEntity(secret),
			)
			continue
		}
		if err := models.EnvironmentSecret.Archive(ctx, tx, environmentID, secret.ID); err != nil {
			return err
		}
	}
	currentConnections, err := models.EnvironmentResource.ActiveForEnvironment(
		ctx,
		tx,
		environmentID,
	)
	if err != nil {
		return err
	}
	for _, connection := range currentConnections {
		if err := models.EnvironmentResource.Archive(ctx, tx, connection.ID); err != nil {
			return err
		}
	}
	resourceStates := make([]models.EnvironmentResourceState, 0, len(preparedResources))
	secretDescriptors := append([]models.EnvironmentSecretDescriptor{}, userSecretDescriptors...)
	desiredCredentialIDs := make(map[uuid.UUID]struct{}, len(preparedResources))
	for _, prepared := range preparedResources {
		if prepared.credentialInput != nil {
			createdCredential, createCredentialErr := service.resources.createCredential(
				ctx,
				tx,
				prepared.resource,
				*prepared.credentialInput,
			)
			if createCredentialErr != nil {
				return createCredentialErr
			}
			prepared.credential = &createdCredential
			prepared.input.CredentialID = &createdCredential.ID
		}
		if prepared.input.CredentialID != nil {
			desiredCredentialIDs[*prepared.input.CredentialID] = struct{}{}
		}
		resourceConfiguration, configurationErr := encodeEnvironmentResourceConfiguration(
			prepared.input.CredentialProjection, prepared.input.CredentialID,
			preparedCredentialSource(prepared),
			prepared.environmentKeys, prepared.environmentKeyOverrides,
		)
		if configurationErr != nil {
			return configurationErr
		}
		connection, createErr := models.EnvironmentResource.Create(
			ctx,
			tx,
			models.CreateEnvironmentResourceData{
				ID:                   prepared.connectionID,
				Alias:                prepared.input.Alias,
				Configuration:        resourceConfiguration,
				EnvironmentID:        environmentID,
				ResourceID:           prepared.resource.ID,
				ResourceEndpointID:   prepared.endpoint.ID,
				ResourceCredentialID: prepared.input.CredentialID,
			},
		)
		if createErr != nil {
			return createErr
		}
		for _, secret := range prepared.secrets {
			secret.SourceID = connection.ID
			entity, createSecretErr := service.secrets.CreatePrepared(ctx, tx, secret)
			if createSecretErr != nil {
				return createSecretErr
			}
			secretDescriptors = append(
				secretDescriptors,
				models.EnvironmentSecretDescriptorFromEntity(entity),
			)
		}
		resourceStates = append(resourceStates, models.EnvironmentResourceState{
			EnvironmentResourceID: connection.ID,
			ResourceID:            prepared.resource.ID,
			Kind:                  prepared.resource.Engine(),
			EndpointID:            prepared.endpoint.ID,
			CredentialID:          prepared.input.CredentialID,
			Alias:                 prepared.input.Alias,
			Database:              prepared.input.Database,
			EnvironmentKeys:       prepared.environmentKeys,
		})
	}
	for _, connection := range currentConnections {
		if connection.ResourceCredentialID == nil {
			continue
		}
		if _, retained := desiredCredentialIDs[*connection.ResourceCredentialID]; retained {
			continue
		}
		credential, findCredentialErr := models.ResourceCredential.Find(
			ctx,
			tx,
			*connection.ResourceCredentialID,
		)
		if findCredentialErr != nil {
			return findCredentialErr
		}
		if resourceCredentialMetadataEnvironmentID(credential.Metadata) != environmentID {
			continue
		}
		now := time.Now().UTC()
		if archiveCredentialErr := models.ResourceCredential.ArchiveID(
			ctx,
			tx,
			credential.ID,
			now,
		); archiveCredentialErr != nil {
			return archiveCredentialErr
		}
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:      sequence,
		Kind:          "environment_update",
		TriggerType:   "user",
		ActorType:     "user",
		ActorID:       &userID,
		CorrelationID: uuid.New(),
		Summary:       "Update Environment configuration",
		Status:        "committed",
		RequestedAt:   now,
		CommittedAt:   sql.NullTime{Time: now, Valid: true},
		EnvironmentID: environmentID,
	})
	if err != nil {
		return err
	}
	state := models.EnvironmentDesiredState{
		SchemaVersion: models.EnvironmentStateSchemaVersion,
		Runtime: models.EnvironmentRuntimeState{
			Runtime: string(runtime), BPGOTargets: goTargets, RestartPolicy: "unless-stopped",
		},
		Processes: processStatesFromInputs(processes),
		Domain: models.EnvironmentDomainState{
			ID:       updatedDomain.ID,
			Hostname: updatedDomain.Hostname,
			Primary:  true,
		},
		Resources: resourceStates,
		Secrets:   secretDescriptors,
	}
	canonicalState, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return errors.Join(models.ErrDomainValidation, err)
	}
	revision, err := models.EnvironmentStateRevision.Create(
		ctx,
		tx,
		models.CreateEnvironmentStateRevisionData{
			State: canonicalState, EnvironmentID: environmentID, ChangeID: change.ID,
		},
	)
	if err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, tx, models.CreateChangeStateRevisionData{
		Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID,
	}); err != nil {
		return err
	}
	for _, target := range newTargets {
		if _, err := models.EnvironmentTargetState.Create(
			ctx,
			tx,
			models.CreateEnvironmentTargetStateData{
				ObservedState:       json.RawMessage(`{}`),
				State:               "pending",
				EnvironmentTargetID: target.ID,
				DesiredRevisionID:   &revision.ID,
			},
		); err != nil {
			return err
		}
	}
	if err := models.EnvironmentTargetState.MarkEnvironmentPending(
		ctx,
		tx,
		environmentID,
		revision.ID,
		now,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (service *EnvironmentSetup) cleanupEnvironment(
	ctx context.Context,
	environmentID uuid.UUID,
) error {
	jobsToDelete, err := models.Job.ForEnvironment(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return fmt.Errorf("load Environment background jobs: %w", err)
	}
	for _, job := range jobsToDelete {
		if job.State == "available" || job.State == "pending" || job.State == "retryable" ||
			job.State == "running" ||
			job.State == "scheduled" {
			if err := service.jobControl.CancelJob(ctx, job.ID); err != nil {
				return fmt.Errorf("cancel Environment background job %d: %w", job.ID, err)
			}
		}
	}

	routeIDs, err := models.CaddyRoute.ExternalIDsForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return fmt.Errorf("load Environment Caddy routes: %w", err)
	}
	for _, routeID := range routeIDs {
		if err := service.caddy.Delete(ctx, routeID); err != nil {
			return fmt.Errorf("delete Environment Caddy route: %w", err)
		}
	}
	serverIDs, err := models.EnvironmentTarget.ServerIDsForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
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
			if errors.Is(err, rivertype.ErrJobRunning) {
				return fmt.Errorf(
					"Environment background job %d is still stopping; retry deletion",
					job.ID,
				)
			}
			return fmt.Errorf("delete Environment background job %d: %w", job.ID, err)
		}
	}
	if err := service.deleteEnvironmentBuildCaches(ctx, environmentID); err != nil {
		return fmt.Errorf("delete Environment Build caches: %w", err)
	}
	return nil
}

func (service *EnvironmentSetup) rehomeDurableChanges(
	ctx context.Context,
	db storage.Executor,
	environmentIDs []uuid.UUID,
) error {
	if len(environmentIDs) == 0 {
		return nil
	}
	if err := models.Change.RehomeDurableForEnvironments(
		ctx,
		db,
		environmentIDs,
		time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("rehome durable Environment history: %w", err)
	}
	return nil
}

func deleteEnvironmentResourceCredentials(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) error {
	return models.ResourceCredential.DeleteForEnvironment(ctx, db, environmentID)
}

func (service *EnvironmentSetup) DeleteEnvironment(
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
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil || application.IsSystem() {
		return models.ErrSystemApplicationImmutable
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := models.Application.LockNonSystem(ctx, tx, applicationID); err != nil {
		return err
	}
	locked, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || locked.ApplicationID != applicationID {
		return errors.New("Environment is unavailable")
	}
	if err := service.dns.RemoveForEnvironment(ctx, environmentID); err != nil {
		return fmt.Errorf("remove managed Environment DNS: %w", err)
	}
	if err := service.cleanupEnvironment(ctx, environmentID); err != nil {
		return err
	}
	if err := service.rehomeDurableChanges(ctx, tx, []uuid.UUID{environmentID}); err != nil {
		return err
	}
	if err := models.Environment.Destroy(ctx, tx, environmentID); err != nil {
		return fmt.Errorf("delete Environment data: %w", err)
	}
	if err := deleteEnvironmentResourceCredentials(ctx, tx, environmentID); err != nil {
		return fmt.Errorf("delete Environment Resource credentials: %w", err)
	}
	remaining, err := models.Environment.CountForApplication(ctx, tx, applicationID)
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

func (service *EnvironmentSetup) DeleteApplication(
	ctx context.Context,
	applicationID uuid.UUID,
) error {
	application, err := models.Application.Find(ctx, service.db.Executor(), applicationID)
	if err != nil {
		return err
	}
	if application.IsSystem() {
		return models.ErrSystemApplicationImmutable
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := models.Application.LockNonSystem(ctx, tx, applicationID); err != nil {
		return err
	}
	lockedEnvironmentIDs, err := models.Environment.LockIDsForApplication(ctx, tx, applicationID)
	if err != nil {
		return err
	}
	for _, environmentID := range lockedEnvironmentIDs {
		if err := service.dns.RemoveForEnvironment(ctx, environmentID); err != nil {
			return fmt.Errorf("remove managed DNS for Environment %s: %w", environmentID, err)
		}
		if err := service.cleanupEnvironment(ctx, environmentID); err != nil {
			return fmt.Errorf("clean up Environment %s: %w", environmentID, err)
		}
	}
	if err := service.rehomeDurableChanges(ctx, tx, lockedEnvironmentIDs); err != nil {
		return err
	}
	if err := models.Application.Destroy(ctx, tx, applicationID); err != nil {
		return fmt.Errorf("delete Application data: %w", err)
	}
	for _, environmentID := range lockedEnvironmentIDs {
		if err := deleteEnvironmentResourceCredentials(ctx, tx, environmentID); err != nil {
			return fmt.Errorf(
				"delete Resource credentials for Environment %s: %w",
				environmentID,
				err,
			)
		}
	}
	return tx.Commit()
}

func (service *EnvironmentSetup) deleteEnvironmentBuildCaches(
	ctx context.Context,
	environmentID uuid.UUID,
) error {
	serverIDs, err := models.Build.CacheServerIDs(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return err
	}
	configuredServerID, err := models.BuildpackConfiguration.ServerIDForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err == nil {
		serverIDs = append(serverIDs, configuredServerID)
	} else if !errors.Is(
		err,
		sql.ErrNoRows,
	) {
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
			if _, err := service.servers.RunRootCommand(
				ctx,
				target,
				nil,
				remoteDockerExecutable,
				"volume",
				"rm",
				cache,
			); err != nil {
				message := strings.ToLower(err.Error())
				if strings.Contains(message, "no such volume") ||
					strings.Contains(message, "not found") {
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
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return models.DeploymentEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	defer tx.Rollback()
	previous, err := models.Deployment.Lock(ctx, tx, deploymentID)
	if err != nil || (previous.Status != "failed" && previous.Status != "cancelled") {
		return models.DeploymentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("only a failed or cancelled Deployment can be retried"),
		)
	}
	release, err := models.Release.Find(ctx, tx, previous.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID || release.RegistryResourceID == nil ||
		release.RegistryCredentialID == nil ||
		!release.RegistryEndpoint.Valid {
		return models.DeploymentEntity{}, errors.New(
			"failed Deployment does not belong to this Environment",
		)
	}
	target, err := models.EnvironmentTarget.Find(ctx, tx, previous.EnvironmentTargetID)
	if err != nil || target.EnvironmentID != environmentID || target.DetachedAt.Valid {
		return models.DeploymentEntity{}, errors.New("failed Deployment target is unavailable")
	}
	revision, err := models.EnvironmentStateRevision.FindResultForChange(ctx, tx, previous.ChangeID)
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
		Sequence:    sequence,
		Kind:        "deployment_retry",
		TriggerType: "user",
		ActorType:   "user",
		ActorID:     &userID,
		CauseSystem: sql.NullString{
			String: "deployment",
			Valid:  true,
		},
		CauseReference: sql.NullString{String: previous.ID.String(), Valid: true},
		CorrelationID:  uuid.New(),
		Summary:        "Retry failed Deployment",
		Status:         "committed",
		RequestedAt:    now,
		CommittedAt:    sql.NullTime{Time: now, Valid: true},
		EnvironmentID:  environmentID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return models.DeploymentEntity{}, err
	}
	deployment, err := service.releases.QueueTargetTx(
		ctx,
		tx,
		change.ID,
		release.ID,
		target.ID,
		previous.Attempt+1,
		previous.Strategy,
		state.Processes,
		now,
	)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.DeploymentEntity{}, err
	}
	return deployment, nil
}

func (service *EnvironmentSetup) StopDeployment(
	ctx context.Context,
	applicationID, environmentID, deploymentID uuid.UUID,
) error {
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return errors.New("Deployment does not belong to this Application")
	}
	deployment, err := models.Deployment.Find(ctx, service.db.Executor(), deploymentID)
	if err != nil {
		return errors.New("Deployment does not belong to this Environment")
	}
	release, err := models.Release.Find(ctx, service.db.Executor(), deployment.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID {
		return errors.New("Deployment does not belong to this Environment")
	}
	if deployment.Status != "queued" && deployment.Status != "running" &&
		deployment.Status != "cancelling" {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("only an active Deployment can be stopped"),
		)
	}
	if deployment.Status != "cancelling" {
		job, err := models.Job.FindForDeployment(ctx, service.db.Executor(), deployment.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.New("Deployment background job is unavailable")
		}
		if err == nil && (job.State == "available" || job.State == "pending" ||
			job.State == "retryable" || job.State == "running" || job.State == "scheduled") {
			if err := service.jobControl.CancelJob(ctx, job.ID); err != nil {
				return fmt.Errorf("stop Deployment background job: %w", err)
			}
		}
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	return service.deployments.Cancel(
		cleanupCtx,
		deployment.ID,
		"Deployment cancelled by user",
	)
}

func validateEnvironmentSetupInput(
	input EnvironmentSetupInput,
	runtime models.BuildpackRuntime,
) error {
	builder := validation.NewBuilder()
	if _, err := models.ValidateEnvironmentProcessFormation(
		normalizedProcessFormation(input.Processes, input.ContainerPort, input.HealthPath),
	); err != nil {
		return err
	}
	goTargets := goProcessTargetsFromProcesses(input.Processes)
	if runtime != models.BuildpackRuntimeGo && len(goTargets) > 0 {
		builder.Add("processes", "target", "Go targets require the Go runtime")
		return builder.Err()
	}
	if err := models.ValidateGoProcessTargets(goTargets); err != nil {
		builder.Add("processes", "target", err.Error())
	}
	if len(goTargets) > 0 {
		webHasTarget := false
		for _, process := range input.Processes {
			if process.Kind == models.EnvironmentProcessWeb && process.Target != nil {
				webHasTarget = true
				break
			}
		}
		if !webHasTarget {
			builder.Add(
				"processes",
				"target",
				"web must declare a Go target when any other process does",
			)
		}
	}
	if len(input.Resources) > 32 || len(input.Secrets) > 128 {
		builder.Add("resources", "max_items", "Environment setup exceeds the supported item count")
	}
	return builder.Err()
}

func normalizedProcessFormation(
	configured []models.EnvironmentProcessInput,
	containerPort int32,
	healthPath string,
) []models.EnvironmentProcessInput {
	if len(configured) == 0 {
		return []models.EnvironmentProcessInput{
			{
				Name:          models.EnvironmentProcessWeb,
				Kind:          models.EnvironmentProcessWeb,
				Arguments:     []string{},
				Replicas:      1,
				ContainerPort: &containerPort,
				HealthPath:    strings.TrimSpace(healthPath),
			},
		}
	}
	normalized, err := models.ValidateEnvironmentProcessFormation(configured)
	if err != nil {
		return configured
	}
	return normalized
}

const goTargetExecutableDirectory = "/layers/paketo-buildpacks_go-build/targets/bin"

func deriveGoProcessCommands(
	processes []models.EnvironmentProcessInput,
) []models.EnvironmentProcessInput {
	for index := range processes {
		process := &processes[index]
		if process.Target == nil {
			continue
		}
		switch process.Kind {
		case models.EnvironmentProcessWorker, models.EnvironmentProcessRelease:
			command := goTargetExecutableDirectory + "/" + path.Base(*process.Target)
			process.Command = &command
		}
	}
	return processes
}

func goProcessTargetsFromProcesses(
	processes []models.EnvironmentProcessInput,
) []models.GoProcessTarget {
	var web *models.GoProcessTarget
	workers := make([]models.GoProcessTarget, 0, len(processes))
	var release *models.GoProcessTarget
	for _, process := range processes {
		if process.Target == nil {
			continue
		}
		target := models.GoProcessTarget{Process: process.Name, Target: *process.Target}
		switch process.Kind {
		case models.EnvironmentProcessWeb:
			web = &target
		case models.EnvironmentProcessWorker:
			workers = append(workers, target)
		case models.EnvironmentProcessRelease:
			release = &target
		}
	}
	targets := make([]models.GoProcessTarget, 0, len(processes))
	if web != nil {
		targets = append(targets, *web)
	}
	targets = append(targets, workers...)
	if release != nil {
		targets = append(targets, *release)
	}
	return targets
}

func processStatesFromInputs(
	inputs []models.EnvironmentProcessInput,
) []models.EnvironmentProcessState {
	states := make([]models.EnvironmentProcessState, 0, len(inputs))
	for _, input := range inputs {
		state := models.EnvironmentProcessState{
			Name:       input.Name,
			Kind:       input.Kind,
			Command:    input.Command,
			Arguments:  input.Arguments,
			Replicas:   input.Replicas,
			HealthPath: input.HealthPath,
		}
		if input.ContainerPort != nil {
			state.ContainerPort = *input.ContainerPort
		}
		if input.TimeoutSeconds != nil {
			state.TimeoutSeconds = *input.TimeoutSeconds
		}
		states = append(states, state)
	}
	return states
}

func processInputsFromState(
	states []models.EnvironmentProcessState,
) []models.EnvironmentProcessInput {
	inputs := make([]models.EnvironmentProcessInput, 0, len(states))
	for _, state := range states {
		input := models.EnvironmentProcessInput{
			Name:       state.Name,
			Kind:       state.Kind,
			Command:    state.Command,
			Arguments:  state.Arguments,
			Replicas:   state.Replicas,
			HealthPath: state.HealthPath,
		}
		if state.Kind == models.EnvironmentProcessWeb {
			port := state.ContainerPort
			input.ContainerPort = &port
		}
		if state.Kind == models.EnvironmentProcessRelease {
			timeout := state.TimeoutSeconds
			input.TimeoutSeconds = &timeout
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (service *EnvironmentSetup) loadSource(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (environmentSetupSource, models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	source, err := models.EnvironmentSource.SetupSource(
		ctx, service.db.Executor(), applicationID, environmentID,
	)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	source.SetupComplete, err = models.Environment.SetupComplete(
		ctx,
		service.db.Executor(),
		environmentID,
	)
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
	installation, err := models.GitHubInstallation.Find(
		ctx,
		service.db.Executor(),
		source.InstallationID,
	)
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

func (service *EnvironmentSetup) runtimePlacements(
	ctx context.Context,
	serverIDs []uuid.UUID,
) ([]preparedRuntimePlacement, uuid.UUID, error) {
	if len(serverIDs) == 0 {
		return nil, uuid.Nil, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "required",
					Message: "select at least one runtime Server target",
				},
			},
		)
	}
	placements := make([]preparedRuntimePlacement, 0, len(serverIDs))
	var networkID uuid.UUID
	for _, serverID := range serverIDs {
		resolvedServerID, resolvedNetworkID, network, err := service.runtimePlacement(ctx, serverID)
		if err != nil {
			return nil, uuid.Nil, err
		}
		if networkID != uuid.Nil && networkID != resolvedNetworkID {
			return nil, uuid.Nil, errors.Join(
				models.ErrDomainValidation,
				errors.New(
					"selected runtime Server targets do not share the DeployCrate private network",
				),
			)
		}
		networkID = resolvedNetworkID
		placements = append(
			placements,
			preparedRuntimePlacement{serverID: resolvedServerID, network: network},
		)
	}
	return placements, networkID, nil
}

func (service *EnvironmentSetup) runtimePlacement(
	ctx context.Context,
	serverID uuid.UUID,
) (uuid.UUID, uuid.UUID, models.ServerNetworkEntity, error) {
	server, err := models.Server.Find(ctx, service.db.Executor(), serverID)
	if err != nil || server.ArchivedAt.Valid || !server.IsConfigured ||
		(server.Kind != "self_hosted" && server.Kind != "worker") {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "unavailable",
					Message: "selected runtime Server is unavailable",
				},
			},
		)
	}
	capabilities, err := models.ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Runtime {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "capability",
					Message: "selected Server does not support application runtimes",
				},
			},
		)
	}
	row, err := models.Application.SystemRuntimePlacement(ctx, service.db.Executor())
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, err
	}
	network, err := models.ServerNetwork.ActiveForServerNetwork(
		ctx, service.db.Executor(), serverID, row.NetworkID,
	)
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "network",
					Message: "selected Server is not attached to the DeployCrate private network",
				},
			},
		)
	}
	return serverID, row.NetworkID, network, nil
}

func (service *EnvironmentSetup) prepareResources(
	ctx context.Context,
	environmentID uuid.UUID,
	serverIDs []uuid.UUID,
	networkID uuid.UUID,
	inputs []EnvironmentSetupResourceInput,
) ([]preparedSetupResource, error) {
	prepared := make([]preparedSetupResource, 0, len(inputs))
	connectionConfigurations := make(map[uuid.UUID]environmentResourceConfiguration)
	if environmentID != uuid.Nil {
		connections, err := models.EnvironmentResource.ActiveForEnvironment(
			ctx, service.db.Executor(), environmentID,
		)
		if err != nil {
			return nil, err
		}
		for _, connection := range connections {
			configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
			if err != nil {
				return nil, err
			}
			connectionConfigurations[connection.ResourceID] = configuration
		}
	}
	aliases := make(map[string]struct{}, len(inputs))
	resources := make(map[uuid.UUID]struct{}, len(inputs))
	selectedServers := make(map[uuid.UUID]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		selectedServers[serverID] = struct{}{}
	}
	for index, input := range inputs {
		if _, exists := resources[input.ResourceID]; exists {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.resourceId", index),
						Code:    "duplicate",
						Message: "Resource is already selected",
					},
				},
			)
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), input.ResourceID)
		_, supported := models.FindResourceEngine(resource.Engine())
		if err != nil || resource.ArchivedAt.Valid || !supported {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("selected Resource is unavailable"),
			)
		}
		if !resource.EnvironmentAttachable {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.resourceId", index),
						Code:    "not_attachable",
						Message: "selected Resource cannot be attached to an Environment",
					},
				},
			)
		}
		input.Alias = strings.ToUpper(strings.TrimSpace(input.Alias))
		input.CredentialProjection = strings.ToLower(strings.TrimSpace(input.CredentialProjection))
		if input.Alias == "" {
			input.Alias = strings.ToUpper(resource.Engine())
		}
		if input.CredentialProjection != resourceCredentialProjectionConnectionURL &&
			input.CredentialProjection != resourceCredentialProjectionIndividualParts {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.credentialProjection", index),
						Code:    "unsupported",
						Message: "Choose Connection URL or Individual parts",
					},
				},
			)
		}
		if input.CredentialProjection == resourceCredentialProjectionConnectionURL &&
			resource.Engine() != "postgresql" {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.credentialProjection", index),
						Code:    "unsupported",
						Message: "Connection URL is not supported for this Resource",
					},
				},
			)
		}
		if _, exists := aliases[input.Alias]; exists {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.alias", index),
						Code:    "duplicate",
						Message: "Resource alias is already selected",
					},
				},
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), input.EndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID ||
			(endpoint.PrivateNetworkID != nil && *endpoint.PrivateNetworkID != networkID) {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("selected Resource endpoint is unavailable from the Environment target"),
			)
		}
		installation, installationErr := models.ResourceInstallation.FindActiveForResource(
			ctx, service.db.Executor(), resource.ID,
		)
		if installationErr == nil {
			if _, selected := selectedServers[installation.ServerID]; !selected {
				return nil, errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   fmt.Sprintf("resources.%d.resourceId", index),
							Code:    "placement",
							Message: "selected managed Resource is not installed on a selected runtime Server target",
						},
					},
				)
			}
		} else if !errors.Is(installationErr, sql.ErrNoRows) {
			return nil, installationErr
		}
		if input.CredentialID == nil && resource.Engine() == "postgresql" {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("PostgreSQL application credential is required"),
			)
		}
		connectionID := uuid.New()
		var credential *models.ResourceCredentialEntity
		credentialValues := make(map[string]string)
		if input.CredentialID != nil {
			selectedCredential, findErr := models.ResourceCredential.Find(
				ctx,
				service.db.Executor(),
				*input.CredentialID,
			)
			if findErr != nil || selectedCredential.ArchivedAt.Valid ||
				selectedCredential.ResourceID != resource.ID {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected Resource credential is unavailable"),
				)
			}
			if resourceCredentialMetadataPurpose(selectedCredential.Metadata) != "application" {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New(
						"Resource administrator credentials cannot be injected into an Environment",
					),
				)
			}
			if resource.Engine() == "opentelemetry" &&
				resourceCredentialMetadataEnvironmentID(
					selectedCredential.Metadata,
				) != environmentID {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected OpenTelemetry credential belongs to another Environment"),
				)
			}
			input.Database = resourceCredentialMetadataDatabase(selectedCredential.Metadata)
			if input.Database != "" && !resourceHasDatabase(resource, input.Database) {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected Resource Database is unavailable"),
				)
			}
			credentialValues, err = service.resources.credentialSecretValues(selectedCredential)
			if err != nil {
				return nil, err
			}
			credential = &selectedCredential
		}
		overrides := make(map[string]string)
		effectiveKeys := resource.EnvironmentKeys()
		if configuration, exists := connectionConfigurations[resource.ID]; exists {
			overrides = maps.Clone(configuration.EnvironmentKeyOverrides)
			effectiveKeys = connectionEnvironmentKeys(resource, configuration)
		}
		identityToken := credentialValues["token"]
		var credentialInput *ResourceCredentialInput
		if resource.Engine() == "opentelemetry" {
			if identityToken == "" {
				existingCredential, findCredentialErr := models.ResourceCredential.ActiveApplicationForEnvironment(
					ctx,
					service.db.Executor(),
					resource.ID,
					environmentID,
				)
				if findCredentialErr == nil {
					credentialValues, err = service.resources.credentialSecretValues(
						existingCredential,
					)
					if err != nil {
						return nil, err
					}
					identityToken = credentialValues["token"]
					credential = &existingCredential
					input.CredentialID = &existingCredential.ID
				} else if !errors.Is(findCredentialErr, sql.ErrNoRows) {
					return nil, findCredentialErr
				}
			}
			if identityToken == "" {
				identityToken, err = service.telemetry.EnvironmentToken(environmentID)
				if err != nil {
					return nil, err
				}
				metadata, metadataErr := json.Marshal(map[string]string{
					"purpose": "application", "environment_id": environmentID.String(),
				})
				if metadataErr != nil {
					return nil, metadataErr
				}
				credentialInput = &ResourceCredentialInput{
					Name: "Environment " + environmentID.String(), Metadata: metadata,
					SecretValues: map[string]string{"token": identityToken},
				}
			}
		}
		values, environmentKeys, projectionErr := resourceProjectionValues(
			resource,
			endpoint,
			credential,
			credentialValues,
			input.CredentialProjection,
			effectiveKeys,
			identityToken,
		)
		if projectionErr != nil {
			return nil, projectionErr
		}
		secrets := make([]PreparedEnvironmentSecret, 0, len(values))
		for key, value := range values {
			secret, prepareErr := service.secrets.Prepare(
				environmentID,
				key,
				value,
				models.EnvironmentSecretSourceResource,
				connectionID,
			)
			if prepareErr != nil {
				return nil, prepareErr
			}
			secrets = append(secrets, secret)
		}
		prepared = append(prepared, preparedSetupResource{
			input:                   input,
			connectionID:            connectionID,
			resource:                resource,
			endpoint:                endpoint,
			credential:              credential,
			credentialInput:         credentialInput,
			environmentKeys:         environmentKeys,
			environmentKeyOverrides: overrides,
			secrets:                 secrets,
		})
		aliases[input.Alias] = struct{}{}
		resources[input.ResourceID] = struct{}{}
	}
	return prepared, nil
}

func preparedCredentialSource(resource preparedSetupResource) string {
	if resource.resource.Engine() == "opentelemetry" {
		return "platform"
	}
	return ""
}

func resourceProjectionValues(
	resource models.ResourceEntity,
	endpoint models.ResourceEndpointEntity,
	credential *models.ResourceCredentialEntity,
	credentialValues map[string]string,
	projection string,
	resourceKeys map[string]string,
	identityToken string,
) (map[string]string, map[string]string, error) {
	definition, supported := models.FindResourceEngine(resource.Engine())
	if !supported {
		return nil, nil, errors.New("Resource engine is unsupported")
	}
	projectedKeys := make(map[string]string)
	valueFor := func(logicalName, value string, values map[string]string) error {
		key := resourceKeys[logicalName]
		if key == "" {
			return errors.New("Resource Environment key mapping is incomplete")
		}
		projectedKeys[logicalName] = key
		if value != "" {
			values[key] = value
		}
		return nil
	}
	values := make(map[string]string)
	if resource.Engine() == "opentelemetry" {
		settings := endpoint.ParsedSettings()
		if projection != resourceCredentialProjectionIndividualParts ||
			!endpoint.IsEnvironmentEndpoint() ||
			settings.Transport != models.ResourceEndpointTransportOTLPHTTP ||
			settings.Authentication != models.ResourceEndpointAuthSignedIdentity ||
			strings.TrimSpace(identityToken) == "" {
			return nil, nil, errors.New(
				"selected OpenTelemetry Resource endpoint cannot be projected into an Environment",
			)
		}
		for logicalName, value := range map[string]string{
			"endpoint": endpoint.URL(), "protocol": settings.Transport,
			"headers": "Authorization=Bearer " + identityToken,
		} {
			if err := valueFor(logicalName, value, values); err != nil {
				return nil, nil, err
			}
		}
		return values, projectedKeys, nil
	}
	database := ""
	if credential != nil {
		database = resourceCredentialMetadataDatabase(credential.Metadata)
	}
	if projection == resourceCredentialProjectionConnectionURL {
		password := credentialValues["password"]
		if resource.Engine() != "postgresql" || credential == nil || password == "" ||
			!credential.Username.Valid ||
			database == "" {
			return nil, nil, errors.New("selected PostgreSQL application credential is incomplete")
		}
		uri := &url.URL{
			Scheme: "postgresql",
			Host:   fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port),
			Path:   "/" + database,
			User:   url.UserPassword(credential.Username.String, password),
		}
		query := uri.Query()
		query.Set("sslmode", endpoint.TLSMode)
		uri.RawQuery = query.Encode()
		if err := valueFor("url", uri.String(), values); err != nil {
			return nil, nil, err
		}
		return values, projectedKeys, nil
	}
	if projection != resourceCredentialProjectionIndividualParts {
		return nil, nil, errors.New("Resource credential projection is unsupported")
	}
	for logicalName, value := range map[string]string{
		"host": endpoint.Address, "port": fmt.Sprint(endpoint.Port),
		"protocol": endpoint.Protocol, "tls_mode": endpoint.TLSMode,
	} {
		if err := valueFor(logicalName, value, values); err != nil {
			return nil, nil, err
		}
	}
	if definition.ResourceType == models.ResourceTypeDatabase && database != "" {
		if err := valueFor("database", database, values); err != nil {
			return nil, nil, err
		}
	}
	if credential != nil {
		username := ""
		if credential.Username.Valid {
			username = credential.Username.String
		}
		if err := valueFor("username", username, values); err != nil {
			return nil, nil, err
		}
		for _, field := range definition.CredentialFields {
			if err := valueFor(field.Name, credentialValues[field.Name], values); err != nil {
				return nil, nil, err
			}
		}
	}
	return values, projectedKeys, nil
}

func lockPreparedSetupResources(
	ctx context.Context,
	db storage.Executor,
	prepared []preparedSetupResource,
) error {
	ordered := append([]preparedSetupResource(nil), prepared...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].resource.ID.String() < ordered[right].resource.ID.String()
	})
	for _, item := range ordered {
		resource, err := models.Resource.LockActive(ctx, db, item.resource.ID)
		if err != nil {
			return errors.New("selected Resource changed while the Environment was being prepared")
		}
		if !resource.UpdatedAt.Equal(item.resource.UpdatedAt) {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   "resources",
						Code:    "stale",
						Message: "a selected Resource changed; review the attachment and try again",
					},
				},
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, db, item.endpoint.ID)
		if err != nil || !endpoint.UpdatedAt.Equal(item.endpoint.UpdatedAt) ||
			endpoint.ArchivedAt.Valid {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   "resources",
						Code:    "stale",
						Message: "a selected Resource endpoint changed; review the attachment and try again",
					},
				},
			)
		}
		if item.credential != nil {
			credential, err := models.ResourceCredential.Find(ctx, db, item.credential.ID)
			if err != nil || !credential.UpdatedAt.Equal(item.credential.UpdatedAt) ||
				credential.ArchivedAt.Valid {
				return errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   "resources",
							Code:    "stale",
							Message: "a selected Resource credential changed; review the attachment and try again",
						},
					},
				)
			}
		}
	}
	return nil
}
