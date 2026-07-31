package services

import (
	"context"
	"database/sql"
	containerclient "deploycrate-ce/clients/container"
	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/telemetry"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PermanentDeploymentError struct{ Err error }

func (failure *PermanentDeploymentError) Error() string { return failure.Err.Error() }
func (failure *PermanentDeploymentError) Unwrap() error { return failure.Err }

type deploymentScope struct {
	Deployment       models.DeploymentEntity
	Release          models.ReleaseEntity
	Target           models.EnvironmentTargetEntity
	Environment      models.EnvironmentEntity
	Revision         models.EnvironmentStateRevisionEntity
	State            models.EnvironmentDesiredState
	Instance         models.InstanceEntity
	Domain           models.EnvironmentDomainEntity
	Runtime          models.RuntimeConfigurationEntity
	ApplicationID    uuid.UUID
	RegistryID       uuid.UUID
	RegistryEndpoint string
}

type DeploymentExecution struct {
	db        storage.Pool
	secrets   *EnvironmentSecrets
	builds    *BuildExecution
	caddy     CaddyRouteService
	container containerclient.WorkloadClient
	registry  registryclient.Client
}

func NewDeploymentExecution(db storage.Pool, secrets *EnvironmentSecrets, builds *BuildExecution, caddy CaddyRouteService) *DeploymentExecution {
	return &DeploymentExecution{db: db, secrets: secrets, builds: builds, caddy: caddy, container: containerclient.NewWorkload(), registry: registryclient.New()}
}

func (service *DeploymentExecution) Execute(ctx context.Context, deploymentID uuid.UUID) error {
	deployment, err := service.claim(ctx, deploymentID)
	if err != nil {
		return err
	}
	if deployment.Status == "succeeded" || deployment.Status == "failed" {
		return nil
	}
	fail := func(operationErr error) error {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		now := time.Now().UTC()
		_ = models.Deployment.MarkFailed(persistCtx, service.db.Executor(), deploymentID, operationErr, now)
		_ = models.Change.MarkFailed(persistCtx, service.db.Executor(), deployment.ChangeID, operationErr, now)
		_, _ = service.db.Executor().NewUpdate().TableExpr("environment_target_states AS state").Set("state = 'failed'").
			Set("applying_revision_id = NULL").Set("updated_at = ?", now).
			Where("EXISTS (SELECT 1 FROM deployments deployment WHERE deployment.id = ? AND deployment.environment_target_id = state.environment_target_id)", deploymentID).Exec(persistCtx)
		_, _ = service.db.Executor().NewUpdate().TableExpr("instances").Set("state = 'failed'").Set("updated_at = ?", now).
			Where("deployment_id = ?", deploymentID).Where("state <> 'serving'").Exec(persistCtx)
		_ = service.recordEvent(persistCtx, deploymentID, "failed", "failed", operationErr.Error(), operationErr)
		return &PermanentDeploymentError{Err: operationErr}
	}
	scope, err := service.loadScope(ctx, deployment)
	if err != nil {
		return fail(err)
	}
	if err := service.advance(ctx, deploymentID, "resolving_secrets"); err != nil {
		return err
	}
	if _, err := service.db.Executor().NewUpdate().TableExpr("environment_target_states").
		Set("state = 'applying'").Set("applying_revision_id = ?", scope.Revision.ID).Set("updated_at = ?", time.Now().UTC()).
		Where("environment_target_id = ?", scope.Target.ID).Exec(ctx); err != nil {
		return err
	}
	resolved, err := service.secrets.ResolveRevision(ctx, scope.Revision)
	if err != nil {
		return fail(fmt.Errorf("resolve exact Environment revision secrets: %w", err))
	}
	networkName, err := service.container.ReconcileNetwork(ctx, scope.Environment.ID)
	if err != nil {
		return err
	}
	environment, resourceAttachments, err := service.composeEnvironment(ctx, scope, resolved)
	if err != nil {
		return fail(err)
	}
	for _, attachment := range resourceAttachments {
		if err := service.container.ConnectResourceContainer(ctx, scope.Environment.ID, attachment); err != nil {
			return fail(err)
		}
	}
	if err := service.advance(ctx, deploymentID, "docker_candidate"); err != nil {
		return err
	}
	credentials, err := service.builds.RegistryCredentials(ctx, scope.RegistryID, scope.RegistryEndpoint)
	if err != nil {
		return err
	}
	authentication, err := service.registry.Authenticate(ctx, credentials)
	if err != nil {
		return err
	}
	defer authentication.Close()
	if err := service.registry.Pull(ctx, authentication, scope.Release.ArtifactReference); err != nil {
		return err
	}
	candidate, err := service.container.Find(ctx, scope.Deployment.ID, scope.Instance.ID)
	if err != nil {
		return err
	}
	if candidate.Exists {
		if err := validateCandidateOwnership(candidate, scope); err != nil {
			return fail(err)
		}
	} else {
		candidate, err = service.container.Run(ctx, containerclient.WorkloadRunSpec{
			ApplicationID: scope.ApplicationID, EnvironmentID: scope.Environment.ID, DeploymentID: scope.Deployment.ID,
			InstanceID: scope.Instance.ID, ReleaseID: scope.Release.ID,
			ContainerName:  "dc-app-" + scope.Environment.ID.String() + "-" + scope.Deployment.ID.String(),
			ImageReference: scope.Release.ArtifactReference, NetworkName: networkName, RestartPolicy: "unless-stopped",
			ContainerPort: scope.State.Runtime.ContainerPort, Environment: environment, DockerEnvironment: authentication.Environment(),
		})
		if err != nil {
			return err
		}
	}
	if candidate.HostPort == 0 {
		return fail(errors.New("candidate workload did not publish its loopback port"))
	}
	encodedPorts, _ := json.Marshal(map[string]int32{"http": candidate.HostPort})
	ports := json.RawMessage(encodedPorts)
	instanceState := "running"
	if !candidate.Running {
		instanceState = "failed"
	}
	if _, err := service.db.Executor().NewUpdate().TableExpr("instances").Set("external_id = ?", candidate.ID).
		Set("state = ?", instanceState).Set("ports = ?", ports).Set("observed_at = ?", time.Now().UTC()).Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", scope.Instance.ID).Exec(ctx); err != nil {
		return err
	}
	route, previous, first, err := service.prepareCaddy(ctx, scope)
	if err != nil {
		return err
	}
	if _, err := service.caddy.Reconcile(ctx, route.ID); err != nil {
		return err
	}
	if err := service.caddy.Verify(ctx, route.ExternalID); err != nil {
		return err
	}
	if !candidate.Running {
		return fail(errors.New("candidate workload exited after publishing its loopback port"))
	}
	if err := waitForWorkloadHealth(ctx, candidate.HostPort, scope.State.Runtime.HealthPath); err != nil {
		_ = service.container.Remove(context.WithoutCancel(ctx), scope.Deployment.ID, scope.Instance.ID)
		return fail(err)
	}
	if err := service.advance(ctx, deploymentID, "traffic_switch"); err != nil {
		return err
	}
	if !first {
		weights := map[uuid.UUID]int32{scope.Instance.ID: 100}
		for _, old := range previous {
			weights[old.ID] = 0
		}
		if err := service.caddy.SwitchTraffic(ctx, route.ID, scope.Release.ID, weights); err != nil {
			return err
		}
	}
	if err := service.caddy.Verify(ctx, route.ExternalID); err != nil {
		return err
	}
	if err := service.caddy.VerifyPublic(ctx, scope.Domain.Hostname, scope.State.Runtime.HealthPath); err != nil {
		if !first && len(previous) > 0 {
			rollback := map[uuid.UUID]int32{scope.Instance.ID: 0}
			fallback := uuid.Nil
			for _, old := range previous {
				rollback[old.ID] = 0
				if fallback == uuid.Nil && old.State == "serving" {
					fallback = old.ID
				}
			}
			if fallback != uuid.Nil {
				rollback[fallback] = 100
				_ = service.caddy.SwitchTraffic(context.WithoutCancel(ctx), route.ID, previousRelease(previous, fallback), rollback)
				if service.container.Remove(context.WithoutCancel(ctx), scope.Deployment.ID, scope.Instance.ID) == nil {
					_ = service.caddy.RemoveBackend(context.WithoutCancel(ctx), route.ID, scope.Instance.ID)
				}
			}
		}
		return fail(fmt.Errorf("public Environment health verification failed: %w", err))
	}
	if err := service.markSucceeded(ctx, scope, candidate, ports); err != nil {
		return err
	}
	for _, old := range previous {
		if err := service.container.Remove(ctx, old.DeploymentID, old.ID); err != nil {
			_ = service.recordEvent(ctx, deploymentID, "cleanup", "warning", "previous container cleanup will be retried", err)
			continue
		}
		if err := service.caddy.RemoveBackend(ctx, route.ID, old.ID); err != nil {
			_ = service.recordEvent(ctx, deploymentID, "cleanup", "warning", "previous backend cleanup will be retried", err)
			continue
		}
		now := time.Now().UTC()
		_, _ = service.db.Executor().NewUpdate().TableExpr("instances").Set("state = 'removed'").Set("removed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", old.ID).Exec(ctx)
	}
	if err := cleanupUnroutedWorkloadInstances(ctx, service.db, service.container, scope.Target.ID); err != nil {
		_ = service.recordEvent(ctx, deploymentID, "cleanup", "warning", "stale candidate cleanup will be retried", err)
	}
	if err := service.container.PruneResourceContainers(ctx, scope.Environment.ID, resourceAttachments); err != nil {
		_ = service.recordEvent(ctx, deploymentID, "cleanup", "warning", "stale Resource network access cleanup will be retried", err)
	}
	return nil
}

func (service *DeploymentExecution) advance(ctx context.Context, deploymentID uuid.UUID, step string) error {
	now := time.Now().UTC()
	if _, err := service.db.Executor().NewUpdate().TableExpr("deployments").Set("current_step = ?", step).
		Set("updated_at = ?", now).Where("id = ?", deploymentID).Where("status = 'running'").Exec(ctx); err != nil {
		return err
	}
	return service.recordEvent(ctx, deploymentID, "progress", "running", step, nil)
}

func previousRelease(instances []models.InstanceEntity, id uuid.UUID) uuid.UUID {
	for _, instance := range instances {
		if instance.ID == id {
			return instance.ReleaseID
		}
	}
	return uuid.Nil
}

func (service *DeploymentExecution) Fail(ctx context.Context, deploymentID uuid.UUID, operationErr error) error {
	deployment, err := models.Deployment.Find(ctx, service.db.Executor(), deploymentID)
	if err != nil || deployment.Status == "succeeded" || deployment.Status == "failed" {
		return err
	}
	now := time.Now().UTC()
	if err := models.Deployment.MarkFailed(ctx, service.db.Executor(), deployment.ID, operationErr, now); err != nil {
		return err
	}
	_, _ = service.db.Executor().NewUpdate().TableExpr("environment_target_states").Set("state = 'failed'").
		Set("applying_revision_id = NULL").Set("updated_at = ?", now).Where("environment_target_id = ?", deployment.EnvironmentTargetID).Exec(ctx)
	return models.Change.MarkFailed(ctx, service.db.Executor(), deployment.ChangeID, operationErr, now)
}

func (service *DeploymentExecution) claim(ctx context.Context, id uuid.UUID) (models.DeploymentEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	defer tx.Rollback()
	deployment, err := models.Deployment.Lock(ctx, tx, id)
	if err != nil {
		return deployment, err
	}
	if deployment.Status == "queued" || deployment.Status == "running" {
		now := time.Now().UTC()
		if err := models.Deployment.MarkRunning(ctx, tx, id, "preflight", now); err != nil {
			return deployment, err
		}
		if err := models.Change.MarkRunning(ctx, tx, deployment.ChangeID, now); err != nil {
			return deployment, err
		}
		deployment.Status = "running"
	}
	return deployment, tx.Commit()
}

func (service *DeploymentExecution) loadScope(ctx context.Context, deployment models.DeploymentEntity) (deploymentScope, error) {
	scope := deploymentScope{Deployment: deployment}
	var err error
	scope.Release, err = models.Release.Find(ctx, service.db.Executor(), deployment.ReleaseID)
	if err != nil {
		return scope, err
	}
	scope.Target, err = models.EnvironmentTarget.Find(ctx, service.db.Executor(), deployment.EnvironmentTargetID)
	if err != nil {
		return scope, err
	}
	if scope.Target.DetachedAt.Valid || scope.Release.EnvironmentID != scope.Target.EnvironmentID {
		return scope, errors.New("Deployment Release and target do not belong to the same active Environment")
	}
	scope.Environment, err = models.Environment.Find(ctx, service.db.Executor(), scope.Target.EnvironmentID)
	if err != nil || scope.Environment.ArchivedAt.Valid {
		return scope, errors.New("Deployment Environment is unavailable")
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, service.db.Executor(), scope.Environment.ID)
	if err != nil || !setupComplete {
		return scope, errors.New("Deployment Environment setup is incomplete")
	}
	scope.ApplicationID = scope.Environment.ApplicationID
	err = service.db.Executor().NewSelect().Model(&scope.Revision).
		Join("JOIN change_state_revisions AS association ON association.environment_state_revision_id = environment_state_revisions.id").
		Where("association.change_id = ?", deployment.ChangeID).Where("association.role = 'result'").Limit(1).Scan(ctx)
	if err != nil || scope.Revision.EnvironmentID != scope.Environment.ID {
		return scope, errors.New("Deployment state revision is unavailable or mismatched")
	}
	scope.State, err = models.ParseEnvironmentDesiredState(scope.Revision.State)
	if err != nil {
		return scope, err
	}
	err = service.db.Executor().NewSelect().Model(&scope.Instance).Where("deployment_id = ?", deployment.ID).Where("removed_at IS NULL").Limit(1).Scan(ctx)
	if err != nil || scope.Instance.ReleaseID != scope.Release.ID || scope.Instance.EnvironmentTargetID != scope.Target.ID {
		return scope, errors.New("Deployment candidate Instance is unavailable or mismatched")
	}
	scope.Domain, err = models.EnvironmentDomain.Find(ctx, service.db.Executor(), scope.State.Domain.ID)
	if err != nil || scope.Domain.EnvironmentID != scope.Environment.ID || scope.Domain.ArchivedAt.Valid || !scope.Domain.IsPrimary || scope.Domain.Hostname != scope.State.Domain.Hostname {
		return scope, errors.New("Deployment primary domain is unavailable or mismatched")
	}
	err = service.db.Executor().NewSelect().Model(&scope.Runtime).Where("environment_id = ?", scope.Environment.ID).Limit(1).Scan(ctx)
	if err != nil || scope.Runtime.Runtime != "go" || scope.Runtime.Replicas != 1 || scope.Runtime.RestartPolicy != "unless-stopped" {
		return scope, errors.New("Deployment Go runtime configuration is unavailable")
	}
	if scope.Release.BuildID == nil {
		return scope, errors.New("workload Release has no Build")
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), *scope.Release.BuildID)
	if err != nil || build.EnvironmentID != scope.Environment.ID || build.Status != "succeeded" {
		return scope, errors.New("workload Release Build is unavailable")
	}
	snapshot, err := parseBuildSnapshot(build)
	if err != nil {
		return scope, errors.New("workload Release Build snapshot is invalid")
	}
	scope.RegistryID, scope.RegistryEndpoint = snapshot.ContainerRegistryID, snapshot.RegistryEndpoint
	return scope, nil
}

func (service *DeploymentExecution) composeEnvironment(ctx context.Context, scope deploymentScope, secrets []ResolvedEnvironmentSecret) (map[string]string, []containerclient.ResourceContainerAttachment, error) {
	values := map[string]string{
		"PORT":                       strconv.Itoa(int(scope.State.Runtime.ContainerPort)),
		"DEPLOYCRATE_APPLICATION_ID": scope.ApplicationID.String(),
		"DEPLOYCRATE_ENVIRONMENT_ID": scope.Environment.ID.String(),
		"DEPLOYCRATE_RELEASE_ID":     scope.Release.ID.String(),
	}
	put := func(key, value string) error {
		if _, exists := values[key]; exists {
			return fmt.Errorf("runtime variable %s conflicts with a platform or Resource value", key)
		}
		values[key] = value
		return nil
	}
	type dockerEndpoint struct {
		host string
		port int32
	}
	attachments := make([]containerclient.ResourceContainerAttachment, 0, len(scope.State.Resources))
	dockerURLs := make(map[string]dockerEndpoint)
	for _, resourceState := range scope.State.Resources {
		connection, err := models.EnvironmentResource.Find(ctx, service.db.Executor(), resourceState.EnvironmentResourceID)
		if err != nil || connection.EnvironmentID != scope.Environment.ID || connection.ResourceID != resourceState.ResourceID || connection.ResourceEndpointID != resourceState.EndpointID || connection.ArchivedAt.Valid {
			return nil, nil, errors.New("Environment Resource connection is unavailable or mismatched")
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), connection.ResourceID)
		if err != nil || resource.ArchivedAt.Valid || resource.Kind != resourceState.Kind {
			return nil, nil, errors.New("Environment Resource is unavailable or mismatched")
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), connection.ResourceEndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
			return nil, nil, errors.New("Environment Resource endpoint is unavailable or mismatched")
		}
		if (connection.ResourceCredentialID == nil) != (resourceState.CredentialID == nil) || (connection.ResourceCredentialID != nil && *connection.ResourceCredentialID != *resourceState.CredentialID) {
			return nil, nil, errors.New("Environment Resource credential projection is mismatched")
		}
		if connection.ResourceCredentialID != nil {
			credential, err := models.ResourceCredential.Find(ctx, service.db.Executor(), *connection.ResourceCredentialID)
			if err != nil || credential.ArchivedAt.Valid || credential.ResourceID != resource.ID {
				return nil, nil, errors.New("Environment Resource credential is unavailable or mismatched")
			}
		}
		var projected *dockerEndpoint
		if endpoint.ResourceInstallationID != nil {
			installation, err := models.ResourceInstallation.Find(ctx, service.db.Executor(), *endpoint.ResourceInstallationID)
			if err != nil || installation.ArchivedAt.Valid || installation.ResourceID != resource.ID || installation.ServerID != scope.Target.ServerID {
				return nil, nil, errors.New("Docker Resource installation is unavailable or not colocated with the Environment target")
			}
			var configuration struct {
				PortMappings []models.ResourceInstallationPortMapping `json:"portMappings"`
			}
			definition, supported := models.FindResourceKind(resource.Kind)
			if json.Unmarshal(installation.Configuration, &configuration) != nil || len(configuration.PortMappings) != 1 || !supported {
				return nil, nil, errors.New("Docker Resource installation port mapping is invalid")
			}
			mapping := configuration.PortMappings[0]
			if mapping.HostPort != endpoint.Port || mapping.ContainerPort != definition.DefaultPort || mapping.Protocol != "tcp" {
				return nil, nil, errors.New("Docker Resource endpoint does not match its container port mapping")
			}
			attachment := containerclient.ResourceContainerAttachment{InstallationID: installation.ID, ContainerName: installation.ContainerName}
			attachments = append(attachments, attachment)
			projected = &dockerEndpoint{host: installation.ContainerName, port: mapping.ContainerPort}
			var connectionConfiguration struct {
				CredentialProjection string `json:"credential_projection"`
			}
			if json.Unmarshal(connection.Configuration, &connectionConfiguration) != nil {
				return nil, nil, errors.New("Docker Resource credential projection is invalid")
			}
			switch connectionConfiguration.CredentialProjection {
			case resourceCredentialProjectionConnectionURL:
				dockerURLs[resourceState.Alias+"_URL"] = *projected
			case resourceCredentialProjectionIndividualParts:
			default:
				return nil, nil, errors.New("Docker Resource credential projection is unsupported")
			}
		}
		for key, value := range resourceState.Variables {
			if projected != nil {
				switch key {
				case resourceState.Alias + "_HOST":
					value = projected.host
				case resourceState.Alias + "_PORT":
					value = strconv.Itoa(int(projected.port))
				}
			}
			if err := put(key, value); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, secret := range secrets {
		value := secret.Value
		if endpoint, exists := dockerURLs[secret.Key]; exists {
			connectionURL, err := url.Parse(value)
			if err != nil || connectionURL.Scheme == "" || connectionURL.User == nil {
				return nil, nil, fmt.Errorf("Docker Resource connection URL %s is invalid", secret.Key)
			}
			connectionURL.Host = net.JoinHostPort(endpoint.host, strconv.Itoa(int(endpoint.port)))
			value = connectionURL.String()
		}
		if err := put(secret.Key, value); err != nil {
			return nil, nil, err
		}
	}
	return values, attachments, nil
}

func validateCandidateOwnership(candidate containerclient.WorkloadState, scope deploymentScope) error {
	expected := map[string]string{
		"com.deploycrate.application": scope.ApplicationID.String(), "com.deploycrate.environment": scope.Environment.ID.String(),
		"com.deploycrate.deployment": scope.Deployment.ID.String(), "com.deploycrate.instance": scope.Instance.ID.String(),
		"com.deploycrate.release": scope.Release.ID.String(),
	}
	for key, value := range expected {
		if candidate.Labels[key] != value {
			return errors.New("existing workload container has invalid ownership labels")
		}
	}
	if candidate.ImageReference != scope.Release.ArtifactReference {
		return errors.New("existing workload container uses the wrong immutable Release image")
	}
	return nil
}

func waitForWorkloadHealth(ctx context.Context, port int32, healthPath string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	check := func() bool {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
		if healthPath == "" {
			connection, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				_ = connection.Close()
				return true
			}
			return false
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+healthPath, nil)
		response, err := telemetry.NewHTTPClient(2 * time.Second).Do(request)
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode >= 200 && response.StatusCode < 400
	}
	for {
		if check() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("candidate workload health check timed out")
		case <-ticker.C:
		}
	}
}

func (service *DeploymentExecution) prepareCaddy(ctx context.Context, scope deploymentScope) (models.CaddyRouteEntity, []models.InstanceEntity, bool, error) {
	var route models.CaddyRouteEntity
	err := service.db.Executor().NewSelect().Model(&route).Where("environment_target_id = ?", scope.Target.ID).
		Where("environment_domain_id = ?", scope.Domain.ID).Where("removed_at IS NULL").OrderExpr("created_at").Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		route, err = models.CaddyRoute.Create(ctx, service.db.Executor(), models.CreateCaddyRouteData{
			ExternalID: "deploycrate_environment_" + strings.ReplaceAll(scope.Environment.ID.String(), "-", ""), State: "pending",
			EnvironmentTargetID: scope.Target.ID, EnvironmentDomainID: scope.Domain.ID, ReleaseID: scope.Release.ID,
		})
		if err != nil {
			return route, nil, false, err
		}
		_, err = models.CaddyRouteBackend.Create(ctx, service.db.Executor(), models.CreateCaddyRouteBackendData{Weight: 100, CaddyRouteID: route.ID, InstanceID: scope.Instance.ID})
		return route, nil, true, err
	}
	if err != nil {
		return route, nil, false, err
	}
	previous := make([]models.InstanceEntity, 0)
	if err := service.db.Executor().NewSelect().Model(&previous).
		Join("JOIN caddy_route_backends AS backend ON backend.instance_id = instances.id").
		Where("backend.caddy_route_id = ?", route.ID).Where("backend.removed_at IS NULL").Where("instances.id <> ?", scope.Instance.ID).
		Where("instances.removed_at IS NULL").OrderExpr("instances.created_at DESC").Scan(ctx); err != nil {
		return route, nil, false, err
	}
	count, err := service.db.Executor().NewSelect().Model((*models.CaddyRouteBackendEntity)(nil)).Where("caddy_route_id = ?", route.ID).
		Where("instance_id = ?", scope.Instance.ID).Where("removed_at IS NULL").Count(ctx)
	if err != nil {
		return route, nil, false, err
	}
	if count == 0 {
		if err := service.caddy.AddBackend(ctx, route.ID, scope.Instance.ID, 0); err != nil {
			return route, nil, false, err
		}
	}
	return route, previous, false, nil
}

func (service *DeploymentExecution) markSucceeded(ctx context.Context, scope deploymentScope, candidate containerclient.WorkloadState, ports json.RawMessage) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	deployment, err := models.Deployment.Lock(ctx, tx, scope.Deployment.ID)
	if err != nil {
		return err
	}
	if deployment.Status == "succeeded" {
		return tx.Commit()
	}
	if deployment.Status != "running" {
		return errors.New("Deployment is no longer running")
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("instances").Set("external_id = ?", candidate.ID).Set("state = 'serving'").Set("ports = ?", ports).
		Set("observed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", scope.Instance.ID).Exec(ctx); err != nil {
		return err
	}
	encodedObserved, _ := json.Marshal(map[string]any{"schema_version": 1, "container_id": candidate.ID, "image_id": candidate.ImageID, "host_port": candidate.HostPort})
	observed := json.RawMessage(encodedObserved)
	if _, err := tx.NewUpdate().TableExpr("environment_target_states").Set("state = 'applied'").Set("observed_state = ?", observed).
		Set("applying_revision_id = NULL").Set("applied_revision_id = ?", scope.Revision.ID).Set("observed_at = ?", now).Set("updated_at = ?", now).
		Where("environment_target_id = ?", scope.Target.ID).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("deployments").Set("status = 'succeeded'").Set("current_step = 'serving'").Set("finished_at = ?", now).
		Set("error = NULL").Set("updated_at = ?", now).Where("id = ?", scope.Deployment.ID).Where("status = 'running'").Exec(ctx); err != nil {
		return err
	}
	if err := models.Change.MarkCompleted(ctx, tx, scope.Deployment.ChangeID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return service.recordEvent(ctx, scope.Deployment.ID, "serving", "succeeded", "candidate is serving public traffic", nil)
}

func (service *DeploymentExecution) recordEvent(ctx context.Context, deploymentID uuid.UUID, eventType, status, message string, operationErr error) error {
	var sequence int64
	if err := service.db.Executor().NewSelect().TableExpr("deployment_events").ColumnExpr("COALESCE(MAX(sequence), 0) + 1").Where("deployment_id = ?", deploymentID).Scan(ctx, &sequence); err != nil {
		return err
	}
	failure := sql.NullString{}
	if operationErr != nil {
		value := operationErr.Error()
		if len(value) > 2048 {
			value = value[:2048]
		}
		failure = sql.NullString{String: value, Valid: true}
	}
	_, err := models.DeploymentEvent.Create(ctx, service.db.Executor(), models.CreateDeploymentEventData{
		Sequence: sequence, EventType: eventType, Status: sql.NullString{String: status, Valid: status != ""},
		Message: message, Metadata: json.RawMessage(`{}`), Error: failure, OccurredAt: time.Now().UTC(), DeploymentID: deploymentID,
	})
	return err
}
