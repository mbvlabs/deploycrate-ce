package services

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"

	"filippo.io/age"
	"github.com/google/uuid"
)

type BackupVerifier struct {
	db       storage.Pool
	queue    storage.InsertQueue
	config   config.Config
	executor *BackupExecutor
	database *DatabaseBackup
}

func NewBackupVerifier(
	db storage.Pool,
	queue storage.InsertQueue,
	configuration config.Config,
	executor *BackupExecutor,
	database *DatabaseBackup,
) *BackupVerifier {
	return &BackupVerifier{
		db: db, queue: queue, config: configuration, executor: executor, database: database,
	}
}

func (service *BackupVerifier) Verify(ctx context.Context, backupID uuid.UUID) error {
	scope, err := service.executor.loadScope(ctx, backupID)
	if err != nil {
		return err
	}
	if scope.Backup.Status == models.BackupStatusVerified || scope.Backup.Status == models.BackupStatusPruned {
		return nil
	}
	if scope.Backup.Status != models.BackupStatusUploaded &&
		scope.Backup.Status != models.BackupStatusVerificationFailed {
		return fmt.Errorf("backup %s is not ready for verification", backupID)
	}
	plaintext, err := secretcrypto.Decrypt(scope.CredentialPayload, service.config.App.SessionEncryptionKey)
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
		err = verifyServerBackup(ctx, scope, credential)
	} else {
		var store *objectstorage.Client
		store, err = objectstorage.New(
			ctx,
			scope.ObjectStorageConfig(),
			objectstorage.Credentials{
				AccessKeyID: credential.AccessKeyID, SecretAccessKey: credential.SecretAccessKey,
			},
		)
		if err == nil {
			err = service.verifyDatabaseBackup(ctx, scope, credential, store)
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
	if _, err := tx.NewUpdate().TableExpr("changes").
		Set("status = ?", "completed").
		Set("finished_at = ?", now).
		Set("error = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", scope.Backup.ChangeID).
		Exec(ctx); err != nil {
		return fmt.Errorf("complete backup change: %w", err)
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
	return errors.Join(verificationErr, recordErr, changeErr)
}

func verifyServerBackup(
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
	output, err := runRestic(ctx, environment, "ls", "--json", snapshot.ID)
	if err != nil {
		return fmt.Errorf("list Restic snapshot: %w", err)
	}
	requiredPaths := []string{
		"/etc/deploycrate-ce",
		"/var/lib/deploycrate-ce",
		"/var/lib/deploycrate-ce/recovery-manifests/" + scope.Backup.ID.String() + ".json",
	}
	foundPaths := map[string]bool{}
	for line := range strings.SplitSeq(string(output), "\n") {
		var node struct {
			StructType string `json:"struct_type"`
			Path       string `json:"path"`
		}
		if json.Unmarshal([]byte(line), &node) == nil && node.StructType == "node" {
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
	var verification struct {
		RepositoryCheck string `json:"repository_check"`
		DataSubsetParts int    `json:"data_subset_parts"`
	}
	_ = json.Unmarshal(scope.PolicyVerification, &verification)
	checkArguments := []string{"check", "--no-lock"}
	checkRequired := verification.RepositoryCheck == "always" ||
		verification.RepositoryCheck == "weekly" && scope.Backup.ScheduledAt.Weekday() == time.Sunday ||
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
		if _, err := runRestic(ctx, environment, checkArguments...); err != nil {
			return fmt.Errorf("check Restic repository: %w", err)
		}
	}
	return nil
}

func containsString(values []string, expected string) bool {
	return slices.Contains(values, expected)
}

func (service *BackupVerifier) verifyDatabaseBackup(
	ctx context.Context,
	scope BackupScope,
	credential BackupCredentialPayload,
	store objectstorage.Store,
) error {
	if err := os.MkdirAll(backupWorkRoot, 0o700); err != nil {
		return fmt.Errorf("create database verification work root: %w", err)
	}
	workDirectory, err := os.MkdirTemp(backupWorkRoot, "verify-"+scope.Backup.ID.String()+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDirectory)
	encryptedPath := filepath.Join(workDirectory, "database.tar.age")
	encrypted, err := os.OpenFile(encryptedPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	remote, getErr := store.Get(ctx, scope.Backup.ArtifactReference, io.MultiWriter(encrypted, digest))
	closeErr := encrypted.Close()
	if getErr != nil || closeErr != nil {
		return errors.Join(getErr, closeErr)
	}
	if !scope.Backup.SizeBytes.Valid || remote.Size != scope.Backup.SizeBytes.Int64 {
		return errors.New("database backup object size does not match the backup record")
	}
	if !equalBytes(digest.Sum(nil), scope.Backup.Digest) {
		return errors.New("database backup ciphertext digest does not match the backup record")
	}
	if scope.Backup.ResourceID == nil {
		return errors.New("database backup record is missing its resource identity")
	}
	expectedMetadata := map[string]string{
		"backup-id":      scope.Backup.ID.String(),
		"policy-id":      scope.Backup.BackupPolicyID.String(),
		"resource-id":    scope.Backup.ResourceID.String(),
		"sha256":         fmt.Sprintf("%x", scope.Backup.Digest),
		"format-version": scope.Backup.FormatVersion,
	}
	for key, expected := range expectedMetadata {
		if remote.Metadata[key] != expected {
			return fmt.Errorf("database backup object metadata %q does not match the backup record", key)
		}
	}
	identity, err := age.ParseX25519Identity(credential.AgeIdentity)
	if err != nil {
		return errors.New("parse database backup recovery identity")
	}
	input, err := os.Open(encryptedPath)
	if err != nil {
		return err
	}
	decrypted, err := age.Decrypt(input, identity)
	if err != nil {
		input.Close()
		return fmt.Errorf("decrypt database backup: %w", err)
	}
	dumpPath := filepath.Join(workDirectory, "database.dump")
	required := map[string]bool{"database.dump": false, "globals.sql": false, "manifest.json": false}
	var manifestBytes []byte
	archive := tar.NewReader(decrypted)
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			input.Close()
			return fmt.Errorf("read database backup archive: %w", nextErr)
		}
		if _, ok := required[header.Name]; !ok {
			input.Close()
			return fmt.Errorf("database backup contains unexpected entry %q", header.Name)
		}
		if required[header.Name] {
			input.Close()
			return fmt.Errorf("database backup contains duplicate entry %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			input.Close()
			return fmt.Errorf("database backup entry %q is not a regular file", header.Name)
		}
		required[header.Name] = true
		if header.Name == "database.dump" {
			output, err := os.OpenFile(dumpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				input.Close()
				return err
			}
			_, copyErr := io.Copy(output, archive)
			closeErr := output.Close()
			if copyErr != nil || closeErr != nil {
				input.Close()
				return errors.Join(copyErr, closeErr)
			}
		} else if header.Name == "manifest.json" {
			if header.Size > 1024*1024 {
				input.Close()
				return errors.New("database backup manifest is too large")
			}
			manifestBytes, err = io.ReadAll(archive)
			if err != nil {
				input.Close()
				return fmt.Errorf("read database backup manifest: %w", err)
			}
		}
	}
	if err := input.Close(); err != nil {
		return err
	}
	for entry, found := range required {
		if !found {
			return fmt.Errorf("database backup archive is missing %s", entry)
		}
	}
	var manifest struct {
		ArtifactVersion string    `json:"artifact_version"`
		BackupID        string    `json:"backup_id"`
		PolicyID        string    `json:"policy_id"`
		ResourceID      string    `json:"resource_id"`
		ScheduledAt     time.Time `json:"scheduled_at"`
		ProducerVersion string    `json:"producer_version"`
		Format          string    `json:"format"`
		RiverExcluded   bool      `json:"river_table_data_excluded"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("decode database backup manifest: %w", err)
	}
	if manifest.ArtifactVersion != scope.Backup.FormatVersion ||
		manifest.BackupID != scope.Backup.ID.String() ||
		manifest.PolicyID != scope.Backup.BackupPolicyID.String() ||
		manifest.ResourceID != scope.Backup.ResourceID.String() ||
		!manifest.ScheduledAt.Equal(scope.Backup.ScheduledAt) ||
		manifest.ProducerVersion != scope.Backup.ProducerVersion ||
		manifest.Format != "postgresql-custom+globals+tar+age" ||
		!manifest.RiverExcluded {
		return errors.New("database backup manifest does not match the backup record")
	}
	if err := service.database.validateDump(ctx, dumpPath); err != nil {
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
