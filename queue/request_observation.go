package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type RequestObservationWorker struct {
	river.WorkerDefaults[jobs.RequestObservationArgs]
	service *services.RequestAnalytics
}

func NewRequestObservationWorker(service *services.RequestAnalytics) *RequestObservationWorker {
	return &RequestObservationWorker{service: service}
}

func (worker *RequestObservationWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *RequestObservationWorker) Work(
	ctx context.Context,
	_ *river.Job[jobs.RequestObservationArgs],
) error {
	return worker.service.Process(ctx)
}

func NewRequestObservationPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			args := jobs.RequestObservationArgs{}
			opts := args.InsertOpts()
			return args, &opts
		},
		&river.PeriodicJobOpts{ID: "request-observation-one-minute"},
	)
}
