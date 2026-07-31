package routes

import "deploycrate-ce/internal/routing"

const SystemTasksPrefix = "/system/tasks"

var SystemTasks = routing.NewSimpleRoute(
	"",
	"system.tasks",
	SystemTasksPrefix,
)

var SystemTask = routing.NewRouteWithBigSerialID(
	"/:id",
	"system.task",
	SystemTasksPrefix,
)

var SystemTaskRun = routing.NewRouteWithBigSerialID("/:id/run", "system.task.run", SystemTasksPrefix)
var SystemTaskRetry = routing.NewRouteWithBigSerialID("/:id/retry", "system.task.retry", SystemTasksPrefix)
var SystemTaskCancel = routing.NewRouteWithBigSerialID("/:id/cancel", "system.task.cancel", SystemTasksPrefix)
var SystemTaskDestroy = routing.NewRouteWithBigSerialID("/:id", "system.task.destroy", SystemTasksPrefix)
