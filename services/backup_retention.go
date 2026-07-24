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

type retentionDocument struct {
	KeepLast    int `json:"keep_last"`
	KeepDaily   int `json:"keep_daily"`
	KeepWeekly  int `json:"keep_weekly"`
	KeepMonthly int `json:"keep_monthly"`
}

func (service *BackupRetention) Apply(ctx context.Context, policyID uuid.UUID) error {
	var backups []models.BackupEntity
	policy, err := models.BackupPolicy.Find(ctx, service.db.Executor(), policyID)
	if err != nil {
		return fmt.Errorf("load backup retention policy: %w", err)
	}
	if err := service.db.Executor().NewSelect().
		Model(&backups).
		Where("backups.backup_policy_id = ?", policyID).
		Where("backups.status = ?", models.BackupStatusVerified).
		OrderExpr("backups.scheduled_at DESC").
		Scan(ctx, &backups); err != nil {
		return fmt.Errorf("load verified backups for retention: %w", err)
	}
	if len(backups) <= 1 {
		return nil
	}
	var document retentionDocument
	if err := json.Unmarshal(policy.Retention, &document); err != nil {
		return fmt.Errorf("decode backup retention policy: %w", err)
	}
	prune := selectBackupsToPrune(backups, document)
	if len(prune) == 0 {
		return nil
	}

	scope, err := service.executor.loadScope(ctx, backups[0].ID)
	if err != nil {
		return err
	}
	plaintext, err := secretcrypto.Decrypt(scope.CredentialPayload, service.config.App.SessionEncryptionKey)
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
				"backup-id:"+candidate.ID.String(),
			)
			if err != nil {
				return fmt.Errorf("inspect expired Restic backup %s: %w", candidate.ID, err)
			}
			if found {
				if snapshot.ID != candidate.ArtifactReference {
					return fmt.Errorf("expired Restic backup %s has conflicting artifact identity", candidate.ID)
				}
				arguments = append(arguments, snapshot.ID)
				remotePrune = append(remotePrune, candidate)
			}
		}
		if len(remotePrune) > 0 {
			arguments = append(arguments, "--prune")
			if _, err := runRestic(ctx, resticEnvironment(scope, credential, repository), arguments...); err != nil {
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

func selectBackupsToPrune(
	backups []models.BackupEntity,
	document retentionDocument,
) []models.BackupEntity {
	keep := map[uuid.UUID]bool{backups[0].ID: true}
	for index := 0; index < min(document.KeepLast, len(backups)); index++ {
		keep[backups[index].ID] = true
	}
	daily := map[string]bool{}
	weekly := map[string]bool{}
	monthly := map[string]bool{}
	for _, backup := range backups {
		date := backup.ScheduledAt.UTC()
		dayKey := date.Format("2006-01-02")
		year, week := date.ISOWeek()
		weekKey := fmt.Sprintf("%04d-%02d", year, week)
		monthKey := date.Format("2006-01")
		if len(daily) < document.KeepDaily && !daily[dayKey] {
			daily[dayKey] = true
			keep[backup.ID] = true
		}
		if len(weekly) < document.KeepWeekly && !weekly[weekKey] {
			weekly[weekKey] = true
			keep[backup.ID] = true
		}
		if len(monthly) < document.KeepMonthly && !monthly[monthKey] {
			monthly[monthKey] = true
			keep[backup.ID] = true
		}
	}
	prune := make([]models.BackupEntity, 0, len(backups))
	for _, backup := range backups[1:] {
		if !keep[backup.ID] {
			prune = append(prune, backup)
		}
	}
	return prune
}
