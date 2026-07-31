package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type DatabaseRestoreApplyWorker struct {
	river.WorkerDefaults[jobs.DatabaseRestoreApplyArgs]
	service *services.DatabaseRestoreWorkflow
}

func NewDatabaseRestoreApplyWorker(service *services.DatabaseRestoreWorkflow) *DatabaseRestoreApplyWorker {
	return &DatabaseRestoreApplyWorker{service: service}
}

func (worker *DatabaseRestoreApplyWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *DatabaseRestoreApplyWorker) Timeout(*river.Job[jobs.DatabaseRestoreApplyArgs]) time.Duration {
	return 12 * time.Hour
}

func (worker *DatabaseRestoreApplyWorker) Work(ctx context.Context, job *river.Job[jobs.DatabaseRestoreApplyArgs]) error {
	return runBackupWorker(ctx, "restore_apply", job.ID, job.Attempt,
		[]any{"database_restore_id", job.Args.DatabaseRestoreID},
		func() error { return worker.service.Apply(ctx, job.Args.DatabaseRestoreID) })
}
