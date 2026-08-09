package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ReleaseDeployment struct {
	db    storage.Pool
	queue storage.InsertQueue
}

func NewReleaseDeployment(db storage.Pool, queue storage.InsertQueue) *ReleaseDeployment {
	return &ReleaseDeployment{db: db, queue: queue}
}

type ReleaseOrchestrationResult struct {
	Deployment     models.DeploymentEntity
	ReleaseCommand *models.ReleaseCommandExecutionEntity
}

func (service *ReleaseDeployment) OrchestrateTx(
	ctx context.Context,
	tx bun.Tx,
	release models.ReleaseEntity,
	change models.ChangeEntity,
	revision models.EnvironmentStateRevisionEntity,
) (ReleaseOrchestrationResult, error) {
	if err := models.Release.LockOrchestration(ctx, tx, release.EnvironmentID); err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	active, err := models.Release.ActiveOperationsExcluding(
		ctx,
		tx,
		release.EnvironmentID,
		release.ID,
		change.ID,
	)
	if err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	if active.Commands != 0 || active.Deployments != 0 {
		return ReleaseOrchestrationResult{}, errors.New(
			"Environment already has an active release operation",
		)
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, release.EnvironmentID)
	if err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	if len(targets) == 0 {
		return ReleaseOrchestrationResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment has no runtime Server targets"),
		)
	}
	existing, findErr := models.ReleaseCommandExecution.ForRelease(ctx, tx, release.ID)
	if findErr == nil {
		switch existing.Status {
		case "succeeded":
			return service.fanOutTx(ctx, tx, release, change, revision, state, targets)
		case "queued", "running":
			if existing.ChangeID == change.ID {
				return ReleaseOrchestrationResult{ReleaseCommand: &existing}, nil
			}
			return ReleaseOrchestrationResult{}, errors.New(
				"Release command is already active for this Release",
			)
		default:
			return ReleaseOrchestrationResult{}, errors.New(
				"Release command requires manual recovery before deployment",
			)
		}
	}
	if !errors.Is(findErr, sql.ErrNoRows) {
		return ReleaseOrchestrationResult{}, findErr
	}
	if releaseProcess, configured := state.ReleaseProcess(); configured {
		configuration, digest, err := models.CanonicalReleaseCommandConfiguration(releaseProcess)
		if err != nil {
			return ReleaseOrchestrationResult{}, err
		}
		execution, err := models.ReleaseCommandExecution.Create(
			ctx,
			tx,
			models.CreateReleaseCommandExecutionData{
				Configuration:              configuration,
				ConfigurationDigest:        digest,
				ReleaseID:                  release.ID,
				EnvironmentStateRevisionID: revision.ID,
				EnvironmentTargetID:        targets[0].ID,
				ChangeID:                   change.ID,
			},
		)
		if err != nil {
			return ReleaseOrchestrationResult{}, err
		}
		if _, err := service.queue.InsertTx(
			ctx,
			tx.Tx,
			jobs.ReleaseCommandArgs{
				ReleaseCommandExecutionID: execution.ID,
				Attempt:                   execution.Attempt,
			},
			jobs.ReleaseCommandInsertOpts(execution.ID, execution.Attempt),
		); err != nil {
			return ReleaseOrchestrationResult{}, err
		}
		return ReleaseOrchestrationResult{ReleaseCommand: &execution}, nil
	}
	return service.fanOutTx(ctx, tx, release, change, revision, state, targets)
}

func (service *ReleaseDeployment) FanOutSucceededReleaseCommand(
	ctx context.Context,
	executionID uuid.UUID,
	exitCode int32,
	externalID string,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	execution, err := models.ReleaseCommandExecution.Lock(ctx, tx, executionID)
	if err != nil {
		return err
	}
	if execution.Status == "succeeded" {
		return tx.Commit()
	}
	if execution.Status != "running" {
		return errors.New("release command execution is no longer running")
	}
	release, err := models.Release.Find(ctx, tx, execution.ReleaseID)
	if err != nil {
		return err
	}
	change, err := models.Change.Lock(ctx, tx, execution.ChangeID)
	if err != nil {
		return err
	}
	revision, err := models.EnvironmentStateRevision.Find(
		ctx,
		tx,
		execution.EnvironmentStateRevisionID,
	)
	if err != nil {
		return err
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return err
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, release.EnvironmentID)
	if err != nil || len(targets) == 0 {
		if err == nil {
			err = errors.New("Environment has no active runtime targets")
		}
		return err
	}
	if _, err := service.fanOutTx(ctx, tx, release, change, revision, state, targets); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ReleaseCommandExecution.MarkSucceeded(
		ctx,
		tx,
		execution.ID,
		externalID,
		exitCode,
		now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ReleaseDeployment) fanOutTx(
	ctx context.Context,
	tx bun.Tx,
	release models.ReleaseEntity,
	change models.ChangeEntity,
	revision models.EnvironmentStateRevisionEntity,
	state models.EnvironmentDesiredState,
	targets []models.EnvironmentTargetEntity,
) (ReleaseOrchestrationResult, error) {
	existing, err := models.Deployment.CountForChange(ctx, tx, change.ID)
	if err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	if existing > 0 {
		first, err := models.Deployment.FirstForChange(ctx, tx, change.ID)
		return ReleaseOrchestrationResult{Deployment: first}, err
	}
	now := time.Now().UTC()
	if err := models.EnvironmentTargetState.SetDesiredRevisionForEnvironment(
		ctx,
		tx,
		release.EnvironmentID,
		revision.ID,
		now,
	); err != nil {
		return ReleaseOrchestrationResult{}, err
	}
	var first models.DeploymentEntity
	for targetIndex, target := range targets {
		deployment, err := service.QueueTargetTx(
			ctx,
			tx,
			change.ID,
			release.ID,
			target.ID,
			1,
			json.RawMessage(`{"type":"blue_green","web_replicas":1}`),
			state.Processes,
			now,
		)
		if err != nil {
			return ReleaseOrchestrationResult{}, err
		}
		if targetIndex == 0 {
			first = deployment
		}
	}
	return ReleaseOrchestrationResult{Deployment: first}, nil
}

func (service *ReleaseDeployment) QueueTargetTx(
	ctx context.Context,
	tx bun.Tx,
	changeID, releaseID, targetID uuid.UUID,
	attempt int32,
	strategy json.RawMessage,
	processes []models.EnvironmentProcessState,
	now time.Time,
) (models.DeploymentEntity, error) {
	processSnapshot, err := json.Marshal(processes)
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	deployment, err := models.Deployment.Create(ctx, tx, models.CreateDeploymentData{
		Attempt:              attempt,
		Strategy:             strategy,
		ProcessConfiguration: processSnapshot,
		Status:               "queued",
		CurrentStep:          sql.NullString{String: "queued", Valid: true},
		ChangeID:             changeID,
		ReleaseID:            releaseID,
		EnvironmentTargetID:  targetID,
	})
	if err != nil {
		return models.DeploymentEntity{}, err
	}
	for _, process := range processes {
		if process.Kind != models.EnvironmentProcessWeb &&
			process.Kind != models.EnvironmentProcessWorker {
			continue
		}
		for replica := int32(1); replica <= process.Replicas; replica++ {
			replicaName := fmt.Sprintf("%d", replica)
			replicaKey := process.Kind + "/" + process.Name + "/" + replicaName
			if process.Kind == models.EnvironmentProcessWeb {
				replicaKey = "web/primary"
			}
			if _, err := models.Instance.Create(ctx, tx, models.CreateInstanceData{
				ExternalID:  "pending:" + deployment.ID.String() + ":" + replicaKey,
				Slot:        "candidate",
				ProcessName: process.Name,
				ProcessKind: process.Kind,
				ReplicaKey:  replicaKey,
				State:       "candidate",
				Ports: json.RawMessage(
					`{}`,
				),
				ObservedAt:          now,
				DeploymentID:        deployment.ID,
				ReleaseID:           releaseID,
				EnvironmentTargetID: targetID,
			}); err != nil {
				return models.DeploymentEntity{}, err
			}
		}
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.DeployReleaseArgs{DeploymentID: deployment.ID},
		jobs.DeployReleaseInsertOpts(deployment.ID),
	); err != nil {
		return models.DeploymentEntity{}, err
	}
	return deployment, nil
}
