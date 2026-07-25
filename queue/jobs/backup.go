package jobs

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const BackupQueue = "backups"

type BackupScheduleArgs struct {
	TargetType  string    `json:"target_type" river:"unique"`
	ScheduledAt time.Time `json:"scheduled_at" river:"unique"`
}

func (BackupScheduleArgs) Kind() string { return "backup_schedule" }

func (BackupScheduleArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
		Tags:        []string{"backup", "schedule"},
	}
}

type BackupExecuteArgs struct {
	BackupID uuid.UUID `json:"backup_id" river:"unique"`
}

func (BackupExecuteArgs) Kind() string { return "backup_execute" }

func (BackupExecuteArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: BackupQueue, MaxAttempts: 8,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"backup", "execute"},
	}
}

type BackupVerifyArgs struct {
	BackupID uuid.UUID `json:"backup_id" river:"unique"`
}

func (BackupVerifyArgs) Kind() string { return "backup_verify" }

func (BackupVerifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: BackupQueue, MaxAttempts: 8, Priority: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"backup", "verify"},
	}
}

type BackupRetentionArgs struct {
	BackupPolicyID uuid.UUID `json:"backup_policy_id" river:"unique"`
}

func (BackupRetentionArgs) Kind() string { return "backup_retention" }

func (BackupRetentionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: BackupQueue, MaxAttempts: 8,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"backup", "retention"},
	}
}
