package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type BackupVerifyWorker struct {
	river.WorkerDefaults[jobs.BackupVerifyArgs]
	service *services.BackupVerifier
}

func NewBackupVerifyWorker(service *services.BackupVerifier) *BackupVerifyWorker {
	return &BackupVerifyWorker{service: service}
}

func (worker *BackupVerifyWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupVerifyWorker) Timeout(*river.Job[jobs.BackupVerifyArgs]) time.Duration {
	return 12 * time.Hour
}

func (worker *BackupVerifyWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupVerifyArgs],
) error {
	return runBackupWorker(
		ctx,
		"verify",
		job.ID,
		job.Attempt,
		[]any{"backup_id", job.Args.BackupID},
		func() error { return worker.service.Verify(ctx, job.Args.BackupID) },
	)
}
