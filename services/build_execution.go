package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	buildpacksclient "deploycrate-ce/clients/buildpacks"
	githubclient "deploycrate-ce/clients/github"
	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/buildpacks/nodeassets"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/sudo"
	"deploycrate-ce/models"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	buildWorkspaceRoot        = "/var/lib/deploycrate-builds"
	buildWorkspaceOwner       = "deploycrate"
	maxExtractedArchiveBytes  = int64(2 << 30)
	maxArchiveFiles           = 100000
	maxPersistedBuildLogBytes = int64(2 << 20)
	buildTimeout              = 45 * time.Minute
)

type PermanentBuildError struct{ Err error }

func (failure *PermanentBuildError) Error() string { return failure.Err.Error() }
func (failure *PermanentBuildError) Unwrap() error { return failure.Err }

type BuildExecution struct {
	db       storage.Pool
	config   config.Config
	github   *GitHubConnection
	pack     buildpacksclient.Client
	registry registryclient.Client
	servers  *ServerExecution
	releases *ReleaseDeployment
}

type buildLogger struct {
	ctx                context.Context
	db                 storage.Pool
	buildID            uuid.UUID
	mutex              sync.Mutex
	nextSequence       int64
	persistedPackBytes int64
	truncated          bool
	persistenceErr     error
}

func newBuildLogger(ctx context.Context, db storage.Pool, buildID uuid.UUID) (*buildLogger, error) {
	sequence, err := models.BuildLog.NextSequence(ctx, db.Executor(), buildID)
	if err != nil {
		return nil, err
	}
	packBytes, err := models.BuildLog.PackBytes(ctx, db.Executor(), buildID)
	if err != nil {
		return nil, err
	}
	return &buildLogger{
		ctx:                ctx,
		db:                 db,
		buildID:            buildID,
		nextSequence:       sequence,
		persistedPackBytes: packBytes,
	}, nil
}

func (logger *buildLogger) Write(value []byte) (int, error) {
	original := len(value)
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	if logger.persistenceErr != nil {
		return original, nil
	}
	remaining := maxPersistedBuildLogBytes - logger.persistedPackBytes
	if remaining <= 0 {
		logger.recordTruncationLocked()
		return original, nil
	}
	if int64(len(value)) > remaining {
		value = value[:remaining]
		logger.truncated = true
	}
	message := strings.ToValidUTF8(string(value), "�")
	for message != "" {
		chunk := truncateBuildLogMessage(message, false)
		if strings.TrimSpace(chunk) != "" {
			if err := logger.appendLocked(logger.ctx, "pack", chunk); err != nil {
				logger.persistenceErr = err
				slog.WarnContext(
					logger.ctx,
					"Build output could not be persisted",
					"build_id",
					logger.buildID,
					"error",
					err,
				)
				return original, nil
			}
			logger.persistedPackBytes += int64(len(chunk))
		}
		message = message[len(chunk):]
	}
	if logger.truncated {
		logger.recordTruncationLocked()
	}
	return original, nil
}

func (logger *buildLogger) System(ctx context.Context, message string) error {
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	return logger.appendLocked(ctx, "system", truncateBuildLogMessage(message, true))
}

func (logger *buildLogger) PersistenceError() error {
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	return logger.persistenceErr
}

func (logger *buildLogger) recordTruncationLocked() {
	if !logger.truncated || logger.persistenceErr != nil {
		return
	}
	logger.truncated = false
	if err := logger.appendLocked(
		logger.ctx,
		"system",
		"Pack output reached the 2 MiB persistence limit. Later output is omitted, but the terminal error will still be retained.",
	); err != nil {
		logger.persistenceErr = err
	}
}

func (logger *buildLogger) appendLocked(ctx context.Context, stream, message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	_, err := models.BuildLog.Create(ctx, logger.db.Executor(), models.CreateBuildLogData{
		Sequence:   logger.nextSequence,
		Stream:     stream,
		Message:    message,
		OccurredAt: time.Now().UTC(),
		BuildID:    logger.buildID,
	})
	if err == nil {
		logger.nextSequence++
	}
	return err
}

func truncateBuildLogMessage(message string, preserveTail bool) string {
	message = strings.ReplaceAll(strings.ToValidUTF8(message, "�"), "\x00", "�")
	if len(message) <= models.MaxBuildLogMessageBytes {
		return message
	}
	if preserveTail {
		message = message[len(message)-models.MaxBuildLogMessageBytes:]
		for !utf8.ValidString(message) {
			message = message[1:]
		}
		return message
	}
	message = message[:models.MaxBuildLogMessageBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}

func NewBuildExecution(
	db storage.Pool,
	cfg config.Config,
	github *GitHubConnection,
	servers *ServerExecution,
	releases *ReleaseDeployment,
) *BuildExecution {
	return &BuildExecution{
		db:       db,
		config:   cfg,
		github:   github,
		pack:     buildpacksclient.New(),
		registry: registryclient.New(),
		servers:  servers,
		releases: releases,
	}
}

func (service *BuildExecution) Execute(ctx context.Context, buildID uuid.UUID) error {
	build, err := service.claim(ctx, buildID)
	if err != nil {
		return err
	}
	if build.Status == "succeeded" || build.Status == "failed" || build.Status == "cancelled" {
		return nil
	}
	logger, err := newBuildLogger(ctx, service.db, build.ID)
	if err != nil {
		return fmt.Errorf("initialize Build logging: %w", err)
	}
	recordTiming := func(phase string, started time.Time) {
		duration := max(time.Since(started), 0)
		message := fmt.Sprintf(
			"Timing: %s completed in %s",
			phase,
			duration.Round(time.Millisecond),
		)
		if logErr := logger.System(ctx, message); logErr != nil {
			slog.WarnContext(
				ctx,
				"Build timing could not be persisted",
				"build_id",
				build.ID,
				"phase",
				phase,
				"error",
				logErr,
			)
		}
		slog.InfoContext(
			ctx,
			"Build phase completed",
			"build_id",
			build.ID,
			"phase",
			phase,
			"duration",
			duration,
		)
	}
	if build.StartedAt.Valid {
		queueWait := max(build.StartedAt.Time.Sub(build.CreatedAt), 0)
		message := fmt.Sprintf(
			"Timing: queue wait completed in %s",
			queueWait.Round(time.Millisecond),
		)
		if err := logger.System(ctx, message); err != nil {
			slog.WarnContext(
				ctx,
				"Build queue timing could not be persisted",
				"build_id",
				build.ID,
				"error",
				err,
			)
		}
	}
	fail := func(operationErr error) error {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		current, currentErr := models.Build.Find(persistCtx, service.db.Executor(), build.ID)
		if currentErr == nil && current.Status == "cancelled" {
			return &PermanentBuildError{Err: errors.New("Build cancelled by user")}
		}
		_ = models.Build.MarkFailed(
			persistCtx,
			service.db.Executor(),
			build.ID,
			operationErr,
			time.Now().UTC(),
		)
		_ = models.Change.MarkFailed(
			persistCtx,
			service.db.Executor(),
			build.ChangeID,
			operationErr,
			time.Now().UTC(),
		)
		slog.ErrorContext(persistCtx, "Build failed", "build_id", build.ID, "error", operationErr)
		return &PermanentBuildError{Err: operationErr}
	}
	progress := func(step, message string) error {
		now := time.Now().UTC()
		if err := models.Build.MarkProgress(
			ctx,
			service.db.Executor(),
			build.ID,
			step,
			now,
		); err != nil {
			return err
		}
		if err := logger.System(ctx, message); err != nil {
			return err
		}
		slog.InfoContext(
			ctx,
			"Build progress",
			"build_id",
			build.ID,
			"step",
			step,
			"message",
			message,
		)
		return nil
	}
	if err := progress(
		"validating_configuration",
		"Validating the Build configuration",
	); err != nil {
		return err
	}
	snapshot, err := parseBuildSnapshot(build)
	if err != nil {
		return fail(err)
	}
	buildTarget, err := service.servers.Target(ctx, snapshot.ServerID, models.ServerCapabilityBuild)
	if err != nil {
		return fail(fmt.Errorf("load selected Build Server: %w", err))
	}
	if _, err := buildpacksclient.ProfileForArchitecture(
		string(snapshot.parsedSettings.Runtime),
		buildTarget.Server.Architecture.String,
	); err != nil {
		return fail(err)
	}
	if err := progress("loading_source", "Loading the GitHub source configuration"); err != nil {
		return err
	}
	repository, installation, err := service.loadGitHubSource(ctx, build)
	if err != nil {
		return fail(fmt.Errorf("load GitHub Build source: %w", err))
	}
	if err := progress("preparing_workspace", "Preparing the Build workspace"); err != nil {
		return err
	}
	workspace := filepath.Join(buildWorkspaceRoot, build.ID.String())
	if err := removeBuildWorkspace(ctx, workspace); err != nil {
		return fmt.Errorf("clear stale Build workspace: %w", err)
	}
	if err := createBuildDirectory(ctx, workspace); err != nil {
		return fmt.Errorf("create Build workspace: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cleanupCancel()
		_ = removeBuildWorkspace(cleanupCtx, workspace)
	}()
	archivePath := filepath.Join(workspace, "source.tar.gz")
	if err := progress("downloading_source", "Downloading the exact GitHub revision"); err != nil {
		return err
	}
	downloadStarted := time.Now()
	archiveCommand := sudo.CommandContext(
		ctx,
		"/usr/bin/install",
		"-m",
		"0600",
		"/dev/stdin",
		archivePath,
	)
	archiveInput, err := archiveCommand.StdinPipe()
	if err != nil {
		return fmt.Errorf("open privileged Build archive input: %w", err)
	}
	var archiveError bytes.Buffer
	archiveCommand.Stderr = &archiveError
	if err := archiveCommand.Start(); err != nil {
		return fmt.Errorf("create privileged Build archive: %w", err)
	}
	downloadErr := service.github.DownloadArchive(
		ctx,
		installation,
		repository,
		build.SourceRevision,
		archiveInput,
	)
	closeErr := archiveInput.Close()
	archiveWriteErr := archiveCommand.Wait()
	if downloadErr != nil {
		if errors.Is(downloadErr, context.Canceled) ||
			errors.Is(downloadErr, context.DeadlineExceeded) {
			return downloadErr
		}
		if errors.Is(downloadErr, githubclient.ErrUnauthorized) ||
			errors.Is(downloadErr, githubclient.ErrNotFound) {
			return fail(downloadErr)
		}
		if archiveWriteErr == nil {
			return downloadErr
		}
	}
	if archiveWriteErr != nil {
		return fmt.Errorf(
			"write privileged Build archive: %w: %s",
			archiveWriteErr,
			strings.TrimSpace(archiveError.String()),
		)
	}
	if closeErr != nil {
		return fmt.Errorf("close Build archive: %w", closeErr)
	}
	recordTiming("source download", downloadStarted)
	sourceRoot := filepath.Join(workspace, "source")
	if err := progress(
		"extracting_source",
		"Validating and extracting the GitHub archive",
	); err != nil {
		return err
	}
	extractionStarted := time.Now()
	if err := extractGitHubArchive(ctx, archivePath, sourceRoot); err != nil {
		return fail(err)
	}
	recordTiming("source extraction", extractionStarted)
	if err := progress(
		"validating_context",
		"Validating the configured Buildpacks context",
	); err != nil {
		return err
	}
	contextPath, err := secureBuildContext(ctx, sourceRoot, snapshot.ContextPath)
	if err != nil {
		if isRetryableBuildFailure(err) {
			return err
		}
		return fail(err)
	}
	if snapshot.parsedSettings.Runtime == models.BuildpackRuntimeGo {
		if err := sudo.CommandContext(ctx, "/usr/bin/test", "-f", filepath.Join(contextPath, "go.mod")).
			Run(); err != nil {
			return fail(errors.New("Go Buildpacks context must contain go.mod"))
		}
	}
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	if err := progress("loading_registry", "Loading registry credentials"); err != nil {
		return err
	}
	registryStarted := time.Now()
	credentials, err := service.registryCredentials(ctx, snapshot)
	if err != nil {
		operationErr := fmt.Errorf("load Build registry credentials: %w", err)
		if isRetryableBuildFailure(operationErr) {
			return operationErr
		}
		return fail(operationErr)
	}
	var dockerEnvironment []string
	var closeAuthentication func() error
	if !buildTarget.Remote {
		authentication, authenticateErr := service.registry.Authenticate(ctx, credentials)
		if authenticateErr != nil {
			operationErr := fmt.Errorf("authenticate Build registry: %w", authenticateErr)
			if isRetryableBuildFailure(operationErr) {
				return operationErr
			}
			return fail(operationErr)
		}
		dockerEnvironment = authentication.Environment()
		closeAuthentication = authentication.Close
	}
	recordTiming("registry authentication", registryStarted)
	if closeAuthentication != nil {
		defer func() {
			if closeErr := closeAuthentication(); closeErr != nil {
				slog.WarnContext(
					context.WithoutCancel(ctx),
					"Build registry authentication cleanup failed",
					"build_id",
					build.ID,
					"error",
					closeErr,
				)
			}
		}()
	}
	imageTag := strings.TrimSuffix(
		snapshot.RegistryEndpoint,
		"/",
	) + "/" + strings.Trim(
		snapshot.ImageRepository,
		"/",
	) + ":build-" + build.ID.String()
	caches, err := buildpacksclient.EnvironmentCacheNames(build.EnvironmentID)
	if err != nil {
		return fail(err)
	}
	previousImage := ""
	previousRelease, err := models.Release.LatestBuildForEnvironment(
		ctx,
		service.db.Executor(),
		build.EnvironmentID,
	)
	if err == nil {
		previousImage = previousRelease.ArtifactReference
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if previousImage == "" {
		if err := logger.System(
			ctx,
			fmt.Sprintf(
				"Using Pack caches %s and %s without a previous Release image",
				caches.Build,
				caches.Launch,
			),
		); err != nil {
			return err
		}
	} else if err := logger.System(ctx, fmt.Sprintf("Using Pack caches %s and %s with previous Release image %s", caches.Build, caches.Launch, previousImage)); err != nil {
		return err
	}
	reportDirectory := filepath.Join(workspace, "report")
	if err := createBuildDirectory(ctx, reportDirectory); err != nil {
		return fmt.Errorf("create Pack report directory: %w", err)
	}
	temporaryDirectory := filepath.Join(workspace, "tmp")
	if err := createBuildDirectory(ctx, temporaryDirectory); err != nil {
		return fmt.Errorf("create Pack temporary directory: %w", err)
	}
	if err := progress(
		"building_image",
		"Running Cloud Native Buildpacks and publishing the application image",
	); err != nil {
		return err
	}
	frontendScript := ""
	frontendDirectory := ""
	if snapshot.parsedSettings.Frontend != nil {
		frontendScript = snapshot.parsedSettings.Frontend.Script
		frontendDirectory = snapshot.parsedSettings.Frontend.Directory
	}
	packStarted := time.Now()
	buildSpec := buildpacksclient.BuildSpec{
		Image:              imageTag,
		Path:               contextPath,
		ReportDirectory:    reportDirectory,
		TemporaryDirectory: temporaryDirectory,
		BuildCache:         caches.Build,
		LaunchCache:        caches.Launch,
		PreviousImage:      previousImage,
		PullPolicy:         buildpacksclient.PullPolicyIfNotPresent,
		DockerEnvironment:  dockerEnvironment,
		BPGOTargets:        snapshot.BPGOTargets,
		FrontendScript:     frontendScript,
		FrontendDirectory:  frontendDirectory,
		Runtime:            string(snapshot.parsedSettings.Runtime),
		Output:             logger,
	}
	if buildTarget.Remote {
		err = service.buildRemote(
			buildCtx,
			buildTarget,
			build,
			snapshot,
			archivePath,
			buildSpec,
			credentials,
			logger,
		)
	} else {
		_, err = service.pack.Build(buildCtx, buildSpec)
	}
	recordTiming("Pack execution", packStarted)
	if persistenceErr := logger.PersistenceError(); persistenceErr != nil {
		slog.WarnContext(
			ctx,
			"Build output logging became unavailable",
			"build_id",
			build.ID,
			"error",
			persistenceErr,
		)
	}
	if err != nil {
		return fail(err)
	}
	if err := progress("resolving_artifact", "Resolving the published image digest"); err != nil {
		return err
	}
	digestStarted := time.Now()
	immutableReference, err := service.registry.ResolveRemoteDigest(ctx, credentials, imageTag)
	if err != nil {
		return err
	}
	digest, err := hex.DecodeString(
		strings.TrimPrefix(
			immutableReference[strings.LastIndex(immutableReference, "@")+1:],
			"sha256:",
		),
	)
	if err != nil || len(digest) != 32 {
		return errors.New("published registry digest is invalid")
	}
	recordTiming("artifact digest resolution", digestStarted)
	if err := progress(
		"finalizing",
		"Creating the Release and queueing its Deployment",
	); err != nil {
		return err
	}
	finalizationStarted := time.Now()
	if err := service.complete(ctx, build.ID, snapshot, immutableReference, digest); err != nil {
		return err
	}
	recordTiming("Build finalization", finalizationStarted)
	if err := logger.System(ctx, "Build completed successfully"); err != nil {
		slog.WarnContext(
			ctx,
			"Build completion log could not be persisted",
			"build_id",
			build.ID,
			"error",
			err,
		)
	}
	slog.InfoContext(ctx, "Build completed", "build_id", build.ID, "artifact", immutableReference)
	return nil
}

func (service *BuildExecution) buildRemote(
	ctx context.Context,
	target ServerExecutionTarget,
	build models.BuildEntity,
	snapshot buildSnapshot,
	archivePath string,
	spec buildpacksclient.BuildSpec,
	credentials registryclient.Credentials,
	logger io.Writer,
) error {
	workspace := filepath.Join(buildWorkspaceRoot, build.ID.String())
	sourceRoot := filepath.Join(workspace, "source")
	reportDirectory := filepath.Join(workspace, "report")
	temporaryDirectory := filepath.Join(workspace, "tmp")
	contextPath := filepath.Join(sourceRoot, filepath.FromSlash(snapshot.ContextPath))
	if contextPath != sourceRoot &&
		!strings.HasPrefix(contextPath, sourceRoot+string(filepath.Separator)) {
		return errors.New("remote Buildpacks context escaped the Build workspace")
	}
	setup := []byte("set -euo pipefail\n" +
		shellJoin("/usr/bin/rm", "-rf", "--", workspace) + "\n" +
		shellJoin(
			"/usr/bin/install",
			"-d",
			"-m",
			"0700",
			workspace,
			sourceRoot,
			reportDirectory,
			temporaryDirectory,
		) + "\n")
	if _, err := service.servers.RunRootScript(ctx, target, setup); err != nil {
		return fmt.Errorf("prepare remote Build workspace: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_, _ = service.servers.RunRootCommand(
			cleanupCtx,
			target,
			nil,
			"/usr/bin/rm",
			"-rf",
			"--",
			workspace,
		)
	}()
	if err := service.streamCommandToRemote(
		ctx,
		target,
		sudo.CommandContext(ctx, "/usr/bin/cat", "--", archivePath),
		"/usr/bin/tar",
		"--extract",
		"--gzip",
		"--file",
		"-",
		"--directory",
		sourceRoot,
		"--strip-components",
		"1",
		"--no-same-owner",
		"--no-same-permissions",
	); err != nil {
		return fmt.Errorf("stage source on selected Build Server: %w", err)
	}
	if snapshot.parsedSettings.Runtime == models.BuildpackRuntimeGo {
		if _, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			"/usr/bin/test",
			"-f",
			filepath.Join(contextPath, "go.mod"),
		); err != nil {
			return errors.New(
				"Go Buildpacks context must contain go.mod on the selected Build Server",
			)
		}
	}

	assetsBuildpack := ""
	if spec.FrontendScript != "" {
		localAssets, err := nodeassets.Materialize(
			filepath.Join(spec.TemporaryDirectory, "buildpacks"),
		)
		if err != nil {
			return err
		}
		remoteAssetsParent := filepath.Join(temporaryDirectory, "buildpacks")
		assetsBuildpack = filepath.Join(remoteAssetsParent, "node-assets")
		if _, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			"/usr/bin/install",
			"-d",
			"-m",
			"0755",
			assetsBuildpack,
		); err != nil {
			return err
		}
		if err := service.streamCommandToRemote(
			ctx,
			target,
			exec.CommandContext(
				ctx,
				"/usr/bin/tar",
				"--create",
				"--gzip",
				"--file",
				"-",
				"--directory",
				localAssets,
				".",
			),
			"/usr/bin/tar",
			"--extract",
			"--gzip",
			"--file",
			"-",
			"--directory",
			assetsBuildpack,
		); err != nil {
			return fmt.Errorf("stage frontend Buildpack on selected Build Server: %w", err)
		}
	}

	arguments, err := remotePackArguments(
		target.Server.Architecture.String,
		spec,
		contextPath,
		reportDirectory,
		assetsBuildpack,
	)
	if err != nil {
		return err
	}
	authDirectory := filepath.Join(workspace, "registry-auth")
	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	script.WriteString(shellJoin("/usr/bin/install", "-d", "-m", "0700", authDirectory))
	script.WriteString("\nexport HOME=")
	script.WriteString(shellQuote(authDirectory))
	script.WriteString(" DOCKER_CONFIG=")
	script.WriteString(shellQuote(authDirectory))
	script.WriteString(" TMPDIR=")
	script.WriteString(shellQuote(temporaryDirectory))
	script.WriteString("\n/usr/bin/printf '%s' ")
	script.WriteString(shellQuote(base64.StdEncoding.EncodeToString([]byte(credentials.Password))))
	script.WriteString(" | /usr/bin/base64 --decode | ")
	script.WriteString(
		shellJoin(
			remoteDockerExecutable,
			"login",
			credentials.Endpoint,
			"--username",
			credentials.Username,
			"--password-stdin",
		),
	)
	script.WriteString(" >/dev/null\n")
	script.WriteString(shellJoin("/usr/local/bin/pack", arguments...))
	script.WriteString("\n")
	scriptBytes := []byte(script.String())
	defer clear(scriptBytes)
	result, err := service.servers.RunRootScript(ctx, target, scriptBytes)
	if result.Stdout != "" {
		_, _ = io.WriteString(logger, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = io.WriteString(logger, result.Stderr)
	}
	if err != nil {
		return fmt.Errorf("Pack build failed on Server %s: %w", target.Server.Name, err)
	}
	return nil
}

func (service *BuildExecution) streamCommandToRemote(
	ctx context.Context,
	target ServerExecutionTarget,
	command *exec.Cmd,
	remoteExecutable string,
	remoteArguments ...string,
) error {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	_, remoteErr := service.servers.RunRootCommand(
		ctx,
		target,
		stdout,
		remoteExecutable,
		remoteArguments...)
	_ = stdout.Close()
	waitErr := command.Wait()
	if remoteErr != nil {
		return remoteErr
	}
	if waitErr != nil {
		return fmt.Errorf(
			"read staged Build content: %w: %s",
			waitErr,
			strings.TrimSpace(stderr.String()),
		)
	}
	return nil
}

func remotePackArguments(
	architecture string,
	spec buildpacksclient.BuildSpec,
	contextPath, reportDirectory, assetsBuildpack string,
) ([]string, error) {
	profile, err := buildpacksclient.ProfileForArchitecture(spec.Runtime, architecture)
	if err != nil {
		return nil, err
	}
	arguments := []string{"build", spec.Image, "--path", contextPath, "--builder", profile.Builder}
	if assetsBuildpack != "" {
		arguments = append(
			arguments,
			"--buildpack",
			buildpacksclient.NodeEngineBuildpack,
			"--buildpack",
			assetsBuildpack,
			"--env",
			"BP_DEPLOYCRATE_FRONTEND_SCRIPT="+spec.FrontendScript,
			"--env",
			"BP_DEPLOYCRATE_FRONTEND_DIRECTORY="+spec.FrontendDirectory,
		)
	}
	arguments = append(
		arguments,
		"--buildpack",
		profile.Buildpack,
		"--trust-extra-buildpacks",
		"--run-image",
		profile.RunImage,
		"--publish",
		"--cache",
		"type=build;format=volume;name="+spec.BuildCache,
		"--cache",
		"type=launch;format=volume;name="+spec.LaunchCache,
		"--pull-policy",
		spec.PullPolicy,
		"--timestamps",
		"--report-output-dir",
		reportDirectory,
	)
	for _, environment := range profile.Environment {
		arguments = append(arguments, "--env", environment)
	}
	if spec.PreviousImage != "" {
		arguments = append(arguments, "--previous-image", spec.PreviousImage)
	}
	if spec.BPGOTargets != "" {
		arguments = append(arguments, "--env", "BP_GO_TARGETS="+spec.BPGOTargets)
	}
	return arguments, nil
}

func (service *BuildExecution) Fail(
	ctx context.Context,
	buildID uuid.UUID,
	operationErr error,
) error {
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil || build.Status == "succeeded" || build.Status == "failed" ||
		build.Status == "cancelled" {
		return err
	}
	now := time.Now().UTC()
	if err := models.Build.MarkFailed(
		ctx,
		service.db.Executor(),
		build.ID,
		operationErr,
		now,
	); err != nil {
		return err
	}
	slog.ErrorContext(
		ctx,
		"Build failed after exhausting background job attempts",
		"build_id",
		build.ID,
		"error",
		operationErr,
	)
	return models.Change.MarkFailed(ctx, service.db.Executor(), build.ChangeID, operationErr, now)
}

func (service *BuildExecution) RecordRetry(
	ctx context.Context,
	buildID uuid.UUID,
	attempt int,
	maxAttempts int,
	operationErr error,
) error {
	build, err := models.Build.Find(ctx, service.db.Executor(), buildID)
	if err != nil || build.Status == "succeeded" || build.Status == "failed" ||
		build.Status == "cancelled" {
		return err
	}
	logger, err := newBuildLogger(ctx, service.db, build.ID)
	if err != nil {
		return err
	}
	return logger.System(ctx, fmt.Sprintf(
		"Build attempt %d of %d failed: %s\nRiver will retry this Build automatically.",
		attempt,
		maxAttempts,
		operationErr.Error(),
	))
}

func isRetryableBuildFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"connection refused", "connection reset", "context deadline exceeded", "i/o timeout", "network is unreachable", "tls handshake timeout", "temporary failure", "unexpected eof", "docker daemon", "registry unavailable", "service unavailable"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (service *BuildExecution) claim(
	ctx context.Context,
	id uuid.UUID,
) (models.BuildEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BuildEntity{}, err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, id)
	if err != nil {
		return build, err
	}
	if build.Status == "pending" || build.Status == "running" {
		startedAt := time.Now().UTC()
		if err := models.Build.MarkRunning(ctx, tx, id, startedAt); err != nil {
			return build, err
		}
		if err := models.Change.MarkRunning(ctx, tx, build.ChangeID, startedAt); err != nil {
			return build, err
		}
		build.Status = "running"
		if !build.StartedAt.Valid {
			build.StartedAt = sql.NullTime{Time: startedAt, Valid: true}
		}
	}
	return build, tx.Commit()
}

func parseBuildSnapshot(build models.BuildEntity) (buildSnapshot, error) {
	if build.BuildMethod != "buildpacks" {
		return buildSnapshot{}, errors.New("only Buildpacks builds are supported")
	}
	var snapshot buildSnapshot
	decoder := json.NewDecoder(strings.NewReader(string(build.BuildConfiguration)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, errors.New("Build configuration snapshot is invalid")
	}
	missingField := ""
	switch {
	case snapshot.SchemaVersion != 2:
		missingField = "schema_version"
	case snapshot.SourceEventID == uuid.Nil:
		missingField = "source_event_id"
	case snapshot.EnvironmentStateRevisionID == uuid.Nil:
		missingField = "environment_state_revision_id"
	case strings.TrimSpace(snapshot.Repository) == "":
		missingField = "repository"
	case strings.TrimSpace(snapshot.Reference) == "":
		missingField = "reference"
	case snapshot.SourceRevision != build.SourceRevision:
		missingField = "source_revision"
	case strings.TrimSpace(snapshot.ImageRepository) == "":
		missingField = "image_repository"
	case snapshot.RegistryResourceID == uuid.Nil:
		missingField = "registry_resource_id"
	case snapshot.RegistryCredentialID == uuid.Nil:
		missingField = "registry_credential_id"
	case strings.TrimSpace(snapshot.RegistryEndpoint) == "":
		missingField = "registry_endpoint"
	case snapshot.ServerID == uuid.Nil:
		missingField = "server_id"
	}
	if missingField != "" {
		return snapshot, fmt.Errorf(
			"Build configuration snapshot is incomplete: %s is missing or invalid",
			missingField,
		)
	}
	if snapshot.BuilderReference != nil && strings.TrimSpace(*snapshot.BuilderReference) != "" {
		return snapshot, errors.New("custom Buildpacks builders are not supported")
	}
	settings, err := models.ParseBuildpackSettings(snapshot.Settings)
	if err != nil {
		return snapshot, fmt.Errorf("Buildpacks settings are invalid: %w", err)
	}
	if settings.Runtime != models.BuildpackRuntimeGo && snapshot.BPGOTargets != "" {
		return snapshot, errors.New("BP_GO_TARGETS is available only for the Go runtime")
	}
	if snapshot.BPGOTargets != "" &&
		(!goTargetsPattern.MatchString(snapshot.BPGOTargets) || strings.Contains(snapshot.BPGOTargets, "..")) {
		return snapshot, errors.New("BP_GO_TARGETS is invalid")
	}
	snapshot.parsedSettings = settings
	return snapshot, nil
}

func (service *BuildExecution) loadGitHubSource(
	ctx context.Context,
	build models.BuildEntity,
) (models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	repository, err := models.GitHubRepository.FindActiveByEnvironmentSource(
		ctx,
		service.db.Executor(),
		build.EnvironmentSourceID,
	)
	if err != nil {
		return repository, models.GitHubInstallationEntity{}, err
	}
	installation, err := models.GitHubInstallation.Find(
		ctx,
		service.db.Executor(),
		repository.GitHubInstallationID,
	)
	return repository, installation, err
}

func (service *BuildExecution) registryCredentials(
	ctx context.Context,
	snapshot buildSnapshot,
) (registryclient.Credentials, error) {
	return service.RegistryCredentials(
		ctx,
		snapshot.RegistryResourceID,
		snapshot.RegistryCredentialID,
		snapshot.RegistryEndpoint,
	)
}

func (service *BuildExecution) RegistryCredentials(
	ctx context.Context,
	registryID, credentialID uuid.UUID,
	expectedEndpoint string,
) (registryclient.Credentials, error) {
	registry, err := models.RegistryResource.Snapshot(ctx, service.db.Executor(), registryID)
	if err != nil || registry.Provider != "distribution" || registry.Endpoint != expectedEndpoint ||
		registry.EndpointCount != 1 ||
		registry.CredentialCount != 1 {
		return registryclient.Credentials{}, errors.New(
			"Build Registry snapshot no longer matches an active Registry Resource",
		)
	}
	credential, err := models.ResourceCredential.Find(ctx, service.db.Executor(), credentialID)
	if err != nil || credential.ArchivedAt.Valid || credential.ResourceID != registryID ||
		!credential.Username.Valid {
		return registryclient.Credentials{}, errors.New(
			"Registry Resource credential is unavailable",
		)
	}
	plaintext, err := secretcrypto.DecryptForPurpose(
		credential.EncPayload,
		service.config.App.SessionEncryptionKey,
		registryCredentialPurpose,
	)
	if err != nil {
		return registryclient.Credentials{}, errors.New("registry credential cannot be decrypted")
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 ||
		payload.Values["password"] == "" {
		return registryclient.Credentials{}, errors.New(
			"Registry Resource credential payload is invalid",
		)
	}
	return registryclient.Credentials{
		Endpoint: registry.Endpoint,
		Username: credential.Username.String,
		Password: payload.Values["password"],
	}, nil
}

func extractGitHubArchive(ctx context.Context, archivePath, destination string) error {
	archiveCommand := sudo.CommandContext(ctx, "/usr/bin/cat", "--", archivePath)
	archive, err := archiveCommand.StdoutPipe()
	if err != nil {
		return err
	}
	var archiveError bytes.Buffer
	archiveCommand.Stderr = &archiveError
	if err := archiveCommand.Start(); err != nil {
		return fmt.Errorf("read privileged GitHub archive: %w", err)
	}
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		_ = archive.Close()
		_ = archiveCommand.Wait()
		return errors.New("GitHub archive is not a valid gzip stream")
	}
	reader := tar.NewReader(compressed)
	var root string
	var total int64
	files := 0
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive is invalid")
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		cleaned := path.Clean(header.Name)
		if cleaned == "." || strings.HasPrefix(cleaned, "/") || cleaned == ".." ||
			strings.HasPrefix(cleaned, "../") {
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive contains an unsafe path")
		}
		parts := strings.Split(cleaned, "/")
		if root == "" {
			root = parts[0]
		}
		if parts[0] != root {
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive contains more than one repository root")
		}
		if len(parts) == 1 {
			continue
		}
		relative := path.Join(parts[1:]...)
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if !filepath.IsLocal(relative) ||
			!strings.HasPrefix(target, destination+string(filepath.Separator)) {
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive escaped the Build workspace")
		}
		files++
		if files > maxArchiveFiles {
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive contains too many files")
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			total += header.Size
			if header.Size < 0 || total > maxExtractedArchiveBytes {
				_ = compressed.Close()
				_ = archive.Close()
				_ = archiveCommand.Wait()
				return errors.New("GitHub archive exceeds the extracted size limit")
			}
		default:
			_ = compressed.Close()
			_ = archive.Close()
			_ = archiveCommand.Wait()
			return errors.New("GitHub archive contains an unsupported link or special file")
		}
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil {
		_ = compressed.Close()
		_ = archive.Close()
		_ = archiveCommand.Wait()
		return errors.New("GitHub archive is invalid")
	}
	if err := compressed.Close(); err != nil {
		_ = archive.Close()
		_ = archiveCommand.Wait()
		return errors.New("GitHub archive is invalid")
	}
	if err := archive.Close(); err != nil {
		_ = archiveCommand.Wait()
		return errors.New("GitHub archive could not be closed")
	}
	if err := archiveCommand.Wait(); err != nil {
		return fmt.Errorf(
			"read privileged GitHub archive: %w: %s",
			err,
			strings.TrimSpace(archiveError.String()),
		)
	}
	if root == "" {
		return errors.New("GitHub archive is empty")
	}
	if err := createBuildDirectory(ctx, destination); err != nil {
		return err
	}
	if err := runPrivilegedBuildCommand(ctx, "extract GitHub archive", "/usr/bin/tar",
		"--extract", "--gzip", "--file", archivePath, "--directory", destination,
		"--strip-components", "1", "--no-same-owner", "--no-same-permissions"); err != nil {
		return err
	}
	if err := runPrivilegedBuildCommand(
		ctx,
		"protect extracted GitHub archive",
		"/usr/bin/chmod",
		"-R",
		"u+rwX,go-rwx",
		destination,
	); err != nil {
		return err
	}
	return runPrivilegedBuildCommand(ctx, "assign extracted GitHub archive", "/usr/bin/chown", "-R",
		buildWorkspaceOwner+":"+buildWorkspaceOwner, destination)
}

func secureBuildContext(ctx context.Context, sourceRoot, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = "."
	}
	if !filepath.IsLocal(configured) {
		return "", errors.New("Buildpacks context must stay beneath the repository root")
	}
	contextPath := filepath.Clean(filepath.Join(sourceRoot, configured))
	if contextPath != sourceRoot &&
		!strings.HasPrefix(contextPath, sourceRoot+string(filepath.Separator)) {
		return "", errors.New("Buildpacks context escaped the repository root")
	}
	if err := sudo.CommandContext(ctx, "/usr/bin/test", "-d", contextPath).Run(); err != nil {
		return "", errors.New("Buildpacks context does not exist")
	}
	return contextPath, nil
}

func createBuildDirectory(ctx context.Context, directory string) error {
	return runPrivilegedBuildCommand(
		ctx,
		"create Build directory",
		"/usr/bin/install",
		"-d",
		"-m",
		"0700",
		"-o",
		buildWorkspaceOwner,
		"-g",
		buildWorkspaceOwner,
		directory,
	)
}

func removeBuildWorkspace(ctx context.Context, workspace string) error {
	if filepath.Dir(workspace) != buildWorkspaceRoot {
		return errors.New("Build workspace cleanup path is invalid")
	}
	return runPrivilegedBuildCommand(
		ctx,
		"remove Build workspace",
		"/usr/bin/rm",
		"-rf",
		"--",
		workspace,
	)
}

func runPrivilegedBuildCommand(
	ctx context.Context,
	operation, command string,
	arguments ...string,
) error {
	output, err := sudo.CommandContext(ctx, command, arguments...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s: %s", operation, message)
	}
	return nil
}

func (service *BuildExecution) complete(
	ctx context.Context,
	buildID uuid.UUID,
	snapshot buildSnapshot,
	reference string,
	digest []byte,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	build, err := models.Build.Lock(ctx, tx, buildID)
	if err != nil {
		return err
	}
	if build.Status == "succeeded" {
		return tx.Commit()
	}
	if build.Status != "running" {
		return errors.New("Build is no longer running")
	}
	stateRevision, err := models.EnvironmentStateRevision.Find(
		ctx,
		tx,
		snapshot.EnvironmentStateRevisionID,
	)
	if err != nil || stateRevision.EnvironmentID != build.EnvironmentID {
		return errors.New("Build state revision does not belong to its Environment")
	}
	now := time.Now().UTC()
	if err := models.Build.MarkSucceeded(ctx, tx, build.ID, reference, digest, now); err != nil {
		return err
	}
	if err := models.Change.MarkCompleted(ctx, tx, build.ChangeID, now); err != nil {
		return err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		SourceRevision: sql.NullString{
			String: build.SourceRevision,
			Valid:  true,
		},
		ArtifactReference:    reference,
		ArtifactDigest:       digest,
		EnvironmentID:        build.EnvironmentID,
		EnvironmentSourceID:  &build.EnvironmentSourceID,
		BuildID:              &build.ID,
		CreatedByChangeID:    build.ChangeID,
		RegistryResourceID:   &snapshot.RegistryResourceID,
		RegistryCredentialID: &snapshot.RegistryCredentialID,
		RegistryEndpoint:     sql.NullString{String: snapshot.RegistryEndpoint, Valid: true},
	})
	if err != nil {
		return err
	}
	sequence, err := models.Change.NextSequence(ctx, tx, build.EnvironmentID)
	if err != nil {
		return err
	}
	deployChange, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "deploy",
		TriggerType: "system",
		ActorType:   "system",
		CauseSystem: sql.NullString{
			String: "build",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: build.ID.String(), Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           "Deploy successful Build",
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     build.EnvironmentID,
	})
	if err != nil {
		return err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: deployChange.ID, ReleaseID: release.ID},
	); err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   deployChange.ID,
			EnvironmentStateRevisionID: stateRevision.ID,
		},
	); err != nil {
		return err
	}
	if _, err := service.releases.OrchestrateTx(
		ctx,
		tx,
		release,
		deployChange,
		stateRevision,
	); err != nil {
		return err
	}
	return tx.Commit()
}
