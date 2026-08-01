package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type NodeEnrollmentWorker struct {
	river.WorkerDefaults[jobs.NodeEnrollmentArgs]
	service *services.NodeEnrollment
}

func NewNodeEnrollmentWorker(service *services.NodeEnrollment) *NodeEnrollmentWorker {
	return &NodeEnrollmentWorker{service: service}
}

func (worker *NodeEnrollmentWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *NodeEnrollmentWorker) Timeout(*river.Job[jobs.NodeEnrollmentArgs]) time.Duration {
	return 30 * time.Minute
}

func (worker *NodeEnrollmentWorker) Work(ctx context.Context, job *river.Job[jobs.NodeEnrollmentArgs]) error {
	if err := worker.service.Execute(ctx, job.Args.EnrollmentID); err != nil {
		return river.JobCancel(err)
	}
	return nil
}
