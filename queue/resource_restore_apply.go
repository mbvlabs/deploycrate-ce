package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type ResourceRestoreApplyWorker struct {
	river.WorkerDefaults[jobs.ResourceRestoreApplyArgs]
	service *services.ResourceRestore
}

func NewResourceRestoreApplyWorker(service *services.ResourceRestore) *ResourceRestoreApplyWorker {
	return &ResourceRestoreApplyWorker{service: service}
}

func (worker *ResourceRestoreApplyWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *ResourceRestoreApplyWorker) Timeout(*river.Job[jobs.ResourceRestoreApplyArgs]) time.Duration {
	return 12 * time.Hour
}

func (worker *ResourceRestoreApplyWorker) Work(ctx context.Context, job *river.Job[jobs.ResourceRestoreApplyArgs]) error {
	return runBackupWorker(ctx, "restore_apply", job.ID, job.Attempt,
		[]any{"resource_restore_id", job.Args.ResourceRestoreID},
		func() error { return worker.service.Apply(ctx, job.Args.ResourceRestoreID) })
}
