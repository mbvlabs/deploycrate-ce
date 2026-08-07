package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"github.com/google/uuid"
)

type BackupVerifier struct {
	db       storage.Pool
	queue    storage.InsertQueue
	config   config.Config
	executor *BackupExecutor
	database *DatabaseBackup
	artifact *DatabaseArtifact
}

func NewBackupVerifier(
	db storage.Pool,
	queue storage.InsertQueue,
	configuration config.Config,
	executor *BackupExecutor,
	database *DatabaseBackup,
	artifact *DatabaseArtifact,
) *BackupVerifier {
	return &BackupVerifier{
		db: db, queue: queue, config: configuration, executor: executor, database: database,
		artifact: artifact,
	}
}

func (service *BackupVerifier) Verify(ctx context.Context, backupID uuid.UUID) error {
	scope, err := service.executor.loadScope(ctx, backupID)
	if err != nil {
		return err
	}
	if scope.Backup.Status == models.BackupStatusVerified ||
		scope.Backup.Status == models.BackupStatusPruned {
		return nil
	}
	if scope.Backup.Status != models.BackupStatusUploaded &&
		scope.Backup.Status != models.BackupStatusVerificationFailed {
		return fmt.Errorf("backup %s is not ready for verification", backupID)
	}
	if err := validateBackupScope(scope); err != nil {
		return service.recordVerificationFailure(
			ctx,
			scope,
			fmt.Errorf("validate backup scope: %w", err),
		)
	}
	plaintext, err := secretcrypto.Decrypt(
		scope.CredentialPayload,
		service.config.App.SessionEncryptionKey,
	)
	if err != nil {
		return service.recordVerificationFailure(
			ctx,
			scope,
			fmt.Errorf("decrypt backup credential for verification: %w", err),
		)
	}
	defer clear(plaintext)
	var credential BackupCredentialPayload
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		return service.recordVerificationFailure(
			ctx,
			scope,
			errors.New("decode backup credential for verification"),
		)
	}
	if credential.AccessKeyID == "" || credential.SecretAccessKey == "" ||
		credential.ResticPassword == "" || credential.AgeIdentity == "" {
		return service.recordVerificationFailure(
			ctx,
			scope,
			errors.New("backup credential payload is incomplete for verification"),
		)
	}
	if scope.Backup.TargetType == "server" {
		err = service.verifyServerBackup(ctx, scope, credential)
	} else {
		var target PostgreSQLBackupTarget
		target, err = service.executor.postgreSQLTarget(scope)
		if err != nil {
			return service.recordVerificationFailure(ctx, scope, err)
		}
		var store *objectstorage.Client
		store, err = objectstorage.New(
			ctx,
			scope.ObjectStorageConfig(),
			objectstorage.Credentials{
				AccessKeyID: credential.AccessKeyID, SecretAccessKey: credential.SecretAccessKey,
			},
		)
		if err == nil {
			err = service.verifyDatabaseBackup(ctx, scope, target, credential, store)
		}
	}
	if err != nil {
		return service.recordVerificationFailure(ctx, scope, err)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.Backup.MarkVerified(ctx, tx, backupID); err != nil {
		return fmt.Errorf("record verified backup: %w", err)
	}
	now := time.Now().UTC()
	if err := models.Change.MarkCompleted(ctx, tx, scope.Backup.ChangeID, now); err != nil {
		return fmt.Errorf("complete backup change: %w", err)
	}
	if err := advanceDatabaseRestoreAfterSafetyBackup(
		ctx,
		tx,
		service.queue,
		backupID,
	); err != nil {
		return fmt.Errorf("advance Database restore after safety backup: %w", err)
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BackupRetentionArgs{BackupPolicyID: scope.Backup.BackupPolicyID},
		nil,
	); err != nil {
		return fmt.Errorf("insert backup retention job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.InfoContext(
		ctx,
		"backup verification completed",
		"backup_id", backupID,
		"backup_policy_id", scope.Backup.BackupPolicyID,
		"target_type", scope.Backup.TargetType,
		"target_id", backupTargetID(scope.Backup),
		"driver", scope.Backup.Driver,
		"trigger_type", scope.Backup.TriggerType,
		"lifecycle_status", models.BackupStatusVerified,
		"verification_result", "verified",
	)
	return nil
}

func (service *BackupVerifier) recordVerificationFailure(
	ctx context.Context,
	scope BackupScope,
	verificationErr error,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	recordErr := models.Backup.MarkVerificationFailed(
		persistCtx,
		service.db.Executor(),
		scope.Backup.ID,
		verificationErr,
	)
	changeErr := markBackupChangeFailed(
		persistCtx,
		service.db.Executor(),
		scope.Backup,
		verificationErr,
	)
	restoreErr := failDatabaseRestoreSafetyBackup(
		persistCtx,
		service.db,
		scope.Backup.ID,
		verificationErr,
	)
	slog.ErrorContext(
		ctx,
		"backup verification failed",
		"backup_id", scope.Backup.ID,
		"backup_policy_id", scope.Backup.BackupPolicyID,
		"target_type", scope.Backup.TargetType,
		"target_id", backupTargetID(scope.Backup),
		"driver", scope.Backup.Driver,
		"trigger_type", scope.Backup.TriggerType,
		"lifecycle_status", models.BackupStatusVerificationFailed,
		"error_category", "backup_verification",
		"error", verificationErr,
	)
	return errors.Join(verificationErr, recordErr, changeErr, restoreErr)
}

func (service *BackupVerifier) verifyServerBackup(
	ctx context.Context,
	scope BackupScope,
	credential BackupCredentialPayload,
) error {
	if scope.Backup.ServerID == nil {
		return errors.New("server verification is missing its target")
	}
	repository, err := resticRepository(scope, scope.Backup.ServerID.String())
	if err != nil {
		return err
	}
	environment := resticEnvironment(scope, credential, repository)
	snapshot, found, err := findResticSnapshot(
		ctx,
		environment,
		scope.DestinationPathStyle,
		"backup-id:"+scope.Backup.ID.String(),
	)
	if err != nil {
		return err
	}
	if !found || snapshot.ID != scope.Backup.ArtifactReference {
		return errors.New("Restic snapshot identity does not match the backup record")
	}
	expectedTags := []string{
		"backup-id:" + scope.Backup.ID.String(),
		"policy-id:" + scope.Backup.BackupPolicyID.String(),
		"server-id:" + scope.Backup.ServerID.String(),
		"trigger:" + scope.Backup.TriggerType,
	}
	for _, expected := range expectedTags {
		if !containsString(snapshot.Tags, expected) {
			return fmt.Errorf("Restic snapshot is missing immutable tag %q", expected)
		}
	}
	output, err := runRestic(
		ctx, environment, scope.DestinationPathStyle, "ls", "--json", snapshot.ID,
	)
	if err != nil {
		return fmt.Errorf("list Restic snapshot: %w", err)
	}
	manifestPath := backupRecoveryManifestDirectory + "/" + scope.Backup.ID.String() + ".json"
	clickHousePath := backupRecoveryManifestDirectory + "/" + scope.Backup.ID.String() +
		"-clickhouse-metric-rollups.jsonl"
	requiredPaths := []string{
		"/etc/deploycrate-ce",
		"/var/lib/deploycrate-ce",
		"/var/lib/caddy",
		"/opt/deploycrate-ce/releases",
		"/opt/deploycrate-ce/slots",
		sshCARecoveryBundlePath,
		manifestPath,
		clickHousePath,
	}
	foundPaths := map[string]bool{}
	snapshotPaths := map[string]bool{}
	for line := range strings.SplitSeq(string(output), "\n") {
		var node struct {
			StructType string `json:"struct_type"`
			Path       string `json:"path"`
		}
		if json.Unmarshal([]byte(line), &node) == nil && node.StructType == "node" {
			snapshotPaths[node.Path] = true
			for _, required := range requiredPaths {
				if node.Path == required || strings.HasPrefix(node.Path, required+"/") {
					foundPaths[required] = true
				}
			}
		}
	}
	for _, required := range requiredPaths {
		if !foundPaths[required] {
			return fmt.Errorf("Restic snapshot is missing required manifest path %s", required)
		}
	}
	if snapshotPaths[installerSecretsPath] {
		return errors.New("Restic snapshot contains transient installer secrets")
	}
	manifestBytes, err := runRestic(
		ctx,
		environment,
		scope.DestinationPathStyle,
		"dump",
		snapshot.ID,
		manifestPath,
	)
	if err != nil {
		return fmt.Errorf("read server backup manifest: %w", err)
	}
	var manifest serverRecoveryManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("decode server backup manifest: %w", err)
	}
	if manifest.Version != 1 || manifest.FormatVersion != scope.Backup.FormatVersion ||
		manifest.InstanceID != service.config.App.InstanceID ||
		manifest.BackupID != scope.Backup.ID.String() ||
		manifest.PolicyID != scope.Backup.BackupPolicyID.String() ||
		manifest.ServerID != scope.Backup.ServerID.String() ||
		manifest.ProducerVersion != scope.Backup.ProducerVersion {
		return errors.New("server backup manifest does not match the backup record")
	}
	scheduledAt, err := time.Parse(time.RFC3339Nano, manifest.ScheduledAt)
	if err != nil || !scheduledAt.Equal(scope.Backup.ScheduledAt) {
		return errors.New("server backup manifest has an invalid scheduled time")
	}
	if len(manifest.ReleaseDigests) == 0 || manifest.Slots["blue"] == "" ||
		len(manifest.IdentityFingerprints) != 4 {
		return errors.New("server backup manifest is missing release or identity metadata")
	}
	for releasePath, digestValue := range manifest.ReleaseDigests {
		if !strings.HasPrefix(releasePath, "/opt/deploycrate-ce/releases/") ||
			!validSHA256(digestValue) || !snapshotPaths[releasePath] {
			return errors.New("server backup manifest contains invalid release metadata")
		}
	}
	for _, digestValue := range manifest.IdentityFingerprints {
		if !validSHA256(digestValue) {
			return errors.New("server backup manifest contains an invalid identity fingerprint")
		}
	}
	if !validSHA256(manifest.SSHCARecoverySHA256) {
		return errors.New("server backup manifest contains an invalid SSH CA recovery digest")
	}
	sshCARecoveryBundle, err := runRestic(
		ctx,
		environment,
		scope.DestinationPathStyle,
		"dump",
		snapshot.ID,
		sshCARecoveryBundlePath,
	)
	if err != nil {
		return fmt.Errorf("read encrypted SSH CA recovery bundle: %w", err)
	}
	sshDigest := sha256.Sum256(sshCARecoveryBundle)
	if hex.EncodeToString(sshDigest[:]) != manifest.SSHCARecoverySHA256 {
		return errors.New(
			"encrypted SSH CA recovery bundle digest does not match the server manifest",
		)
	}
	if manifest.ClickHouse.Path != clickHousePath || manifest.ClickHouse.Format != "JSONEachRow" ||
		manifest.ClickHouse.SchemaVersion != clickHouseMetricRollupSchemaVersion ||
		!validSHA256(manifest.ClickHouse.SHA256) || manifest.ClickHouse.SizeBytes < 0 ||
		manifest.ClickHouse.Rows < 0 {
		return errors.New("server backup manifest contains invalid ClickHouse metadata")
	}
	clickHouseBytes, err := runRestic(
		ctx,
		environment,
		scope.DestinationPathStyle,
		"dump",
		snapshot.ID,
		clickHousePath,
	)
	if err != nil {
		return fmt.Errorf("read ClickHouse metric rollup export: %w", err)
	}
	if err := verifyClickHouseExport(clickHouseBytes, manifest.ClickHouse); err != nil {
		return err
	}
	var verification struct {
		RepositoryCheck string `json:"repository_check"`
		DataSubsetParts int    `json:"data_subset_parts"`
	}
	_ = json.Unmarshal(scope.PolicyVerification, &verification)
	checkArguments := []string{"check", "--no-lock"}
	checkRequired := verification.RepositoryCheck == "always" ||
		verification.RepositoryCheck == "weekly" &&
			scope.Backup.ScheduledAt.Weekday() == time.Sunday ||
		scope.Backup.TriggerType == "installer"
	if verification.DataSubsetParts > 0 {
		day := int(scope.Backup.ScheduledAt.UTC().Unix() / int64((24*time.Hour)/time.Second))
		subset := day%verification.DataSubsetParts + 1
		checkArguments = append(
			checkArguments,
			"--read-data-subset",
			fmt.Sprintf("%d/%d", subset, verification.DataSubsetParts),
		)
		checkRequired = true
	}
	if checkRequired {
		if _, err := runRestic(
			ctx, environment, scope.DestinationPathStyle, checkArguments...,
		); err != nil {
			return fmt.Errorf("check Restic repository: %w", err)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	digest, err := hex.DecodeString(value)
	return err == nil && len(digest) == sha256.Size
}

func verifyClickHouseExport(value []byte, expected ClickHouseBackupArtifact) error {
	if int64(len(value)) != expected.SizeBytes {
		return errors.New("ClickHouse metric rollup export size does not match the server manifest")
	}
	digest := sha256.Sum256(value)
	if hex.EncodeToString(digest[:]) != expected.SHA256 {
		return errors.New(
			"ClickHouse metric rollup export digest does not match the server manifest",
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	rows := int64(0)
	firstBucket := ""
	lastBucket := ""
	requiredColumns := []string{
		"bucket_start", "observed_at", "scope", "component", "metric", "average", "maximum", "last",
		"server", "application", "environment", "release", "deployment", "target", "instance",
		"resource", "installation", "runtime_id", "observation_id",
	}
	for {
		var row map[string]json.RawMessage
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("decode ClickHouse metric rollup export: %w", err)
		}
		for _, column := range requiredColumns {
			if _, ok := row[column]; !ok {
				return fmt.Errorf("ClickHouse metric rollup export is missing column %s", column)
			}
		}
		var bucketStart, scope, metricName, server, observationID string
		if json.Unmarshal(row["bucket_start"], &bucketStart) != nil ||
			json.Unmarshal(row["scope"], &scope) != nil ||
			json.Unmarshal(row["metric"], &metricName) != nil ||
			json.Unmarshal(row["server"], &server) != nil ||
			json.Unmarshal(row["observation_id"], &observationID) != nil {
			return errors.New("ClickHouse metric rollup export contains invalid identity fields")
		}
		if bucketStart == "" {
			return errors.New("ClickHouse metric rollup export contains an empty bucket_start")
		}
		if !slices.Contains([]string{"host", "container", "native"}, scope) || metricName == "" {
			return errors.New("ClickHouse metric rollup export contains an invalid metric scope")
		}
		if _, err := uuid.Parse(server); err != nil {
			return errors.New("ClickHouse metric rollup export contains an invalid server identity")
		}
		if _, err := uuid.Parse(observationID); err != nil {
			return errors.New(
				"ClickHouse metric rollup export contains an invalid observation identity",
			)
		}
		rows++
		if firstBucket == "" || bucketStart < firstBucket {
			firstBucket = bucketStart
		}
		if lastBucket == "" || bucketStart > lastBucket {
			lastBucket = bucketStart
		}
	}
	if rows != expected.Rows || firstBucket != expected.FirstBucket ||
		lastBucket != expected.LastBucket {
		return errors.New(
			"ClickHouse metric rollup export contents do not match the server manifest",
		)
	}
	return nil
}

func containsString(values []string, expected string) bool {
	return slices.Contains(values, expected)
}

func (service *BackupVerifier) verifyDatabaseBackup(
	ctx context.Context,
	scope BackupScope,
	target PostgreSQLBackupTarget,
	credential BackupCredentialPayload,
	store objectstorage.Store,
) error {
	loaded, err := service.artifact.Load(ctx, scope, target, credential, store)
	if err != nil {
		return err
	}
	defer loaded.Close()
	if err := service.database.validateDump(ctx, target, loaded.DumpPath); err != nil {
		return fmt.Errorf("validate downloaded PostgreSQL dump: %w", err)
	}
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	difference := byte(0)
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}
