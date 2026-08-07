package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	ResourceRestoreStatusPending      = "pending"
	ResourceRestoreStatusSafetyBackup = "safety_backup"
	ResourceRestoreStatusRestoring    = "restoring"
	ResourceRestoreStatusCompleted    = "completed"
	ResourceRestoreStatusRolledBack   = "rolled_back"
	ResourceRestoreStatusFailed       = "failed"
)

type ResourceRestoreEntity struct {
	bun.BaseModel  `bun:"table:resource_restores,alias:resource_restore"`
	ID             uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time       `bun:"created_at"`
	UpdatedAt      time.Time       `bun:"updated_at"`
	Status         string          `bun:"status"`
	RequestedAt    time.Time       `bun:"requested_at"`
	StartedAt      sql.NullTime    `bun:"started_at"`
	FinishedAt     sql.NullTime    `bun:"finished_at"`
	VerifiedAt     sql.NullTime    `bun:"verified_at"`
	CutoverAt      sql.NullTime    `bun:"cutover_at"`
	RolledBackAt   sql.NullTime    `bun:"rolled_back_at"`
	Error          sql.NullString  `bun:"error"`
	Target         json.RawMessage `bun:"target,type:jsonb"`
	ChangeID       uuid.UUID       `bun:"change_id,type:uuid"`
	ChangeTaskID   uuid.UUID       `bun:"change_task_id,type:uuid"`
	BackupID       uuid.UUID       `bun:"backup_id,type:uuid"`
	SafetyBackupID *uuid.UUID      `bun:"safety_backup_id,type:uuid"`
	ResourceID     uuid.UUID       `bun:"resource_id,type:uuid"`
}

func (entity *ResourceRestoreEntity) Validate() error {
	if entity.ID == uuid.Nil || entity.ChangeID == uuid.Nil || entity.ChangeTaskID == uuid.Nil ||
		entity.BackupID == uuid.Nil ||
		entity.ResourceID == uuid.Nil {
		return errors.New("Resource restore identities are required")
	}
	valid := map[string]bool{
		ResourceRestoreStatusPending:      true,
		ResourceRestoreStatusSafetyBackup: true,
		ResourceRestoreStatusRestoring:    true,
		ResourceRestoreStatusCompleted:    true,
		ResourceRestoreStatusRolledBack:   true,
		ResourceRestoreStatusFailed:       true,
	}
	if !valid[entity.Status] {
		return errors.New("Resource restore status is invalid")
	}
	if !validJSONObject(entity.Target) {
		return errors.New("Resource restore target is invalid")
	}
	return nil
}

type CreateResourceRestoreData struct {
	ID                               uuid.UUID
	Status                           string
	RequestedAt                      time.Time
	Target                           json.RawMessage
	ChangeID, ChangeTaskID, BackupID uuid.UUID
	ResourceID                       uuid.UUID
}

func (resourceRestore) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceRestoreData,
) (ResourceRestoreEntity, error) {
	now := time.Now().UTC()
	entity := ResourceRestoreEntity{
		ID:           data.ID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Status:       data.Status,
		RequestedAt:  data.RequestedAt,
		Target:       data.Target,
		ChangeID:     data.ChangeID,
		ChangeTaskID: data.ChangeTaskID,
		BackupID:     data.BackupID,
		ResourceID:   data.ResourceID,
	}
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}
	if err := validation.Validate(&entity); err != nil {
		return ResourceRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(
		ctx,
		db,
		"resource-active-restore:"+entity.ResourceID.String()+":"+string(entity.Target),
		db.NewSelect().
			Model((*ResourceRestoreEntity)(nil)).
			Where("resource_id = ?", entity.ResourceID).
			Where("target = ?", entity.Target).
			Where("status IN (?, ?, ?)", ResourceRestoreStatusPending, ResourceRestoreStatusSafetyBackup, ResourceRestoreStatusRestoring),
		"target",
		"the Resource target already has an active restore",
	); err != nil {
		return ResourceRestoreEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}
	return entity, nil
}

func (resourceRestore) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceRestoreEntity, error) {
	var entity ResourceRestoreEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("resource_restore.id = ?", id).
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}
	return entity, nil
}

func (resourceRestore) FindForUpdate(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceRestoreEntity, error) {
	var entity ResourceRestoreEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("resource_restore.id = ?", id).
		For("UPDATE").
		Scan(ctx); err != nil {
		return ResourceRestoreEntity{}, err
	}
	return entity, nil
}

func (resourceRestore) MarkSafetyBackup(
	ctx context.Context,
	db storage.Executor,
	id, backupID uuid.UUID,
	at time.Time,
) error {
	result, err := db.NewUpdate().
		TableExpr("resource_restores").
		Set("status = ?", ResourceRestoreStatusSafetyBackup).
		Set("safety_backup_id = ?", backupID).
		Set("started_at = COALESCE(started_at, ?)", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = ?", ResourceRestoreStatusPending).
		Exec(ctx)
	return oneResourceRestoreTransition(result, err)
}

func (resourceRestore) MarkRestoringBySafetyBackup(
	ctx context.Context,
	db storage.Executor,
	backupID uuid.UUID,
	at time.Time,
) (*uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewUpdate().
		TableExpr("resource_restores").
		Set("status = ?", ResourceRestoreStatusRestoring).
		Set("updated_at = ?", at).
		Where("safety_backup_id = ?", backupID).
		Where("status = ?", ResourceRestoreStatusSafetyBackup).
		Returning("id").
		Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (resourceRestore) MarkCompleted(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	cutoverAt, at time.Time,
) error {
	result, err := db.NewUpdate().
		TableExpr("resource_restores").
		Set("status = ?", ResourceRestoreStatusCompleted).
		Set("cutover_at = ?", cutoverAt).
		Set("verified_at = ?", at).
		Set("finished_at = ?", at).
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = ?", ResourceRestoreStatusRestoring).
		Exec(ctx)
	return oneResourceRestoreTransition(result, err)
}

func (resourceRestore) MarkFailed(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	operationErr error,
	rolledBack bool,
	cutoverAt *time.Time,
	at time.Time,
) error {
	status := ResourceRestoreStatusFailed
	if rolledBack {
		status = ResourceRestoreStatusRolledBack
	}
	message := strings.TrimSpace(operationErr.Error())
	if len(message) > 2000 {
		message = message[:2000]
	}
	query := db.NewUpdate().
		TableExpr("resource_restores").
		Set("status = ?", status).
		Set("finished_at = ?", at).
		Set("error = ?", message).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN (?, ?, ?)", ResourceRestoreStatusPending, ResourceRestoreStatusSafetyBackup, ResourceRestoreStatusRestoring)
	if rolledBack {
		query = query.Set("rolled_back_at = ?", at)
	}
	if cutoverAt != nil {
		query = query.Set("cutover_at = ?", *cutoverAt)
	}
	_, err := query.Exec(ctx)
	return err
}

type ResourceRestoreHistory struct {
	ID                uuid.UUID  `bun:"id"`
	Status            string     `bun:"status"`
	RequestedAt       time.Time  `bun:"requested_at"`
	StartedAt         *time.Time `bun:"started_at"`
	FinishedAt        *time.Time `bun:"finished_at"`
	VerifiedAt        *time.Time `bun:"verified_at"`
	CutoverAt         *time.Time `bun:"cutover_at"`
	RolledBackAt      *time.Time `bun:"rolled_back_at"`
	Error             string     `bun:"error"`
	BackupID          uuid.UUID  `bun:"backup_id"`
	BackupScheduledAt time.Time  `bun:"backup_scheduled_at"`
	SafetyBackupID    *uuid.UUID `bun:"safety_backup_id"`
}

func (resourceRestore) RecentForResourceDatabase(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	databaseName string,
	limit int,
) ([]ResourceRestoreHistory, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	items := make([]ResourceRestoreHistory, 0, limit)
	err := db.NewSelect().
		TableExpr("resource_restores AS restore").
		ColumnExpr("restore.id, restore.status, restore.requested_at, restore.started_at, restore.finished_at").
		ColumnExpr("restore.verified_at, restore.cutover_at, restore.rolled_back_at").
		ColumnExpr("LEFT(COALESCE(restore.error, ''), 800) AS error").
		ColumnExpr("restore.backup_id, backup.scheduled_at AS backup_scheduled_at, restore.safety_backup_id").
		Join("JOIN backups AS backup ON backup.id = restore.backup_id").
		Where("restore.resource_id = ?", resourceID).
		Where("restore.target ->> 'database' = ?", databaseName).
		OrderExpr("restore.requested_at DESC").
		Limit(limit).
		Scan(ctx, &items)
	return items, err
}

func oneResourceRestoreTransition(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return rowsErr
	}
	if rows != 1 {
		return errors.New("Resource restore state changed concurrently")
	}
	return nil
}
