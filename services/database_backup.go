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
	"path"
	"path/filepath"
	"time"

	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/models"

	"filippo.io/age"
	"github.com/google/uuid"
)

const backupWorkRoot = "/var/lib/deploycrate-ce/runtime/backups"

type DatabaseBackup struct {
	config    config.Config
	version   CurrentVersion
	container *ContainerExecution
}

func NewDatabaseBackup(configuration config.Config, version CurrentVersion, container *ContainerExecution) *DatabaseBackup {
	return &DatabaseBackup{config: configuration, version: version, container: container}
}

type PostgreSQLBackupTarget struct {
	ResourceID            uuid.UUID
	InstallationID        uuid.UUID
	ServerID              uuid.UUID
	ContainerName         string
	DatabaseName          string
	Username              string
	Password              string
	ExcludeRiverTableData bool
}

func (service *DatabaseBackup) Run(
	ctx context.Context,
	scope BackupScope,
	target PostgreSQLBackupTarget,
	credential BackupCredentialPayload,
	store objectstorage.Store,
) (BackupArtifact, error) {
	if scope.Backup.ResourceID == nil {
		return BackupArtifact{}, errors.New("database backup is missing its Resource target")
	}
	objectKey := path.Join(
		"resources",
		scope.Backup.ResourceID.String(),
		target.DatabaseName,
		scope.Backup.ID.String()+".tar.age",
	)
	if remote, err := store.Head(ctx, objectKey); err == nil {
		expected := map[string]string{
			"backup-id": scope.Backup.ID.String(), "policy-id": scope.Backup.BackupPolicyID.String(),
			"resource-id": target.ResourceID.String(), "resource-installation-id": target.InstallationID.String(),
			"database-name": target.DatabaseName, "instance-id": service.config.App.InstanceID,
			"format-version": scope.Backup.FormatVersion,
		}
		for key, value := range expected {
			if remote.Metadata[key] != value {
				return BackupArtifact{}, errors.New("existing database object has conflicting backup identity")
			}
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
	if err := os.Chmod(workDirectory, 0o700); err != nil {
		return BackupArtifact{}, fmt.Errorf("secure database backup work directory: %w", err)
	}
	defer os.RemoveAll(workDirectory)
	dumpPath := filepath.Join(workDirectory, "database.dump")
	globalsPath := filepath.Join(workDirectory, "globals.sql")
	manifestPath := filepath.Join(workDirectory, "manifest.json")
	archivePath := filepath.Join(workDirectory, "database.tar")
	encryptedPath := archivePath + ".age"

	if err := service.dumpDatabase(ctx, target, dumpPath); err != nil {
		return BackupArtifact{}, err
	}
	if err := service.dumpGlobals(ctx, target, globalsPath); err != nil {
		return BackupArtifact{}, err
	}
	if err := service.validateDump(ctx, target, dumpPath); err != nil {
		return BackupArtifact{}, fmt.Errorf("validate PostgreSQL custom dump: %w", err)
	}
	manifest := map[string]any{
		"artifact_version":          scope.Backup.FormatVersion,
		"instance_id":               service.config.App.InstanceID,
		"backup_id":                 scope.Backup.ID.String(),
		"policy_id":                 scope.Backup.BackupPolicyID.String(),
		"resource_id":               target.ResourceID.String(),
		"resource_installation_id":  target.InstallationID.String(),
		"database_name":             target.DatabaseName,
		"scheduled_at":              scope.Backup.ScheduledAt.UTC().Format(time.RFC3339Nano),
		"producer_version":          string(service.version),
		"format":                    "postgresql-custom+globals+tar+age",
		"river_table_data_excluded": target.ExcludeRiverTableData,
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
		"backup-id":                scope.Backup.ID.String(),
		"policy-id":                scope.Backup.BackupPolicyID.String(),
		"resource-id":              target.ResourceID.String(),
		"resource-installation-id": target.InstallationID.String(),
		"database-name":            target.DatabaseName,
		"instance-id":              service.config.App.InstanceID,
		"sha256":                   hex.EncodeToString(digestBytes),
		"format-version":           scope.Backup.FormatVersion,
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

func (service *DatabaseBackup) dumpDatabase(ctx context.Context, target PostgreSQLBackupTarget, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	arguments := []string{
		"--username", target.Username,
		"--dbname", target.DatabaseName,
		"--format=custom",
		"--no-password",
	}
	if target.ExcludeRiverTableData {
		arguments = append(arguments, "--exclude-table-data=river_*")
	}
	runErr := service.runContainerPostgres(ctx, target, nil, file, "pg_dump", arguments...)
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

func (service *DatabaseBackup) dumpGlobals(ctx context.Context, target PostgreSQLBackupTarget, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	arguments := []string{
		"--username", target.Username,
		"--globals-only",
		"--no-password",
	}
	runErr := service.runContainerPostgres(ctx, target, nil, file, "pg_dumpall", arguments...)
	closeErr := file.Close()
	if runErr != nil {
		return fmt.Errorf("create PostgreSQL globals dump: %w", runErr)
	}
	return closeErr
}

func (service *DatabaseBackup) validateDump(ctx context.Context, target PostgreSQLBackupTarget, dumpPath string) error {
	dump, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer dump.Close()
	return service.runContainerPostgres(ctx, target, dump, nil, "pg_restore", "--list")
}

func (service *DatabaseBackup) runContainerPostgres(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	stdin io.Reader,
	stdout io.Writer,
	executable string,
	arguments ...string,
) error {
	return service.container.Exec(ctx, target.ServerID, models.ServerCapabilityResource, containerclient.ExecSpec{
		InstallationID: target.InstallationID.String(), ContainerName: target.ContainerName,
		Executable: executable, Arguments: arguments,
		Environment: map[string]string{"PGPASSWORD": target.Password}, Stdin: stdin, Stdout: stdout,
	})
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
