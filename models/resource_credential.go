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

type ResourceCredentialEntity struct {
	bun.BaseModel          `bun:"table:resource_credentials,alias:resource_credentials"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	ResourceID             uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID      `bun:"resource_installation_id,type:uuid"`
	Name                   string          `bun:"name"`
	Role                   string          `bun:"role"`
	Username               sql.NullString  `bun:"username"`
	Metadata               json.RawMessage `bun:"metadata,type:jsonb"`
	EncPayload             []byte          `bun:"enc_payload"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
}

func (e *ResourceCredentialEntity) Validate() error {
	return nil
}

func (rc resourceCredential) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceCredentialEntity, error) {
	var entity ResourceCredentialEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}

type CreateResourceCredentialData struct {
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	Name                   string
	Role                   string
	Username               sql.NullString
	Metadata               json.RawMessage
	EncPayload             []byte
	ArchivedAt             sql.NullTime
}

func (rc resourceCredential) Create(ctx context.Context, db storage.Executor, data CreateResourceCredentialData) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		ArchivedAt:             data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}

type UpdateResourceCredentialData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	Name                   string
	Role                   string
	Username               sql.NullString
	Metadata               json.RawMessage
	EncPayload             []byte
	ArchivedAt             sql.NullTime
}

func (rc resourceCredential) Update(ctx context.Context, db storage.Executor, data UpdateResourceCredentialData) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		ArchivedAt:             data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("resource_id").
		Column("resource_installation_id").
		Column("name").
		Column("role").
		Column("username").
		Column("metadata").
		Column("enc_payload").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}

func (rc resourceCredential) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceCredentialEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rc resourceCredential) All(ctx context.Context, db storage.Executor) ([]ResourceCredentialEntity, error) {
	var entities []ResourceCredentialEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceCredentials struct {
	ResourceCredentials []ResourceCredentialEntity
	TotalCount          int64
	Page                int64
	PageSize            int64
	TotalPages          int64
}

func (rc resourceCredential) Paginate(ctx context.Context, db storage.Executor, page, pageSize int64) (PaginatedResourceCredentials, error) {
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
		Model(&ResourceCredentialEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceCredentials{}, err
	}

	entities := make([]ResourceCredentialEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceCredentials{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceCredentials{
		ResourceCredentials: entities,
		TotalCount:          int64(totalCount),
		Page:                page,
		PageSize:            pageSize,
		TotalPages:          totalPages,
	}, nil
}

func (rc resourceCredential) Upsert(ctx context.Context, db storage.Executor, data CreateResourceCredentialData) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		ArchivedAt:             data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("resource_id = excluded.resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("name = excluded.name").
		Set("role = excluded.role").
		Set("username = excluded.username").
		Set("metadata = excluded.metadata").
		Set("enc_payload = excluded.enc_payload").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}
