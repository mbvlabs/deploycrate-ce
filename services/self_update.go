package services

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
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
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
	"go.uber.org/fx"
)

const (
	defaultReleaseRepository = "mbvlabs/deploycrate-ce"
	defaultReleaseRoot       = "/opt/deploycrate-ce/releases"
	defaultSlotsRoot         = "/opt/deploycrate-ce/slots"
	defaultStatusPath        = "/var/lib/deploycrate-ce/self-update.json"
	defaultCaddyAdminURL     = "http://127.0.0.1:2019"
	updateLockPath           = "/tmp/deploycrate-ce-update.lock"
	greenInstance            = "green"
	blueInstance             = "blue"
	greenService             = "deploycrate-ce@green.service"
	blueService              = "deploycrate-ce@blue.service"
	greenPort                = 8081
	bluePort                 = 8080
)

var (
	ErrUpdateInProgress       = errors.New("a self-update is already in progress")
	ErrAlreadyCurrent         = errors.New("DeployCrate CE is already current")
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

type ReleaseStatus struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type updateRelease struct {
	Version     string
	ArchiveName string
	ArchiveURL  string
	ChecksumURL string
	ReleaseURL  string
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
	release updateRelease
}

type SelfUpdate struct {
	mu             sync.RWMutex
	status         SelfUpdateStatus
	localRunning   bool
	releaseMu      sync.Mutex
	cachedRelease  updateRelease
	releaseChecked time.Time
	currentVersion string
	repository     string
	releaseRoot    string
	slotsRoot      string
	statusPath     string
	caddyAdminURL  string
	publicHealth   string
	client         *http.Client
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

	service := &SelfUpdate{
		status: SelfUpdateStatus{
			State:  SelfUpdateIdle,
			Events: []SelfUpdateEvent{},
		},
		currentVersion: currentVersion,
		repository:     environmentOr("DEPLOYCRATE_CE_RELEASE_REPOSITORY", defaultReleaseRepository),
		releaseRoot:    environmentOr("DEPLOYCRATE_CE_RELEASE_ROOT", defaultReleaseRoot),
		slotsRoot:      environmentOr("DEPLOYCRATE_CE_SLOTS_ROOT", defaultSlotsRoot),
		statusPath:     environmentOr("DEPLOYCRATE_CE_UPDATE_STATUS_PATH", defaultStatusPath),
		caddyAdminURL:  strings.TrimRight(environmentOr("DEPLOYCRATE_CE_CADDY_ADMIN_URL", defaultCaddyAdminURL), "/"),
		publicHealth:   environmentOr("DEPLOYCRATE_CE_PUBLIC_HEALTH_URL", strings.TrimRight(config.BaseURL, "/")+"/api/health"),
		client:         &http.Client{Timeout: 30 * time.Second},
		queue:          make(chan updateJob, 1),
		db:             db,
	}
	service.loadStatus()
	if service.status.UpdatedAt.IsZero() {
		service.status.UpdatedAt = time.Now()
	}

	var done chan struct{}
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
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

func (s *SelfUpdate) Check(ctx context.Context) (ReleaseStatus, error) {
	release, err := s.latestRelease(ctx)
	if err != nil {
		return ReleaseStatus{CurrentVersion: s.currentVersion}, err
	}

	return ReleaseStatus{
		CurrentVersion:  s.currentVersion,
		LatestVersion:   release.Version,
		UpdateAvailable: normalizeVersion(s.currentVersion) != normalizeVersion(release.Version),
		ReleaseURL:      release.ReleaseURL,
	}, nil
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
	release, err := s.latestRelease(ctx)
	if err != nil {
		return s.Status(), err
	}
	if normalizeVersion(release.Version) == normalizeVersion(s.currentVersion) {
		return s.Status(), ErrAlreadyCurrent
	}

	s.mu.Lock()
	if s.localRunning || selfUpdateLockHeld() {
		status := s.status
		s.mu.Unlock()
		return status, ErrUpdateInProgress
	}
	s.localRunning = true
	s.mu.Unlock()

	topology, err := s.loadSystemTopology(ctx)
	if err != nil {
		s.mu.Lock()
		s.localRunning = false
		s.mu.Unlock()
		return s.Status(), err
	}
	deployment, err := s.createDeploymentRecords(ctx, actorID, release, topology)
	if err != nil {
		s.mu.Lock()
		s.localRunning = false
		s.mu.Unlock()
		return s.Status(), err
	}

	s.mu.Lock()
	now := time.Now()
	s.status = SelfUpdateStatus{
		State:         SelfUpdateQueued,
		CurrentStep:   "queued",
		TargetVersion: release.Version,
		StartedAt:     &now,
		UpdatedAt:     now,
		Events: []SelfUpdateEvent{{
			ID:         uuid.NewString(),
			Message:    "Update queued",
			OccurredAt: now,
		}},
	}
	s.currentDeployment = deployment
	s.persistStatusLocked()
	status := s.status
	s.mu.Unlock()

	select {
	case s.queue <- updateJob{release: release}:
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
			s.execute(ctx, job.release)
		}
	}
}

func (s *SelfUpdate) execute(parent context.Context, release updateRelease) {
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

	s.mu.RLock()
	deployment := s.currentDeployment
	s.mu.RUnlock()
	if deployment == nil {
		s.fail("load_deployment", errors.New("self-update deployment record is missing"))
		return
	}

	activeInstance, err := s.runningInstance(ctx)
	if err != nil {
		s.fail("detect_running_service", err)
		return
	}
	if activeInstance != deployment.Topology.ActiveInstanceSlot {
		s.fail("detect_running_service", fmt.Errorf(
			"running slot %q does not match the persisted active slot %q",
			activeInstance,
			deployment.Topology.ActiveInstanceSlot,
		))
		return
	}
	inactiveInstance := deployment.InactiveSlot
	s.setInstances(activeInstance, activeInstance)

	workDir, err := os.MkdirTemp("", "deploycrate-ce-update-*")
	if err != nil {
		s.fail("prepare_artifact", fmt.Errorf("create update directory: %w", err))
		return
	}
	defer os.RemoveAll(workDir)

	archivePath := filepath.Join(workDir, release.ArchiveName)
	checksumPath := filepath.Join(workDir, "checksums.txt")
	s.transition(SelfUpdateInProgress, "download_artifact", "Downloading release artifact")
	if err := s.download(ctx, release.ArchiveURL, archivePath); err != nil {
		s.fail("download_artifact", err)
		return
	}
	if err := s.download(ctx, release.ChecksumURL, checksumPath); err != nil {
		s.fail("download_checksum", err)
		return
	}

	s.transition(SelfUpdateInProgress, "verify_checksum", "Verifying release checksum")
	if err := verifyReleaseChecksum(archivePath, checksumPath, release.ArchiveName); err != nil {
		s.fail("verify_checksum", err)
		return
	}

	stagedBinary := filepath.Join(workDir, "deploycrate-ce")
	if err := extractReleaseBinary(archivePath, stagedBinary); err != nil {
		s.fail("extract_binary", err)
		return
	}
	digest, err := fileDigest(stagedBinary)
	if err != nil {
		s.fail("digest_binary", err)
		return
	}
	if err := s.recordArtifact(digest); err != nil {
		s.fail("record_artifact", fmt.Errorf("record release artifact digest: %w", err))
		return
	}

	s.transition(SelfUpdateInProgress, "install_release", "Installing verified release")
	previousSlotTarget, err := s.stageReleaseAndSlot(ctx, stagedBinary, release.Version, inactiveInstance)
	if err != nil {
		s.fail("install_release", err)
		return
	}

	startedInactive := false
	trafficSwitched := false
	bootStateSwitched := false
	rollback := func(cause error) {
		var rollbackErr error
		if trafficSwitched {
			rollbackErr = errors.Join(rollbackErr, s.setTraffic(ctx, deployment.Topology.CaddyRouteExternalID, activeInstance))
		}
		if bootStateSwitched {
			rollbackErr = errors.Join(rollbackErr, runSystemctl(ctx, "disable", serviceForInstance(inactiveInstance)))
			rollbackErr = errors.Join(rollbackErr, runSystemctl(ctx, "enable", serviceForInstance(activeInstance)))
		}
		if startedInactive {
			rollbackErr = errors.Join(rollbackErr, runSystemctl(ctx, "stop", serviceForInstance(inactiveInstance)))
		}
		rollbackErr = errors.Join(rollbackErr, s.restoreSlot(ctx, inactiveInstance, previousSlotTarget))
		s.setInstances(activeInstance, activeInstance)
		if rollbackErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
		}
		s.fail(s.Status().CurrentStep, cause)
	}

	s.transition(SelfUpdateInProgress, "prepare_traffic", "Preparing blue-green traffic route")
	if err := s.configureTrafficTopology(ctx, deployment.Topology.CaddyRouteExternalID, activeInstance); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "start_inactive_instance", "Starting inactive instance")
	if err := runSystemctl(ctx, "start", serviceForInstance(inactiveInstance)); err != nil {
		rollback(err)
		return
	}
	startedInactive = true

	s.transition(SelfUpdateInProgress, "verify_inactive_instance", "Waiting for inactive instance health")
	if err := s.waitForHealth(ctx, internalHealthURL(inactiveInstance), 30*time.Second); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "switch_traffic", "Switching traffic to updated instance")
	if err := s.setTraffic(ctx, deployment.Topology.CaddyRouteExternalID, inactiveInstance); err != nil {
		rollback(err)
		return
	}
	trafficSwitched = true
	s.setInstances(activeInstance, inactiveInstance)

	s.transition(SelfUpdateInProgress, "verify_public_path", "Verifying public application health")
	if err := s.waitForHealth(ctx, s.publicHealth, 15*time.Second); err != nil {
		rollback(err)
		return
	}

	s.transition(SelfUpdateInProgress, "update_service_boot_state", "Updating enabled service instance")
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
	bootStateSwitched = true

	s.transition(SelfUpdateInProgress, "stop_previous_instance", "Stopping previous instance")
	s.succeed(release.Version, inactiveInstance)
	if err := runSystemctl(ctx, "stop", serviceForInstance(activeInstance)); err != nil {
		slog.ErrorContext(ctx, "failed to stop previous DeployCrate CE instance", "service", serviceForInstance(activeInstance), "error", err)
	}
}

func (s *SelfUpdate) latestRelease(ctx context.Context) (updateRelease, error) {
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	if s.cachedRelease.Version != "" && time.Since(s.releaseChecked) < 5*time.Minute {
		return s.cachedRelease, nil
	}

	release, err := s.fetchLatestRelease(ctx)
	if err != nil {
		return updateRelease{}, err
	}
	s.cachedRelease = release
	s.releaseChecked = time.Now()
	return release, nil
}

func (s *SelfUpdate) fetchLatestRelease(ctx context.Context) (updateRelease, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", s.repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return updateRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "deploycrate-ce-self-update")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.client.Do(req)
	if err != nil {
		return updateRelease{}, fmt.Errorf("check latest GitHub release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return updateRelease{}, fmt.Errorf("check latest GitHub release: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return updateRelease{}, fmt.Errorf("decode latest GitHub release: %w", err)
	}
	version := normalizeVersion(release.TagName)
	if version == "" {
		return updateRelease{}, errors.New("latest GitHub release has no version tag")
	}

	archiveName := fmt.Sprintf("deploycrate-ce_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var archiveURL, checksumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case archiveName:
			archiveURL = asset.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if archiveURL == "" || checksumURL == "" {
		return updateRelease{}, fmt.Errorf("release %s does not contain %s and checksums.txt", release.TagName, archiveName)
	}

	return updateRelease{
		Version:     version,
		ArchiveName: archiveName,
		ArchiveURL:  archiveURL,
		ChecksumURL: checksumURL,
		ReleaseURL:  release.HTMLURL,
	}, nil
}

func (s *SelfUpdate) download(ctx context.Context, sourceURL, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "deploycrate-ce-self-update")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", sourceURL, resp.StatusCode)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	return errors.Join(copyErr, closeErr)
}

func (s *SelfUpdate) runningInstance(ctx context.Context) (string, error) {
	greenActive, greenErr := serviceActive(ctx, greenService)
	blueActive, blueErr := serviceActive(ctx, blueService)
	if greenErr != nil || blueErr != nil {
		return "", fmt.Errorf("%w: expected %s.service and %s.service: %v", ErrBlueGreenNotConfigured, greenService, blueService, errors.Join(greenErr, blueErr))
	}
	switch {
	case greenActive && blueActive:
		return "", errors.New("both DeployCrate CE instances are active before update")
	case !greenActive && !blueActive:
		return "", fmt.Errorf("%w: neither %s nor %s is active", ErrBlueGreenNotConfigured, greenService, blueService)
	case greenActive:
		return greenInstance, nil
	default:
		return blueInstance, nil
	}
}

func (s *SelfUpdate) stageReleaseAndSlot(ctx context.Context, sourcePath, version, slot string) (string, error) {
	releasePath := s.releaseBinaryPath(version)
	slotPath := s.slotBinaryPath(slot)
	previousTarget, err := os.Readlink(slotPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read current %s slot target: %w", slot, err)
	}
	if err := runSudo(ctx, "install", "-d", "-m", "0755", filepath.Dir(releasePath), filepath.Dir(slotPath)); err != nil {
		return "", fmt.Errorf("create release and slot directories: %w", err)
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
	if err := s.replaceSlotLink(ctx, slotPath, releasePath); err != nil {
		return "", err
	}
	return previousTarget, nil
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

func (s *SelfUpdate) releaseBinaryPath(version string) string {
	return filepath.Join(s.releaseRoot, releaseDirectoryName(version), "deploycrate-ce")
}

func (s *SelfUpdate) slotBinaryPath(slot string) string {
	return filepath.Join(s.slotsRoot, slot, "deploycrate-ce")
}

func (s *SelfUpdate) configureTrafficTopology(ctx context.Context, routeID, activeInstance string) error {
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

func (s *SelfUpdate) readTrafficRoute(ctx context.Context, routeID string) (caddyRoute, string, error) {
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
		return caddyRoute{}, "", fmt.Errorf("read Caddy self-update route: status %d", resp.StatusCode)
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

func (s *SelfUpdate) patchTrafficWeights(ctx context.Context, routeID, handlePath string, upstreams []caddyUpstream, targetInstance string) error {
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
	return s.patchCaddyValue(ctx, routeID, handlePath+"/load_balancing/selection_policy/weights", weights, "switch Caddy traffic")
}

func (s *SelfUpdate) patchCaddyValue(ctx context.Context, routeID, path string, value any, operation string) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	patchURL := fmt.Sprintf("%s/id/%s/%s", s.caddyAdminURL, url.PathEscape(routeID), path)
	patch, err := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, strings.NewReader(string(body)))
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
		return fmt.Errorf("%s: status %d: %s", operation, patchResponse.StatusCode, strings.TrimSpace(string(responseBody)))
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
			if handle, nestedPath := findReverseProxy(handles[index].Routes[routeIndex].Handle, fmt.Sprintf("%s/routes/%d/handle", path, routeIndex)); handle != nil {
				return handle, nestedPath
			}
		}
	}
	return nil, ""
}

func (s *SelfUpdate) waitForHealth(ctx context.Context, healthURL string, wait time.Duration) error {
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
	s.status.Events = append(s.status.Events, SelfUpdateEvent{ID: uuid.NewString(), Message: message, OccurredAt: s.status.UpdatedAt})
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

func (s *SelfUpdate) fail(step string, err error) {
	s.mu.Lock()
	now := time.Now()
	s.status.State = SelfUpdateFailed
	s.status.CurrentStep = step
	s.status.Error = err.Error()
	s.status.FinishedAt = &now
	s.status.UpdatedAt = now
	s.status.Events = append(s.status.Events, SelfUpdateEvent{ID: uuid.NewString(), Message: "Update failed: " + err.Error(), OccurredAt: now})
	s.persistStatusLocked()
	s.mu.Unlock()
	s.finishDeployment(false, err)
	slog.Error("DeployCrate CE self-update failed", "step", step, "error", err)
}

func (s *SelfUpdate) succeed(version, instance string) {
	s.mu.Lock()
	now := time.Now()
	s.status.State = SelfUpdateSucceeded
	s.status.CurrentStep = "completed"
	s.status.TargetVersion = version
	s.status.ActiveInstance = instance
	s.status.Error = ""
	s.status.FinishedAt = &now
	s.status.UpdatedAt = now
	s.status.Events = append(s.status.Events, SelfUpdateEvent{ID: uuid.NewString(), Message: "Update completed", OccurredAt: now})
	s.persistStatusLocked()
	s.mu.Unlock()
	s.finishDeployment(true, nil)
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

func verifyReleaseChecksum(archivePath, checksumsPath, archiveName string) error {
	checksums, err := os.Open(checksumsPath)
	if err != nil {
		return err
	}
	defer checksums.Close()

	expected := ""
	scanner := bufio.NewScanner(checksums)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == archiveName {
			expected = fields[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s was not found", archiveName)
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, archive); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archiveName, expected, actual)
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
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '.' || value == '-' || value == '_' {
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

func extractReleaseBinary(archivePath, destination string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open release archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "deploycrate-ce" {
			continue
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(tarReader, 512<<20))
		closeErr := output.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		return os.Chmod(destination, 0o755)
	}
	return errors.New("release archive does not contain deploycrate-ce")
}

func serviceActive(ctx context.Context, service string) (bool, error) {
	command := exec.CommandContext(ctx, "sudo", "-n", "systemctl", "is-active", "--quiet", service)
	err := command.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 3 {
		return false, nil
	}
	return false, err
}

func runSystemctl(ctx context.Context, action, service string) error {
	return runSudo(ctx, "systemctl", action, service)
}

func runSudo(ctx context.Context, command string, args ...string) error {
	allArgs := append([]string{"-n", command}, args...)
	output, err := exec.CommandContext(ctx, "sudo", allArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo %s %s: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
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
