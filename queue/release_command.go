package queue

import (
	"context"
	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"
	"time"

	"github.com/riverqueue/river"
)

type ReleaseCommandWorker struct {
	river.WorkerDefaults[jobs.ReleaseCommandArgs]
	service *services.ReleaseCommandExecution
}

func NewReleaseCommandWorker(service *services.ReleaseCommandExecution) *ReleaseCommandWorker {
	return &ReleaseCommandWorker{service: service}
}

func (worker *ReleaseCommandWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *ReleaseCommandWorker) Timeout(*river.Job[jobs.ReleaseCommandArgs]) time.Duration {
	return 65 * time.Minute
}

func (worker *ReleaseCommandWorker) Work(ctx context.Context, job *river.Job[jobs.ReleaseCommandArgs]) error {
	if err := worker.service.Execute(ctx, job.Args.ReleaseCommandExecutionID); err != nil {
		return river.JobCancel(err)
	}
	return nil
}
