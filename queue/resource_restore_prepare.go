package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type DatabaseRestorePrepareWorker struct {
	river.WorkerDefaults[jobs.DatabaseRestorePrepareArgs]
	service *services.DatabaseRestoreWorkflow
}

func NewDatabaseRestorePrepareWorker(
	service *services.DatabaseRestoreWorkflow,
) *DatabaseRestorePrepareWorker {
	return &DatabaseRestorePrepareWorker{service: service}
}

func (worker *DatabaseRestorePrepareWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *DatabaseRestorePrepareWorker) Timeout(
	*river.Job[jobs.DatabaseRestorePrepareArgs],
) time.Duration {
	return time.Hour
}

func (worker *DatabaseRestorePrepareWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.DatabaseRestorePrepareArgs],
) error {
	return runBackupWorker(ctx, "restore_prepare", job.ID, job.Attempt,
		[]any{"database_restore_id", job.Args.DatabaseRestoreID},
		func() error { return worker.service.Prepare(ctx, job.Args.DatabaseRestoreID) })
}
