package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"deploycrate-ce/clients/cloudflare"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"go.uber.org/fx"
)

const (
	defaultReleaseRoot   = "/opt/deploycrate-ce/releases"
	defaultSlotsRoot     = "/opt/deploycrate-ce/slots"
	defaultStatusPath    = "/var/lib/deploycrate-ce/self-update.json"
	defaultCaddyAdminURL = "http://127.0.0.1:2019"
	updateLockPath       = "/tmp/deploycrate-ce-update.lock"
	greenInstance        = "green"
	blueInstance         = "blue"
	greenService         = "deploycrate-ce@green.service"
	blueService          = "deploycrate-ce@blue.service"
	greenPort            = 8081
	bluePort             = 8080
)

var (
	ErrUpdateInProgress       = errors.New("a self-update is already in progress")
	ErrBlueGreenNotConfigured = errors.New("blue-green services are not configured")
)

type CurrentVersion string

type SelfUpdateState string

const (
	SelfUpdateIdle       SelfUpdateState = "idle"
	SelfUpdateQueued     SelfUpdateState = "queued"
	SelfUpdateInProgress SelfUpdateState = "in_progress"
	SelfUpdateSucceeded  SelfUpdateState = "succeeded"
	SelfUpdateFailed     SelfUpdateState = "failed"
)

type SelfUpdateEvent struct {
	ID         string    `json:"id"`
	Message    string    `json:"message"`
	OccurredAt time.Time `json:"occurredAt"`
}

type SelfUpdateStatus struct {
	State                SelfUpdateState   `json:"state"`
	CurrentStep          string            `json:"currentStep"`
	TargetVersion        string            `json:"targetVersion"`
	ActiveInstanceBefore string            `json:"activeInstanceBefore"`
	ActiveInstance       string            `json:"activeInstance"`
	Error                string            `json:"error"`
	StartedAt            *time.Time        `json:"startedAt"`
	FinishedAt           *time.Time        `json:"finishedAt"`
	UpdatedAt            time.Time         `json:"updatedAt"`
	Events               []SelfUpdateEvent `json:"events"`
}

func (s SelfUpdateStatus) Running() bool {
	return s.State == SelfUpdateQueued || s.State == SelfUpdateInProgress
}

type updateRelease struct {
	Version string
}

type caddyRoute struct {
	ID     string        `json:"@id"`
	Handle []caddyHandle `json:"handle"`
}

type caddyHandle struct {
	Handler       string             `json:"handler"`
	LoadBalancing caddyLoadBalancing `json:"load_balancing"`
	Routes        []caddySubroute    `json:"routes,omitempty"`
	Upstreams     []caddyUpstream    `json:"upstreams"`
}

type caddySubroute struct {
	Handle []caddyHandle `json:"handle"`
}

type caddyLoadBalancing struct {
	SelectionPolicy caddySelectionPolicy `json:"selection_policy"`
}

type caddySelectionPolicy struct {
	Policy  string `json:"policy"`
	Weights []int  `json:"weights"`
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}

type updateJob struct {
	actorID uuid.UUID
	source  config.ReleaseSource
}

type SelfUpdate struct {
	mu             sync.RWMutex
	status         SelfUpdateStatus
	localRunning   bool
	currentVersion string
	releaseRoot    string
	slotsRoot      string
	statusPath     string
	caddyAdminURL  string
	publicHealth   string
	client         *http.Client
	r2             *cloudflare.R2
	queue          chan updateJob
	db             storage.Pool

	currentDeployment *selfUpdateDeployment
}

func NewSelfUpdate(
	lifecycle fx.Lifecycle,
	appCtx context.Context,
	version CurrentVersion,
	db storage.Pool,
) *SelfUpdate {
	currentVersion := normalizeVersion(string(version))
	if currentVersion == "" {
		currentVersion = "dev"
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	service := &SelfUpdate{
		status: SelfUpdateStatus{
			State:  SelfUpdateIdle,
			Events: []SelfUpdateEvent{},
		},
		currentVersion: currentVersion,
		releaseRoot:    environmentOr("DEPLOYCRATE_CE_RELEASE_ROOT", defaultReleaseRoot),
		slotsRoot:      environmentOr("DEPLOYCRATE_CE_SLOTS_ROOT", defaultSlotsRoot),
		statusPath:     environmentOr("DEPLOYCRATE_CE_UPDATE_STATUS_PATH", defaultStatusPath),
		caddyAdminURL: strings.TrimRight(
			environmentOr("DEPLOYCRATE_CE_CADDY_ADMIN_URL", defaultCaddyAdminURL),
			"/",
		),
		publicHealth: environmentOr(
			"DEPLOYCRATE_CE_PUBLIC_HEALTH_URL",
			strings.TrimRight(config.BaseURL, "/")+"/api/health",
		),
		client: httpClient,
		r2:     cloudflare.NewR2(httpClient),
		queue:  make(chan updateJob, 1),
		db:     db,
	}
	service.loadStatus()
	if service.status.UpdatedAt.IsZero() {
		service.status.UpdatedAt = time.Now()
	}

	var done chan struct{}
	lifecycle.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			if err := service.reconcileOnStartup(startCtx, appCtx); err != nil {
				return err
			}
			done = make(chan struct{})
			go func() {
				defer close(done)
				service.run(appCtx)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if done == nil {
				return nil
			}
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	return service
}

func (s *SelfUpdate) CurrentVersion() string {
	return s.currentVersion
}

func (s *SelfUpdate) Status() SelfUpdateStatus {
	s.loadStatus()
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := s.status
	status.Events = append([]SelfUpdateEvent{}, s.status.Events...)
	return status
}

func (s *SelfUpdate) Start(ctx context.Context, actorID uuid.UUID) (SelfUpdateStatus, error) {
	unresolved, err := s.loadUnresolvedDeployment(ctx)
	if err != nil {
		return s.Status(), err
	}
	if unresolved != nil {
		return s.Status(), ErrUpdateInProgress
	}

	source, err := config.ResolveReleaseSource(s.currentVersion)
	if err != nil {
		return s.Status(), err
	}

	s.mu.Lock()
	if s.localRunning || selfUpdateLockHeld() {
		status := s.status
		s.mu.Unlock()
		return status, ErrUpdateInProgress
	}
	s.localRunning = true
	now := time.Now()
	s.status = SelfUpdateStatus{
		State:       SelfUpdateQueued,
		CurrentStep: "queued",
		StartedAt:   &now,
		UpdatedAt:   now,
		Events: []SelfUpdateEvent{{
			ID:         uuid.NewString(),
			Message:    "Update queued",
			OccurredAt: now,
		}},
	}
	s.currentDeployment = nil
	s.persistStatusLocked()
	status := s.status
	s.mu.Unlock()

	select {
	case s.queue <- updateJob{actorID: actorID, source: source}:
		return status, nil
	case <-ctx.Done():
		s.mu.Lock()
		s.localRunning = false
		s.mu.Unlock()
		s.fail("queued", ctx.Err())
		return s.Status(), ctx.Err()
	}
}

func (s *SelfUpdate) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			s.execute(ctx, job)
		}
	}
}

func (s *SelfUpdate) reconcileOnStartup(_ context.Context, appCtx context.Context) error {
	go s.reconcileWhenUnlocked(appCtx)
	return nil
}

func (s *SelfUpdate) reconcileWhenUnlocked(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		lock, err := acquireSelfUpdateLock()
		if err == nil {
			if reconcileErr := s.reconcileLocked(
				ctx,
			); reconcileErr != nil &&
				!errors.Is(reconcileErr, context.Canceled) {
				slog.ErrorContext(
					ctx,
					"failed to reconcile interrupted self-update",
					"error",
					reconcileErr,
				)
			}
			lock.release()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *SelfUpdate) reconcileLocked(ctx context.Context) error {
	record, err := s.loadUnresolvedDeployment(ctx)
	if err != nil || record == nil {
		return err
	}

	s.mu.Lock()
	s.currentDeployment = record
	s.localRunning = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.localRunning = false
		s.mu.Unlock()
	}()

	activeInstance := record.SystemState.ActiveInstanceSlot
	targetInstance := record.InactiveSlot
	s.setInstances(activeInstance, activeInstance)
	s.transition(SelfUpdateInProgress, "reconcile", "Reconciling interrupted update")

	targetActive, targetActiveErr := serviceActive(ctx, serviceForInstance(targetInstance))
	trafficTarget, trafficErr := s.trafficTarget(ctx, record.SystemState.CaddyRouteExternalID)
	slotTarget, slotErr := readSlotTarget(s.slotBinaryPath(targetInstance))
	_, activeBootErr := serviceEnabled(ctx, serviceForInstance(activeInstance))
	_, targetBootErr := serviceEnabled(ctx, serviceForInstance(targetInstance))
	inspectionErr := errors.Join(targetActiveErr, trafficErr, slotErr, activeBootErr, targetBootErr)

	targetHealthy := false
	if inspectionErr == nil && targetActive &&
		filepath.Clean(slotTarget) == filepath.Clean(record.ReleasePath) {
		targetHealthy = s.waitForHealth(
			ctx,
			internalHealthURL(targetInstance),
			5*time.Second,
		) == nil
	}
	if targetHealthy && trafficTarget == targetInstance {
		return s.completeRecoveredUpdate(record)
	}

	cause := errors.New("interrupted update did not have a healthy target serving traffic")
	if inspectionErr != nil {
		cause = errors.Join(cause, inspectionErr)
	}
	if err := s.rollbackDeployment(record); err != nil {
		return errors.Join(cause, err)
	}
	return s.fail("reconcile", cause)
}

func (s *SelfUpdate) completeRecoveredUpdate(record *selfUpdateDeployment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	activeInstance := record.SystemState.ActiveInstanceSlot
	targetInstance := record.InactiveSlot
	s.transition(
		SelfUpdateInProgress,
		"update_service_boot_state",
		"Finalizing enabled service instance",
	)
	if err := runSystemctl(ctx, "enable", serviceForInstance(targetInstance)); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "disable", serviceForInstance(activeInstance)); err != nil {
		return err
	}
	record.Checkpoint.BootStateSwitched = true
	record.Checkpoint.Phase = "boot_state_switched"
	if err := s.persistCheckpoint(ctx, record); err != nil {
		return err
	}

	s.transition(SelfUpdateInProgress, "stop_previous_instance", "Stopping previous instance")
	if err := stopServiceAndWait(
		ctx,
		serviceForInstance(activeInstance),
		30*time.Second,
	); err != nil {
		return err
	}
	running, err := s.runningInstance(ctx)
	if err != nil {
		return err
	}
	if running != targetInstance {
		return fmt.Errorf("expected %s to be the only active slot, got %s", targetInstance, running)
	}
	record.Checkpoint.Phase = "old_instance_stopped"
	if err := s.persistCheckpoint(ctx, record); err != nil {
		return err
	}
	if err := s.succeed(record.Version, targetInstance); err != nil {
		return err
	}
	if err := s.pruneReleases(ctx); err != nil {
		slog.WarnContext(ctx, "failed to prune orphaned DeployCrate CE releases", "error", err)
	}
	return nil
}

func (s *SelfUpdate) rollbackDeployment(record *selfUpdateDeployment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	activeInstance := record.SystemState.ActiveInstanceSlot
	targetInstance := record.InactiveSlot
	var rollbackErr error
	rollbackErr = errors.Join(
		rollbackErr,
		runSystemctl(ctx, "start", serviceForInstance(activeInstance)),
	)
	rollbackErr = errors.Join(
		rollbackErr,
		s.configureTraffic(ctx, record.SystemState.CaddyRouteExternalID, activeInstance),
	)
	rollbackErr = errors.Join(
		rollbackErr,
		runSystemctl(ctx, "enable", serviceForInstance(activeInstance)),
	)
	rollbackErr = errors.Join(
		rollbackErr,
		runSystemctl(ctx, "disable", serviceForInstance(targetInstance)),
	)
	rollbackErr = errors.Join(
		rollbackErr,
		s.restoreSlot(ctx, targetInstance, record.Checkpoint.PreviousSlotTarget),
	)
	rollbackErr = errors.Join(
		rollbackErr,
		stopServiceAndWait(ctx, serviceForInstance(targetInstance), 30*time.Second),
	)

	running, err := s.runningInstance(ctx)
	rollbackErr = errors.Join(rollbackErr, err)
	if err == nil && running != activeInstance {
		rollbackErr = errors.Join(
			rollbackErr,
			fmt.Errorf(
				"expected %s to be the only active slot after rollback, got %s",
				activeInstance,
				running,
			),
		)
	}
	if rollbackErr != nil {
		return rollbackErr
	}

	record.Checkpoint.TargetStarted = false
	record.Checkpoint.TrafficSwitched = false
	record.Checkpoint.BootStateSwitched = false
	record.Checkpoint.Phase = "rolled_back"
	return s.persistCheckpoint(ctx, record)
}

func (s *SelfUpdate) execute(parent context.Context, job updateJob) {
	defer func() {
		s.mu.Lock()
		s.localRunning = false
		s.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	s.transition(SelfUpdateInProgress, "acquire_lock", "Acquiring update lock")
	lock, err := acquireSelfUpdateLock()
	if err != nil {
		s.fail("acquire_lock", err)
		return
	}
	defer lock.release()

	workDir, err := os.MkdirTemp("", "deploycrate-ce-update-*")
	if err != nil {
		s.fail("prepare_artifact", fmt.Errorf("create update directory: %w", err))
		return
	}
	defer os.RemoveAll(workDir)

	stagedBinary := filepath.Join(workDir, "deploycrate-ce")
	checksumPath := filepath.Join(workDir, "deploycrate-ce.sha256")
	s.transition(SelfUpdateInProgress, "download_artifact", "Downloading release artifact")
	if err := s.r2.Download(
		ctx,
		job.source.BaseURL,
		config.ReleaseApplicationPath,
		stagedBinary,
	); err != nil {
		s.fail("download_artifact", err)
		return
	}
	if err := s.r2.Download(
		ctx,
		job.source.BaseURL,
		config.ReleaseChecksumPath,
		checksumPath,
	); err != nil {
		s.fail("download_checksum", err)
		return
	}

	s.transition(SelfUpdateInProgress, "verify_checksum", "Verifying release checksum")
	if err := verifyReleaseChecksum(stagedBinary, checksumPath, "deploycrate-ce"); err != nil {
		s.fail("verify_checksum", err)
		return
	}
	if err := os.Chmod(stagedBinary, 0o700); err != nil {
		s.fail("prepare_binary", fmt.Errorf("make release binary executable: %w", err))
		return
	}
	stagedVersion, err := binaryVersion(ctx, stagedBinary)
	if err != nil {
		s.fail("verify_version", err)
		return
	}
	if job.source.Development && !strings.HasPrefix(stagedVersion, "development-") {
		s.fail(
			"verify_version",
			fmt.Errorf("development release reported unexpected version %q", stagedVersion),
		)
		return
	}
	release := updateRelease{Version: stagedVersion}
	s.mu.Lock()
	s.status.TargetVersion = release.Version
	s.status.UpdatedAt = time.Now()
	s.persistStatusLocked()
	s.mu.Unlock()

	digest, err := fileDigest(stagedBinary)
	if err != nil {
		s.fail("digest_binary", err)
		return
	}

	systemState, err := s.loadSystemState(ctx)
	if err != nil {
		s.fail("load_system_state", err)
		return
	}
	activeInstance, err := s.runningInstance(ctx)
	if err != nil {
		s.fail("detect_running_service", err)
		return
	}
	if activeInstance != systemState.ActiveInstanceSlot {
		s.fail("detect_running_service", fmt.Errorf(
			"running slot %q does not match the persisted active slot %q",
			activeInstance,
			systemState.ActiveInstanceSlot,
		))
		return
	}
	deployment, err := s.createDeploymentRecords(ctx, job.actorID, release, systemState)
	if err != nil {
		s.fail("create_deployment", err)
		return
	}
	s.mu.Lock()
	s.currentDeployment = deployment
	s.mu.Unlock()

	inactiveInstance := deployment.InactiveSlot
	s.setInstances(activeInstance, activeInstance)
	rollback := func(cause error) {
		if rollbackErr := s.rollbackDeployment(deployment); rollbackErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
			s.reportUnresolvedFailure(s.Status().CurrentStep, cause)
			return
		}
		s.setInstances(activeInstance, activeInstance)
		s.fail(s.Status().CurrentStep, cause)
	}

	if err := s.recordArtifact(digest); err != nil {
		s.fail("record_artifact", fmt.Errorf("record release artifact digest: %w", err))
		return
	}

	previousSlotTarget, err := readSlotTarget(s.slotBinaryPath(inactiveInstance))
	if err != nil {
		s.fail("read_slot", err)
		return
	}
	deployment.Checkpoint.PreviousSlotTarget = previousSlotTarget
	deployment.Checkpoint.Phase = "artifact_verified"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		s.fail("record_checkpoint", err)
		return
	}

	s.transition(SelfUpdateInProgress, "install_release", "Installing verified release")
	installedBinary, err := s.installRelease(ctx, stagedBinary, release.Version)
	if err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.Phase = "release_installed"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "migrate_database", "Running target database migrations")
	if err := runReleaseCommand(ctx, installedBinary, "migrate"); err != nil {
		rollback(fmt.Errorf("run target migrations: %w", err))
		return
	}
	deployment.Checkpoint.Phase = "migrations_completed"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "update_slot_link", "Updating inactive slot link")
	if err := s.replaceSlotLink(
		ctx,
		s.slotBinaryPath(inactiveInstance),
		installedBinary,
	); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.Phase = "slot_link_updated"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "prepare_traffic", "Preparing blue-green traffic route")
	if err := s.configureTraffic(
		ctx,
		deployment.SystemState.CaddyRouteExternalID,
		activeInstance,
	); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.Phase = "traffic_prepared"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "start_inactive_instance", "Starting inactive instance")
	if err := runSystemctl(ctx, "start", serviceForInstance(inactiveInstance)); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.TargetStarted = true
	deployment.Checkpoint.Phase = "target_started"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(
		SelfUpdateInProgress,
		"verify_inactive_instance",
		"Waiting for inactive instance health",
	)
	if err := s.waitForHealth(
		ctx,
		internalHealthURL(inactiveInstance),
		30*time.Second,
	); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.Phase = "target_healthy"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "switch_traffic", "Switching traffic to updated instance")
	if err := s.setTraffic(
		ctx,
		deployment.SystemState.CaddyRouteExternalID,
		inactiveInstance,
	); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.TrafficSwitched = true
	deployment.Checkpoint.Phase = "traffic_switched"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}
	s.setInstances(activeInstance, inactiveInstance)

	s.transition(SelfUpdateInProgress, "verify_public_path", "Verifying public application health")
	if err := s.waitForHealth(ctx, s.publicHealth, 15*time.Second); err != nil {
		rollback(err)
		return
	}
	deployment.Checkpoint.Phase = "public_healthy"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(
		SelfUpdateInProgress,
		"update_service_boot_state",
		"Updating enabled service instance",
	)
	if err := runSystemctl(ctx, "enable", serviceForInstance(inactiveInstance)); err != nil {
		rollback(err)
		return
	}
	if err := runSystemctl(ctx, "disable", serviceForInstance(activeInstance)); err != nil {
		_ = runSystemctl(ctx, "disable", serviceForInstance(inactiveInstance))
		_ = runSystemctl(ctx, "enable", serviceForInstance(activeInstance))
		rollback(err)
		return
	}
	deployment.Checkpoint.BootStateSwitched = true
	deployment.Checkpoint.Phase = "boot_state_switched"
	if err := s.persistCheckpoint(ctx, deployment); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "stop_previous_instance", "Stopping previous instance")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := runSudo(
		stopCtx,
		"systemctl",
		"stop",
		"--no-block",
		serviceForInstance(activeInstance),
	); err != nil {
		rollback(err)
		return
	}
}

func (s *SelfUpdate) runningInstance(ctx context.Context) (string, error) {
	greenActive, greenErr := serviceActive(ctx, greenService)
	blueActive, blueErr := serviceActive(ctx, blueService)
	if greenErr != nil || blueErr != nil {
		return "", fmt.Errorf(
			"%w: expected %s and %s: %v",
			ErrBlueGreenNotConfigured,
			greenService,
			blueService,
			errors.Join(greenErr, blueErr),
		)
	}
	switch {
	case greenActive && blueActive:
		return "", errors.New("both DeployCrate CE instances are active before update")
	case !greenActive && !blueActive:
		return "", fmt.Errorf(
			"%w: neither %s nor %s is active",
			ErrBlueGreenNotConfigured,
			greenService,
			blueService,
		)
	case greenActive:
		return greenInstance, nil
	default:
		return blueInstance, nil
	}
}

func (s *SelfUpdate) installRelease(
	ctx context.Context,
	sourcePath, version string,
) (string, error) {
	releasePath := s.releaseBinaryPath(version)
	if err := runSudo(ctx, "install", "-d", "-m", "0755", filepath.Dir(releasePath)); err != nil {
		return "", fmt.Errorf("create release directory: %w", err)
	}
	nextReleasePath := releasePath + ".next"
	if err := runSudo(ctx, "install", "-m", "0755", sourcePath, nextReleasePath); err != nil {
		_ = runSudo(ctx, "rm", "-f", nextReleasePath)
		return "", fmt.Errorf("stage release binary: %w", err)
	}
	if err := runSudo(ctx, "mv", "-f", nextReleasePath, releasePath); err != nil {
		_ = runSudo(ctx, "rm", "-f", nextReleasePath)
		return "", fmt.Errorf("activate release binary: %w", err)
	}
	return releasePath, nil
}

func (s *SelfUpdate) restoreSlot(ctx context.Context, slot, target string) error {
	if target == "" {
		if err := runSudo(ctx, "rm", "-f", s.slotBinaryPath(slot)); err != nil {
			return fmt.Errorf("remove newly created %s slot link: %w", slot, err)
		}
		return nil
	}
	return s.replaceSlotLink(ctx, s.slotBinaryPath(slot), target)
}

func (s *SelfUpdate) replaceSlotLink(ctx context.Context, slotPath, target string) error {
	if err := runSudo(ctx, "install", "-d", "-m", "0755", filepath.Dir(slotPath)); err != nil {
		return fmt.Errorf("create slot directory: %w", err)
	}
	nextPath := slotPath + ".next"
	_ = runSudo(ctx, "rm", "-f", nextPath)
	if err := runSudo(ctx, "ln", "-s", target, nextPath); err != nil {
		return fmt.Errorf("stage slot link: %w", err)
	}
	if err := runSudo(ctx, "mv", "-Tf", nextPath, slotPath); err != nil {
		_ = runSudo(ctx, "rm", "-f", nextPath)
		return fmt.Errorf("activate slot link: %w", err)
	}
	return nil
}

func readSlotTarget(slotPath string) (string, error) {
	target, err := os.Readlink(slotPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read slot target %s: %w", slotPath, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(slotPath), target)
	}
	return filepath.Clean(target), nil
}

func (s *SelfUpdate) releaseBinaryPath(version string) string {
	return filepath.Join(s.releaseRoot, releaseDirectoryName(version), "deploycrate-ce")
}

func (s *SelfUpdate) slotBinaryPath(slot string) string {
	return filepath.Join(s.slotsRoot, slot, "deploycrate-ce")
}

func (s *SelfUpdate) configureTraffic(ctx context.Context, routeID, activeInstance string) error {
	route, handlePath, err := s.readTrafficRoute(ctx, routeID)
	if err != nil {
		return err
	}
	handle, _ := findReverseProxy(route.Handle, "handle")
	if handle == nil {
		return errors.New("Caddy self-update route has no reverse_proxy handler")
	}

	upstreams := []caddyUpstream{
		{Dial: fmt.Sprintf("127.0.0.1:%d", bluePort)},
		{Dial: fmt.Sprintf("127.0.0.1:%d", greenPort)},
	}
	handle.Upstreams = upstreams
	switch activeInstance {
	case blueInstance:
		handle.LoadBalancing.SelectionPolicy.Weights = []int{100, 0}
	case greenInstance:
		handle.LoadBalancing.SelectionPolicy.Weights = []int{0, 100}
	default:
		return fmt.Errorf("invalid active instance %q", activeInstance)
	}
	return s.patchCaddyValue(ctx, routeID, handlePath, handle, "configure Caddy blue-green route")
}

func (s *SelfUpdate) setTraffic(ctx context.Context, routeID, targetInstance string) error {
	route, handlePath, err := s.readTrafficRoute(ctx, routeID)
	if err != nil {
		return err
	}
	handle, _ := findReverseProxy(route.Handle, "handle")
	if handle == nil {
		return errors.New("Caddy self-update route has no reverse_proxy handler")
	}
	return s.patchTrafficWeights(ctx, routeID, handlePath, handle.Upstreams, targetInstance)
}

func (s *SelfUpdate) trafficTarget(ctx context.Context, routeID string) (string, error) {
	route, _, err := s.readTrafficRoute(ctx, routeID)
	if err != nil {
		return "", err
	}
	handle, _ := findReverseProxy(route.Handle, "handle")
	if handle == nil {
		return "", errors.New("Caddy self-update route has no reverse_proxy handler")
	}
	if len(handle.Upstreams) != len(handle.LoadBalancing.SelectionPolicy.Weights) {
		return "", errors.New("Caddy self-update route has mismatched upstreams and weights")
	}

	target := ""
	for index, upstream := range handle.Upstreams {
		weight := handle.LoadBalancing.SelectionPolicy.Weights[index]
		if weight == 0 {
			continue
		}
		if weight != 100 || target != "" {
			return "", errors.New(
				"Caddy self-update route does not have exactly one active upstream",
			)
		}
		switch normalizeDial(upstream.Dial) {
		case fmt.Sprintf("127.0.0.1:%d", greenPort):
			target = greenInstance
		case fmt.Sprintf("127.0.0.1:%d", bluePort):
			target = blueInstance
		default:
			return "", fmt.Errorf("unexpected Caddy upstream %q", upstream.Dial)
		}
	}
	if target == "" {
		return "", errors.New("Caddy self-update route has no active upstream")
	}
	return target, nil
}

func (s *SelfUpdate) readTrafficRoute(
	ctx context.Context,
	routeID string,
) (caddyRoute, string, error) {
	routeURL := fmt.Sprintf("%s/id/%s", s.caddyAdminURL, url.PathEscape(routeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, routeURL, nil)
	if err != nil {
		return caddyRoute{}, "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return caddyRoute{}, "", fmt.Errorf("read Caddy self-update route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return caddyRoute{}, "", fmt.Errorf(
			"read Caddy self-update route: status %d",
			resp.StatusCode,
		)
	}

	var route caddyRoute
	if err := json.NewDecoder(resp.Body).Decode(&route); err != nil {
		return caddyRoute{}, "", fmt.Errorf("decode Caddy self-update route: %w", err)
	}
	handle, handlePath := findReverseProxy(route.Handle, "handle")
	if handle == nil || handlePath == "" {
		return caddyRoute{}, "", errors.New("Caddy self-update route has no reverse_proxy handler")
	}
	if handle.LoadBalancing.SelectionPolicy.Policy != "weighted_round_robin" {
		return caddyRoute{}, "", errors.New("Caddy self-update route is not weighted_round_robin")
	}
	return route, handlePath, nil
}

func (s *SelfUpdate) patchTrafficWeights(
	ctx context.Context,
	routeID, handlePath string,
	upstreams []caddyUpstream,
	targetInstance string,
) error {
	weights := make([]int, len(upstreams))
	foundGreen := false
	foundBlue := false
	for index, upstream := range upstreams {
		switch normalizeDial(upstream.Dial) {
		case fmt.Sprintf("127.0.0.1:%d", greenPort):
			foundGreen = true
			if targetInstance == greenInstance {
				weights[index] = 100
			}
		case fmt.Sprintf("127.0.0.1:%d", bluePort):
			foundBlue = true
			if targetInstance == blueInstance {
				weights[index] = 100
			}
		default:
			return fmt.Errorf("unexpected Caddy upstream %q", upstream.Dial)
		}
	}
	if !foundGreen || !foundBlue {
		return errors.New("Caddy self-update route must contain green and blue upstreams")
	}
	return s.patchCaddyValue(
		ctx,
		routeID,
		handlePath+"/load_balancing/selection_policy/weights",
		weights,
		"switch Caddy traffic",
	)
}

func (s *SelfUpdate) patchCaddyValue(
	ctx context.Context,
	routeID, path string,
	value any,
	operation string,
) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	patchURL := fmt.Sprintf("%s/id/%s/%s", s.caddyAdminURL, url.PathEscape(routeID), path)
	patch, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		patchURL,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	patch.Header.Set("Content-Type", "application/json")
	patchResponse, err := s.client.Do(patch)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	defer patchResponse.Body.Close()
	if patchResponse.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(patchResponse.Body, 1024))
		return fmt.Errorf(
			"%s: status %d: %s",
			operation,
			patchResponse.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}
	return nil
}

func findReverseProxy(handles []caddyHandle, prefix string) (*caddyHandle, string) {
	for index := range handles {
		path := fmt.Sprintf("%s/%d", prefix, index)
		if handles[index].Handler == "reverse_proxy" {
			return &handles[index], path
		}
		for routeIndex := range handles[index].Routes {
			if handle, nestedPath := findReverseProxy(
				handles[index].Routes[routeIndex].Handle,
				fmt.Sprintf("%s/routes/%d/handle", path, routeIndex),
			); handle != nil {
				return handle, nestedPath
			}
		}
	}
	return nil, ""
}

func (s *SelfUpdate) waitForHealth(
	ctx context.Context,
	healthURL string,
	wait time.Duration,
) error {
	deadline := time.Now().Add(wait)
	var lastErr error
	for {
		requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, healthURL, nil)
		if err == nil {
			resp, requestErr := s.client.Do(req)
			if requestErr == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					cancel()
					return nil
				}
				lastErr = fmt.Errorf("health check returned status %d", resp.StatusCode)
			} else {
				lastErr = requestErr
			}
		} else {
			lastErr = err
		}
		cancel()

		if time.Now().After(deadline) {
			return fmt.Errorf("health check %s failed: %w", healthURL, lastErr)
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *SelfUpdate) transition(state SelfUpdateState, step, message string) {
	s.mu.Lock()
	s.status.State = state
	s.status.CurrentStep = step
	s.status.Error = ""
	s.status.UpdatedAt = time.Now()
	s.status.Events = append(
		s.status.Events,
		SelfUpdateEvent{ID: uuid.NewString(), Message: message, OccurredAt: s.status.UpdatedAt},
	)
	s.persistStatusLocked()
	s.mu.Unlock()
	s.recordTransition(state, step, message)
	slog.Info("DeployCrate CE self-update", "step", step, "message", message)
}

func (s *SelfUpdate) setInstances(before, current string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.ActiveInstanceBefore = before
	s.status.ActiveInstance = current
	s.status.UpdatedAt = time.Now()
	s.persistStatusLocked()
}

func (s *SelfUpdate) fail(step string, err error) error {
	finalized := true
	if finishErr := s.finishDeployment(false, err); finishErr != nil {
		finalized = false
		err = errors.Join(err, fmt.Errorf("record failed deployment: %w", finishErr))
	}
	s.mu.Lock()
	now := time.Now()
	s.status.State = SelfUpdateFailed
	s.status.CurrentStep = step
	s.status.Error = err.Error()
	s.status.FinishedAt = &now
	s.status.UpdatedAt = now
	s.status.Events = append(
		s.status.Events,
		SelfUpdateEvent{
			ID:         uuid.NewString(),
			Message:    "Update failed: " + err.Error(),
			OccurredAt: now,
		},
	)
	s.persistStatusLocked()
	s.mu.Unlock()
	if finalized {
		pruneCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if pruneErr := s.pruneReleases(pruneCtx); pruneErr != nil {
			slog.WarnContext(
				pruneCtx,
				"failed to prune orphaned DeployCrate CE releases",
				"error",
				pruneErr,
			)
		}
		cancel()
	}
	slog.Error("DeployCrate CE self-update failed", "step", step, "error", err)
	return err
}

func (s *SelfUpdate) reportUnresolvedFailure(step string, err error) {
	s.mu.Lock()
	now := time.Now()
	s.status.State = SelfUpdateFailed
	s.status.CurrentStep = step
	s.status.Error = err.Error()
	s.status.FinishedAt = &now
	s.status.UpdatedAt = now
	s.status.Events = append(
		s.status.Events,
		SelfUpdateEvent{
			ID:         uuid.NewString(),
			Message:    "Update recovery is still pending: " + err.Error(),
			OccurredAt: now,
		},
	)
	s.persistStatusLocked()
	s.mu.Unlock()
	slog.Error("DeployCrate CE self-update recovery remains unresolved", "step", step, "error", err)
}

func (s *SelfUpdate) succeed(version, instance string) error {
	if err := s.finishDeployment(true, nil); err != nil {
		return err
	}
	s.mu.Lock()
	now := time.Now()
	s.status.State = SelfUpdateSucceeded
	s.status.CurrentStep = "completed"
	s.status.TargetVersion = version
	s.status.ActiveInstance = instance
	s.status.Error = ""
	s.status.FinishedAt = &now
	s.status.UpdatedAt = now
	s.status.Events = append(
		s.status.Events,
		SelfUpdateEvent{ID: uuid.NewString(), Message: "Update completed", OccurredAt: now},
	)
	s.persistStatusLocked()
	s.mu.Unlock()
	return nil
}

func (s *SelfUpdate) loadStatus() {
	content, err := os.ReadFile(s.statusPath)
	if err != nil {
		return
	}
	var persisted SelfUpdateStatus
	if err := json.Unmarshal(content, &persisted); err != nil {
		return
	}
	if persisted.Events == nil {
		persisted.Events = []SelfUpdateEvent{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if persisted.UpdatedAt.After(s.status.UpdatedAt) {
		s.status = persisted
	}
}

func (s *SelfUpdate) persistStatusLocked() {
	content, err := json.MarshalIndent(s.status, "", "  ")
	if err != nil {
		slog.Warn("failed to encode self-update status", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.statusPath), 0o755); err != nil {
		slog.Warn("failed to create self-update status directory", "error", err)
		return
	}
	temporary := s.statusPath + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		slog.Warn("failed to write self-update status", "error", err)
		return
	}
	if err := os.Rename(temporary, s.statusPath); err != nil {
		slog.Warn("failed to install self-update status", "error", err)
	}
}

func (s *SelfUpdate) pruneReleases(ctx context.Context) error {
	root, err := filepath.Abs(s.releaseRoot)
	if err != nil {
		return fmt.Errorf("resolve release root: %w", err)
	}
	retained := map[string]struct{}{}
	retainBinary := func(binaryPath string) {
		binaryPath = filepath.Clean(binaryPath)
		directory := filepath.Dir(binaryPath)
		if filepath.Dir(directory) == root && filepath.Base(binaryPath) == "deploycrate-ce" {
			retained[directory] = struct{}{}
		}
	}
	for _, slot := range []string{blueInstance, greenInstance} {
		target, targetErr := readSlotTarget(s.slotBinaryPath(slot))
		if targetErr != nil {
			return targetErr
		}
		retainBinary(target)
	}
	unresolved, err := models.Release.UnresolvedSystemUpdateArtifacts(ctx, s.db.Executor())
	if err != nil {
		return fmt.Errorf("load retained release artifacts: %w", err)
	}
	for _, artifact := range unresolved {
		retainBinary(artifact)
	}

	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read release root: %w", err)
	}
	var pruneErr error
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if name == "." || name == ".." || releaseDirectoryName(name) != name {
			continue
		}
		candidate := filepath.Clean(filepath.Join(root, name))
		if filepath.Dir(candidate) != root {
			continue
		}
		binaryInfo, err := os.Lstat(filepath.Join(candidate, "deploycrate-ce"))
		if err != nil || !binaryInfo.Mode().IsRegular() {
			continue
		}
		if _, keep := retained[candidate]; keep {
			continue
		}
		if err := runSudo(ctx, "rm", "-rf", "--", candidate); err != nil {
			pruneErr = errors.Join(pruneErr, fmt.Errorf("remove orphan release %s: %w", name, err))
		}
	}
	return pruneErr
}

func verifyReleaseChecksum(binaryPath, checksumsPath, binaryName string) error {
	checksums, err := os.Open(checksumsPath)
	if err != nil {
		return err
	}
	defer checksums.Close()

	expected := ""
	scanner := bufio.NewScanner(checksums)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == binaryName {
			expected = fields[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s was not found", binaryName)
	}
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("checksum for %s is not a valid SHA-256 digest", binaryName)
	}

	binary, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer binary.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, binary); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf(
			"checksum mismatch for %s: expected %s, got %s",
			binaryName,
			expected,
			actual,
		)
	}
	return nil
}

func fileDigest(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

func releaseDirectoryName(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "release"
	}
	var builder strings.Builder
	for _, value := range version {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9' ||
			value == '.' ||
			value == '-' ||
			value == '_' {
			builder.WriteRune(value)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "release"
	}
	return builder.String()
}

func binaryVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := exec.CommandContext(ctx, binaryPath, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"read staged binary version: %w: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}
	version := normalizeVersion(string(output))
	if version == "" || strings.ContainsAny(version, "\r\n\t ") {
		return "", fmt.Errorf(
			"staged binary reported invalid version %q",
			strings.TrimSpace(string(output)),
		)
	}
	return version, nil
}

func runReleaseCommand(ctx context.Context, binaryPath string, arguments ...string) error {
	output, err := exec.CommandContext(ctx, binaryPath, arguments...).CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"run %s %s: %w: %s",
			binaryPath,
			strings.Join(arguments, " "),
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return nil
}

func serviceActive(ctx context.Context, service string) (bool, error) {
	command := exec.CommandContext(ctx, "sudo", "-n", "systemctl", "is-active", "--quiet", service)
	output, err := command.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 3 {
		return false, nil
	}
	return false, fmt.Errorf(
		"inspect active state for %s: %w: %s",
		service,
		err,
		strings.TrimSpace(string(output)),
	)
}

func serviceEnabled(ctx context.Context, service string) (bool, error) {
	command := exec.CommandContext(ctx, "sudo", "-n", "systemctl", "is-enabled", service)
	output, err := command.CombinedOutput()
	state := strings.TrimSpace(string(output))
	if err == nil {
		return state == "enabled" || state == "enabled-runtime", nil
	}
	if state == "disabled" || state == "masked" {
		return false, nil
	}
	return false, fmt.Errorf("inspect boot state for %s: %w: %s", service, err, state)
}

func stopServiceAndWait(ctx context.Context, service string, wait time.Duration) error {
	active, err := serviceActive(ctx, service)
	if err != nil {
		return err
	}
	if active {
		if err := runSystemctl(ctx, "stop", service); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(wait)
	for {
		active, err := serviceActive(ctx, service)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service %s did not become inactive within %s", service, wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func runSystemctl(ctx context.Context, action, service string) error {
	return runSudo(ctx, "systemctl", action, service)
}

func runSudo(ctx context.Context, command string, args ...string) error {
	allArgs := append([]string{"-n", command}, args...)
	output, err := exec.CommandContext(ctx, "sudo", allArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"sudo %s %s: %w: %s",
			command,
			strings.Join(args, " "),
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return nil
}

func internalHealthURL(instance string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/api/health", portForInstance(instance))
}

func serviceForInstance(instance string) string {
	if instance == greenInstance {
		return greenService
	}
	return blueService
}

func portForInstance(instance string) int {
	if instance == greenInstance {
		return greenPort
	}
	return bluePort
}

func otherInstance(instance string) string {
	if instance == greenInstance {
		return blueInstance
	}
	return greenInstance
}

func normalizeDial(dial string) string {
	return strings.Replace(strings.TrimSpace(dial), "localhost:", "127.0.0.1:", 1)
}

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func environmentOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type selfUpdateLock struct {
	file *os.File
}

func acquireSelfUpdateLock() (*selfUpdateLock, error) {
	file, err := os.OpenFile(updateLockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, ErrUpdateInProgress
	}
	return &selfUpdateLock{file: file}, nil
}

func selfUpdateLockHeld() bool {
	lock, err := acquireSelfUpdateLock()
	if err != nil {
		return true
	}
	lock.release()
	return false
}

func (l *selfUpdateLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
