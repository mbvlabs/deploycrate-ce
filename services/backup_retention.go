package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type BackupRetention struct {
	db       storage.Pool
	config   config.Config
	executor *BackupExecutor
}

func NewBackupRetention(
	db storage.Pool,
	configuration config.Config,
	executor *BackupExecutor,
) *BackupRetention {
	return &BackupRetention{db: db, config: configuration, executor: executor}
}

func (service *BackupRetention) Apply(ctx context.Context, policyID uuid.UUID) error {
	policy, err := models.BackupPolicy.Find(ctx, service.db.Executor(), policyID)
	if err != nil {
		return fmt.Errorf("load backup retention policy: %w", err)
	}
	backups, err := models.Backup.FindVerifiedByPolicy(ctx, service.db.Executor(), policyID)
	if err != nil {
		return fmt.Errorf("load verified backups for retention: %w", err)
	}
	if len(backups) <= 1 {
		return nil
	}
	document, err := policy.RetentionPolicy()
	if err != nil {
		return fmt.Errorf("decode backup retention policy: %w", err)
	}
	prune := models.SelectBackupsToPrune(backups, document)
	if len(prune) == 0 {
		return nil
	}

	scope, err := service.executor.loadScope(ctx, backups[0].ID)
	if err != nil {
		return err
	}
	plaintext, err := secretcrypto.Decrypt(
		scope.CredentialPayload,
		service.config.App.SessionEncryptionKey,
	)
	if err != nil {
		return err
	}
	defer clear(plaintext)
	var credential BackupCredentialPayload
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		return errors.New("decode backup credential for retention")
	}

	if backups[0].TargetType == "server" {
		if backups[0].ServerID == nil {
			return errors.New("server retention is missing its target")
		}
		repository, err := resticRepository(scope, backups[0].ServerID.String())
		if err != nil {
			return err
		}
		arguments := []string{"forget"}
		remotePrune := make([]models.BackupEntity, 0, len(prune))
		for _, candidate := range prune {
			snapshot, found, err := findResticSnapshot(
				ctx,
				resticEnvironment(scope, credential, repository),
				scope.DestinationPathStyle,
				"backup-id:"+candidate.ID.String(),
			)
			if err != nil {
				return fmt.Errorf("inspect expired Restic backup %s: %w", candidate.ID, err)
			}
			if found {
				if snapshot.ID != candidate.ArtifactReference {
					return fmt.Errorf(
						"expired Restic backup %s has conflicting artifact identity",
						candidate.ID,
					)
				}
				arguments = append(arguments, snapshot.ID)
				remotePrune = append(remotePrune, candidate)
			}
		}
		if len(remotePrune) > 0 {
			arguments = append(arguments, "--prune")
			if _, err := runRestic(
				ctx,
				resticEnvironment(scope, credential, repository),
				scope.DestinationPathStyle,
				arguments...,
			); err != nil {
				return fmt.Errorf("prune Restic backups: %w", err)
			}
		}
	} else {
		store, err := objectstorage.New(
			ctx,
			scope.ObjectStorageConfig(),
			objectstorage.Credentials{
				AccessKeyID: credential.AccessKeyID, SecretAccessKey: credential.SecretAccessKey,
			},
		)
		if err != nil {
			return err
		}
		for _, candidate := range prune {
			if err := store.Delete(ctx, candidate.ArtifactReference); err != nil {
				return fmt.Errorf("delete expired database backup %s: %w", candidate.ID, err)
			}
		}
	}
	for _, candidate := range prune {
		if err := models.Backup.MarkPruned(ctx, service.db.Executor(), candidate.ID); err != nil {
			return fmt.Errorf("record pruned backup %s: %w", candidate.ID, err)
		}
	}
	slog.InfoContext(
		ctx,
		"backup retention completed",
		"backup_policy_id", policyID,
		"target_type", backups[0].TargetType,
		"driver", backups[0].Driver,
		"lifecycle_status", models.BackupStatusPruned,
		"pruned_artifacts", len(prune),
	)
	return nil
}
