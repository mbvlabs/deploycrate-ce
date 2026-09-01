package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type DeployReleaseWorker struct {
	river.WorkerDefaults[jobs.DeployReleaseArgs]
	service *services.DeploymentExecution
}

func NewDeployReleaseWorker(service *services.DeploymentExecution) *DeployReleaseWorker {
	return &DeployReleaseWorker{service: service}
}

func (worker *DeployReleaseWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *DeployReleaseWorker) Timeout(*river.Job[jobs.DeployReleaseArgs]) time.Duration {
	return services.DeploymentExecutionTimeout
}

func (worker *DeployReleaseWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.DeployReleaseArgs],
) error {
	err := worker.service.Execute(ctx, job.Args.DeploymentID)
	if errors.Is(err, context.DeadlineExceeded) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		timeoutErr := worker.service.Cancel(
			cleanupCtx,
			job.Args.DeploymentID,
			fmt.Sprintf(
				"Deployment timed out after %s",
				services.DeploymentExecutionTimeout,
			),
		)
		return river.JobCancel(errors.Join(err, timeoutErr))
	}
	if _, ok := errors.AsType[*services.PermanentDeploymentError](err); ok {
		return river.JobCancel(err)
	}
	if err != nil && job.Attempt >= job.MaxAttempts {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		rollbackErr := worker.service.Cancel(
			cleanupCtx,
			job.Args.DeploymentID,
			fmt.Sprintf("Deployment stopped after %d unsuccessful attempts: %v", job.Attempt, err),
		)
		return river.JobCancel(errors.Join(err, rollbackErr))
	}
	return err
}
