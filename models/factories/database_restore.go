package factories

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"time"

	"github.com/google/uuid"
)

func BuildDatabaseRestore(changeID, changeTaskID, backupID, databaseID uuid.UUID) models.DatabaseRestoreEntity {
	return models.DatabaseRestoreEntity{ID: uuid.New(), Status: models.DatabaseRestoreStatusPending, RequestedAt: time.Now().UTC(), ChangeID: changeID, ChangeTaskID: changeTaskID, BackupID: backupID, DatabaseID: databaseID}
}

func CreateDatabaseRestore(ctx context.Context, exec storage.Executor, changeID, changeTaskID, backupID, databaseID uuid.UUID) (models.DatabaseRestoreEntity, error) {
	built := BuildDatabaseRestore(changeID, changeTaskID, backupID, databaseID)
	return models.DatabaseRestore.Create(ctx, exec, models.CreateDatabaseRestoreData{ID: built.ID, Status: built.Status, RequestedAt: built.RequestedAt, ChangeID: built.ChangeID, ChangeTaskID: built.ChangeTaskID, BackupID: built.BackupID, DatabaseID: built.DatabaseID})
}
