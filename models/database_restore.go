package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	DatabaseRestoreStatusPending      = "pending"
	DatabaseRestoreStatusSafetyBackup = "safety_backup"
	DatabaseRestoreStatusRestoring    = "restoring"
	DatabaseRestoreStatusCompleted    = "completed"
	DatabaseRestoreStatusRolledBack   = "rolled_back"
	DatabaseRestoreStatusFailed       = "failed"
)

type DatabaseRestoreEntity struct {
	bun.BaseModel  `bun:"table:database_restores,alias:database_restore"`
	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time      `bun:"created_at"`
	UpdatedAt      time.Time      `bun:"updated_at"`
	Status         string         `bun:"status"`
	RequestedAt    time.Time      `bun:"requested_at"`
	StartedAt      sql.NullTime   `bun:"started_at"`
	FinishedAt     sql.NullTime   `bun:"finished_at"`
	VerifiedAt     sql.NullTime   `bun:"verified_at"`
	CutoverAt      sql.NullTime   `bun:"cutover_at"`
	RolledBackAt   sql.NullTime   `bun:"rolled_back_at"`
	Error          sql.NullString `bun:"error"`
	ChangeID       uuid.UUID      `bun:"change_id,type:uuid"`
	ChangeTaskID   uuid.UUID      `bun:"change_task_id,type:uuid"`
	BackupID       uuid.UUID      `bun:"backup_id,type:uuid"`
	SafetyBackupID *uuid.UUID     `bun:"safety_backup_id,type:uuid"`
	DatabaseID     uuid.UUID      `bun:"database_id,type:uuid"`
}

func (entity *DatabaseRestoreEntity) Validate() error {
	if entity.ID == uuid.Nil || entity.ChangeID == uuid.Nil || entity.ChangeTaskID == uuid.Nil || entity.BackupID == uuid.Nil || entity.DatabaseID == uuid.Nil {
		return errors.New("Database restore identities are required")
	}
	valid := map[string]bool{DatabaseRestoreStatusPending: true, DatabaseRestoreStatusSafetyBackup: true, DatabaseRestoreStatusRestoring: true, DatabaseRestoreStatusCompleted: true, DatabaseRestoreStatusRolledBack: true, DatabaseRestoreStatusFailed: true}
	if !valid[entity.Status] {
		return errors.New("Database restore status is invalid")
	}
	return nil
}

type CreateDatabaseRestoreData struct {
	ID                                           uuid.UUID
	Status                                       string
	RequestedAt                                  time.Time
	ChangeID, ChangeTaskID, BackupID, DatabaseID uuid.UUID
}

func (databaseRestore) Create(ctx context.Context, db storage.Executor, data CreateDatabaseRestoreData) (DatabaseRestoreEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseRestoreEntity{ID: data.ID, CreatedAt: now, UpdatedAt: now, Status: data.Status, RequestedAt: data.RequestedAt, ChangeID: data.ChangeID, ChangeTaskID: data.ChangeTaskID, BackupID: data.BackupID, DatabaseID: data.DatabaseID}
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseRestoreEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(ctx, db, "database-active-restore:"+entity.DatabaseID.String(), db.NewSelect().Model((*DatabaseRestoreEntity)(nil)).Where("database_id = ?", entity.DatabaseID).Where("status IN (?, ?, ?)", DatabaseRestoreStatusPending, DatabaseRestoreStatusSafetyBackup, DatabaseRestoreStatusRestoring), "databaseId", "the Database already has an active restore"); err != nil {
		return DatabaseRestoreEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseRestoreEntity{}, err
	}
	return entity, nil
}

func (databaseRestore) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseRestoreEntity, error) {
	var entity DatabaseRestoreEntity
	if err := db.NewSelect().Model(&entity).Where("database_restore.id = ?", id).Scan(ctx); err != nil {
		return DatabaseRestoreEntity{}, err
	}
	return entity, nil
}
func (databaseRestore) FindForUpdate(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseRestoreEntity, error) {
	var entity DatabaseRestoreEntity
	if err := db.NewSelect().Model(&entity).Where("database_restore.id = ?", id).For("UPDATE").Scan(ctx); err != nil {
		return DatabaseRestoreEntity{}, err
	}
	return entity, nil
}

func (databaseRestore) MarkSafetyBackup(ctx context.Context, db storage.Executor, id, backupID uuid.UUID, at time.Time) error {
	result, err := db.NewUpdate().TableExpr("database_restores").Set("status = ?", DatabaseRestoreStatusSafetyBackup).Set("safety_backup_id = ?", backupID).Set("started_at = COALESCE(started_at, ?)", at).Set("updated_at = ?", at).Where("id = ?", id).Where("status = ?", DatabaseRestoreStatusPending).Exec(ctx)
	return oneDatabaseRestoreTransition(result, err)
}

func (databaseRestore) MarkRestoringBySafetyBackup(ctx context.Context, db storage.Executor, backupID uuid.UUID, at time.Time) (*uuid.UUID, error) {
	var id uuid.UUID
	err := db.NewUpdate().TableExpr("database_restores").Set("status = ?", DatabaseRestoreStatusRestoring).Set("updated_at = ?", at).Where("safety_backup_id = ?", backupID).Where("status = ?", DatabaseRestoreStatusSafetyBackup).Returning("id").Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (databaseRestore) MarkCompleted(ctx context.Context, db storage.Executor, id uuid.UUID, cutoverAt, at time.Time) error {
	result, err := db.NewUpdate().TableExpr("database_restores").Set("status = ?", DatabaseRestoreStatusCompleted).Set("cutover_at = ?", cutoverAt).Set("verified_at = ?", at).Set("finished_at = ?", at).Set("error = NULL").Set("updated_at = ?", at).Where("id = ?", id).Where("status = ?", DatabaseRestoreStatusRestoring).Exec(ctx)
	return oneDatabaseRestoreTransition(result, err)
}

func (databaseRestore) MarkFailed(ctx context.Context, db storage.Executor, id uuid.UUID, operationErr error, rolledBack bool, cutoverAt *time.Time, at time.Time) error {
	status := DatabaseRestoreStatusFailed
	if rolledBack {
		status = DatabaseRestoreStatusRolledBack
	}
	message := strings.TrimSpace(operationErr.Error())
	if len(message) > 2000 {
		message = message[:2000]
	}
	query := db.NewUpdate().TableExpr("database_restores").Set("status = ?", status).Set("finished_at = ?", at).Set("error = ?", message).Set("updated_at = ?", at).Where("id = ?", id).Where("status IN (?, ?, ?)", DatabaseRestoreStatusPending, DatabaseRestoreStatusSafetyBackup, DatabaseRestoreStatusRestoring)
	if rolledBack {
		query = query.Set("rolled_back_at = ?", at)
	}
	if cutoverAt != nil {
		query = query.Set("cutover_at = ?", *cutoverAt)
	}
	_, err := query.Exec(ctx)
	return err
}

type DatabaseRestoreHistory struct {
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

func (databaseRestore) RecentForDatabase(ctx context.Context, db storage.Executor, databaseID uuid.UUID, limit int) ([]DatabaseRestoreHistory, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	items := make([]DatabaseRestoreHistory, 0, limit)
	err := db.NewSelect().TableExpr("database_restores AS restore").ColumnExpr("restore.id, restore.status, restore.requested_at, restore.started_at, restore.finished_at").ColumnExpr("restore.verified_at, restore.cutover_at, restore.rolled_back_at").ColumnExpr("LEFT(COALESCE(restore.error, ''), 800) AS error").ColumnExpr("restore.backup_id, backup.scheduled_at AS backup_scheduled_at, restore.safety_backup_id").Join("JOIN backups AS backup ON backup.id = restore.backup_id").Where("restore.database_id = ?", databaseID).OrderExpr("restore.requested_at DESC").Limit(limit).Scan(ctx, &items)
	return items, err
}

func oneDatabaseRestoreTransition(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return rowsErr
	}
	if rows != 1 {
		return errors.New("Database restore state changed concurrently")
	}
	return nil
}
