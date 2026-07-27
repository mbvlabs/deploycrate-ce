package services

import (
	"archive/tar"
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

	"filippo.io/age"
)

const backupWorkRoot = "/var/lib/deploycrate-ce/runtime/backups"

type DatabaseBackup struct {
	config  config.Config
	version CurrentVersion
}

func NewDatabaseBackup(configuration config.Config, version CurrentVersion) *DatabaseBackup {
	return &DatabaseBackup{config: configuration, version: version}
}

func (service *DatabaseBackup) Run(
	ctx context.Context,
	scope BackupScope,
	credential BackupCredentialPayload,
	store objectstorage.Store,
) (BackupArtifact, error) {
	if scope.Backup.ResourceID == nil {
		return BackupArtifact{}, errors.New("database backup is missing its resource target")
	}
	objectKey := path.Join(
		"databases",
		scope.Backup.ResourceID.String(),
		scope.Backup.ID.String()+".tar.age",
	)
	if remote, err := store.Head(ctx, objectKey); err == nil {
		if remote.Metadata["backup-id"] != scope.Backup.ID.String() {
			return BackupArtifact{}, errors.New("existing database object has conflicting backup identity")
		}
		digest, decodeErr := hex.DecodeString(remote.Metadata["sha256"])
		if decodeErr != nil || len(digest) != sha256.Size {
			return BackupArtifact{}, errors.New("existing database object has invalid digest metadata")
		}
		metadata, _ := json.Marshal(remote.Metadata)
		return BackupArtifact{Reference: objectKey, Metadata: metadata, Size: remote.Size, Digest: digest}, nil
	} else if !objectstorage.IsNotFound(err) {
		return BackupArtifact{}, err
	}

	workDirectory := filepath.Join(backupWorkRoot, scope.Backup.ID.String())
	if err := os.MkdirAll(workDirectory, 0o700); err != nil {
		return BackupArtifact{}, fmt.Errorf("create database backup work directory: %w", err)
	}
	defer os.RemoveAll(workDirectory)
	dumpPath := filepath.Join(workDirectory, "database.dump")
	globalsPath := filepath.Join(workDirectory, "globals.sql")
	manifestPath := filepath.Join(workDirectory, "manifest.json")
	archivePath := filepath.Join(workDirectory, "database.tar")
	encryptedPath := archivePath + ".age"

	if err := service.dumpDatabase(ctx, dumpPath); err != nil {
		return BackupArtifact{}, err
	}
	if err := service.dumpGlobals(ctx, globalsPath); err != nil {
		return BackupArtifact{}, err
	}
	if err := service.validateDump(ctx, dumpPath); err != nil {
		return BackupArtifact{}, fmt.Errorf("validate PostgreSQL custom dump: %w", err)
	}
	manifest := map[string]any{
		"artifact_version":          scope.Backup.FormatVersion,
		"instance_id":               service.config.App.InstanceID,
		"backup_id":                 scope.Backup.ID.String(),
		"policy_id":                 scope.Backup.BackupPolicyID.String(),
		"resource_id":               scope.Backup.ResourceID.String(),
		"scheduled_at":              scope.Backup.ScheduledAt.UTC().Format(time.RFC3339Nano),
		"producer_version":          string(service.version),
		"format":                    "postgresql-custom+globals+tar+age",
		"river_table_data_excluded": true,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupArtifact{}, err
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		return BackupArtifact{}, fmt.Errorf("write database backup manifest: %w", err)
	}
	if err := createBackupArchive(archivePath, dumpPath, globalsPath, manifestPath); err != nil {
		return BackupArtifact{}, err
	}
	if err := encryptBackupArchive(archivePath, encryptedPath, credential.AgeIdentity); err != nil {
		return BackupArtifact{}, err
	}

	file, err := os.Open(encryptedPath)
	if err != nil {
		return BackupArtifact{}, err
	}
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		file.Close()
		return BackupArtifact{}, fmt.Errorf("digest encrypted database backup: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return BackupArtifact{}, err
	}
	digestBytes := digest.Sum(nil)
	metadata := map[string]string{
		"backup-id":      scope.Backup.ID.String(),
		"policy-id":      scope.Backup.BackupPolicyID.String(),
		"resource-id":    scope.Backup.ResourceID.String(),
		"instance-id":    service.config.App.InstanceID,
		"sha256":         hex.EncodeToString(digestBytes),
		"format-version": scope.Backup.FormatVersion,
	}
	remote, uploadErr := store.Put(ctx, objectKey, file, metadata)
	closeErr := file.Close()
	if uploadErr != nil {
		return BackupArtifact{}, fmt.Errorf("upload database backup: %w", uploadErr)
	}
	if closeErr != nil {
		return BackupArtifact{}, closeErr
	}
	if remote.Size != size {
		return BackupArtifact{}, fmt.Errorf("uploaded database backup size mismatch: local %d, remote %d", size, remote.Size)
	}
	providerMetadata, err := json.Marshal(metadata)
	if err != nil {
		return BackupArtifact{}, err
	}
	return BackupArtifact{
		Reference: objectKey, Metadata: providerMetadata, Size: size, Digest: digestBytes,
	}, nil
}

func (service *DatabaseBackup) dumpDatabase(ctx context.Context, destination string) error {
	database := service.config.DB
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	arguments := []string{
		"--username", database.User,
		"--dbname", database.Name,
		"--format=custom",
		"--no-password",
		"--exclude-table-data=river_*",
	}
	runErr := service.runContainerPostgres(ctx, nil, file, "pg_dump", arguments...)
	closeErr := file.Close()
	if runErr != nil || closeErr != nil {
		if runErr != nil {
			return fmt.Errorf("create PostgreSQL database dump: %w", runErr)
		}
		return closeErr
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("create PostgreSQL database dump: %w", err)
	}
	return nil
}

func (service *DatabaseBackup) dumpGlobals(ctx context.Context, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	database := service.config.DB
	arguments := []string{
		"--username", database.User,
		"--globals-only",
		"--no-password",
	}
	runErr := service.runContainerPostgres(ctx, nil, file, "pg_dumpall", arguments...)
	closeErr := file.Close()
	if runErr != nil {
		return fmt.Errorf("create PostgreSQL globals dump: %w", runErr)
	}
	return closeErr
}

func (service *DatabaseBackup) validateDump(ctx context.Context, dumpPath string) error {
	dump, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer dump.Close()
	return service.runContainerPostgres(ctx, dump, nil, "pg_restore", "--list")
}

func (service *DatabaseBackup) runContainerPostgres(
	ctx context.Context,
	stdin io.Reader,
	stdout io.Writer,
	executable string,
	arguments ...string,
) error {
	containerArguments := []string{
		"exec", "--interactive", "--env", "PGPASSWORD", "deploycrate-ce-postgres", executable,
	}
	command := exec.CommandContext(ctx, "/usr/bin/docker", append(containerArguments, arguments...)...)
	command.Env = []string{"PGPASSWORD=" + service.config.DB.Password}
	command.Stdin = stdin
	command.Stdout = stdout
	var stderr strings.Builder
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return fmt.Errorf("%s: %w", limitedError(stderr.String()), err)
	}
	return nil
}

func limitedError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 800 {
		return value[:800]
	}
	return value
}

func createBackupArchive(destination string, sources ...string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archive := tar.NewWriter(file)
	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			archive.Close()
			file.Close()
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			archive.Close()
			file.Close()
			return err
		}
		header.Name = filepath.Base(source)
		header.Mode = 0o600
		if err := archive.WriteHeader(header); err != nil {
			archive.Close()
			file.Close()
			return err
		}
		input, err := os.Open(source)
		if err != nil {
			archive.Close()
			file.Close()
			return err
		}
		_, copyErr := io.Copy(archive, input)
		closeErr := input.Close()
		if copyErr != nil || closeErr != nil {
			archive.Close()
			file.Close()
			return errors.Join(copyErr, closeErr)
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func encryptBackupArchive(source, destination, identityValue string) error {
	identity, err := age.ParseX25519Identity(identityValue)
	if err != nil {
		return errors.New("parse database backup recovery identity")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encrypted, err := age.Encrypt(output, identity.Recipient())
	if err != nil {
		output.Close()
		return err
	}
	_, copyErr := io.Copy(encrypted, input)
	closeEncryptErr := encrypted.Close()
	closeOutputErr := output.Close()
	return errors.Join(copyErr, closeEncryptErr, closeOutputErr)
}
