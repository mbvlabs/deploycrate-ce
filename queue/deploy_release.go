package queue

import (
	"context"
	"errors"
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
	return 15 * time.Minute
}

func (worker *DeployReleaseWorker) Work(ctx context.Context, job *river.Job[jobs.DeployReleaseArgs]) error {
	err := worker.service.Execute(ctx, job.Args.DeploymentID)
	var permanent *services.PermanentDeploymentError
	if errors.As(err, &permanent) {
		return river.JobCancel(err)
	}
	if err != nil && job.Attempt >= job.MaxAttempts {
		_ = worker.service.Fail(context.WithoutCancel(ctx), job.Args.DeploymentID, err)
		return river.JobCancel(err)
	}
	return err
}
