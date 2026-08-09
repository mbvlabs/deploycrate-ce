package services

import (
	"context"
	"database/sql"
	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/telemetry"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PermanentDeploymentError struct{ Err error }

func (failure *PermanentDeploymentError) Error() string { return failure.Err.Error() }
func (failure *PermanentDeploymentError) Unwrap() error { return failure.Err }

type deploymentScope struct {
	Deployment           models.DeploymentEntity
	Release              models.ReleaseEntity
	Target               models.EnvironmentTargetEntity
	Environment          models.EnvironmentEntity
	Application          models.ApplicationEntity
	Revision             models.EnvironmentStateRevisionEntity
	State                models.EnvironmentDesiredState
	Instance             models.InstanceEntity
	Instances            []models.InstanceEntity
	Domain               models.EnvironmentDomainEntity
	Runtime              models.RuntimeConfigurationEntity
	ApplicationID        uuid.UUID
	RegistryID           uuid.UUID
	RegistryCredentialID uuid.UUID
	RegistryEndpoint     string
}

type DeploymentExecution struct {
	db        storage.Pool
	secrets   *EnvironmentSecrets
	builds    *BuildExecution
	caddy     CaddyRouteService
	workloads *WorkloadExecution
}

func NewDeploymentExecution(
	db storage.Pool,
	secrets *EnvironmentSecrets,
	builds *BuildExecution,
	caddy CaddyRouteService,
	workloads *WorkloadExecution,
) *DeploymentExecution {
	return &DeploymentExecution{
		db:        db,
		secrets:   secrets,
		builds:    builds,
		caddy:     caddy,
		workloads: workloads,
	}
}

func (service *DeploymentExecution) Execute(ctx context.Context, deploymentID uuid.UUID) error {
	deployment, err := service.claim(ctx, deploymentID)
	if err != nil {
		return err
	}
	if deployment.Status == "succeeded" || deployment.Status == "failed" {
		return nil
	}
	if err := service.recordEvent(
		ctx,
		deploymentID,
		"progress",
		"running",
		"preflight",
		"Deployment claimed, running preflight checks",
		nil,
	); err != nil {
		return err
	}
	fail := func(operationErr error) error {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		now := time.Now().UTC()
		_ = models.Deployment.MarkFailed(
			persistCtx,
			service.db.Executor(),
			deploymentID,
			operationErr,
			now,
		)
		_ = models.Change.MarkFailed(
			persistCtx,
			service.db.Executor(),
			deployment.ChangeID,
			operationErr,
			now,
		)
		_, _ = service.db.Executor().
			NewUpdate().
			TableExpr("environment_target_states AS state").
			Set("state = 'failed'").
			Set("applying_revision_id = NULL").
			Set("updated_at = ?", now).
			Where("EXISTS (SELECT 1 FROM deployments deployment WHERE deployment.id = ? AND deployment.environment_target_id = state.environment_target_id)", deploymentID).
			Exec(persistCtx)
		_, _ = service.db.Executor().
			NewUpdate().
			TableExpr("instances").
			Set("state = 'failed'").
			Set("updated_at = ?", now).
			Where("deployment_id = ?", deploymentID).
			Where("state <> 'serving'").
			Exec(persistCtx)
		_ = service.recordEvent(
			persistCtx,
			deploymentID,
			"failed",
			"failed",
			"failed",
			operationErr.Error(),
			operationErr,
		)
		return &PermanentDeploymentError{Err: operationErr}
	}
	scope, err := service.loadScope(ctx, deployment)
	if err != nil {
		return fail(err)
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		scope.Environment.ID,
	)
	if err != nil {
		return err
	}
	if !deployability.Deployable {
		return fail(
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(deployability.Missing, ", "),
			),
		)
	}
	if err := service.advance(
		ctx,
		deploymentID,
		"resolving_secrets",
		"Resolving Environment secrets and configuration",
	); err != nil {
		return err
	}
	resolutionStarted := time.Now()
	if _, err := service.db.Executor().NewUpdate().TableExpr("environment_target_states").
		Set("state = 'applying'").
		Set("applying_revision_id = ?", scope.Revision.ID).Set("updated_at = ?", time.Now().UTC()).
		Where("environment_target_id = ?", scope.Target.ID).Exec(ctx); err != nil {
		return err
	}
	resolved, err := service.secrets.ResolveRevision(ctx, scope.Revision)
	if err != nil {
		return fail(fmt.Errorf("resolve exact Environment revision secrets: %w", err))
	}
	networkName, err := service.workloads.ReconcileNetwork(
		ctx,
		scope.Target.ServerID,
		scope.Environment.ID,
	)
	if err != nil {
		return err
	}
	_, resourceAttachments, err := service.composeEnvironment(ctx, scope, resolved, false)
	if err != nil {
		return fail(err)
	}
	for _, attachment := range resourceAttachments {
		if err := service.workloads.ConnectResourceContainer(
			ctx,
			scope.Target.ServerID,
			scope.Environment.ID,
			attachment,
		); err != nil {
			return fail(err)
		}
	}
	service.recordTiming(
		ctx,
		deploymentID,
		"resolving_secrets",
		"Secrets and Resource configuration",
		resolutionStarted,
	)
	if err := service.advance(
		ctx,
		deploymentID,
		"docker_candidate",
		"Pulling candidate image and preparing workloads",
	); err != nil {
		return err
	}
	candidateStarted := time.Now()
	credentials, err := service.builds.RegistryCredentials(
		ctx,
		scope.RegistryID,
		scope.RegistryCredentialID,
		scope.RegistryEndpoint,
	)
	if err != nil {
		return err
	}
	candidates := make(map[uuid.UUID]containerclient.WorkloadState, len(scope.Instances))
	processes := make(map[string]models.EnvironmentProcessState, len(scope.State.Processes))
	for _, process := range scope.State.Processes {
		processes[process.Name] = process
	}
	startInstance := func(instance models.InstanceEntity) error {
		process, exists := processes[instance.ProcessName]
		if !exists || process.Kind != instance.ProcessKind {
			return errors.New("Deployment Instance process snapshot is mismatched")
		}
		_ = service.recordEvent(
			ctx,
			scope.Deployment.ID,
			"instance",
			"running",
			"instance_start",
			fmt.Sprintf(
				"starting %s process %s (replica %s) from %s",
				instance.ProcessKind,
				instance.ProcessName,
				instance.ReplicaKey,
				scope.Release.ArtifactReference,
			),
			nil,
		)
		includePort := process.Kind == models.EnvironmentProcessWeb
		environment, _, err := service.composeEnvironment(ctx, scope, resolved, includePort)
		if err != nil {
			return err
		}
		candidate, err := service.workloads.Find(
			ctx,
			scope.Target.ServerID,
			scope.Deployment.ID,
			instance.ID,
		)
		if err != nil {
			return err
		}
		if candidate.Exists {
			if err := validateCandidateOwnership(candidate, scope, instance); err != nil {
				return err
			}
		} else {
			command := make([]string, 0, len(process.Arguments)+1)
			if process.Command != nil {
				command = append(command, *process.Command)
			}
			command = append(command, process.Arguments...)
			candidate, err = service.workloads.Run(
				ctx,
				scope.Target.ServerID,
				containerclient.WorkloadRunSpec{
					ApplicationID:  scope.ApplicationID,
					EnvironmentID:  scope.Environment.ID,
					TargetID:       scope.Target.ID,
					DeploymentID:   scope.Deployment.ID,
					InstanceID:     instance.ID,
					ReleaseID:      scope.Release.ID,
					ProcessName:    instance.ProcessName,
					ProcessKind:    instance.ProcessKind,
					ProcessReplica: instance.ReplicaKey,
					ContainerName: scope.Application.Slug + "-" + scope.Environment.Slug + "-" + scope.Environment.ID.String() + "-" + scope.Deployment.ID.String() + "-" + instance.ProcessName + "-" + strings.ReplaceAll(
						instance.ReplicaKey,
						"/",
						"-",
					),
					ImageReference: scope.Release.ArtifactReference,
					NetworkName:    networkName,
					RestartPolicy:  "unless-stopped",
					ContainerPort:  process.ContainerPort,
					Environment:    environment,
					Command:        command,
				},
				credentials,
			)
			if err != nil {
				return err
			}
		}
		ports := json.RawMessage(`{}`)
		if includePort {
			if candidate.HostAddress == "" || candidate.HostPort == 0 {
				return errors.New(
					"candidate web workload did not publish its target address and port",
				)
			}
			encoded, _ := json.Marshal(
				map[string]any{"host": candidate.HostAddress, "http": candidate.HostPort},
			)
			ports = encoded
		}
		instanceState := "running"
		if !candidate.Running {
			instanceState = "failed"
		}
		now := time.Now().UTC()
		if _, err := service.db.Executor().
			NewUpdate().
			TableExpr("instances").
			Set("external_id = ?", candidate.ID).
			Set("state = ?", instanceState).
			Set("ports = ?", ports).
			Set("observed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", instance.ID).
			Exec(ctx); err != nil {
			return err
		}
		if !candidate.Running {
			return fmt.Errorf(
				"candidate %s process %s exited during startup",
				process.Kind,
				process.Name,
			)
		}
		candidates[instance.ID] = candidate
		return nil
	}
	if err := startInstance(scope.Instance); err != nil {
		service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
		return fail(err)
	}
	service.recordTiming(
		ctx,
		deploymentID,
		"docker_candidate",
		"Candidate web workload startup",
		candidateStarted,
	)
	webProcess, _ := scope.State.WebProcess()
	webCandidate := candidates[scope.Instance.ID]
	if err := service.recordEvent(
		ctx,
		deploymentID,
		"health",
		"running",
		"health_check",
		fmt.Sprintf(
			"Waiting for candidate web workload to become healthy at 0.0.0.0:%d%s",
			webCandidate.HostPort,
			webProcess.HealthPath,
		),
		nil,
	); err != nil {
		return err
	}
	healthStarted := time.Now()
	if err := waitForWorkloadHealth(
		ctx,
		webCandidate.HostAddress,
		webCandidate.HostPort,
		webProcess.HealthPath,
	); err != nil {
		service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
		return fail(err)
	}
	service.recordTiming(
		ctx,
		deploymentID,
		"health_check",
		"Candidate health checks",
		healthStarted,
	)
	if err := service.advance(
		ctx,
		deploymentID,
		"worker_candidates",
		"Starting worker processes",
	); err != nil {
		return err
	}
	workersStarted := time.Now()
	for _, instance := range scope.Instances {
		if instance.ProcessKind != models.EnvironmentProcessWorker {
			continue
		}
		if err := startInstance(instance); err != nil {
			service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
			return fail(err)
		}
	}
	if err := service.stabilizeWorkers(ctx, scope, candidates); err != nil {
		service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
		return fail(err)
	}
	service.recordTiming(ctx, deploymentID, "worker_candidates", "Worker startup", workersStarted)
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
	if err := service.advance(
		ctx,
		deploymentID,
		"traffic_switch",
		"Routing public traffic to candidate",
	); err != nil {
		return err
	}
	trafficStarted := time.Now()
	if !first {
		weights := map[uuid.UUID]int32{scope.Instance.ID: 100}
		for _, old := range previous {
			weights[old.ID] = 0
		}
		if err := service.caddy.SwitchTraffic(
			ctx,
			route.ID,
			scope.Release.ID,
			weights,
		); err != nil {
			return err
		}
	}
	if err := service.caddy.Verify(ctx, route.ExternalID); err != nil {
		return err
	}
	service.recordTiming(
		ctx,
		deploymentID,
		"traffic_switch",
		"Traffic switch",
		trafficStarted,
	)
	if err := waitForWorkloadHealth(
		ctx,
		webCandidate.HostAddress,
		webCandidate.HostPort,
		webProcess.HealthPath,
	); err != nil {
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
				_ = service.caddy.SwitchTraffic(
					context.WithoutCancel(ctx),
					route.ID,
					previousRelease(previous, fallback),
					rollback,
				)
				service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
				_ = service.caddy.RemoveBackend(
					context.WithoutCancel(ctx),
					route.ID,
					scope.Instance.ID,
				)
			}
		} else if first {
			_ = service.caddy.DestroyManaged(context.WithoutCancel(ctx), route.ID)
			service.removeCandidateFormation(context.WithoutCancel(ctx), scope)
		}
		return fail(
			fmt.Errorf("workload health verification after route configuration failed: %w", err),
		)
	}
	if err := service.markSucceeded(ctx, scope, candidates); err != nil {
		return err
	}
	if err := service.recordEvent(
		ctx,
		deploymentID,
		"progress",
		"running",
		"cleanup",
		"Cleaning up previous Deployment formation",
		nil,
	); err != nil {
		return err
	}
	previousFormation, err := service.previousFormation(ctx, previous)
	if err != nil {
		return err
	}
	for _, old := range previousFormation {
		if err := service.workloads.Remove(
			ctx,
			scope.Target.ServerID,
			old.DeploymentID,
			old.ID,
		); err != nil {
			_ = service.recordEvent(
				ctx,
				deploymentID,
				"cleanup",
				"warning",
				"cleanup",
				"previous container cleanup will be retried",
				err,
			)
			continue
		}
		if old.ProcessKind == models.EnvironmentProcessWeb {
			if err := service.caddy.RemoveBackend(ctx, route.ID, old.ID); err != nil {
				_ = service.recordEvent(
					ctx,
					deploymentID,
					"cleanup",
					"warning",
					"cleanup",
					"previous backend cleanup will be retried",
					err,
				)
				continue
			}
		}
		now := time.Now().UTC()
		_, _ = service.db.Executor().
			NewUpdate().
			TableExpr("instances").
			Set("state = 'removed'").
			Set("removed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", old.ID).
			Exec(ctx)
	}
	if err := cleanupUnroutedWorkloadInstances(
		ctx,
		service.db,
		service.workloads,
		scope.Target.ID,
	); err != nil {
		_ = service.recordEvent(
			ctx,
			deploymentID,
			"cleanup",
			"warning",
			"cleanup",
			"stale candidate cleanup will be retried",
			err,
		)
	}
	if err := service.workloads.PruneResourceContainers(
		ctx,
		scope.Target.ServerID,
		scope.Environment.ID,
		resourceAttachments,
	); err != nil {
		_ = service.recordEvent(
			ctx,
			deploymentID,
			"cleanup",
			"warning",
			"cleanup",
			"stale Resource network access cleanup will be retried",
			err,
		)
	}
	return nil
}

func (service *DeploymentExecution) advance(
	ctx context.Context,
	deploymentID uuid.UUID,
	step, message string,
) error {
	now := time.Now().UTC()
	if _, err := service.db.Executor().
		NewUpdate().
		TableExpr("deployments").
		Set("current_step = ?", step).
		Set("updated_at = ?", now).
		Where("id = ?", deploymentID).
		Where("status = 'running'").
		Exec(ctx); err != nil {
		return err
	}
	return service.recordEvent(ctx, deploymentID, "progress", "running", step, message, nil)
}

func (service *DeploymentExecution) recordTiming(
	ctx context.Context,
	deploymentID uuid.UUID,
	step, label string,
	startedAt time.Time,
) {
	message := fmt.Sprintf(
		"%s completed in %s",
		label,
		max(time.Since(startedAt), 0).Round(time.Millisecond),
	)
	_ = service.recordEvent(ctx, deploymentID, "timing", "running", step, message, nil)
}

func previousRelease(instances []models.InstanceEntity, id uuid.UUID) uuid.UUID {
	for _, instance := range instances {
		if instance.ID == id {
			return instance.ReleaseID
		}
	}
	return uuid.Nil
}

func (service *DeploymentExecution) Fail(
	ctx context.Context,
	deploymentID uuid.UUID,
	operationErr error,
) error {
	deployment, err := models.Deployment.Find(ctx, service.db.Executor(), deploymentID)
	if err != nil || deployment.Status == "succeeded" || deployment.Status == "failed" {
		return err
	}
	now := time.Now().UTC()
	if err := models.Deployment.MarkFailed(
		ctx,
		service.db.Executor(),
		deployment.ID,
		operationErr,
		now,
	); err != nil {
		return err
	}
	_, _ = service.db.Executor().
		NewUpdate().
		TableExpr("environment_target_states").
		Set("state = 'failed'").
		Set("applying_revision_id = NULL").
		Set("updated_at = ?", now).
		Where("environment_target_id = ?", deployment.EnvironmentTargetID).
		Exec(ctx)
	return models.Change.MarkFailed(
		ctx,
		service.db.Executor(),
		deployment.ChangeID,
		operationErr,
		now,
	)
}

func (service *DeploymentExecution) claim(
	ctx context.Context,
	id uuid.UUID,
) (models.DeploymentEntity, error) {
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

func (service *DeploymentExecution) loadScope(
	ctx context.Context,
	deployment models.DeploymentEntity,
) (deploymentScope, error) {
	scope := deploymentScope{Deployment: deployment}
	var err error
	scope.Release, err = models.Release.Find(ctx, service.db.Executor(), deployment.ReleaseID)
	if err != nil {
		return scope, err
	}
	scope.Target, err = models.EnvironmentTarget.Find(
		ctx,
		service.db.Executor(),
		deployment.EnvironmentTargetID,
	)
	if err != nil {
		return scope, err
	}
	if scope.Target.DetachedAt.Valid || scope.Release.EnvironmentID != scope.Target.EnvironmentID {
		return scope, errors.New(
			"Deployment Release and target do not belong to the same active Environment",
		)
	}
	scope.Environment, err = models.Environment.Find(
		ctx,
		service.db.Executor(),
		scope.Target.EnvironmentID,
	)
	if err != nil || scope.Environment.ArchivedAt.Valid {
		return scope, errors.New("Deployment Environment is unavailable")
	}
	setupComplete, err := models.Environment.SetupComplete(
		ctx,
		service.db.Executor(),
		scope.Environment.ID,
	)
	if err != nil || !setupComplete {
		return scope, errors.New("Deployment Environment setup is incomplete")
	}
	scope.ApplicationID = scope.Environment.ApplicationID
	scope.Application, err = models.Application.FindIncludingSystem(
		ctx,
		service.db.Executor(),
		scope.ApplicationID,
	)
	if err != nil || scope.Application.Slug == "" {
		return scope, errors.New("Deployment Application is unavailable")
	}
	err = service.db.Executor().NewSelect().Model(&scope.Revision).
		Join("JOIN change_state_revisions AS association ON association.environment_state_revision_id = environment_state_revisions.id").
		Where("association.change_id = ?", deployment.ChangeID).
		Where("association.role = 'result'").Limit(1).Scan(ctx)
	if err != nil || scope.Revision.EnvironmentID != scope.Environment.ID {
		return scope, errors.New("Deployment state revision is unavailable or mismatched")
	}
	scope.State, err = models.ParseEnvironmentDesiredState(scope.Revision.State)
	if err != nil {
		return scope, err
	}
	var processSnapshot []models.EnvironmentProcessState
	if json.Unmarshal(deployment.ProcessConfiguration, &processSnapshot) != nil ||
		len(processSnapshot) == 0 {
		return scope, errors.New("Deployment process configuration snapshot is invalid")
	}
	processInputs := make([]models.EnvironmentProcessInput, 0, len(processSnapshot))
	for _, process := range processSnapshot {
		input := models.EnvironmentProcessInput{
			Name:       process.Name,
			Kind:       process.Kind,
			Command:    process.Command,
			Arguments:  process.Arguments,
			Replicas:   process.Replicas,
			HealthPath: process.HealthPath,
		}
		if process.Kind == models.EnvironmentProcessWeb {
			port := process.ContainerPort
			input.ContainerPort = &port
		}
		if process.Kind == models.EnvironmentProcessRelease {
			timeout := process.TimeoutSeconds
			input.TimeoutSeconds = &timeout
		}
		processInputs = append(processInputs, input)
	}
	if _, err := models.ValidateEnvironmentProcessFormation(processInputs); err != nil {
		return scope, errors.New("Deployment process configuration snapshot is invalid")
	}
	scope.State.Processes = processSnapshot
	err = service.db.Executor().
		NewSelect().
		Model(&scope.Instances).
		Where("deployment_id = ?", deployment.ID).
		Where("removed_at IS NULL").
		OrderExpr("process_kind, process_name, replica_key").
		Scan(ctx)
	if err != nil || len(scope.Instances) == 0 {
		return scope, errors.New("Deployment candidate formation is unavailable")
	}
	expectedInstances := make(map[string]string)
	for _, process := range processSnapshot {
		if process.Kind != models.EnvironmentProcessWeb &&
			process.Kind != models.EnvironmentProcessWorker {
			continue
		}
		for replica := int32(1); replica <= process.Replicas; replica++ {
			replicaKey := fmt.Sprintf("%s/%s/%d", process.Kind, process.Name, replica)
			if process.Kind == models.EnvironmentProcessWeb {
				replicaKey = "web/primary"
			}
			expectedInstances[process.Name+"\x00"+replicaKey] = process.Kind
		}
	}
	if len(expectedInstances) != len(scope.Instances) {
		return scope, errors.New(
			"Deployment candidate formation does not match its process snapshot",
		)
	}
	for _, instance := range scope.Instances {
		if instance.ReleaseID != scope.Release.ID ||
			instance.EnvironmentTargetID != scope.Target.ID {
			return scope, errors.New("Deployment candidate Instance is mismatched")
		}
		key := instance.ProcessName + "\x00" + instance.ReplicaKey
		if expectedKind, exists := expectedInstances[key]; !exists ||
			expectedKind != instance.ProcessKind {
			return scope, errors.New(
				"Deployment candidate formation does not match its process snapshot",
			)
		}
		delete(expectedInstances, key)
		if instance.ProcessKind == models.EnvironmentProcessWeb {
			if scope.Instance.ID != uuid.Nil {
				return scope, errors.New("Deployment has multiple web Instances")
			}
			scope.Instance = instance
		}
	}
	if scope.Instance.ID == uuid.Nil {
		return scope, errors.New("Deployment web Instance is unavailable")
	}
	if len(expectedInstances) != 0 {
		return scope, errors.New("Deployment candidate formation is incomplete")
	}
	scope.Domain, err = models.EnvironmentDomain.Find(
		ctx,
		service.db.Executor(),
		scope.State.Domain.ID,
	)
	if err != nil || scope.Domain.EnvironmentID != scope.Environment.ID ||
		scope.Domain.ArchivedAt.Valid ||
		!scope.Domain.IsPrimary ||
		scope.Domain.Hostname != scope.State.Domain.Hostname {
		return scope, errors.New("Deployment primary domain is unavailable or mismatched")
	}
	err = service.db.Executor().
		NewSelect().
		Model(&scope.Runtime).
		Where("environment_id = ?", scope.Environment.ID).
		Limit(1).
		Scan(ctx)
	if err != nil || scope.Runtime.Runtime != "go" ||
		scope.Runtime.RestartPolicy != "unless-stopped" {
		return scope, errors.New("Deployment Go runtime configuration is unavailable")
	}
	if scope.Release.RegistryResourceID != nil && scope.Release.RegistryCredentialID != nil &&
		scope.Release.RegistryEndpoint.Valid {
		scope.RegistryID = *scope.Release.RegistryResourceID
		scope.RegistryCredentialID = *scope.Release.RegistryCredentialID
		scope.RegistryEndpoint = scope.Release.RegistryEndpoint.String
		return scope, nil
	}
	if scope.Release.BuildID == nil {
		return scope, errors.New("workload Release has no registry pull snapshot")
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), *scope.Release.BuildID)
	if err != nil || build.EnvironmentID != scope.Environment.ID || build.Status != "succeeded" {
		return scope, errors.New("workload Release Build is unavailable")
	}
	snapshot, err := parseBuildSnapshot(build)
	if err != nil {
		return scope, errors.New("workload Release Build snapshot is invalid")
	}
	scope.RegistryID, scope.RegistryCredentialID, scope.RegistryEndpoint = snapshot.RegistryResourceID, snapshot.RegistryCredentialID, snapshot.RegistryEndpoint
	return scope, nil
}

func (service *DeploymentExecution) composeEnvironment(
	ctx context.Context,
	scope deploymentScope,
	secrets []ResolvedEnvironmentSecret,
	includePort bool,
) (map[string]string, []containerclient.ResourceContainerAttachment, error) {
	values := map[string]string{
		"DEPLOYCRATE_APPLICATION_ID": scope.ApplicationID.String(),
		"DEPLOYCRATE_ENVIRONMENT_ID": scope.Environment.ID.String(),
		"DEPLOYCRATE_RELEASE_ID":     scope.Release.ID.String(),
	}
	if includePort {
		web, exists := scope.State.WebProcess()
		if !exists {
			return nil, nil, errors.New("Environment web process is unavailable")
		}
		values["PORT"] = strconv.Itoa(int(web.ContainerPort))
	}
	put := func(key, value string) error {
		if _, exists := values[key]; exists {
			return fmt.Errorf(
				"runtime variable %s conflicts with a platform or Resource value",
				key,
			)
		}
		values[key] = value
		return nil
	}
	type dockerEndpoint struct {
		host string
		port int32
	}
	attachments := make(
		[]containerclient.ResourceContainerAttachment,
		0,
		len(scope.State.Resources),
	)
	dockerURLs := make(map[string]dockerEndpoint)
	dockerSecretValues := make(map[string]string)
	resourceSecretKeys := make(map[uuid.UUID]map[string]struct{})
	for _, descriptor := range scope.State.Secrets {
		if descriptor.SourceType != models.EnvironmentSecretSourceResource {
			continue
		}
		keys := resourceSecretKeys[descriptor.SourceID]
		if keys == nil {
			keys = make(map[string]struct{})
			resourceSecretKeys[descriptor.SourceID] = keys
		}
		keys[descriptor.Key] = struct{}{}
	}
	for _, resourceState := range scope.State.Resources {
		connection, err := models.EnvironmentResource.Find(
			ctx,
			service.db.Executor(),
			resourceState.EnvironmentResourceID,
		)
		if err != nil || connection.EnvironmentID != scope.Environment.ID ||
			connection.ResourceID != resourceState.ResourceID ||
			connection.ResourceEndpointID != resourceState.EndpointID ||
			connection.ArchivedAt.Valid {
			return nil, nil, errors.New(
				"Environment Resource connection is unavailable or mismatched",
			)
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), connection.ResourceID)
		if err != nil || resource.ArchivedAt.Valid || resource.Engine() != resourceState.Kind {
			return nil, nil, errors.New("Environment Resource is unavailable or mismatched")
		}
		endpoint, err := models.ResourceEndpoint.Find(
			ctx,
			service.db.Executor(),
			connection.ResourceEndpointID,
		)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
			return nil, nil, errors.New(
				"Environment Resource endpoint is unavailable or mismatched",
			)
		}
		if (connection.ResourceCredentialID == nil) != (resourceState.CredentialID == nil) ||
			(connection.ResourceCredentialID != nil && *connection.ResourceCredentialID != *resourceState.CredentialID) {
			return nil, nil, errors.New("Environment Resource credential projection is mismatched")
		}
		if connection.ResourceCredentialID != nil {
			credential, err := models.ResourceCredential.Find(
				ctx,
				service.db.Executor(),
				*connection.ResourceCredentialID,
			)
			if err != nil || credential.ArchivedAt.Valid || credential.ResourceID != resource.ID {
				return nil, nil, errors.New(
					"Environment Resource credential is unavailable or mismatched",
				)
			}
		}
		var projected *dockerEndpoint
		var installation models.ResourceInstallationEntity
		installationErr := service.db.Executor().NewSelect().Model(&installation).
			Where("resource_id = ?", resource.ID).
			Where("archived_at IS NULL").
			OrderExpr("created_at").Limit(1).Scan(ctx)
		if installationErr == nil {
			if installation.ServerID != scope.Target.ServerID {
				return nil, nil, errors.New(
					"managed Resource is not installed on the Environment runtime Server",
				)
			}
			var configuration struct {
				PortMappings []models.ResourceInstallationPortMapping `json:"portMappings"`
			}
			definition, supported := models.FindResourceEngine(resource.Engine())
			if json.Unmarshal(installation.Configuration, &configuration) != nil ||
				len(configuration.PortMappings) != 1 ||
				!supported {
				return nil, nil, errors.New("Docker Resource installation port mapping is invalid")
			}
			mapping := configuration.PortMappings[0]
			if mapping.HostPort != endpoint.Port ||
				mapping.ContainerPort != definition.DefaultPort ||
				mapping.Protocol != "tcp" {
				return nil, nil, errors.New(
					"Docker Resource endpoint does not match its container port mapping",
				)
			}
			attachment := containerclient.ResourceContainerAttachment{
				InstallationID: installation.ID,
				ContainerName:  installation.ContainerName,
			}
			attachments = append(attachments, attachment)
			projected = &dockerEndpoint{
				host: installation.ContainerName,
				port: mapping.ContainerPort,
			}
			if len(resourceState.EnvironmentKeys) > 0 {
				projectedSecretKeys := resourceSecretKeys[resourceState.EnvironmentResourceID]
				if key := strings.TrimSpace(resourceState.EnvironmentKeys["host"]); key != "" {
					if _, exists := projectedSecretKeys[key]; exists {
						dockerSecretValues[key] = projected.host
					}
				}
				if key := strings.TrimSpace(resourceState.EnvironmentKeys["port"]); key != "" {
					if _, exists := projectedSecretKeys[key]; exists {
						dockerSecretValues[key] = strconv.Itoa(int(projected.port))
					}
				}
				if key := strings.TrimSpace(resourceState.EnvironmentKeys["url"]); key != "" {
					if _, exists := projectedSecretKeys[key]; exists {
						dockerURLs[key] = *projected
					}
				}
			} else {
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
					return nil, nil, errors.New(
						"Docker Resource credential projection is unsupported",
					)
				}
			}
		} else if !errors.Is(installationErr, sql.ErrNoRows) {
			return nil, nil, installationErr
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
		if projectedValue, exists := dockerSecretValues[secret.Key]; exists {
			value = projectedValue
		}
		if endpoint, exists := dockerURLs[secret.Key]; exists {
			connectionURL, err := url.Parse(value)
			if err != nil || connectionURL.Scheme == "" || connectionURL.User == nil {
				return nil, nil, fmt.Errorf(
					"Docker Resource connection URL %s is invalid",
					secret.Key,
				)
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

func validateCandidateOwnership(
	candidate containerclient.WorkloadState,
	scope deploymentScope,
	instance models.InstanceEntity,
) error {
	expected := map[string]string{
		"com.deploycrate.application":               scope.ApplicationID.String(),
		"com.deploycrate.environment":               scope.Environment.ID.String(),
		"com.deploycrate.target":                    scope.Target.ID.String(),
		"com.deploycrate.deployment":                scope.Deployment.ID.String(),
		"com.deploycrate.instance":                  instance.ID.String(),
		"com.deploycrate.release":                   scope.Release.ID.String(),
		containerclient.WorkloadLabelProcessName:    instance.ProcessName,
		containerclient.WorkloadLabelProcessKind:    instance.ProcessKind,
		containerclient.WorkloadLabelProcessReplica: instance.ReplicaKey,
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

func waitForWorkloadHealth(ctx context.Context, host string, port int32, healthPath string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	check := func() bool {
		address := net.JoinHostPort("0.0.0.0", strconv.Itoa(int(port)))

		slog.InfoContext(
			ctx,
			"waitForWorkloadHealth",
			"address",
			address,
			"health_path",
			healthPath,
			"port",
			port,
		)

		if healthPath == "" {
			connection, err := net.DialTimeout("tcp", address, time.Second)
			slog.InfoContext(ctx, "healthPath empty", "connection", connection)
			if err == nil {
				_ = connection.Close()
				return true
			}

			return false
		}

		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"http://"+address+healthPath,
			nil,
		)
		if err != nil {
			slog.InfoContext(ctx, "healthPath not empty | NewRequest", "err", err)
			return false
		}

		response, err := telemetry.NewHTTPClient(2 * time.Second).Do(request)
		if err != nil {
			slog.InfoContext(ctx, "healthPath not empty | NewHTTPClient", "err", err)
			return false
		}

		slog.InfoContext(ctx, "healthPath not empty", "status_code", response.StatusCode)

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

func (service *DeploymentExecution) prepareCaddy(
	ctx context.Context,
	scope deploymentScope,
) (models.CaddyRouteEntity, []models.InstanceEntity, bool, error) {
	var route models.CaddyRouteEntity
	err := service.db.Executor().
		NewSelect().
		Model(&route).
		Where("environment_target_id = ?", scope.Target.ID).
		Where("environment_domain_id = ?", scope.Domain.ID).
		Where("removed_at IS NULL").
		OrderExpr("created_at").
		Limit(1).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		route, err = models.CaddyRoute.Create(
			ctx,
			service.db.Executor(),
			models.CreateCaddyRouteData{
				ExternalID: "deploycrate_environment_" + strings.ReplaceAll(
					scope.Environment.ID.String(),
					"-",
					"",
				),
				State:               "pending",
				EnvironmentTargetID: scope.Target.ID,
				EnvironmentDomainID: scope.Domain.ID,
				ReleaseID:           scope.Release.ID,
			},
		)
		if err != nil {
			return route, nil, false, err
		}
		_, err = models.CaddyRouteBackend.Create(
			ctx,
			service.db.Executor(),
			models.CreateCaddyRouteBackendData{
				Weight:       100,
				CaddyRouteID: route.ID,
				InstanceID:   scope.Instance.ID,
			},
		)
		return route, nil, true, err
	}
	if err != nil {
		return route, nil, false, err
	}
	previous := make([]models.InstanceEntity, 0)
	if err := service.db.Executor().NewSelect().Model(&previous).
		Join("JOIN caddy_route_backends AS backend ON backend.instance_id = instances.id").
		Where("backend.caddy_route_id = ?", route.ID).
		Where("backend.removed_at IS NULL").Where("instances.id <> ?", scope.Instance.ID).
		Where("instances.removed_at IS NULL").
		OrderExpr("instances.created_at DESC").Scan(ctx); err != nil {
		return route, nil, false, err
	}
	count, err := service.db.Executor().
		NewSelect().
		Model((*models.CaddyRouteBackendEntity)(nil)).
		Where("caddy_route_id = ?", route.ID).
		Where("instance_id = ?", scope.Instance.ID).
		Where("removed_at IS NULL").
		Count(ctx)
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

func (service *DeploymentExecution) markSucceeded(
	ctx context.Context,
	scope deploymentScope,
	candidates map[uuid.UUID]containerclient.WorkloadState,
) error {
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
	observedProcesses := make([]map[string]any, 0, len(scope.Instances))
	for _, instance := range scope.Instances {
		candidate, exists := candidates[instance.ID]
		if !exists {
			return errors.New("candidate formation observation is incomplete")
		}
		ports := json.RawMessage(`{}`)
		if instance.ProcessKind == models.EnvironmentProcessWeb {
			encoded, _ := json.Marshal(
				map[string]any{"host": candidate.HostAddress, "http": candidate.HostPort},
			)
			ports = encoded
		}
		if _, err := tx.NewUpdate().
			TableExpr("instances").
			Set("external_id = ?", candidate.ID).
			Set("state = 'serving'").
			Set("ports = ?", ports).
			Set("observed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", instance.ID).
			Exec(ctx); err != nil {
			return err
		}
		observedProcesses = append(
			observedProcesses,
			map[string]any{
				"instance_id":  instance.ID,
				"container_id": candidate.ID,
				"image_id":     candidate.ImageID,
				"process_name": instance.ProcessName,
				"process_kind": instance.ProcessKind,
				"replica_key":  instance.ReplicaKey,
			},
		)
	}
	encodedObserved, _ := json.Marshal(
		map[string]any{"schema_version": 2, "processes": observedProcesses},
	)
	observed := json.RawMessage(encodedObserved)
	if _, err := tx.NewUpdate().
		TableExpr("environment_target_states").
		Set("state = 'applied'").
		Set("observed_state = ?", observed).
		Set("applying_revision_id = NULL").
		Set("applied_revision_id = ?", scope.Revision.ID).
		Set("observed_at = ?", now).
		Set("updated_at = ?", now).
		Where("environment_target_id = ?", scope.Target.ID).
		Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().
		TableExpr("deployments").
		Set("status = 'succeeded'").
		Set("current_step = 'serving'").
		Set("finished_at = ?", now).
		Set("error = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", scope.Deployment.ID).
		Where("status = 'running'").
		Exec(ctx); err != nil {
		return err
	}
	remaining, err := tx.NewSelect().
		TableExpr("deployments").
		Where("change_id = ?", scope.Deployment.ChangeID).
		Where("status <> 'succeeded'").
		Count(ctx)
	if err != nil {
		return err
	}
	if remaining == 0 {
		if err := models.Change.MarkCompleted(ctx, tx, scope.Deployment.ChangeID, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return service.recordEvent(
		ctx,
		scope.Deployment.ID,
		"serving",
		"succeeded",
		"serving",
		"candidate is serving public traffic",
		nil,
	)
}

func (service *DeploymentExecution) removeCandidateFormation(
	ctx context.Context,
	scope deploymentScope,
) {
	for _, instance := range scope.Instances {
		_ = service.workloads.Remove(ctx, scope.Target.ServerID, scope.Deployment.ID, instance.ID)
	}
}

func (service *DeploymentExecution) stabilizeWorkers(
	ctx context.Context,
	scope deploymentScope,
	candidates map[uuid.UUID]containerclient.WorkloadState,
) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	for _, instance := range scope.Instances {
		if instance.ProcessKind != models.EnvironmentProcessWorker {
			continue
		}
		state, err := service.workloads.Find(
			ctx,
			scope.Target.ServerID,
			scope.Deployment.ID,
			instance.ID,
		)
		if err != nil {
			return err
		}
		if !state.Exists || !state.Running {
			return fmt.Errorf(
				"worker process %s replica %s exited during stabilization",
				instance.ProcessName,
				instance.ReplicaKey,
			)
		}
		candidates[instance.ID] = state
	}
	return nil
}

func (service *DeploymentExecution) previousFormation(
	ctx context.Context,
	previousWeb []models.InstanceEntity,
) ([]models.InstanceEntity, error) {
	deploymentIDs := make([]uuid.UUID, 0, len(previousWeb))
	seen := make(map[uuid.UUID]struct{})
	for _, instance := range previousWeb {
		if _, exists := seen[instance.DeploymentID]; !exists {
			seen[instance.DeploymentID] = struct{}{}
			deploymentIDs = append(deploymentIDs, instance.DeploymentID)
		}
	}
	if len(deploymentIDs) == 0 {
		return []models.InstanceEntity{}, nil
	}
	instances := make([]models.InstanceEntity, 0)
	err := service.db.Executor().
		NewSelect().
		Model(&instances).
		Where("deployment_id IN (?)", bun.In(deploymentIDs)).
		Where("removed_at IS NULL").
		OrderExpr("created_at, process_kind, process_name, replica_key").
		Scan(ctx)
	return instances, err
}

func (service *DeploymentExecution) recordEvent(
	ctx context.Context,
	deploymentID uuid.UUID,
	eventType, status, step, message string,
	operationErr error,
) error {
	var sequence int64
	if err := service.db.Executor().
		NewSelect().
		TableExpr("deployment_events").
		ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("deployment_id = ?", deploymentID).
		Scan(ctx, &sequence); err != nil {
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
	_, err := models.DeploymentEvent.Create(
		ctx,
		service.db.Executor(),
		models.CreateDeploymentEventData{
			Sequence:     sequence,
			EventType:    eventType,
			Status:       sql.NullString{String: status, Valid: status != ""},
			Step:         sql.NullString{String: step, Valid: step != ""},
			Message:      message,
			Metadata:     json.RawMessage(`{}`),
			Error:        failure,
			OccurredAt:   time.Now().UTC(),
			DeploymentID: deploymentID,
		},
	)
	return err
}
