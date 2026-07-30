package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type ResourceRestorePrepareWorker struct {
	river.WorkerDefaults[jobs.ResourceRestorePrepareArgs]
	service *services.ResourceRestore
}

func NewResourceRestorePrepareWorker(service *services.ResourceRestore) *ResourceRestorePrepareWorker {
	return &ResourceRestorePrepareWorker{service: service}
}

func (worker *ResourceRestorePrepareWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *ResourceRestorePrepareWorker) Timeout(*river.Job[jobs.ResourceRestorePrepareArgs]) time.Duration {
	return time.Hour
}

func (worker *ResourceRestorePrepareWorker) Work(ctx context.Context, job *river.Job[jobs.ResourceRestorePrepareArgs]) error {
	return runBackupWorker(ctx, "restore_prepare", job.ID, job.Attempt,
		[]any{"resource_restore_id", job.Args.ResourceRestoreID},
		func() error { return worker.service.Prepare(ctx, job.Args.ResourceRestoreID) })
}
