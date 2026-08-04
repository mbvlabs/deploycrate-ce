package queue

import (
	"context"
	"time"

	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
)

type DNSReconciliationWorker struct {
	river.WorkerDefaults[jobs.DNSReconciliationArgs]
	dns          *services.EnvironmentDNS
	environments *services.EnvironmentSetup
}

func NewDNSReconciliationWorker(dns *services.EnvironmentDNS, environments *services.EnvironmentSetup) *DNSReconciliationWorker {
	return &DNSReconciliationWorker{dns: dns, environments: environments}
}

func (worker *DNSReconciliationWorker) Register(workers *river.Workers) error {
	return river.AddWorkerSafely(workers, worker)
}

func (worker *DNSReconciliationWorker) Timeout(*river.Job[jobs.DNSReconciliationArgs]) time.Duration {
	return 2 * time.Minute
}

func (worker *DNSReconciliationWorker) Work(ctx context.Context, job *river.Job[jobs.DNSReconciliationArgs]) error {
	intent, err := worker.dns.Reconcile(ctx, job.Args.BindingID, job.Args.Generation)
	if err != nil {
		return err
	}
	if intent == nil {
		return nil
	}
	if _, err := worker.environments.QueueSourceDeployment(
		ctx, intent.ApplicationID, intent.EnvironmentID, intent.ActorID, intent.TriggerType, intent.Reference,
	); err != nil {
		return err
	}
	return worker.dns.MarkDeploymentDispatched(ctx, intent.BindingID, intent.Generation)
}
