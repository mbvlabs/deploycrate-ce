package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"
)

func (service *EnvironmentSetup) List(ctx context.Context) ([]EnvironmentListItem, error) {
	return models.Environment.ListCatalog(ctx, service.db.Executor())
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

func (service *EnvironmentSetup) environmentSecretActivity(
	ctx context.Context,
	environmentID uuid.UUID,
) ([]EnvironmentSecretActivity, error) {
	targetStates, err := models.EnvironmentTargetState.ActiveForEnvironment(ctx, service.db.Executor(), environmentID)
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
	deployment, err := models.Deployment.EnvironmentActivity(ctx, service.db.Executor(), environmentID, deploymentID)
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
