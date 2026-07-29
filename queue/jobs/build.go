package jobs

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const BuildQueue = "builds"

const DeploymentQueue = "deployments"

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

type DeployReleaseArgs struct {
	DeploymentID uuid.UUID `json:"deployment_id" river:"unique"`
}

func (DeployReleaseArgs) Kind() string { return "deploy_release" }

func (DeployReleaseArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: DeploymentQueue, MaxAttempts: 8,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"deployment", "release"},
	}
}

func DeployReleaseInsertOpts(deploymentID uuid.UUID) *river.InsertOpts {
	opts := DeployReleaseArgs{DeploymentID: deploymentID}.InsertOpts()
	return &opts
}
