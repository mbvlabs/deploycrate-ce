package services

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"

	"filippo.io/age"
)

type DatabaseArtifact struct {
	config config.Config
}

func NewDatabaseArtifact(configuration config.Config) *DatabaseArtifact {
	return &DatabaseArtifact{config: configuration}
}

type LoadedDatabaseArtifact struct {
	DumpPath string
	cleanup  func()
}

func (artifact LoadedDatabaseArtifact) Close() {
	if artifact.cleanup != nil {
		artifact.cleanup()
	}
}

type databaseArtifactManifest struct {
	ArtifactVersion        string    `json:"artifact_version"`
	InstanceID             string    `json:"instance_id"`
	BackupID               string    `json:"backup_id"`
	PolicyID               string    `json:"policy_id"`
	ResourceID             string    `json:"resource_id"`
	ResourceInstallationID string    `json:"resource_installation_id"`
	DatabaseName           string    `json:"database_name"`
	ScheduledAt            time.Time `json:"scheduled_at"`
	ProducerVersion        string    `json:"producer_version"`
	Format                 string    `json:"format"`
	RiverExcluded          bool      `json:"river_table_data_excluded"`
}

func (service *DatabaseArtifact) Load(
	ctx context.Context,
	scope BackupScope,
	target PostgreSQLBackupTarget,
	credential BackupCredentialPayload,
	store objectstorage.Store,
) (LoadedDatabaseArtifact, error) {
	if err := os.MkdirAll(backupWorkRoot, 0o700); err != nil {
		return LoadedDatabaseArtifact{}, fmt.Errorf("create database artifact work root: %w", err)
	}
	workDirectory, err := os.MkdirTemp(backupWorkRoot, "artifact-"+scope.Backup.ID.String()+"-")
	if err != nil {
		return LoadedDatabaseArtifact{}, err
	}
	cleanup := func() { _ = os.RemoveAll(workDirectory) }
	fail := func(loadErr error) (LoadedDatabaseArtifact, error) {
		cleanup()
		return LoadedDatabaseArtifact{}, loadErr
	}

	encryptedPath := filepath.Join(workDirectory, "database.tar.age")
	encrypted, err := os.OpenFile(encryptedPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fail(err)
	}
	digest := sha256.New()
	remote, getErr := store.Get(ctx, scope.Backup.ArtifactReference, io.MultiWriter(encrypted, digest))
	closeErr := encrypted.Close()
	if getErr != nil || closeErr != nil {
		return fail(errors.Join(getErr, closeErr))
	}
	if !scope.Backup.SizeBytes.Valid || remote.Size != scope.Backup.SizeBytes.Int64 {
		return fail(errors.New("database backup object size does not match the backup record"))
	}
	if !equalBytes(digest.Sum(nil), scope.Backup.Digest) {
		return fail(errors.New("database backup ciphertext digest does not match the backup record"))
	}
	if scope.Backup.ResourceID == nil || scope.Backup.ResourceInstallationID == nil {
		return fail(errors.New("database backup record is missing topology identity"))
	}
	expectedMetadata := map[string]string{
		"backup-id": scope.Backup.ID.String(), "policy-id": scope.Backup.BackupPolicyID.String(),
		"resource-id":              scope.Backup.ResourceID.String(),
		"resource-installation-id": scope.Backup.ResourceInstallationID.String(),
		"database-name":            target.DatabaseName, "instance-id": service.config.App.InstanceID,
		"sha256": fmt.Sprintf("%x", scope.Backup.Digest), "format-version": scope.Backup.FormatVersion,
	}
	for key, expected := range expectedMetadata {
		if remote.Metadata[key] != expected {
			return fail(fmt.Errorf("database backup object metadata %q does not match the backup record", key))
		}
	}

	identity, err := age.ParseX25519Identity(credential.AgeIdentity)
	if err != nil {
		return fail(errors.New("parse database backup recovery identity"))
	}
	input, err := os.Open(encryptedPath)
	if err != nil {
		return fail(err)
	}
	decrypted, err := age.Decrypt(input, identity)
	if err != nil {
		input.Close()
		return fail(fmt.Errorf("decrypt database backup: %w", err))
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
			return fail(fmt.Errorf("read database backup archive: %w", nextErr))
		}
		if _, ok := required[header.Name]; !ok || required[header.Name] {
			input.Close()
			return fail(fmt.Errorf("database backup contains invalid entry %q", header.Name))
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			input.Close()
			return fail(fmt.Errorf("database backup entry %q is not a regular file", header.Name))
		}
		required[header.Name] = true
		switch header.Name {
		case "database.dump":
			output, openErr := os.OpenFile(dumpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if openErr != nil {
				input.Close()
				return fail(openErr)
			}
			_, copyErr := io.Copy(output, archive)
			outputCloseErr := output.Close()
			if copyErr != nil || outputCloseErr != nil {
				input.Close()
				return fail(errors.Join(copyErr, outputCloseErr))
			}
		case "manifest.json":
			if header.Size > 1024*1024 {
				input.Close()
				return fail(errors.New("database backup manifest is too large"))
			}
			manifestBytes, err = io.ReadAll(archive)
			if err != nil {
				input.Close()
				return fail(fmt.Errorf("read database backup manifest: %w", err))
			}
		}
	}
	if err := input.Close(); err != nil {
		return fail(err)
	}
	for entry, found := range required {
		if !found {
			return fail(fmt.Errorf("database backup archive is missing %s", entry))
		}
	}
	var manifest databaseArtifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fail(fmt.Errorf("decode database backup manifest: %w", err))
	}
	if manifest.ArtifactVersion != scope.Backup.FormatVersion ||
		manifest.InstanceID != service.config.App.InstanceID ||
		manifest.BackupID != scope.Backup.ID.String() ||
		manifest.PolicyID != scope.Backup.BackupPolicyID.String() ||
		manifest.ResourceID != scope.Backup.ResourceID.String() ||
		manifest.ResourceInstallationID != scope.Backup.ResourceInstallationID.String() ||
		manifest.DatabaseName != target.DatabaseName ||
		!manifest.ScheduledAt.Equal(scope.Backup.ScheduledAt) ||
		manifest.ProducerVersion != scope.Backup.ProducerVersion ||
		manifest.Format != "postgresql-custom+globals+tar+age" ||
		manifest.RiverExcluded != target.ExcludeRiverTableData {
		return fail(errors.New("database backup manifest does not match the backup record"))
	}
	return LoadedDatabaseArtifact{DumpPath: dumpPath, cleanup: cleanup}, nil
}
