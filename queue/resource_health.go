package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type ResourceHealthSweepWorker struct {
	river.WorkerDefaults[jobs.ResourceHealthSweepArgs]
	service *services.ResourceHealth
}

func NewResourceHealthSweepWorker(service *services.ResourceHealth) *ResourceHealthSweepWorker {
	return &ResourceHealthSweepWorker{service: service}
}

func (worker *ResourceHealthSweepWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *ResourceHealthSweepWorker) Timeout(*river.Job[jobs.ResourceHealthSweepArgs]) time.Duration {
	return 10 * time.Minute
}

func (worker *ResourceHealthSweepWorker) Work(
	ctx context.Context,
	_ *river.Job[jobs.ResourceHealthSweepArgs],
) error {
	return worker.service.Sweep(ctx)
}

func NewResourceHealthPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(5*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			args := jobs.ResourceHealthSweepArgs{}
			opts := args.InsertOpts()
			return args, &opts
		},
		&river.PeriodicJobOpts{ID: "resource-health-five-seconds"},
	)
}
