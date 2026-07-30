package factories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type ResourceRestoreFactory struct {
	models.ResourceRestoreEntity
}

type ResourceRestoreOption func(*ResourceRestoreFactory)

func BuildResourceRestore(changeID, changeTaskID, backupID, resourceID, targetInstallationID uuid.UUID, opts ...ResourceRestoreOption) models.ResourceRestoreEntity {
	factory := &ResourceRestoreFactory{ResourceRestoreEntity: models.ResourceRestoreEntity{
		Status: models.ResourceRestoreStatusPending, RequestedAt: time.Now().UTC(),
		ChangeID: changeID, ChangeTaskID: changeTaskID, BackupID: backupID,
		ResourceID: resourceID, TargetInstallationID: targetInstallationID,
	}}
	for _, option := range opts {
		option(factory)
	}
	return factory.ResourceRestoreEntity
}

func CreateResourceRestore(ctx context.Context, exec storage.Executor, changeID, changeTaskID, backupID, resourceID, targetInstallationID uuid.UUID, opts ...ResourceRestoreOption) (models.ResourceRestoreEntity, error) {
	entity := BuildResourceRestore(changeID, changeTaskID, backupID, resourceID, targetInstallationID, opts...)
	entity.ID = uuid.New()
	entity.CreatedAt = time.Now().UTC()
	entity.UpdatedAt = entity.CreatedAt
	if err := exec.NewInsert().Model(&entity).Returning("*").Scan(ctx); err != nil {
		return models.ResourceRestoreEntity{}, err
	}
	return entity, nil
}

func CreateResourceRestores(ctx context.Context, exec storage.Executor, changeID, changeTaskID, backupID, resourceID, targetInstallationID uuid.UUID, count int, opts ...ResourceRestoreOption) ([]models.ResourceRestoreEntity, error) {
	entities := make([]models.ResourceRestoreEntity, 0, count)
	for index := range count {
		entity, err := CreateResourceRestore(ctx, exec, changeID, changeTaskID, backupID, resourceID, targetInstallationID, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create Resource restore %d: %w", index+1, err)
		}
		entities = append(entities, entity)
	}
	return entities, nil
}

func WithResourceRestoresStatus(value string) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.Status = value }
}

func WithResourceRestoresRequestedAt(value time.Time) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.RequestedAt = value }
}

func WithResourceRestoresStartedAt(value sql.NullTime) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.StartedAt = value }
}

func WithResourceRestoresFinishedAt(value sql.NullTime) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.FinishedAt = value }
}

func WithResourceRestoresVerifiedAt(value sql.NullTime) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.VerifiedAt = value }
}

func WithResourceRestoresCutoverAt(value sql.NullTime) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.CutoverAt = value }
}

func WithResourceRestoresRolledBackAt(value sql.NullTime) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.RolledBackAt = value }
}

func WithResourceRestoresError(value sql.NullString) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.Error = value }
}

func WithResourceRestoresSafetyBackupID(value *uuid.UUID) ResourceRestoreOption {
	return func(factory *ResourceRestoreFactory) { factory.SafetyBackupID = value }
}
