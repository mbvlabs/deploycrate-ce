package jobs

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const BuildQueue = "builds"

type BuildSourceArgs struct {
	BuildID uuid.UUID `json:"build_id" river:"unique"`
}

func (BuildSourceArgs) Kind() string { return "build_source" }

func (BuildSourceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       BuildQueue,
		MaxAttempts: 8,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
		Tags:        []string{"build", "source"},
	}
}

func BuildSourceInsertOpts(buildID uuid.UUID) *river.InsertOpts {
	opts := BuildSourceArgs{BuildID: buildID}.InsertOpts()
	return &opts
}
