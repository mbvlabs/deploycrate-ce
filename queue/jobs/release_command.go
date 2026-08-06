package jobs

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const ReleaseCommandQueue = "release_commands"

type ReleaseCommandArgs struct {
	ReleaseCommandExecutionID uuid.UUID `json:"release_command_execution_id" river:"unique"`
	Attempt                   int32     `json:"attempt" river:"unique"`
}

func (ReleaseCommandArgs) Kind() string { return "release_command" }

func (ReleaseCommandArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: ReleaseCommandQueue, MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"release", "command"},
	}
}

func ReleaseCommandInsertOpts(executionID uuid.UUID, attempt int32) *river.InsertOpts {
	opts := ReleaseCommandArgs{ReleaseCommandExecutionID: executionID, Attempt: attempt}.InsertOpts()
	return &opts
}
