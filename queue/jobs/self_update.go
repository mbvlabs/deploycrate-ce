package jobs

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const SelfUpdateQueue = "system_updates"

type SelfUpdateArgs struct {
	ActorID uuid.UUID `json:"actor_id"`
}

func (SelfUpdateArgs) Kind() string { return "self_update" }

func (SelfUpdateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       SelfUpdateQueue,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRetryable,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
		}},
		Tags: []string{"system", "update"},
	}
}

type SelfUpdateReconcileArgs struct{}

func (SelfUpdateReconcileArgs) Kind() string { return "self_update_reconcile" }

func (SelfUpdateReconcileArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       SelfUpdateQueue,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: time.Minute,
		},
		Tags: []string{"system", "update", "reconcile"},
	}
}
