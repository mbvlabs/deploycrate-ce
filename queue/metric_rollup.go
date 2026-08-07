package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"

	"github.com/riverqueue/river"
)

type MetricRollupCollector interface {
	Collect(context.Context) error
}

type MetricRollupWorker struct {
	river.WorkerDefaults[jobs.MetricRollupArgs]
	service MetricRollupCollector
}

func NewMetricRollupWorker(service MetricRollupCollector) *MetricRollupWorker {
	return &MetricRollupWorker{service: service}
}

func (worker *MetricRollupWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *MetricRollupWorker) Work(
	ctx context.Context,
	_ *river.Job[jobs.MetricRollupArgs],
) error {
	return worker.service.Collect(ctx)
}

func NewMetricRollupPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return jobs.MetricRollupArgs{}, nil },
		&river.PeriodicJobOpts{ID: "metric-rollup-one-minute"},
	)
}
