package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	buildpacksclient "deploycrate-ce/clients/buildpacks"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/riverqueue/river/rivertype"
)

func (service *EnvironmentSetup) UpdateEnvironment(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	input EnvironmentEditInput,
) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = slug.Make(strings.TrimSpace(input.Slug))
	input.Kind = strings.TrimSpace(input.Kind)
	currentRuntime, err := models.RuntimeConfiguration.FindForEnvironment(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return err
	}
	runtime := models.BuildpackRuntime(currentRuntime.Runtime)
	if input.Name == "" || input.Slug == "" || input.Kind == "" {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment name, slug, and kind are required"),
		)
	}
	configuration, err := prepareEnvironmentConfiguration(EnvironmentSetupInput{
		ServerIDs:     input.ServerIDs,
		Hostname:      input.Hostname,
		ContainerPort: input.ContainerPort,
		HealthPath:    input.HealthPath,
		Processes:     input.Processes,
		Resources:     input.Resources,
		DNS:           input.DNS,
	}, runtime)
	if err != nil {
		return err
	}
	processes := configuration.processes
	goTargets := configuration.goTargets
	serverIDs := configuration.serverIDs
	placements, selectedNetworkID, err := service.runtimePlacements(ctx, serverIDs)
	if err != nil {
		return err
	}
	networkID, err := models.EnvironmentNetwork.ActivePrivateNetworkID(ctx, service.db.Executor(), environmentID)
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
		configuration.input.Resources,
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
	activeReleaseCommands, err := models.ReleaseCommandExecution.ActiveCountForEnvironment(ctx, tx, environmentID)
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
		ID: persistedRuntime.ID, Runtime: string(runtime), ResourceLimits: persistedRuntime.ResourceLimits,
		RestartPolicy: "unless-stopped", Settings: runtimeSettings, EnvironmentID: environmentID,
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
	keys, err := environmentResourceSecretKeys(preparedResources)
	if err != nil {
		return err
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
	currentConnections, err := models.EnvironmentResource.ActiveForEnvironment(ctx, tx, environmentID)
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
		if archiveCredentialErr := models.ResourceCredential.ArchiveID(ctx, tx, credential.ID, now); archiveCredentialErr != nil {
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
	revision, err := createEnvironmentStateRevision(
		ctx, tx, environmentID, change.ID, runtime, goTargets, processes,
		updatedDomain, resourceStates, secretDescriptors,
	)
	if err != nil {
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
	if err := models.EnvironmentTargetState.MarkEnvironmentPending(ctx, tx, environmentID, revision.ID, now); err != nil {
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

	routeIDs, err := models.CaddyRoute.ExternalIDsForEnvironment(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return fmt.Errorf("load Environment Caddy routes: %w", err)
	}
	for _, routeID := range routeIDs {
		if err := service.caddy.Delete(ctx, routeID); err != nil {
			return fmt.Errorf("delete Environment Caddy route: %w", err)
		}
	}
	serverIDs, err := models.EnvironmentTarget.ServerIDsForEnvironment(ctx, service.db.Executor(), environmentID)
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
	if err := models.Change.RehomeDurableForEnvironments(ctx, db, environmentIDs, time.Now().UTC()); err != nil {
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
	configuredServerID, err := models.BuildpackConfiguration.ServerIDForEnvironment(ctx, service.db.Executor(), environmentID)
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
