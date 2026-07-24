package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"deploycrate-ce/clients/objectstorage"
)

const (
	resticExecutable       = "/usr/local/bin/restic"
	backupSourcesManifest  = "/usr/local/share/deploycrate-ce/backup-sources-v1"
	backupExcludesManifest = "/usr/local/share/deploycrate-ce/backup-excludes-v1"
)

type ServerBackup struct{}

func NewServerBackup() *ServerBackup { return &ServerBackup{} }

type resticSnapshot struct {
	ID      string   `json:"id"`
	ShortID string   `json:"short_id"`
	Tags    []string `json:"tags"`
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

	snapshot, found, lookupErr := findResticSnapshot(ctx, environment, tag)
	if lookupErr != nil {
		if _, initErr := runRestic(ctx, environment, "init"); initErr != nil {
			return BackupArtifact{}, fmt.Errorf(
				"open or initialize Restic repository: %w",
				errors.Join(lookupErr, initErr),
			)
		}
	} else if found {
		return existingServerBackupArtifact(ctx, environment, snapshot)
	}
	recoveryManifest, cleanupManifest, err := createServerRecoveryManifest(ctx, scope)
	if err != nil {
		return BackupArtifact{}, err
	}
	defer cleanupManifest()

	sourcesFile, cleanup, err := existingBackupSources()
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
	}
	for _, immutableTag := range tags {
		arguments = append(arguments, "--tag", immutableTag)
	}
	output, err := runRestic(ctx, environment, arguments...)
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
		if json.Unmarshal(scanner.Bytes(), &candidate) == nil && candidate.MessageType == "summary" {
			summary = candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return BackupArtifact{}, fmt.Errorf("parse Restic output: %w", err)
	}
	if summary.SnapshotID == "" {
		return BackupArtifact{}, errors.New("Restic did not return a snapshot identity")
	}
	return serverBackupArtifact(resticSnapshot{ID: summary.SnapshotID, Tags: tags}, summary.TotalBytesProcessed)
}

func existingServerBackupArtifact(
	ctx context.Context,
	environment []string,
	snapshot resticSnapshot,
) (BackupArtifact, error) {
	output, err := runRestic(ctx, environment, "stats", "--mode", "raw-data", "--json", snapshot.ID)
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

func createServerRecoveryManifest(
	ctx context.Context,
	scope BackupScope,
) (string, func(), error) {
	directory := "/var/lib/deploycrate-ce/recovery-manifests"
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", func() {}, err
	}
	manifestPath := path.Join(directory, scope.Backup.ID.String()+".json")
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
	manifest := map[string]any{
		"version":       1,
		"backup_id":     scope.Backup.ID.String(),
		"server_id":     scope.Backup.ServerID.String(),
		"scheduled_at":  scope.Backup.ScheduledAt.UTC(),
		"packages":      commandOutput("/usr/bin/dpkg-query", "-W", "-f=${Package}\t${Version}\n"),
		"systemd_units": commandOutput("/usr/bin/systemctl", "list-unit-files", "--no-pager", "--no-legend"),
		"containers": commandOutput(
			"/usr/bin/docker", "ps", "--all", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}",
		),
		"slots": map[string]string{
			"blue":  symlinkTarget("/opt/deploycrate-ce/slots/blue"),
			"green": symlinkTarget("/opt/deploycrate-ce/slots/green"),
		},
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

func findResticSnapshot(
	ctx context.Context,
	environment []string,
	tag string,
) (resticSnapshot, bool, error) {
	output, err := runRestic(ctx, environment, "snapshots", "--json", "--tag", tag)
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
		return BackupArtifact{}, errors.New("Restic snapshot identity is not a SHA-256 object digest")
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
	return "s3:" + strings.TrimRight(config.Endpoint, "/") + "/" + config.Bucket + "/" + repositoryPath, nil
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

func runRestic(ctx context.Context, environment []string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/sudo", append([]string{"-n", "-E", resticExecutable}, arguments...)...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 800 {
			message = message[:800]
		}
		return nil, fmt.Errorf("restic command failed: %s: %w", message, err)
	}
	return output, nil
}

func existingBackupSources() (string, func(), error) {
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
		if _, err := os.Stat(source); err == nil {
			if _, err := fmt.Fprintln(file, source); err != nil {
				file.Close()
				cleanup()
				return "", func() {}, err
			}
			count++
		} else if !errors.Is(err, os.ErrNotExist) {
			file.Close()
			cleanup()
			return "", func() {}, fmt.Errorf("inspect backup source %s: %w", source, err)
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
