package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type BackupRetentionWorker struct {
	river.WorkerDefaults[jobs.BackupRetentionArgs]
	service *services.BackupRetention
}

func NewBackupRetentionWorker(service *services.BackupRetention) *BackupRetentionWorker {
	return &BackupRetentionWorker{service: service}
}

func (worker *BackupRetentionWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupRetentionWorker) Timeout(*river.Job[jobs.BackupRetentionArgs]) time.Duration {
	return 12 * time.Hour
}

func (worker *BackupRetentionWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupRetentionArgs],
) error {
	return runBackupWorker(
		ctx,
		"retention",
		job.ID,
		job.Attempt,
		[]any{"backup_policy_id", job.Args.BackupPolicyID},
		func() error { return worker.service.Apply(ctx, job.Args.BackupPolicyID) },
	)
}
