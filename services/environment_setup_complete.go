package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
	configuration, err := prepareEnvironmentConfiguration(input, runtime)
	if err != nil {
		return EnvironmentSetupResult{}, err
	}
	input = configuration.input
	processes := configuration.processes
	goTargets := configuration.goTargets
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
	serverIDs := configuration.serverIDs
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
	keys, err := environmentResourceSecretKeys(preparedResources)
	if err != nil {
		return EnvironmentSetupResult{}, err
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
	revision, err := createEnvironmentStateRevision(
		ctx, tx, environment.ID, change.ID, runtime, goTargets, processes,
		domain, resourceStates, descriptors,
	)
	if err != nil {
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
