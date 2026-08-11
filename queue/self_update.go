package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type SelfUpdateWorker struct {
	river.WorkerDefaults[jobs.SelfUpdateArgs]
	service *services.SelfUpdate
}

func NewSelfUpdateWorker(service *services.SelfUpdate) *SelfUpdateWorker {
	return &SelfUpdateWorker{service: service}
}

func (worker *SelfUpdateWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *SelfUpdateWorker) Timeout(*river.Job[jobs.SelfUpdateArgs]) time.Duration {
	return 20 * time.Minute
}

func (worker *SelfUpdateWorker) Work(
	ctx context.Context,
	job *river.Job[jobs.SelfUpdateArgs],
) error {
	return worker.service.Execute(ctx, job.Args.ActorID)
}

type SelfUpdateReconcileWorker struct {
	river.WorkerDefaults[jobs.SelfUpdateReconcileArgs]
	service *services.SelfUpdate
}

func NewSelfUpdateReconcileWorker(
	service *services.SelfUpdate,
) *SelfUpdateReconcileWorker {
	return &SelfUpdateReconcileWorker{service: service}
}

func (worker *SelfUpdateReconcileWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *SelfUpdateReconcileWorker) Timeout(
	*river.Job[jobs.SelfUpdateReconcileArgs],
) time.Duration {
	return 7 * time.Minute
}

func (worker *SelfUpdateReconcileWorker) Work(
	ctx context.Context,
	_ *river.Job[jobs.SelfUpdateReconcileArgs],
) error {
	return worker.service.Reconcile(ctx)
}

func NewSelfUpdateReconciliationPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(30*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			args := jobs.SelfUpdateReconcileArgs{}
			opts := args.InsertOpts()
			return args, &opts
		},
		&river.PeriodicJobOpts{
			ID:         "self-update-reconciliation",
			RunOnStart: true,
		},
	)
}
