package services

import (
	"bytes"
	"context"
	"database/sql"
	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxPersistedReleaseCommandLogBytes = int64(2 << 20)

type ReleaseCommandExecution struct {
	db          storage.Pool
	queue       storage.InsertQueue
	secrets     *EnvironmentSecrets
	builds      *BuildExecution
	workloads   *WorkloadExecution
	deployments *DeploymentExecution
	releases    *ReleaseDeployment
}

func NewReleaseCommandExecution(
	db storage.Pool,
	queue storage.InsertQueue,
	secrets *EnvironmentSecrets,
	builds *BuildExecution,
	workloads *WorkloadExecution,
	deployments *DeploymentExecution,
	releases *ReleaseDeployment,
) *ReleaseCommandExecution {
	return &ReleaseCommandExecution{
		db:          db,
		queue:       queue,
		secrets:     secrets,
		builds:      builds,
		workloads:   workloads,
		deployments: deployments,
		releases:    releases,
	}
}

type releaseCommandLogger struct {
	ctx                executionContext
	mutex              sync.Mutex
	sequence           int64
	persisted          int64
	truncated          bool
	truncationRecorded bool
	redactions         [][]byte
	pending            map[string][]byte
	maxRedaction       int
}

type executionContext struct {
	db          storage.Pool
	executionID uuid.UUID
	attempt     int32
}

type releaseCommandStreamWriter struct {
	logger *releaseCommandLogger
	stream string
}

func (writer releaseCommandStreamWriter) Write(value []byte) (int, error) {
	return writer.logger.write(writer.stream, value)
}

func newReleaseCommandLogger(
	ctx context.Context,
	db storage.Pool,
	execution models.ReleaseCommandExecutionEntity,
	redactions []string,
) (*releaseCommandLogger, error) {
	sequence, err := models.ReleaseCommandLog.NextSequence(
		ctx,
		db.Executor(),
		execution.ID,
		execution.Attempt,
	)
	if err != nil {
		return nil, err
	}
	encodedRedactions := make([][]byte, 0, len(redactions))
	maxRedaction := 0
	for _, redaction := range redactions {
		if redaction == "" {
			continue
		}
		encoded := []byte(redaction)
		encodedRedactions = append(encodedRedactions, encoded)
		maxRedaction = max(maxRedaction, len(encoded))
	}
	sort.Slice(
		encodedRedactions,
		func(left, right int) bool { return len(encodedRedactions[left]) > len(encodedRedactions[right]) },
	)
	return &releaseCommandLogger{
		ctx: executionContext{
			db:          db,
			executionID: execution.ID,
			attempt:     execution.Attempt,
		},
		sequence:     sequence,
		redactions:   encodedRedactions,
		pending:      make(map[string][]byte),
		maxRedaction: maxRedaction,
	}, nil
}

func (logger *releaseCommandLogger) write(stream string, value []byte) (int, error) {
	original := len(value)
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	logger.pending[stream] = append(logger.pending[stream], value...)
	limit := max(len(logger.pending[stream])-logger.maxRedaction-utf8.UTFMax, 0)
	for limit > 0 && limit < len(logger.pending[stream]) && logger.pending[stream][limit]&0xc0 == 0x80 {
		limit--
	}
	message, consumed := logger.redactPrefix(logger.pending[stream], limit)
	logger.pending[stream] = append([]byte(nil), logger.pending[stream][consumed:]...)
	return original, logger.persist(stream, message)
}

func (logger *releaseCommandLogger) redactPrefix(value []byte, limit int) ([]byte, int) {
	message := make([]byte, 0, limit)
	consumed := 0
	for consumed < limit {
		matched := false
		for _, redaction := range logger.redactions {
			if len(value)-consumed >= len(redaction) &&
				bytes.Equal(value[consumed:consumed+len(redaction)], redaction) {
				message = append(message, "[REDACTED]"...)
				consumed += len(redaction)
				matched = true
				break
			}
		}
		if !matched {
			message = append(message, value[consumed])
			consumed++
		}
	}
	return message, consumed
}

func (logger *releaseCommandLogger) persist(stream string, value []byte) error {
	remaining := maxPersistedReleaseCommandLogBytes - logger.persisted
	if remaining <= 0 {
		logger.truncated = true
		return logger.recordTruncation()
	}
	message := strings.ReplaceAll(strings.ToValidUTF8(string(value), "�"), "\x00", "�")
	if int64(len(message)) > remaining {
		message = message[:remaining]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		logger.truncated = true
	}
	for strings.TrimSpace(message) != "" {
		chunk := message
		if len(chunk) > models.MaxReleaseCommandLogMessageBytes {
			chunk = chunk[:models.MaxReleaseCommandLogMessageBytes]
			for !utf8.ValidString(chunk) {
				chunk = chunk[:len(chunk)-1]
			}
		}
		if _, err := models.ReleaseCommandLog.Create(
			context.Background(),
			logger.ctx.db.Executor(),
			logger.ctx.executionID,
			logger.ctx.attempt,
			logger.sequence,
			stream,
			chunk,
		); err != nil {
			return err
		}
		logger.sequence++
		logger.persisted += int64(len(chunk))
		message = message[len(chunk):]
	}
	return logger.recordTruncation()
}

func (logger *releaseCommandLogger) flush(stream string) error {
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	message, consumed := logger.redactPrefix(logger.pending[stream], len(logger.pending[stream]))
	delete(logger.pending, stream)
	if consumed == 0 {
		return nil
	}
	return logger.persist(stream, message)
}

func (logger *releaseCommandLogger) flushOutput() error {
	return errors.Join(logger.flush("stdout"), logger.flush("stderr"))
}

func (logger *releaseCommandLogger) redactError(operationErr error) error {
	if operationErr == nil {
		return nil
	}
	message := []byte(operationErr.Error())
	for _, redaction := range logger.redactions {
		message = bytes.ReplaceAll(message, redaction, []byte("[REDACTED]"))
	}
	return errors.New(strings.ToValidUTF8(string(message), "�"))
}

func (logger *releaseCommandLogger) system(_ context.Context, message string) error {
	writer := releaseCommandStreamWriter{logger: logger, stream: "system"}
	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}
	return logger.flush("system")
}

func (logger *releaseCommandLogger) recordTruncation() error {
	if !logger.truncated || logger.truncationRecorded {
		return nil
	}
	logger.truncated = false
	logger.truncationRecorded = true
	_, err := models.ReleaseCommandLog.Create(
		context.Background(),
		logger.ctx.db.Executor(),
		logger.ctx.executionID,
		logger.ctx.attempt,
		logger.sequence,
		"system",
		"Release command output reached the 2 MiB persistence limit. Later output is omitted.",
	)
	logger.sequence++
	return err
}

func (service *ReleaseCommandExecution) Execute(ctx context.Context, executionID uuid.UUID) error {
	execution, err := service.claim(ctx, executionID)
	if err != nil {
		return err
	}
	if execution.Status == "succeeded" || execution.Status == "failed" ||
		execution.Status == "ambiguous" {
		return nil
	}
	configuration, err := models.ParseReleaseCommandConfigurationSnapshot(
		execution.Configuration,
		execution.ConfigurationDigest,
	)
	if err != nil {
		return service.fail(context.WithoutCancel(ctx), execution, false, nil, err)
	}
	release, revision, target, scope, err := service.loadScope(ctx, execution)
	if err != nil {
		return service.fail(context.WithoutCancel(ctx), execution, false, nil, err)
	}
	resolved, err := service.secrets.ResolveRevision(ctx, revision)
	if err != nil {
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			false,
			nil,
			fmt.Errorf("resolve exact Environment revision secrets: %w", err),
		)
	}
	redactions := make([]string, 0, len(resolved))
	for _, secret := range resolved {
		redactions = append(redactions, secret.Value)
	}
	logger, err := newReleaseCommandLogger(ctx, service.db, execution, redactions)
	if err != nil {
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			false,
			nil,
			fmt.Errorf("initialize release command logs: %w", err),
		)
	}
	if err := logger.system(
		ctx,
		fmt.Sprintf("Starting release command attempt %d", execution.Attempt),
	); err != nil {
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			false,
			logger,
			fmt.Errorf("persist release command start: %w", err),
		)
	}
	networkName, err := service.workloads.ReconcileNetwork(
		ctx,
		target.ServerID,
		scope.Environment.ID,
	)
	if err != nil {
		return service.fail(context.WithoutCancel(ctx), execution, false, logger, err)
	}
	environment, attachments, err := service.deployments.composeEnvironment(
		ctx,
		scope,
		resolved,
		false,
	)
	if err != nil {
		return service.fail(context.WithoutCancel(ctx), execution, false, logger, err)
	}
	for _, attachment := range attachments {
		if err := service.workloads.ConnectResourceContainer(
			ctx,
			target.ServerID,
			scope.Environment.ID,
			attachment,
		); err != nil {
			return service.fail(context.WithoutCancel(ctx), execution, false, logger, err)
		}
	}
	credentials, err := service.builds.RegistryCredentials(
		ctx,
		scope.RegistryID,
		scope.RegistryCredentialID,
		scope.RegistryEndpoint,
	)
	if err != nil {
		return service.fail(context.WithoutCancel(ctx), execution, false, logger, err)
	}
	commandContext, cancel := context.WithTimeout(
		ctx,
		time.Duration(configuration.TimeoutSeconds)*time.Second,
	)
	defer cancel()
	result, runErr := service.workloads.RunOneOff(
		commandContext,
		target.ServerID,
		containerclient.OneOffRunSpec{
			ApplicationID:             scope.ApplicationID,
			EnvironmentID:             scope.Environment.ID,
			TargetID:                  target.ID,
			ReleaseID:                 release.ID,
			ReleaseCommandExecutionID: execution.ID,
			Attempt:                   execution.Attempt,
			ContainerName: "dc-release-" + release.ID.String() + "-" + fmt.Sprint(
				execution.Attempt,
			),
			ImageReference: release.ArtifactReference,
			NetworkName:    networkName,
			Environment:    environment,
			Command:        append([]string{configuration.Command}, configuration.Arguments...),
			Stdout:         releaseCommandStreamWriter{logger: logger, stream: "stdout"},
			Stderr:         releaseCommandStreamWriter{logger: logger, stream: "stderr"},
			Created: func(identifier string) error {
				_, err := service.db.Executor().
					NewUpdate().
					TableExpr("release_command_executions").
					Set("external_id = ?", identifier).
					Set("updated_at = ?", time.Now().UTC()).
					Where("id = ?", execution.ID).
					Where("status = 'running'").
					Exec(context.WithoutCancel(ctx))
				return err
			},
		},
		credentials,
	)
	logErr := logger.flushOutput()
	if result.ContainerCreated && result.State.Exists && !result.State.Running &&
		result.State.Status == "exited" &&
		result.State.ExitCode == 0 {
		_ = logger.system(ctx, "Release command completed successfully")
		if err := service.releases.FanOutSucceededReleaseCommand(
			context.WithoutCancel(ctx),
			execution.ID,
			result.State.ExitCode,
			result.State.ID,
		); err != nil {
			return service.fail(
				context.WithoutCancel(ctx),
				execution,
				true,
				logger,
				fmt.Errorf("commit release command outcome and deployment fan-out: %w", err),
			)
		}
		_ = service.workloads.RemoveOneOff(
			context.WithoutCancel(ctx),
			target.ServerID,
			result.State.ID,
		)
		return nil
	}
	if runErr != nil {
		var exitCode *int32
		if result.ContainerCreated && result.State.Exists && !result.State.Running &&
			result.State.Status == "exited" {
			value := result.State.ExitCode
			exitCode = &value
		}
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			result.ContainerCreated,
			logger,
			errors.Join(runErrWithExit(runErr, exitCode), logErr),
		)
	}
	if logErr != nil {
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			result.ContainerCreated,
			logger,
			fmt.Errorf("persist release command output: %w", logErr),
		)
	}
	if result.State.ExitCode != 0 {
		exitCode := result.State.ExitCode
		return service.fail(
			context.WithoutCancel(ctx),
			execution,
			true,
			logger,
			runErrWithExit(errors.New("release command exited unsuccessfully"), &exitCode),
		)
	}
	return service.fail(
		context.WithoutCancel(ctx),
		execution,
		result.ContainerCreated,
		logger,
		errors.New("release command outcome is unavailable"),
	)
}

type releaseExitError struct {
	err      error
	exitCode *int32
}

func (err *releaseExitError) Error() string { return err.err.Error() }
func (err *releaseExitError) Unwrap() error { return err.err }

func runErrWithExit(err error, exitCode *int32) error {
	return &releaseExitError{err: err, exitCode: exitCode}
}

func (service *ReleaseCommandExecution) claim(
	ctx context.Context,
	id uuid.UUID,
) (models.ReleaseCommandExecutionEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ReleaseCommandExecutionEntity{}, err
	}
	defer tx.Rollback()
	execution, err := models.ReleaseCommandExecution.Lock(ctx, tx, id)
	if err != nil {
		return execution, err
	}
	if execution.Status == "queued" {
		now := time.Now().UTC()
		if err := models.ReleaseCommandExecution.MarkRunning(ctx, tx, id, now); err != nil {
			return execution, err
		}
		if err := models.Change.MarkRunning(ctx, tx, execution.ChangeID, now); err != nil {
			return execution, err
		}
		execution.Status = "running"
	}
	return execution, tx.Commit()
}

func (service *ReleaseCommandExecution) loadScope(
	ctx context.Context,
	execution models.ReleaseCommandExecutionEntity,
) (models.ReleaseEntity, models.EnvironmentStateRevisionEntity, models.EnvironmentTargetEntity, deploymentScope, error) {
	release, err := models.Release.Find(ctx, service.db.Executor(), execution.ReleaseID)
	if err != nil {
		return release, models.EnvironmentStateRevisionEntity{}, models.EnvironmentTargetEntity{}, deploymentScope{}, err
	}
	revision, err := models.EnvironmentStateRevision.Find(
		ctx,
		service.db.Executor(),
		execution.EnvironmentStateRevisionID,
	)
	if err != nil || revision.EnvironmentID != release.EnvironmentID {
		return release, revision, models.EnvironmentTargetEntity{}, deploymentScope{}, errors.New(
			"release command revision is unavailable or mismatched",
		)
	}
	target, err := models.EnvironmentTarget.Find(
		ctx,
		service.db.Executor(),
		execution.EnvironmentTargetID,
	)
	if err != nil || target.EnvironmentID != release.EnvironmentID || target.DetachedAt.Valid {
		return release, revision, target, deploymentScope{}, errors.New(
			"release command target is unavailable or detached",
		)
	}
	environment, err := models.Environment.Find(ctx, service.db.Executor(), release.EnvironmentID)
	if err != nil || environment.ArchivedAt.Valid {
		return release, revision, target, deploymentScope{}, errors.New(
			"release command Environment is unavailable",
		)
	}
	state, err := models.ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return release, revision, target, deploymentScope{}, err
	}
	scope := deploymentScope{
		Release:       release,
		Target:        target,
		Environment:   environment,
		Revision:      revision,
		State:         state,
		ApplicationID: environment.ApplicationID,
	}
	if release.RegistryResourceID != nil && release.RegistryCredentialID != nil &&
		release.RegistryEndpoint.Valid {
		scope.RegistryID, scope.RegistryCredentialID, scope.RegistryEndpoint = *release.RegistryResourceID, *release.RegistryCredentialID, release.RegistryEndpoint.String
	} else if release.BuildID != nil {
		build, err := models.Build.Find(ctx, service.db.Executor(), *release.BuildID)
		if err != nil {
			return release, revision, target, deploymentScope{}, err
		}
		snapshot, err := parseBuildSnapshot(build)
		if err != nil {
			return release, revision, target, deploymentScope{}, err
		}
		scope.RegistryID, scope.RegistryCredentialID, scope.RegistryEndpoint = snapshot.RegistryResourceID, snapshot.RegistryCredentialID, snapshot.RegistryEndpoint
	} else {
		return release, revision, target, deploymentScope{}, errors.New(
			"Release registry snapshot is unavailable",
		)
	}
	return release, revision, target, scope, nil
}

func (service *ReleaseCommandExecution) fail(
	ctx context.Context,
	execution models.ReleaseCommandExecutionEntity,
	created bool,
	logger *releaseCommandLogger,
	operationErr error,
) error {
	var exitFailure *releaseExitError
	var exitCode *int32
	if errors.As(operationErr, &exitFailure) {
		exitCode = exitFailure.exitCode
	}
	if logger != nil {
		operationErr = logger.redactError(operationErr)
		_ = logger.system(ctx, "Release command failed: "+operationErr.Error())
	}
	status := "failed"
	if created && exitCode == nil {
		status = "ambiguous"
	}
	now := time.Now().UTC()
	if err := models.ReleaseCommandExecution.MarkFailed(
		ctx,
		service.db.Executor(),
		execution.ID,
		status,
		exitCode,
		operationErr,
		now,
	); err != nil {
		return err
	}
	_ = models.Change.MarkFailed(ctx, service.db.Executor(), execution.ChangeID, operationErr, now)
	return nil
}

func (service *ReleaseCommandExecution) Retry(
	ctx context.Context,
	applicationID, environmentID, executionID, actorID uuid.UUID,
	replacementTargetID *uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	execution, err := models.ReleaseCommandExecution.Lock(ctx, tx, executionID)
	if err != nil {
		return err
	}
	release, err := models.Release.Find(ctx, tx, execution.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID {
		return errors.New("release command does not belong to this Environment")
	}
	environment, err := models.Environment.FindForApplication(ctx, tx, applicationID, environmentID)
	if err != nil || environment.ArchivedAt.Valid {
		return errors.New("Environment is unavailable")
	}
	targetID := execution.EnvironmentTargetID
	if replacementTargetID != nil {
		targetID = *replacementTargetID
	}
	target, err := models.EnvironmentTarget.Find(ctx, tx, targetID)
	if err != nil || target.EnvironmentID != environmentID || target.DetachedAt.Valid {
		return errors.New("release command retry target is unavailable")
	}
	if target.ID != execution.EnvironmentTargetID {
		if _, err := tx.NewUpdate().
			TableExpr("release_command_executions").
			Set("environment_target_id = ?", target.ID).
			Where("id = ?", execution.ID).
			Exec(ctx); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	if err := models.ReleaseCommandExecution.ResetForRetry(
		ctx,
		tx,
		execution.ID,
		&actorID,
		now,
	); err != nil {
		return err
	}
	if err := models.Change.ResetForRetry(ctx, tx, execution.ChangeID, now); err != nil {
		return err
	}
	nextAttempt := execution.Attempt + 1
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.ReleaseCommandArgs{ReleaseCommandExecutionID: execution.ID, Attempt: nextAttempt},
		jobs.ReleaseCommandInsertOpts(execution.ID, nextAttempt),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ReleaseCommandExecution) Logs(
	ctx context.Context,
	applicationID, environmentID, executionID uuid.UUID,
) ([]models.ReleaseCommandLogEntity, error) {
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return nil, err
	}
	execution, err := models.ReleaseCommandExecution.Find(ctx, service.db.Executor(), executionID)
	if err != nil {
		return nil, err
	}
	release, err := models.Release.Find(ctx, service.db.Executor(), execution.ReleaseID)
	if err != nil || release.EnvironmentID != environmentID {
		return nil, sql.ErrNoRows
	}
	return models.ReleaseCommandLog.ForExecution(ctx, service.db.Executor(), executionID)
}

var _ io.Writer = releaseCommandStreamWriter{}
