package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type BackupExecuteWorker struct {
	river.WorkerDefaults[jobs.BackupExecuteArgs]
	service *services.BackupExecutor
}

func NewBackupExecuteWorker(service *services.BackupExecutor) *BackupExecuteWorker {
	return &BackupExecuteWorker{service: service}
}

func (worker *BackupExecuteWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupExecuteWorker) Timeout(*river.Job[jobs.BackupExecuteArgs]) time.Duration {
	return 12 * time.Hour
}

func (worker *BackupExecuteWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupExecuteArgs],
) error {
	return runBackupWorker(
		ctx,
		"execute",
		job.ID,
		job.Attempt,
		[]any{"backup_id", job.Args.BackupID},
		func() error { return worker.service.Execute(ctx, job.Args.BackupID) },
	)
}
