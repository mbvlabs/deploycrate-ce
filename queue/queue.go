// Package queue provides queue-specific resources.
package queue

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/queue/jobs"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/robfig/cron/v3"

	"go.uber.org/fx"
)

type Processor struct {
	Client *river.Client[*sql.Tx]
}

const periodicJobsGroup = `group:"periodic_jobs"`

type ProcessorParams struct {
	fx.In

	DB           storage.Pool
	Workers      *river.Workers
	PeriodicJobs []*river.PeriodicJob `group:"periodic_jobs"`
}

func (p Processor) Shutdown(ctx context.Context) error {
	return p.Client.Stop(ctx)
}

func (p Processor) Start(ctx context.Context) error {
	return p.Client.Start(ctx)
}

func (p Processor) Stop(ctx context.Context) error {
	return p.Client.Stop(ctx)
}

func NewProcessor(params ProcessorParams) (Processor, error) {
	riverClient, err := river.NewClient(riverdatabasesql.New(params.DB.Conn()), &river.Config{
		PeriodicJobs: params.PeriodicJobs,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 100},
			jobs.BackupQueue:   {MaxWorkers: 1},
		},
		RescueStuckJobsAfter: 13 * time.Hour,
		Logger:               slog.Default(),
		Workers:              params.Workers,
	})
	if err != nil {
		return Processor{}, err
	}

	return Processor{riverClient}, nil
}

type InsertOnly struct {
	client *river.Client[*sql.Tx]
}

// Insert implements storage.InsertQueue.
func (i *InsertOnly) Insert(
	ctx context.Context,
	args river.JobArgs,
	opts *river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return i.client.Insert(ctx, args, opts)
}

// InsertMany implements storage.InsertQueue.
func (i *InsertOnly) InsertMany(
	ctx context.Context,
	params []river.InsertManyParams,
) ([]*rivertype.JobInsertResult, error) {
	return i.client.InsertMany(ctx, params)
}

// InsertManyFast implements storage.InsertQueue.
func (i *InsertOnly) InsertManyFast(
	ctx context.Context,
	params []river.InsertManyParams,
) (int, error) {
	return i.client.InsertManyFast(ctx, params)
}

// InsertManyFastTx implements storage.InsertQueue.
func (i *InsertOnly) InsertManyFastTx(
	ctx context.Context,
	tx *sql.Tx,
	params []river.InsertManyParams,
) (int, error) {
	return i.client.InsertManyFastTx(ctx, tx, params)
}

// InsertManyTx implements storage.InsertQueue.
func (i *InsertOnly) InsertManyTx(
	ctx context.Context,
	tx *sql.Tx,
	params []river.InsertManyParams,
) ([]*rivertype.JobInsertResult, error) {
	return i.client.InsertManyTx(ctx, tx, params)
}

// InsertTx implements storage.InsertQueue.
func (i *InsertOnly) InsertTx(
	ctx context.Context,
	tx *sql.Tx,
	args river.JobArgs,
	opts *river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return i.client.InsertTx(ctx, tx, args, opts)
}

var _ storage.InsertQueue = (*InsertOnly)(nil)

func NewInsertOnly(db storage.Pool, workers *river.Workers) (InsertOnly, error) {
	riverClient, err := river.NewClient(riverdatabasesql.New(db.Conn()), &river.Config{
		Workers: workers,
	})
	if err != nil {
		return InsertOnly{}, err
	}

	return InsertOnly{riverClient}, nil
}

func NewInsertQueue(insertOnly InsertOnly) storage.InsertQueue {
	return &insertOnly
}

type BackupPeriodicJobs struct {
	fx.Out

	Jobs []*river.PeriodicJob `group:"periodic_jobs,flatten"`
}

func NewBackupPeriodicJobs(configuration config.Config) (BackupPeriodicJobs, error) {
	if !configuration.Backups.Enabled {
		return BackupPeriodicJobs{}, nil
	}

	server, err := backupPeriodicJob("server", configuration.Backups.ServerSchedule)
	if err != nil {
		return BackupPeriodicJobs{}, err
	}
	periodicJobs := []*river.PeriodicJob{server}
	if configuration.Backups.DatabaseEnabled {
		database, err := backupPeriodicJob("resource", configuration.Backups.DatabaseSchedule)
		if err != nil {
			return BackupPeriodicJobs{}, err
		}
		periodicJobs = append(periodicJobs, database)
	}
	return BackupPeriodicJobs{Jobs: periodicJobs}, nil
}

func backupPeriodicJob(targetType, cronSpec string) (*river.PeriodicJob, error) {
	schedule, err := cron.ParseStandard(cronSpec)
	if err != nil {
		return nil, fmt.Errorf("parse %s backup cron: %w", targetType, err)
	}
	return river.NewPeriodicJob(
		schedule,
		func() (river.JobArgs, *river.InsertOpts) {
			return jobs.BackupScheduleArgs{
				TargetType:  targetType,
				ScheduledAt: time.Now().UTC().Truncate(time.Minute),
			}, nil
		},
		&river.PeriodicJobOpts{ID: "backup_" + targetType + "_schedule"},
	), nil
}

var Module = fx.Module(
	"queue",
	fx.Provide(
		NewInsertOnly,
		NewInsertQueue,
		NewProcessor,
		fx.Annotate(NewMetricRollupPeriodicJob, fx.ResultTags(periodicJobsGroup)),
		NewBackupPeriodicJobs,
	),
)
