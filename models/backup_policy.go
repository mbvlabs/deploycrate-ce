package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type BackupPolicyEntity struct {
	bun.BaseModel       `bun:"table:backup_policies,alias:backup_policies"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt           time.Time       `bun:"created_at"`
	UpdatedAt           time.Time       `bun:"updated_at"`
	ResourceID          uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceBindingID   *uuid.UUID      `bun:"resource_binding_id,type:uuid"`
	BackupDestinationID uuid.UUID       `bun:"backup_destination_id,type:uuid"`
	Name                string          `bun:"name"`
	Schedule            string          `bun:"schedule"`
	Retention           json.RawMessage `bun:"retention,type:jsonb"`
	Format              string          `bun:"format"`
	Verification        json.RawMessage `bun:"verification,type:jsonb"`
	Settings            json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt          sql.NullTime    `bun:"archived_at"`
}

func (e *BackupPolicyEntity) Validate() error {
	return nil
}

func (bp backupPolicy) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (BackupPolicyEntity, error) {
	var entity BackupPolicyEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

type CreateBackupPolicyData struct {
	ResourceID          uuid.UUID
	ResourceBindingID   *uuid.UUID
	BackupDestinationID uuid.UUID
	Name                string
	Schedule            string
	Retention           json.RawMessage
	Format              string
	Verification        json.RawMessage
	Settings            json.RawMessage
	ArchivedAt          sql.NullTime
}

func (bp backupPolicy) Create(ctx context.Context, db storage.Executor, data CreateBackupPolicyData) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ResourceID:          data.ResourceID,
		ResourceBindingID:   data.ResourceBindingID,
		BackupDestinationID: data.BackupDestinationID,
		Name:                data.Name,
		Schedule:            data.Schedule,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

type UpdateBackupPolicyData struct {
	ID                  uuid.UUID
	UpdatedAt           time.Time
	ResourceID          uuid.UUID
	ResourceBindingID   *uuid.UUID
	BackupDestinationID uuid.UUID
	Name                string
	Schedule            string
	Retention           json.RawMessage
	Format              string
	Verification        json.RawMessage
	Settings            json.RawMessage
	ArchivedAt          sql.NullTime
}

func (bp backupPolicy) Update(ctx context.Context, db storage.Executor, data UpdateBackupPolicyData) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  data.ID,
		UpdatedAt:           time.Now(),
		ResourceID:          data.ResourceID,
		ResourceBindingID:   data.ResourceBindingID,
		BackupDestinationID: data.BackupDestinationID,
		Name:                data.Name,
		Schedule:            data.Schedule,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_id").
		Column("resource_binding_id").
		Column("backup_destination_id").
		Column("name").
		Column("schedule").
		Column("retention").
		Column("format").
		Column("verification").
		Column("settings").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}

func (bp backupPolicy) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*BackupPolicyEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (bp backupPolicy) All(ctx context.Context, db storage.Executor) ([]BackupPolicyEntity, error) {
	var entities []BackupPolicyEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedBackupPolicies struct {
	BackupPolicies []BackupPolicyEntity
	TotalCount     int64
	Page           int64
	PageSize       int64
	TotalPages     int64
}

func (bp backupPolicy) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedBackupPolicies, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	totalCount, err := db.NewSelect().
		Model(&BackupPolicyEntity{}).Count(ctx)
	if err != nil {
		return PaginatedBackupPolicies{}, err
	}

	entities := make([]BackupPolicyEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedBackupPolicies{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedBackupPolicies{
		BackupPolicies: entities,
		TotalCount:     int64(totalCount),
		Page:           page,
		PageSize:       pageSize,
		TotalPages:     totalPages,
	}, nil
}

func (bp backupPolicy) Upsert(ctx context.Context, db storage.Executor, data CreateBackupPolicyData) (BackupPolicyEntity, error) {
	entity := BackupPolicyEntity{
		ID:                  uuid.New(),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		ResourceID:          data.ResourceID,
		ResourceBindingID:   data.ResourceBindingID,
		BackupDestinationID: data.BackupDestinationID,
		Name:                data.Name,
		Schedule:            data.Schedule,
		Retention:           data.Retention,
		Format:              data.Format,
		Verification:        data.Verification,
		Settings:            data.Settings,
		ArchivedAt:          data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupPolicyEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_id = excluded.resource_id").
		Set("resource_binding_id = excluded.resource_binding_id").
		Set("backup_destination_id = excluded.backup_destination_id").
		Set("name = excluded.name").
		Set("schedule = excluded.schedule").
		Set("retention = excluded.retention").
		Set("format = excluded.format").
		Set("verification = excluded.verification").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return BackupPolicyEntity{}, err
	}

	return entity, nil
}
