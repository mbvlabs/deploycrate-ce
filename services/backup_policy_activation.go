package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
)

type BackupPolicyActivator struct {
	db storage.Pool
}

func NewBackupPolicyActivator(db storage.Pool) *BackupPolicyActivator {
	return &BackupPolicyActivator{db: db}
}

func (service *BackupPolicyActivator) Activate(
	ctx context.Context,
	installationID string,
) (int, error) {
	if strings.TrimSpace(installationID) == "" {
		return 0, errors.New("backup installation ID is required")
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	policies, err := models.BackupPolicy.FindInactiveInstallationPoliciesForUpdate(
		ctx,
		tx,
		installationID,
	)
	if err != nil {
		return 0, fmt.Errorf("find inactive backup policies: %w", err)
	}
	activatedAt := time.Now().UTC()
	for index := range policies {
		policy := &policies[index]
		nextRunAt, err := models.NextBackupRun(policy.Schedule, activatedAt)
		if err != nil {
			return 0, fmt.Errorf("calculate first run for backup policy %s: %w", policy.ID, err)
		}
		policy.ActivatedAt = sql.NullTime{Time: activatedAt, Valid: true}
		policy.NextRunAt = nextRunAt
		if err := policy.Validate(); err != nil {
			return 0, fmt.Errorf("validate backup policy %s: %w", policy.ID, err)
		}
		if err := models.BackupPolicy.Activate(
			ctx,
			tx,
			policy.ID,
			activatedAt,
			nextRunAt,
		); err != nil {
			return 0, fmt.Errorf("activate backup policy %s: %w", policy.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit backup policy activation: %w", err)
	}
	return len(policies), nil
}
