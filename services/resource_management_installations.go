package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func (service *ResourceManagement) CreateInstallation(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	installation, err := service.createInstallation(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	endpoint, err := models.ResourceEndpoint.FindActivePrimaryPublic(ctx, tx, resource.ID)
	if errors.Is(err, sql.ErrNoRows) {
		endpoint, err = service.createManagedPrimaryEndpoint(ctx, tx, resource, installation)
	} else if err == nil {
		err = service.syncManagedEndpoints(ctx, tx, resource, installation)
	}
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	healthChecks, err := models.ResourceHealthCheck.ActiveKindCount(
		ctx,
		tx,
		resource.ID,
		resource.Engine(),
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if healthChecks == 0 {
		var credential *models.ResourceCredentialEntity
		if resource.ResourceType == models.ResourceTypeDatabase {
			administrator, credentialErr := service.resourceAdministratorCredential(
				ctx,
				tx,
				resource.ID,
			)
			if credentialErr != nil {
				return models.ResourceInstallationEntity{}, credentialErr
			}
			credential = &administrator
		}
		healthInput := defaultResourceHealthCheckInput(resource, endpoint, credential)
		if healthInput != nil {
			if _, err := service.createHealthCheck(ctx, tx, resource, *healthInput); err != nil {
				return models.ResourceInstallationEntity{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return installation, nil
}

func (service *ResourceManagement) createInstallation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	installations, err := models.ResourceInstallation.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if installations > 0 {
		return models.ResourceInstallationEntity{}, domainError(
			"installation",
			"topology",
			"only one active Docker installation is supported for a Resource right now",
		)
	}
	if err := service.validatePlacement(
		ctx,
		db,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		exists, err := models.Credential.ActiveExists(ctx, db, *input.RegistryCredentialID)
		if err != nil {
			return models.ResourceInstallationEntity{}, err
		}
		if err := requireChild(
			boolCount(exists),
			"registryCredentialId",
			"registry credential is unavailable",
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	configuration, err := resourceInstallationConfiguration(input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if _, err := managedPrimaryPortMapping(resource.Engine(), configuration); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	created, err := models.ResourceInstallation.Create(
		ctx,
		db,
		models.CreateResourceInstallationData{
			ImageReference: input.ImageReference, ImageDigest: nullableString(input.ImageDigest),
			ContainerName: input.ContainerName, RestartPolicy: input.RestartPolicy,
			Configuration: configuration, ResourceID: resource.ID,
			ServerID: input.ServerID, RegistryCredentialID: input.RegistryCredentialID,
		},
	)
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, &installationID); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	current, err := models.ResourceInstallation.LockActiveForResource(
		ctx,
		tx,
		resourceID,
		installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceInstallationEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := service.validatePlacement(
		ctx,
		tx,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		exists, countErr := models.Credential.ActiveExists(ctx, tx, *input.RegistryCredentialID)
		if countErr != nil {
			return models.ResourceInstallationEntity{}, countErr
		}
		if err := requireChild(
			boolCount(exists),
			"registryCredentialId",
			"registry credential is unavailable",
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	if current.ServerID != input.ServerID {
		activePolicy, policyErr := service.installationHasActiveBackupPolicy(
			ctx,
			tx,
			installationID,
		)
		if policyErr != nil {
			return models.ResourceInstallationEntity{}, policyErr
		}
		if activePolicy {
			return models.ResourceInstallationEntity{}, domainError(
				"serverId",
				"backup_policy",
				"pause or archive the active backup policy before moving this installation",
			)
		}
		if err := service.validateInstallationMove(
			ctx,
			tx,
			installationID,
			input.ServerID,
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	configuration, err := resourceInstallationConfiguration(input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	currentMapping, err := managedPrimaryPortMapping(resource.Engine(), current.Configuration)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	nextMapping, err := managedPrimaryPortMapping(resource.Engine(), configuration)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	privateEndpoints, err := models.ResourceEndpoint.ActivePrivateCount(
		ctx,
		tx,
		resourceID,
		uuid.Nil,
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if privateEndpoints > 0 &&
		(current.ServerID != input.ServerID || currentMapping.HostPort != nextMapping.HostPort) {
		return models.ResourceInstallationEntity{}, domainError(
			"installation",
			"private_access",
			"remove this Resource from its private network before changing the installation Server or host port",
		)
	}
	updated, err := models.ResourceInstallation.Update(
		ctx,
		tx,
		models.UpdateResourceInstallationData{
			ID:                   current.ID,
			ImageReference:       input.ImageReference,
			ImageDigest:          nullableString(input.ImageDigest),
			ContainerName:        input.ContainerName,
			RestartPolicy:        input.RestartPolicy,
			Configuration:        configuration,
			ArchivedAt:           current.ArchivedAt,
			ResourceID:           resourceID,
			ServerID:             input.ServerID,
			RegistryCredentialID: input.RegistryCredentialID,
		},
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	if err := service.syncManagedEndpoints(ctx, tx, resource, updated); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) RunInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	if err := service.requireNoActiveRestore(
		ctx,
		service.db.Executor(),
		resourceID,
		&installationID,
	); err != nil {
		return err
	}
	resource, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return err
	}
	installation, err := models.ResourceInstallation.FindActiveForResourceID(
		ctx, service.db.Executor(), resourceID, installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	if installation.RegistryCredentialID != nil {
		return errors.New("running an installation with a registry credential is not supported yet")
	}

	mapping, err := managedPrimaryPortMapping(resource.Engine(), installation.Configuration)
	if err != nil {
		return err
	}
	portMappings := []containerclient.PortMapping{
		{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			Protocol:      mapping.Protocol,
		},
	}

	volumeMounts, err := models.ResourceVolumeMount.ActiveWithVolumeForInstallation(
		ctx, service.db.Executor(), installationID,
	)
	if err != nil {
		return err
	}
	mounts := make([]containerclient.VolumeMount, 0, len(volumeMounts))
	for _, mount := range volumeMounts {
		mounts = append(mounts, containerclient.VolumeMount{
			Name: mount.Name, MountPath: mount.MountPath, ReadOnly: mount.ReadOnly,
		})
	}

	environment := make(map[string]string)
	if resource.Engine() == "postgresql" || resource.Engine() == "clickhouse" {
		administrator, adminErr := service.resourceAdministratorCredential(
			ctx,
			service.db.Executor(),
			resource.ID,
		)
		if adminErr != nil {
			return adminErr
		}
		values, valuesErr := service.credentialSecretValues(administrator)
		if valuesErr != nil {
			return valuesErr
		}
		if !administrator.Username.Valid ||
			strings.TrimSpace(administrator.Username.String) == "" ||
			values["password"] == "" {
			return fmt.Errorf(
				"%s Resource administrator credential is incomplete",
				resource.Engine(),
			)
		}
		switch resource.Engine() {
		case "postgresql":
			environment["POSTGRES_USER"] = administrator.Username.String
			environment["POSTGRES_PASSWORD"] = values["password"]
		case "clickhouse":
			environment["CLICKHOUSE_USER"] = administrator.Username.String
			environment["CLICKHOUSE_PASSWORD"] = values["password"]
		}
	}
	imageReference := installation.ImageReference
	if installation.ImageDigest.Valid && !strings.Contains(imageReference, "@") {
		imageReference += "@" + installation.ImageDigest.String
	}
	if err := service.container.Run(
		ctx,
		installation.ServerID,
		managedResourceCapability(resource.Engine()),
		containerclient.RunSpec{
			InstallationID: installation.ID.String(), ContainerName: installation.ContainerName,
			ImageReference: imageReference, RestartPolicy: installation.RestartPolicy,
			PortMappings: portMappings, VolumeMounts: mounts, Environment: environment,
		},
	); err != nil {
		return err
	}
	status, err := service.observeDockerInstallation(ctx, installation)
	if err != nil {
		return err
	}
	if status.ServiceState != "running" {
		var state containerclient.State
		_ = json.Unmarshal(status.Details, &state)
		return fmt.Errorf(
			"container did not stay running: state %s, exit code %d; open container logs for details",
			status.ServiceState,
			state.ExitCode,
		)
	}
	return nil
}

func (service *ResourceManagement) StopInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "stop")
}

func (service *ResourceManagement) RestartInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "restart")
}

func (service *ResourceManagement) RemoveInstallationContainer(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "remove")
}

func (service *ResourceManagement) controlInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	action string,
) error {
	if err := service.requireNoActiveRestore(
		ctx,
		service.db.Executor(),
		resourceID,
		&installationID,
	); err != nil {
		return err
	}
	installation, err := service.loadInstallationForControl(ctx, resourceID, installationID)
	if err != nil {
		return err
	}
	capability, err := service.resourceCapability(ctx, resourceID)
	if err != nil {
		return err
	}
	switch action {
	case "stop":
		err = service.container.Stop(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	case "restart":
		err = service.container.Restart(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	case "remove":
		err = service.container.Remove(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	default:
		err = errors.New("unsupported container action")
	}
	if err != nil {
		return err
	}
	_, err = service.observeDockerInstallation(ctx, installation)
	return err
}

func (service *ResourceManagement) loadInstallationForControl(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) (models.ResourceInstallationEntity, error) {
	if _, err := service.loadResource(ctx, service.db.Executor(), resourceID, false); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	installation, err := models.ResourceInstallation.FindActiveForResourceID(
		ctx, service.db.Executor(), resourceID, installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceInstallationEntity{}, models.ErrNotFound
	}
	return installation, err
}

func (service *ResourceManagement) InstallationLogs(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	tail int,
) (string, error) {
	installation, err := service.loadInstallationForControl(ctx, resourceID, installationID)
	if err != nil {
		return "", err
	}
	capability, err := service.resourceCapability(ctx, resourceID)
	if err != nil {
		return "", err
	}
	return service.container.Logs(
		ctx,
		installation.ServerID,
		capability,
		installation.ID.String(),
		installation.ContainerName,
		tail,
	)
}

func (service *ResourceManagement) observeInstallation(
	ctx context.Context,
	detail *models.ResourceInstallationDetail,
) {
	detail.CanControl = true
	status, err := service.observeDockerInstallation(ctx, detail.ResourceInstallationEntity)
	if err != nil {
		detail.State = "unavailable"
		detail.ServiceState = "unknown"
		detail.Health = "unknown"
		detail.HealthReason = err.Error()
		detail.ObservedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		return
	}
	detail.State = status.State
	detail.ServiceState = status.ServiceState
	detail.Health = status.Health
	detail.HealthReason = status.HealthReason.String
	detail.ContainerDetails = status.Details
	detail.ObservedAt = sql.NullTime{Time: status.ObservedAt, Valid: true}
}

func (service *ResourceManagement) observeDockerInstallation(
	ctx context.Context,
	installation models.ResourceInstallationEntity,
) (models.ResourceInstallationStatusEntity, error) {
	capability, err := service.resourceCapability(ctx, installation.ResourceID)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	state, err := service.container.Inspect(
		ctx,
		installation.ServerID,
		capability,
		installation.ID.String(),
		installation.ContainerName,
	)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	details, err := json.Marshal(state)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	now := time.Now().UTC()
	serviceState := state.Status
	stateValue := "installed"
	health := state.Health
	reason := state.Error
	if !state.Exists {
		stateValue = "missing"
		serviceState = "not-created"
		health = "unknown"
		reason = "No Docker container exists for this installation."
	} else if health == "" {
		health = "unknown"
	}
	status, err := models.ResourceInstallationStatus.Upsert(
		ctx,
		service.db.Executor(),
		models.CreateResourceInstallationStatusData{
			ResourceInstallationID: installation.ID,
			ExternalID:             nullableString(state.ID), State: stateValue,
			InstalledVersion: nullableString(state.ImageID), ServiceState: serviceState,
			Health: health, Source: "docker", HealthReason: nullableString(reason),
			Details: details, ObservedAt: now, ExpiresAt: now.Add(30 * time.Second),
		},
	)
	return status, err
}

func (service *ResourceManagement) DeployResource(ctx context.Context, resourceID uuid.UUID) error {
	_, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return err
	}
	installations, err := models.ResourceInstallation.ActiveForResource(
		ctx, service.db.Executor(), resourceID,
	)
	if err != nil {
		return err
	}
	if len(installations) == 0 {
		return errors.New("Resource has no active installations to deploy")
	}
	for _, installation := range installations {
		if err := service.RunInstallation(ctx, resourceID, installation.ID); err != nil {
			return fmt.Errorf("deploy installation %q: %w", installation.ContainerName, err)
		}
	}
	return nil
}

func (service *ResourceManagement) validatePlacement(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
	capability models.ServerCapability,
) error {
	_, err := models.RequireServerCapability(ctx, db, serverID, capability)
	return err
}

func managedResourceCapability(kind string) models.ServerCapability {
	if kind == "registry" {
		return models.ServerCapabilityRepository
	}
	return models.ServerCapabilityResource
}

func (service *ResourceManagement) resourceCapability(
	ctx context.Context,
	resourceID uuid.UUID,
) (models.ServerCapability, error) {
	kind, err := models.Resource.EngineForID(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return "", err
	}
	return managedResourceCapability(kind), nil
}

func (service *ResourceManagement) validateInstallationMove(
	ctx context.Context,
	db storage.Executor,
	installationID, targetServerID uuid.UUID,
) error {
	conflicts, err := models.ResourceInstallation.MoveConflicts(
		ctx,
		db,
		installationID,
		targetServerID,
	)
	if err != nil {
		return err
	}
	if conflicts.UnreachableNetworks > 0 {
		return domainError(
			"serverId",
			"network_topology",
			"target Server cannot reach every endpoint private network",
		)
	}
	if conflicts.IncompatibleVolumes > 0 {
		return domainError(
			"serverId",
			"storage_topology",
			"mounted server-local Volume requires explicit migration or reassignment",
		)
	}
	return nil
}

func (service *ResourceManagement) ArchiveInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, &installationID); err != nil {
		return err
	}
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceInstallation.LockActiveForResource(ctx, tx, resourceID, installationID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := models.ResourceInstallation.ActiveMountDependencyCount(
		ctx,
		tx,
		installationID,
	)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"installation",
			"dependency",
			"installation has active endpoints, credentials, mounts, or health checks",
		)
	}
	activePolicy, err := service.installationHasActiveBackupPolicy(ctx, tx, installationID)
	if err != nil {
		return err
	}
	if activePolicy {
		return domainError(
			"installation",
			"backup_policy",
			"pause or archive the active backup policy before archiving this installation",
		)
	}
	now := time.Now().UTC()
	if err := models.ResourceInstallation.ArchiveID(ctx, tx, installationID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) installationHasActiveBackupPolicy(
	ctx context.Context,
	db storage.Executor,
	installationID uuid.UUID,
) (bool, error) {
	return false, nil
}

func (service *ResourceManagement) requireNoActiveRestore(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	installationID *uuid.UUID,
) error {
	count, err := models.ResourceRestore.ActiveCountForResource(ctx, db, resourceID)
	if err != nil {
		return err
	}
	if count > 0 {
		return domainError(
			"resource",
			"restore_active",
			"Resource lifecycle changes are unavailable while a database restore is active",
		)
	}
	return nil
}
