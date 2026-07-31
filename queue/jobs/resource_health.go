package jobs

import "github.com/riverqueue/river"

type ResourceHealthSweepArgs struct{}

func (ResourceHealthSweepArgs) Kind() string { return "resource_health_sweep" }

func (ResourceHealthSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
		Tags:        []string{"resource", "health"},
	}
}
