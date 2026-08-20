package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"
)

type JobSummary struct {
	bun.BaseModel `bun:"table:river_job,alias:river_job"`
	ID            int64        `bun:"id,pk"`
	State         string       `bun:"state"`
	Attempt       int16        `bun:"attempt"`
	MaxAttempts   int16        `bun:"max_attempts"`
	AttemptedAt   sql.NullTime `bun:"attempted_at"`
	CreatedAt     time.Time    `bun:"created_at"`
	FinalizedAt   sql.NullTime `bun:"finalized_at"`
	ScheduledAt   time.Time    `bun:"scheduled_at"`
	Priority      int16        `bun:"priority"`
	Kind          string       `bun:"kind"`
	Queue         string       `bun:"queue"`
}

type JobFilter struct {
	State  string
	Search string
}

type PaginatedJobs struct {
	Jobs       []JobSummary
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

type JobStats struct {
	Total   int64
	ByState map[string]int64
}

type BuildJobReference struct {
	ID    int64  `bun:"id"`
	State string `bun:"state"`
}

func (job) FindForDeployment(
	ctx context.Context,
	db storage.Executor,
	deploymentID uuid.UUID,
) (BuildJobReference, error) {
	var reference BuildJobReference
	err := db.NewSelect().TableExpr("river_job").ColumnExpr("id, state::text AS state").
		Where("kind = 'deploy_release'").
		Where("args ->> 'deployment_id' = ?", deploymentID.String()).
		OrderExpr("id DESC").Limit(1).Scan(ctx, &reference)
	return reference, err
}

func (job) FindForBuild(
	ctx context.Context,
	db storage.Executor,
	buildID uuid.UUID,
) (BuildJobReference, error) {
	var reference BuildJobReference
	err := db.NewSelect().TableExpr("river_job").ColumnExpr("id, state::text AS state").
		Where("kind = 'build_source'").Where("args ->> 'build_id' = ?", buildID.String()).
		OrderExpr("id DESC").Limit(1).Scan(ctx, &reference)
	return reference, err
}

func (job) DNSReconciliationQueued(
	ctx context.Context,
	db storage.Executor,
	bindingID uuid.UUID,
	generation int64,
) (bool, error) {
	count, err := db.NewSelect().TableExpr("river_job AS job").
		Where("job.kind = 'dns_reconciliation'").
		Where("job.args ->> 'binding_id' = ?", bindingID.String()).
		Where("job.args ->> 'generation' = ?", fmt.Sprint(generation)).
		Where("job.state::text IN ('available', 'pending', 'retryable', 'running', 'scheduled')").
		Count(ctx)
	return count > 0, err
}

func (job) BuildID(ctx context.Context, db storage.Executor, jobID int64) (uuid.UUID, error) {
	var value string
	if err := db.NewSelect().TableExpr("river_job").ColumnExpr("args ->> 'build_id'").
		Where("id = ?", jobID).Where("kind = 'build_source'").Scan(ctx, &value); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(value)
}

func applyJobFilters(query *bun.SelectQuery, filter JobFilter) *bun.SelectQuery {
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}

	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + search + "%"
		query = query.Where(
			"(kind ILIKE ? OR queue ILIKE ? OR CAST(id AS TEXT) = ?)",
			pattern,
			pattern,
			search,
		)
	}

	return query
}

func (job) Paginate(
	ctx context.Context,
	db storage.Executor,
	filter JobFilter,
	page int64,
	pageSize int64,
) (PaginatedJobs, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	totalCount, err := applyJobFilters(
		db.NewSelect().Model((*JobSummary)(nil)),
		filter,
	).Count(ctx)
	if err != nil {
		return PaginatedJobs{}, err
	}

	jobs := make([]JobSummary, 0, pageSize)
	if err := applyJobFilters(
		db.NewSelect().Model(&jobs),
		filter,
	).
		OrderExpr("created_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Scan(ctx); err != nil {
		return PaginatedJobs{}, err
	}

	return PaginatedJobs{
		Jobs:       jobs,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (int64(totalCount) + pageSize - 1) / pageSize,
	}, nil
}

func (job) Stats(ctx context.Context, db storage.Executor) (JobStats, error) {
	stateCounts := make([]struct {
		State string `bun:"state"`
		Count int64  `bun:"count"`
	}, 0, len(rivertype.JobStates()))

	if err := db.NewSelect().
		TableExpr("river_job").
		ColumnExpr("state::text AS state").
		ColumnExpr("COUNT(*) AS count").
		GroupExpr("state").
		Scan(ctx, &stateCounts); err != nil {
		return JobStats{}, err
	}

	stats := JobStats{ByState: make(map[string]int64, len(rivertype.JobStates()))}
	for _, state := range rivertype.JobStates() {
		stats.ByState[string(state)] = 0
	}
	for _, stateCount := range stateCounts {
		stats.ByState[stateCount.State] = stateCount.Count
		stats.Total += stateCount.Count
	}

	return stats, nil
}
