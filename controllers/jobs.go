package controllers

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river/rivertype"
)

const jobPageSize int64 = 25

type Jobs struct {
	db     storage.Pool
	jobs   queue.JobClient
	builds *services.EnvironmentSetup
}

func NewJobs(db storage.Pool, jobs queue.JobClient, builds *services.EnvironmentSetup) Jobs {
	return Jobs{db: db, jobs: jobs, builds: builds}
}

func (j Jobs) RegisterRoutes(r *router.Router) error {
	registrations := []echo.Route{
		{
			Method:      http.MethodGet,
			Path:        routes.SystemTasks.Path(),
			Name:        routes.SystemTasks.Name(),
			Handler:     j.Index,
			Middlewares: []echo.MiddlewareFunc{middleware.AuthOnly},
		},
		{
			Method:      http.MethodPost,
			Path:        routes.SystemTaskRun.Path(),
			Name:        routes.SystemTaskRun.Name(),
			Handler:     j.Run,
			Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		},
		{
			Method:      http.MethodPost,
			Path:        routes.SystemTaskRetry.Path(),
			Name:        routes.SystemTaskRetry.Name(),
			Handler:     j.Retry,
			Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		},
		{
			Method:      http.MethodPost,
			Path:        routes.SystemTaskCancel.Path(),
			Name:        routes.SystemTaskCancel.Name(),
			Handler:     j.Cancel,
			Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		},
		{
			Method:      http.MethodDelete,
			Path:        routes.SystemTaskDestroy.Path(),
			Name:        routes.SystemTaskDestroy.Name(),
			Handler:     j.Destroy,
			Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		},
		{
			Method:      http.MethodGet,
			Path:        routes.SystemTask.Path(),
			Name:        routes.SystemTask.Name(),
			Handler:     j.Show,
			Middlewares: []echo.MiddlewareFunc{middleware.AuthOnly},
		},
	}

	var errList []error
	for _, registration := range registrations {
		if _, err := r.AddRoute(registration); err != nil {
			errList = append(errList, err)
		}
	}

	return errors.Join(errList...)
}

type jobSummaryProps struct {
	ID          int64      `json:"id"`
	State       string     `json:"state"`
	Attempt     int16      `json:"attempt"`
	MaxAttempts int16      `json:"maxAttempts"`
	AttemptedAt *time.Time `json:"attemptedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	FinalizedAt *time.Time `json:"finalizedAt"`
	ScheduledAt time.Time  `json:"scheduledAt"`
	Priority    int16      `json:"priority"`
	Kind        string     `json:"kind"`
	Queue       string     `json:"queue"`
}

func newJobSummaryProps(entity models.JobSummary) jobSummaryProps {
	var attemptedAt *time.Time
	if entity.AttemptedAt.Valid {
		attemptedAt = &entity.AttemptedAt.Time
	}
	var finalizedAt *time.Time
	if entity.FinalizedAt.Valid {
		finalizedAt = &entity.FinalizedAt.Time
	}

	return jobSummaryProps{
		ID:          entity.ID,
		State:       entity.State,
		Attempt:     entity.Attempt,
		MaxAttempts: entity.MaxAttempts,
		AttemptedAt: attemptedAt,
		CreatedAt:   entity.CreatedAt,
		FinalizedAt: finalizedAt,
		ScheduledAt: entity.ScheduledAt,
		Priority:    entity.Priority,
		Kind:        entity.Kind,
		Queue:       entity.Queue,
	}
}

func newJobSummaryPropsList(entities []models.JobSummary) []jobSummaryProps {
	items := make([]jobSummaryProps, 0, len(entities))
	for _, entity := range entities {
		items = append(items, newJobSummaryProps(entity))
	}
	return items
}

type jobProps struct {
	ID           int64                    `json:"id"`
	State        string                   `json:"state"`
	Attempt      int                      `json:"attempt"`
	MaxAttempts  int                      `json:"maxAttempts"`
	AttemptedAt  *time.Time               `json:"attemptedAt"`
	AttemptedBy  []string                 `json:"attemptedBy"`
	CreatedAt    time.Time                `json:"createdAt"`
	FinalizedAt  *time.Time               `json:"finalizedAt"`
	ScheduledAt  time.Time                `json:"scheduledAt"`
	Priority     int                      `json:"priority"`
	Args         json.RawMessage          `json:"args"`
	Errors       []rivertype.AttemptError `json:"errors"`
	Kind         string                   `json:"kind"`
	Metadata     json.RawMessage          `json:"metadata"`
	Queue        string                   `json:"queue"`
	Tags         []string                 `json:"tags"`
	UniqueKey    string                   `json:"uniqueKey"`
	UniqueStates []rivertype.JobState     `json:"uniqueStates"`
	CanRun       bool                     `json:"canRun"`
	CanRestart   bool                     `json:"canRestart"`
	CanCancel    bool                     `json:"canCancel"`
	CanDelete    bool                     `json:"canDelete"`
}

func newJobProps(job *rivertype.JobRow) jobProps {
	attemptedBy := job.AttemptedBy
	if attemptedBy == nil {
		attemptedBy = []string{}
	}
	errorsList := job.Errors
	if errorsList == nil {
		errorsList = []rivertype.AttemptError{}
	}
	tags := job.Tags
	if tags == nil {
		tags = []string{}
	}
	uniqueStates := job.UniqueStates
	if uniqueStates == nil {
		uniqueStates = []rivertype.JobState{}
	}

	data := jobProps{
		ID:           job.ID,
		State:        string(job.State),
		Attempt:      job.Attempt,
		MaxAttempts:  job.MaxAttempts,
		AttemptedAt:  job.AttemptedAt,
		AttemptedBy:  attemptedBy,
		CreatedAt:    job.CreatedAt,
		FinalizedAt:  job.FinalizedAt,
		ScheduledAt:  job.ScheduledAt,
		Priority:     job.Priority,
		Args:         json.RawMessage(job.EncodedArgs),
		Errors:       errorsList,
		Kind:         job.Kind,
		Metadata:     json.RawMessage(job.Metadata),
		Queue:        job.Queue,
		Tags:         tags,
		UniqueKey:    hex.EncodeToString(job.UniqueKey),
		UniqueStates: uniqueStates,
		CanDelete:    job.State != rivertype.JobStateRunning,
	}
	switch job.State {
	case rivertype.JobStateScheduled, rivertype.JobStateRetryable, rivertype.JobStatePending:
		data.CanRun = true
	case rivertype.JobStateCancelled, rivertype.JobStateCompleted, rivertype.JobStateDiscarded:
		data.CanRestart = true
	case rivertype.JobStateAvailable, rivertype.JobStateRunning:
	}
	switch job.State {
	case rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRetryable,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled:
		data.CanCancel = true
	case rivertype.JobStateCancelled, rivertype.JobStateCompleted, rivertype.JobStateDiscarded:
	}
	return data
}

func validJobState(value string) bool {
	for _, state := range rivertype.JobStates() {
		if value == string(state) {
			return true
		}
	}
	return false
}

func (j Jobs) Index(etx *echo.Context) error {
	page := int64(1)
	if parsed, err := strconv.ParseInt(etx.QueryParam("page"), 10, 64); err == nil && parsed > 0 {
		page = parsed
	}

	filter := models.JobFilter{Search: strings.TrimSpace(etx.QueryParam("search"))}
	if state := etx.QueryParam("state"); validJobState(state) {
		filter.State = state
	}

	jobList, err := models.Job.Paginate(
		etx.Request().Context(),
		j.db.Executor(),
		filter,
		page,
		jobPageSize,
	)
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	stats, err := models.Job.Stats(etx.Request().Context(), j.db.Executor())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Page(etx, "Jobs/Index", inertia.Props{
		"auth":  authProps(etx),
		"items": newJobSummaryPropsList(jobList.Jobs),
		"pagination": map[string]int64{
			"page":       jobList.Page,
			"pageSize":   jobList.PageSize,
			"totalCount": jobList.TotalCount,
			"totalPages": jobList.TotalPages,
		},
		"filters": map[string]string{
			"state":  filter.State,
			"search": filter.Search,
		},
		"stats": map[string]any{
			"total":   stats.Total,
			"byState": stats.ByState,
		},
	})
}

func (j Jobs) Show(etx *echo.Context) error {
	jobID, err := strconv.ParseInt(etx.Param("id"), 10, 64)
	if err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}

	job, err := j.jobs.Job(etx.Request().Context(), jobID)
	if errors.Is(err, rivertype.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Page(etx, "Jobs/Show", inertia.Props{
		"auth": authProps(etx),
		"item": newJobProps(job),
	})
}

func jobIDParam(etx *echo.Context) (int64, error) {
	return strconv.ParseInt(etx.Param("id"), 10, 64)
}

func jobActionError(etx *echo.Context, jobID int64, action string, err error) error {
	if flashErr := cookies.AddFlash(
		etx,
		cookies.FlashError,
		fmt.Sprintf("Could not %s System Task: %v", action, err),
	); flashErr != nil {
		return flashErr
	}
	return inertia.Redirect(etx, routes.SystemTask.URL(jobID), http.StatusSeeOther)
}

func jobActionSuccess(etx *echo.Context, jobID int64, message string) error {
	if err := cookies.AddFlash(etx, cookies.FlashSuccess, message); err != nil {
		return err
	}
	return inertia.Redirect(etx, routes.SystemTask.URL(jobID), http.StatusSeeOther)
}

func (j Jobs) Run(etx *echo.Context) error {
	jobID, err := jobIDParam(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	if _, err := j.jobs.RunJobNow(etx.Request().Context(), jobID); err != nil {
		return jobActionError(etx, jobID, "run", err)
	}
	return jobActionSuccess(etx, jobID, "System Task made available to run now")
}

func (j Jobs) Retry(etx *echo.Context) error {
	jobID, err := jobIDParam(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	job, err := j.jobs.Job(etx.Request().Context(), jobID)
	if err == nil && job.Kind == "build_source" {
		err = j.builds.RetryBuildJob(etx.Request().Context(), jobID)
	} else if err == nil {
		_, err = j.jobs.RestartJob(etx.Request().Context(), jobID)
	}
	if err != nil {
		return jobActionError(etx, jobID, "restart", err)
	}
	return jobActionSuccess(etx, jobID, "System Task restarted and made available")
}

func (j Jobs) Cancel(etx *echo.Context) error {
	jobID, err := jobIDParam(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	job, err := j.jobs.Job(etx.Request().Context(), jobID)
	if err == nil && job.Kind == "build_source" {
		err = j.builds.StopBuildJob(etx.Request().Context(), jobID)
	} else if err == nil {
		_, err = j.jobs.CancelJob(etx.Request().Context(), jobID)
	}
	if err != nil {
		return jobActionError(etx, jobID, "cancel", err)
	}
	return jobActionSuccess(etx, jobID, "System Task cancellation requested")
}

func (j Jobs) Destroy(etx *echo.Context) error {
	jobID, err := jobIDParam(etx)
	if err != nil {
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}
	if _, err := j.jobs.DeleteJob(etx.Request().Context(), jobID); err != nil {
		return jobActionError(etx, jobID, "delete", err)
	}
	if err := cookies.AddFlash(
		etx,
		cookies.FlashSuccess,
		"System Task permanently deleted",
	); err != nil {
		return err
	}
	return inertia.Redirect(etx, routes.SystemTasks.URL(), http.StatusSeeOther)
}
