package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type BackupScheduleWorker struct {
	river.WorkerDefaults[jobs.BackupScheduleArgs]
	scheduler *services.BackupScheduler
}

func NewBackupScheduleWorker(scheduler *services.BackupScheduler) *BackupScheduleWorker {
	return &BackupScheduleWorker{scheduler: scheduler}
}

func (worker *BackupScheduleWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupScheduleWorker) Timeout(*river.Job[jobs.BackupScheduleArgs]) time.Duration {
	return time.Minute
}

func (worker *BackupScheduleWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupScheduleArgs],
) error {
	return runBackupWorker(
		ctx,
		"schedule",
		job.ID,
		job.Attempt,
		[]any{
			"backup_policy_id", job.Args.BackupPolicyID,
			"scheduled_at", job.Args.ScheduledAt,
		},
		func() error {
			return worker.scheduler.Schedule(
				ctx,
				job.Args.BackupPolicyID,
				job.Args.ScheduledAt,
			)
		},
	)
}
