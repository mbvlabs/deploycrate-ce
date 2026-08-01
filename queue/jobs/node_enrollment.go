package jobs

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const NodeEnrollmentQueue = "node_enrollments"

type NodeEnrollmentArgs struct {
	EnrollmentID uuid.UUID `json:"enrollment_id" river:"unique"`
}

func (NodeEnrollmentArgs) Kind() string { return "node_enrollment" }

func (args NodeEnrollmentArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: NodeEnrollmentQueue, MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"node", "enrollment"},
	}
}

func NodeEnrollmentInsertOpts(id uuid.UUID) *river.InsertOpts {
	opts := NodeEnrollmentArgs{EnrollmentID: id}.InsertOpts()
	return &opts
}
