// Package queue provides queue-specific resources.
package queue

import (
	"github.com/riverqueue/river"
	"go.uber.org/fx"
)

var wrksConstructors = fx.Provide(
	river.NewWorkers,
	NewSendTransactionalEmailWorker,
	NewSendMarketingEmailWorker,
	NewMetricRollupWorker,
	NewResourceHealthSweepWorker,
	NewBackupScheduleWorker,
	NewBackupExecuteWorker,
	NewBackupVerifyWorker,
	NewBackupRetentionWorker,
	NewResourceRestorePrepareWorker,
	NewResourceRestoreApplyWorker,
	NewBuildSourceWorker,
	NewDeployReleaseWorker,
)

var WorkersModule = fx.Module(
	"queue-workers",
	wrksConstructors,
	fx.Invoke(func(workers *river.Workers, worker *SendTransactionalEmailWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *SendMarketingEmailWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *MetricRollupWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *ResourceHealthSweepWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *BackupScheduleWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *BackupExecuteWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *BackupVerifyWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *BackupRetentionWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *ResourceRestorePrepareWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *ResourceRestoreApplyWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *BuildSourceWorker) error {
		return worker.Register(workers)
	}),
	fx.Invoke(func(workers *river.Workers, worker *DeployReleaseWorker) error {
		return worker.Register(workers)
	}),
)
