package jobs

import (
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type ResourceHealthSweepArgs struct{}

func (ResourceHealthSweepArgs) Kind() string { return "resource_health_sweep" }

func (ResourceHealthSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
		Tags: []string{"resource", "health"},
	}
}
