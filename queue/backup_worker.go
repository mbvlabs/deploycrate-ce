package queue

import (
	"context"
	"log/slog"
	"time"
)

func runBackupWorker(
	ctx context.Context,
	operation string,
	jobID int64,
	attempt int,
	attributes []any,
	work func() error,
) error {
	startedAt := time.Now()
	attributes = append(
		attributes,
		"operation",
		operation,
		"river_job_id",
		jobID,
		"attempt",
		attempt,
	)
	slog.InfoContext(ctx, "backup job started", attributes...)
	if err := work(); err != nil {
		failureAttributes := append(
			attributes,
			"duration", time.Since(startedAt),
			"error_category", "backup_"+operation,
			"error", err,
		)
		slog.ErrorContext(ctx, "backup job failed", failureAttributes...)
		return err
	}
	slog.InfoContext(
		ctx,
		"backup job completed",
		append(attributes, "duration", time.Since(startedAt))...,
	)
	return nil
}
