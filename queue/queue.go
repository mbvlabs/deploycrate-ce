// Package queue provides queue-specific resources.
package queue

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/queue/jobs"
	"deploycrate-ce/services"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"

	"go.uber.org/fx"
)

type Processor struct {
	Client *river.Client[*sql.Tx]
	seeder *BackupScheduleSeeder
}

const periodicJobsGroup = `group:"periodic_jobs"`

type ProcessorParams struct {
	fx.In

	DB           storage.Pool
	Workers      *river.Workers
	PeriodicJobs []*river.PeriodicJob `group:"periodic_jobs"`
	Seeder       *BackupScheduleSeeder
}

func (p Processor) Shutdown(ctx context.Context) error {
	return p.Client.Stop(ctx)
}

func (p Processor) Start(ctx context.Context) error {
	return p.Client.Start(ctx)
}

func (p Processor) Seed(ctx context.Context) error {
	return p.seeder.Seed(ctx)
}

func (p Processor) Stop(ctx context.Context) error {
	return p.Client.Stop(ctx)
}

func (p Processor) Job(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return p.Client.JobGet(ctx, id)
}

func (p Processor) RunJobNow(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return p.Client.JobRetry(ctx, id)
}

func (p Processor) RestartJob(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return p.Client.JobRetry(ctx, id)
}

func (p Processor) CancelJob(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return p.Client.JobCancel(ctx, id)
}

func (p Processor) DeleteJob(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return p.Client.JobDelete(ctx, id)
}

func NewProcessor(params ProcessorParams) (Processor, error) {
	riverClient, err := river.NewClient(riverdatabasesql.New(params.DB.Conn()), &river.Config{
		PeriodicJobs: params.PeriodicJobs,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:   {MaxWorkers: 100},
			jobs.BackupQueue:     {MaxWorkers: 1},
			jobs.BuildQueue:      {MaxWorkers: 1},
			jobs.DeploymentQueue: {MaxWorkers: 1},
		},
		RescueStuckJobsAfter: 13 * time.Hour,
		Logger:               slog.Default(),
		Workers:              params.Workers,
	})
	if err != nil {
		return Processor{}, err
	}

	return Processor{Client: riverClient, seeder: params.Seeder}, nil
}

type InsertOnly struct {
	client *river.Client[*sql.Tx]
}

type JobControlOnly struct {
	client *river.Client[*sql.Tx]
}

func (control JobControlOnly) CancelJob(ctx context.Context, id int64) error {
	_, err := control.client.JobCancel(ctx, id)
	return err
}

func (control JobControlOnly) DeleteJob(ctx context.Context, id int64) error {
	_, err := control.client.JobDelete(ctx, id)
	return err
}

func (control JobControlOnly) CancelJobTx(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := control.client.JobCancelTx(ctx, tx, id)
	return err
}

func (control JobControlOnly) RetryJobTx(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := control.client.JobRetryTx(ctx, tx, id)
	return err
}

func (control JobControlOnly) DeleteJobTx(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := control.client.JobDeleteTx(ctx, tx, id)
	return err
}

// Insert implements storage.InsertQueue.
func (i InsertOnly) Insert(
	ctx context.Context,
	args river.JobArgs,
	opts *river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return i.client.Insert(ctx, args, opts)
}

// InsertMany implements storage.InsertQueue.
func (i InsertOnly) InsertMany(
	ctx context.Context,
	params []river.InsertManyParams,
) ([]*rivertype.JobInsertResult, error) {
	return i.client.InsertMany(ctx, params)
}

// InsertManyFast implements storage.InsertQueue.
func (i InsertOnly) InsertManyFast(
	ctx context.Context,
	params []river.InsertManyParams,
) (int, error) {
	return i.client.InsertManyFast(ctx, params)
}

// InsertManyFastTx implements storage.InsertQueue.
func (i InsertOnly) InsertManyFastTx(
	ctx context.Context,
	tx *sql.Tx,
	params []river.InsertManyParams,
) (int, error) {
	return i.client.InsertManyFastTx(ctx, tx, params)
}

// InsertManyTx implements storage.InsertQueue.
func (i InsertOnly) InsertManyTx(
	ctx context.Context,
	tx *sql.Tx,
	params []river.InsertManyParams,
) ([]*rivertype.JobInsertResult, error) {
	return i.client.InsertManyTx(ctx, tx, params)
}

// InsertTx implements storage.InsertQueue.
func (i InsertOnly) InsertTx(
	ctx context.Context,
	tx *sql.Tx,
	args river.JobArgs,
	opts *river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return i.client.InsertTx(ctx, tx, args, opts)
}

var _ storage.InsertQueue = InsertOnly{}

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

func NewBuildJobControl(insertOnly InsertOnly) services.BuildJobControl {
	return JobControlOnly{client: insertOnly.client}
}

var Module = fx.Module(
	"queue",
	fx.Provide(
		NewInsertOnly,
		NewInsertQueue,
		NewBuildJobControl,
		NewBackupScheduleSeeder,
		NewProcessor,
		fx.Annotate(NewMetricRollupPeriodicJob, fx.ResultTags(periodicJobsGroup)),
	),
)
