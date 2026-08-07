package queue

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type BuildSourceWorker struct {
	river.WorkerDefaults[jobs.BuildSourceArgs]
	service *services.BuildExecution
}

func NewBuildSourceWorker(service *services.BuildExecution) *BuildSourceWorker {
	return &BuildSourceWorker{service: service}
}

func (worker *BuildSourceWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *BuildSourceWorker) Timeout(*river.Job[jobs.BuildSourceArgs]) time.Duration {
	return 50 * time.Minute
}

func (worker *BuildSourceWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.BuildSourceArgs],
) error {
	err := worker.service.Execute(ctx, job.Args.BuildID)
	var permanent *services.PermanentBuildError
	if errors.As(err, &permanent) {
		return river.JobCancel(err)
	}
	if err != nil && job.Attempt >= job.MaxAttempts {
		_ = worker.service.Fail(context.WithoutCancel(ctx), job.Args.BuildID, err)
		return river.JobCancel(err)
	}
	if err != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if logErr := worker.service.RecordRetry(
			persistCtx,
			job.Args.BuildID,
			job.Attempt,
			job.MaxAttempts,
			err,
		); logErr != nil {
			slog.ErrorContext(
				persistCtx,
				"Build retry log could not be persisted",
				"build_id",
				job.Args.BuildID,
				"error",
				logErr,
			)
		}
	}
	return err
}
