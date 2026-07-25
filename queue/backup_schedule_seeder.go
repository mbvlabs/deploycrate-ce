package queue

import (
	"context"
	"fmt"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
)

type BackupScheduleSeeder struct {
	db    storage.Pool
	queue storage.InsertQueue
}

func NewBackupScheduleSeeder(
	db storage.Pool,
	queue storage.InsertQueue,
) *BackupScheduleSeeder {
	return &BackupScheduleSeeder{db: db, queue: queue}
}

func (seeder *BackupScheduleSeeder) Seed(ctx context.Context) error {
	schedules, err := models.BackupPolicy.ActiveSchedules(ctx, seeder.db.Executor())
	if err != nil {
		return fmt.Errorf("load active backup schedules: %w", err)
	}
	for _, schedule := range schedules {
		if _, err := seeder.queue.Insert(
			ctx,
			jobs.BackupScheduleArgs{
				BackupPolicyID: schedule.ID,
				ScheduledAt:    schedule.NextRunAt,
			},
			jobs.BackupScheduleInsertOpts(schedule.NextRunAt),
		); err != nil {
			return fmt.Errorf("seed backup policy %s schedule: %w", schedule.ID, err)
		}
	}
	return nil
}
