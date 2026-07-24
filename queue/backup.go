package queue

import (
	"context"
	"log/slog"
	"time"

	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

type BackupScheduleService interface {
	ScheduleDue(context.Context) error
}

type BackupExecutionService interface {
	Execute(context.Context, uuid.UUID) error
}

type BackupVerificationService interface {
	Verify(context.Context, uuid.UUID) error
}

type BackupRetentionService interface {
	Apply(context.Context, uuid.UUID) error
}

type BackupScheduleWorker struct {
	river.WorkerDefaults[jobs.BackupScheduleTickArgs]
	service BackupScheduleService
}

func NewBackupScheduleWorker(service BackupScheduleService) *BackupScheduleWorker {
	return &BackupScheduleWorker{service: service}
}

func (worker *BackupScheduleWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BackupScheduleWorker) Timeout(*river.Job[jobs.BackupScheduleTickArgs]) time.Duration {
	return time.Minute
}

func (worker *BackupScheduleWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BackupScheduleTickArgs],
) error {
	return runBackupWorker(ctx, "schedule", job.ID, job.Attempt, nil, func() error {
		return worker.service.ScheduleDue(ctx)
	})
}

type BackupExecuteWorker struct {
	river.WorkerDefaults[jobs.BackupExecuteArgs]
	service BackupExecutionService
}

func NewBackupExecuteWorker(service BackupExecutionService) *BackupExecuteWorker {
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

type BackupVerifyWorker struct {
	river.WorkerDefaults[jobs.BackupVerifyArgs]
	service BackupVerificationService
}

func NewBackupVerifyWorker(service BackupVerificationService) *BackupVerifyWorker {
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

type BackupRetentionWorker struct {
	river.WorkerDefaults[jobs.BackupRetentionArgs]
	service BackupRetentionService
}

func NewBackupRetentionWorker(service BackupRetentionService) *BackupRetentionWorker {
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

func runBackupWorker(
	ctx context.Context,
	operation string,
	jobID int64,
	attempt int,
	attributes []any,
	work func() error,
) error {
	startedAt := time.Now()
	attributes = append(attributes, "operation", operation, "river_job_id", jobID, "attempt", attempt)
	slog.InfoContext(ctx, "backup job started", attributes...)
	if err := work(); err != nil {
		failureAttributes := append(
			attributes,
			"duration", time.Since(startedAt),
			"error_category", "backup_"+operation,
			"error", err,
		)
		slog.ErrorContext(ctx, "backup job failed", failureAttributes...)
		return err
	}
	slog.InfoContext(
		ctx,
		"backup job completed",
		append(attributes, "duration", time.Since(startedAt))...,
	)
	return nil
}

func NewBackupSchedulePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return jobs.BackupScheduleTickArgs{}, nil },
		&river.PeriodicJobOpts{ID: "backup-schedule-one-minute", RunOnStart: true},
	)
}
