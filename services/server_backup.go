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
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/sudo"
)

const (
	resticExecutable                = "/usr/local/bin/restic"
	backupSourcesManifest           = "/usr/local/share/deploycrate-ce/backup-sources-v1"
	backupExcludesManifest          = "/usr/local/share/deploycrate-ce/backup-excludes-v1"
	backupRecoveryManifestDirectory = "/var/lib/deploycrate-ce/runtime/recovery-manifests"
	sshCARecoveryBundlePath         = "/var/lib/deploycrate/ssh-ca/deploycrate-ssh-ca-recovery-v1.age"
	installerSecretsPath            = "/etc/deploycrate-ce/installer-secrets.json"
)

type ServerBackup struct {
	config     config.Config
	clickhouse *ClickHouseBackup
}

func NewServerBackup(configuration config.Config, clickhouse *ClickHouseBackup) *ServerBackup {
	return &ServerBackup{config: configuration, clickhouse: clickhouse}
}

type resticSnapshot struct {
	ID      string   `json:"id"`
	ShortID string   `json:"short_id"`
	Tags    []string `json:"tags"`
}

type serverRecoveryManifest struct {
	Version              int                      `json:"version"`
	FormatVersion        string                   `json:"format_version"`
	InstanceID           string                   `json:"instance_id"`
	BackupID             string                   `json:"backup_id"`
	PolicyID             string                   `json:"policy_id"`
	ServerID             string                   `json:"server_id"`
	ScheduledAt          string                   `json:"scheduled_at"`
	ProducerVersion      string                   `json:"producer_version"`
	Packages             string                   `json:"packages"`
	SystemdUnits         string                   `json:"systemd_units"`
	Containers           string                   `json:"containers"`
	Slots                map[string]string        `json:"slots"`
	ReleaseDigests       map[string]string        `json:"release_digests"`
	IdentityFingerprints map[string]string        `json:"identity_fingerprints"`
	SSHCARecoverySHA256  string                   `json:"ssh_ca_recovery_sha256"`
	ClickHouse           ClickHouseBackupArtifact `json:"clickhouse_metric_rollups"`
}

func (service *ServerBackup) Run(
	ctx context.Context,
	scope BackupScope,
	credential BackupCredentialPayload,
) (BackupArtifact, error) {
	if scope.Backup.ServerID == nil {
		return BackupArtifact{}, errors.New("server backup is missing its server target")
	}
	repository, err := resticRepository(scope, scope.Backup.ServerID.String())
	if err != nil {
		return BackupArtifact{}, err
	}
	environment := resticEnvironment(scope, credential, repository)
	tag := "backup-id:" + scope.Backup.ID.String()

	snapshot, found, lookupErr := findResticSnapshot(
		ctx, environment, scope.DestinationPathStyle, tag,
	)
	if lookupErr != nil {
		if _, initErr := runRestic(
			ctx,
			environment,
			scope.DestinationPathStyle,
			"init",
		); initErr != nil {
			return BackupArtifact{}, fmt.Errorf(
				"open or initialize Restic repository: %w",
				errors.Join(lookupErr, initErr),
			)
		}
	} else if found {
		return existingServerBackupArtifact(
			ctx, environment, scope.DestinationPathStyle, snapshot,
		)
	}
	clickHousePath := path.Join(
		backupRecoveryManifestDirectory,
		scope.Backup.ID.String()+"-clickhouse-metric-rollups.jsonl",
	)
	clickHouseArtifact, err := service.clickhouse.Export(ctx, clickHousePath)
	if err != nil {
		return BackupArtifact{}, err
	}
	defer os.Remove(clickHousePath)
	recoveryManifest, cleanupManifest, err := service.createServerRecoveryManifest(
		ctx,
		scope,
		clickHouseArtifact,
	)
	if err != nil {
		return BackupArtifact{}, err
	}
	defer cleanupManifest()

	sourcesFile, cleanup, err := existingBackupSources(ctx)
	if err != nil {
		return BackupArtifact{}, err
	}
	defer cleanup()
	tags := []string{
		tag,
		"policy-id:" + scope.Backup.BackupPolicyID.String(),
		"server-id:" + scope.Backup.ServerID.String(),
		"trigger:" + scope.Backup.TriggerType,
	}
	arguments := []string{
		"backup", "--json", "--files-from", sourcesFile,
		"--exclude-file", backupExcludesManifest,
		recoveryManifest,
		clickHousePath,
	}
	for _, immutableTag := range tags {
		arguments = append(arguments, "--tag", immutableTag)
	}
	output, err := runRestic(ctx, environment, scope.DestinationPathStyle, arguments...)
	if err != nil {
		return BackupArtifact{}, fmt.Errorf("create Restic server backup: %w", err)
	}

	var summary struct {
		MessageType         string `json:"message_type"`
		SnapshotID          string `json:"snapshot_id"`
		TotalBytesProcessed int64  `json:"total_bytes_processed"`
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		var candidate struct {
			MessageType         string `json:"message_type"`
			SnapshotID          string `json:"snapshot_id"`
			TotalBytesProcessed int64  `json:"total_bytes_processed"`
		}
		if json.Unmarshal(scanner.Bytes(), &candidate) == nil &&
			candidate.MessageType == "summary" {
			summary = candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return BackupArtifact{}, fmt.Errorf("parse Restic output: %w", err)
	}
	if summary.SnapshotID == "" {
		return BackupArtifact{}, errors.New("Restic did not return a snapshot identity")
	}
	return serverBackupArtifact(
		resticSnapshot{ID: summary.SnapshotID, Tags: tags},
		summary.TotalBytesProcessed,
	)
}

func existingServerBackupArtifact(
	ctx context.Context,
	environment []string,
	forcePathStyle bool,
	snapshot resticSnapshot,
) (BackupArtifact, error) {
	output, err := runRestic(
		ctx, environment, forcePathStyle, "stats", "--mode", "raw-data", "--json", snapshot.ID,
	)
	if err != nil {
		return BackupArtifact{}, fmt.Errorf("inspect existing Restic snapshot: %w", err)
	}
	var stats struct {
		TotalSize int64 `json:"total_size"`
	}
	if err := json.Unmarshal(output, &stats); err != nil {
		return BackupArtifact{}, fmt.Errorf("parse existing Restic snapshot statistics: %w", err)
	}
	return serverBackupArtifact(snapshot, stats.TotalSize)
}

func (service *ServerBackup) createServerRecoveryManifest(
	ctx context.Context,
	scope BackupScope,
	clickhouse ClickHouseBackupArtifact,
) (string, func(), error) {
	if err := os.MkdirAll(backupRecoveryManifestDirectory, 0o700); err != nil {
		return "", func() {}, err
	}
	manifestPath := path.Join(backupRecoveryManifestDirectory, scope.Backup.ID.String()+".json")
	commandOutput := func(executable string, arguments ...string) string {
		output, err := exec.CommandContext(ctx, executable, arguments...).Output()
		if err != nil {
			return "unavailable: " + err.Error()
		}
		return string(output)
	}
	symlinkTarget := func(value string) string {
		target, err := os.Readlink(value)
		if err != nil {
			return ""
		}
		return target
	}
	releaseDigests, err := applicationReleaseDigests()
	if err != nil {
		return "", func() {}, err
	}
	sshCARecoveryDigest, err := fileSHA256(sshCARecoveryBundlePath)
	if err != nil {
		return "", func() {}, fmt.Errorf("digest SSH CA recovery bundle: %w", err)
	}
	identityFingerprints := map[string]string{}
	for name, filePath := range map[string]string{
		"ssh_user_ca":    "/etc/ssh/deploycrate-user-ca.pub",
		"ssh_host_ca":    "/etc/ssh/deploycrate-host-ca.pub",
		"ssh_host_key":   "/etc/ssh/ssh_host_ed25519_key.pub",
		"wireguard_peer": "/etc/wireguard/deploycrate-ce.pub",
	} {
		fingerprint, err := privilegedFileSHA256(ctx, filePath)
		if err != nil {
			return "", func() {}, fmt.Errorf("fingerprint %s: %w", name, err)
		}
		identityFingerprints[name] = fingerprint
	}
	manifest := serverRecoveryManifest{
		Version:         1,
		FormatVersion:   scope.Backup.FormatVersion,
		InstanceID:      service.config.App.InstanceID,
		BackupID:        scope.Backup.ID.String(),
		PolicyID:        scope.Backup.BackupPolicyID.String(),
		ServerID:        scope.Backup.ServerID.String(),
		ScheduledAt:     scope.Backup.ScheduledAt.UTC().Format(time.RFC3339Nano),
		ProducerVersion: scope.Backup.ProducerVersion,
		Packages:        commandOutput("/usr/bin/dpkg-query", "-W", "-f=${Package}\t${Version}\n"),
		SystemdUnits: commandOutput(
			"/usr/bin/systemctl",
			"list-unit-files",
			"--no-pager",
			"--no-legend",
		),
		Containers: commandOutput(
			"/usr/bin/docker", "ps", "--all", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}",
		),
		Slots: map[string]string{
			"blue":  symlinkTarget("/opt/deploycrate-ce/slots/blue/deploycrate-ce"),
			"green": symlinkTarget("/opt/deploycrate-ce/slots/green/deploycrate-ce"),
		},
		ReleaseDigests:       releaseDigests,
		IdentityFingerprints: identityFingerprints,
		SSHCARecoverySHA256:  sshCARecoveryDigest,
		ClickHouse:           clickhouse,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", func() {}, err
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(manifestPath) }
	return manifestPath, cleanup, nil
}

func applicationReleaseDigests() (map[string]string, error) {
	digests := map[string]string{}
	err := filepath.WalkDir(
		"/opt/deploycrate-ce/releases",
		func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Name() != "deploycrate-ce" {
				return nil
			}
			digest, err := fileSHA256(filePath)
			if err != nil {
				return err
			}
			digests[filePath] = digest
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("digest application releases: %w", err)
	}
	if len(digests) == 0 {
		return nil, errors.New("no application release binaries are available for backup")
	}
	return digests, nil
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func privilegedFileSHA256(ctx context.Context, filePath string) (string, error) {
	output, err := sudo.CommandContext(ctx, "/usr/bin/sha256sum", filePath).Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return "", errors.New("sha256sum returned an invalid result")
	}
	digest, err := hex.DecodeString(fields[0])
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("sha256sum returned an invalid digest")
	}
	return fields[0], nil
}

func findResticSnapshot(
	ctx context.Context,
	environment []string,
	forcePathStyle bool,
	tag string,
) (resticSnapshot, bool, error) {
	output, err := runRestic(
		ctx, environment, forcePathStyle, "snapshots", "--json", "--tag", tag,
	)
	if err != nil {
		return resticSnapshot{}, false, err
	}
	var snapshots []resticSnapshot
	if err := json.Unmarshal(output, &snapshots); err != nil {
		return resticSnapshot{}, false, fmt.Errorf("parse Restic snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		return resticSnapshot{}, false, nil
	}
	return snapshots[len(snapshots)-1], true, nil
}

func serverBackupArtifact(snapshot resticSnapshot, size int64) (BackupArtifact, error) {
	digest, err := hex.DecodeString(snapshot.ID)
	if err != nil || len(digest) != sha256.Size {
		return BackupArtifact{}, errors.New(
			"Restic snapshot identity is not a SHA-256 object digest",
		)
	}
	metadata, err := json.Marshal(map[string]any{
		"snapshot_id": snapshot.ID,
		"tags":        snapshot.Tags,
		"digest_kind": "restic_snapshot_object_sha256",
	})
	if err != nil {
		return BackupArtifact{}, err
	}
	return BackupArtifact{
		Reference: snapshot.ID, Metadata: metadata, Size: size, Digest: digest,
	}, nil
}

func resticRepository(scope BackupScope, serverID string) (string, error) {
	config, err := objectstorage.Normalize(scope.ObjectStorageConfig())
	if err != nil {
		return "", err
	}
	repositoryPath := path.Join(config.Prefix, "repositories", "server", serverID)
	if config.Endpoint == "" {
		return "s3:s3.amazonaws.com/" + config.Bucket + "/" + repositoryPath, nil
	}
	return "s3:" + strings.TrimRight(
		config.Endpoint,
		"/",
	) + "/" + config.Bucket + "/" + repositoryPath, nil
}

func resticEnvironment(
	scope BackupScope,
	credential BackupCredentialPayload,
	repository string,
) []string {
	return []string{
		"AWS_ACCESS_KEY_ID=" + credential.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + credential.SecretAccessKey,
		"AWS_DEFAULT_REGION=" + scope.DestinationRegion,
		"RESTIC_REPOSITORY=" + repository,
		"RESTIC_PASSWORD=" + credential.ResticPassword,
	}
}

func runRestic(
	ctx context.Context,
	environment []string,
	forcePathStyle bool,
	arguments ...string,
) ([]byte, error) {
	bucketLookup := "dns"
	if forcePathStyle {
		bucketLookup = "path"
	}
	resticArguments := []string{"-o", "s3.bucket-lookup=" + bucketLookup}
	resticArguments = append(resticArguments, arguments...)
	command := sudo.CommandContextPreserveEnvironment(
		ctx,
		resticExecutable,
		resticArguments...,
	)
	command.Env = environment
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(string(output))
		}
		if len(message) > 800 {
			message = message[:800]
		}
		return nil, fmt.Errorf("restic command failed: %s: %w", message, err)
	}
	return output, nil
}

func existingBackupSources(ctx context.Context) (string, func(), error) {
	manifest, err := os.Open(backupSourcesManifest)
	if err != nil {
		return "", func() {}, fmt.Errorf("open backup source manifest: %w", err)
	}
	defer manifest.Close()
	file, err := os.CreateTemp("", "deploycrate-backup-sources-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create backup source list: %w", err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		cleanup()
		return "", func() {}, err
	}
	scanner := bufio.NewScanner(manifest)
	count := 0
	for scanner.Scan() {
		source := strings.TrimSpace(scanner.Text())
		if source == "" {
			continue
		}
		exists, err := privilegedPathExists(ctx, source)
		if err != nil {
			file.Close()
			cleanup()
			return "", func() {}, fmt.Errorf("inspect backup source %s: %w", source, err)
		}
		if exists {
			if _, err := fmt.Fprintln(file, source); err != nil {
				file.Close()
				cleanup()
				return "", func() {}, err
			}
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		file.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if count == 0 {
		cleanup()
		return "", func() {}, errors.New("backup source manifest has no available paths")
	}
	return file.Name(), cleanup, nil
}

func privilegedPathExists(ctx context.Context, filePath string) (bool, error) {
	err := sudo.CommandContext(ctx, "/usr/bin/test", "-e", filePath).Run()
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
