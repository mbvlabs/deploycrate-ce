package services

import (
	"context"
	"deploycrate-ce/models"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

func (service *EnvironmentSetup) StartBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	if build.Status != "pending" && build.Status != "running" {
		return errors.New("only a pending or retrying Build can be started")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	if build.Status == "running" && job.State != "scheduled" && job.State != "retryable" &&
		job.State != "pending" {
		return errors.New("Build background job is already running")
	}
	if err := service.jobControl.RetryJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("start Build background job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build start requested by user")
	return nil
}

func (service *EnvironmentSetup) StopBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	if err := service.jobControl.CancelJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("stop Build background job: %w", err)
	}
	now := time.Now().UTC()
	if err := models.Build.MarkCancelled(ctx, tx, build.ID, now); err != nil {
		return err
	}
	if err := models.Change.MarkBuildCancelled(ctx, tx, build.ChangeID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build cancelled by user")
	return nil
}

func (service *EnvironmentSetup) StopBuildJob(ctx context.Context, jobID int64) error {
	buildID, err := models.Job.BuildID(ctx, service.db.Executor(), jobID)
	if err != nil {
		return err
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil {
		return err
	}
	environment, err := models.Environment.Find(ctx, service.db.Executor(), build.EnvironmentID)
	if err != nil {
		return err
	}
	return service.StopBuild(ctx, environment.ApplicationID, environment.ID, build.ID)
}

func (service *EnvironmentSetup) RetryBuild(
	ctx context.Context,
	applicationID, environmentID, buildID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil || build.EnvironmentID != environmentID {
		return errors.New("Build does not belong to this Environment")
	}
	if _, err := models.Environment.FindForApplication(
		ctx,
		tx,
		applicationID,
		environmentID,
	); err != nil {
		return errors.New("Build does not belong to this Application")
	}
	job, err := models.Job.FindForBuild(ctx, tx, build.ID)
	if err != nil {
		return errors.New("Build background job is unavailable")
	}
	now := time.Now().UTC()
	if err := models.Build.ResetForRetry(ctx, tx, build.ID, now); err != nil {
		return err
	}
	if err := models.Change.ResetBuildForRetry(ctx, tx, build.ChangeID, now); err != nil {
		return err
	}
	if err := service.jobControl.RetryJobTx(ctx, tx.Tx, job.ID); err != nil {
		return fmt.Errorf("retry Build background job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	service.recordBuildAction(ctx, build.ID, "Build retry requested by user")
	return nil
}

func (service *EnvironmentSetup) recordBuildAction(
	ctx context.Context,
	buildID uuid.UUID,
	message string,
) {
	logger, err := newBuildLogger(ctx, service.db, buildID)
	if err == nil {
		err = logger.System(ctx, message)
	}
	if err != nil {
		slog.WarnContext(ctx, "Could not record Build action", "build_id", buildID, "error", err)
	}
}

func (service *EnvironmentSetup) RetryBuildJob(ctx context.Context, jobID int64) error {
	buildID, err := models.Job.BuildID(ctx, service.db.Executor(), jobID)
	if err != nil {
		return err
	}
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil {
		return err
	}
	environment, err := models.Environment.Find(ctx, service.db.Executor(), build.EnvironmentID)
	if err != nil {
		return err
	}
	return service.RetryBuild(ctx, environment.ApplicationID, environment.ID, build.ID)
}
