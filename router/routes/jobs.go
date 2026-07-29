package routes

import "deploycrate-ce/internal/routing"

const SystemJobsPrefix = "/system/jobs"

var SystemJobs = routing.NewSimpleRoute(
	"",
	"system.jobs",
	SystemJobsPrefix,
)

var SystemJob = routing.NewRouteWithBigSerialID(
	"/:id",
	"system.job",
	SystemJobsPrefix,
)

var SystemJobRun = routing.NewRouteWithBigSerialID("/:id/run", "system.job.run", SystemJobsPrefix)
var SystemJobRetry = routing.NewRouteWithBigSerialID("/:id/retry", "system.job.retry", SystemJobsPrefix)
var SystemJobCancel = routing.NewRouteWithBigSerialID("/:id/cancel", "system.job.cancel", SystemJobsPrefix)
var SystemJobDestroy = routing.NewRouteWithBigSerialID("/:id", "system.job.destroy", SystemJobsPrefix)
