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

type ResourceCredentialEntity struct {
	bun.BaseModel          `bun:"table:resource_credentials,alias:resource_credentials"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	Name                   string          `bun:"name"`
	Role                   string          `bun:"role"`
	Username               sql.NullString  `bun:"username"`
	Metadata               json.RawMessage `bun:"metadata,type:jsonb"`
	EncPayload             []byte          `bun:"enc_payload"`
	Digest                 []byte          `bun:"digest" json:"-"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
	ResourceID             uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID      `bun:"resource_installation_id,type:uuid"`
}

func (e *ResourceCredentialEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Role = strings.TrimSpace(e.Role)
	e.Username.String = strings.TrimSpace(e.Username.String)
	e.Username.Valid = e.Username.String != ""
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	builder.Required("role", e.Role)
	if len(e.Metadata) == 0 || !json.Valid(e.Metadata) {
		builder.Add("metadata", "invalid", "metadata must be valid JSON")
	} else if settingsContainSecret(e.Metadata) {
		builder.Add("metadata", "secret", "metadata must not contain raw credentials")
	}
	if len(e.EncPayload) == 0 {
		builder.Add("secretValues", "required", "at least one encrypted credential value is required")
	}
	if len(e.Digest) != 32 {
		builder.Add("secretValues", "digest", "credential digest is invalid")
	}
	if e.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "resource is required")
	}
	return builder.Err()
}

func (rc resourceCredential) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceCredentialEntity, error) {
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
	Name                   string
	Role                   string
	Username               sql.NullString
	Metadata               json.RawMessage
	EncPayload             []byte
	Digest                 []byte
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
}

func (rc resourceCredential) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceCredentialData,
) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		Digest:                 data.Digest,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
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
	Name                   string
	Role                   string
	Username               sql.NullString
	Metadata               json.RawMessage
	EncPayload             []byte
	Digest                 []byte
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
}

func (rc resourceCredential) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceCredentialData,
) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		Digest:                 data.Digest,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("role").
		Column("username").
		Column("metadata").
		Column("enc_payload").
		Column("digest").
		Column("archived_at").
		Column("resource_id").
		Column("resource_installation_id").
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

func (rc resourceCredential) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceCredentialEntity, error) {
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

func (rc resourceCredential) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceCredentials, error) {
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

func (rc resourceCredential) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceCredentialData,
) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Username:               data.Username,
		Metadata:               data.Metadata,
		EncPayload:             data.EncPayload,
		Digest:                 data.Digest,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("role = excluded.role").
		Set("username = excluded.username").
		Set("metadata = excluded.metadata").
		Set("enc_payload = excluded.enc_payload").
		Set("digest = excluded.digest").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}
