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
	bun.BaseModel `bun:"table:resource_credentials,alias:resource_credentials"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	Name          string          `bun:"name"`
	Username      sql.NullString  `bun:"username"`
	Metadata      json.RawMessage `bun:"metadata,type:jsonb"`
	EncPayload    []byte          `bun:"enc_payload"`
	Digest        []byte          `bun:"digest"`
	ArchivedAt    sql.NullTime    `bun:"archived_at"`
	ResourceID    uuid.UUID       `bun:"resource_id,type:uuid"`
}

func (e *ResourceCredentialEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Username.String = strings.TrimSpace(e.Username.String)
	e.Username.Valid = e.Username.String != ""
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	if len(e.Metadata) == 0 || !json.Valid(e.Metadata) {
		builder.Add("metadata", "invalid", "metadata must be valid JSON")
	} else if settingsContainSecret(e.Metadata) {
		builder.Add("metadata", "secret", "metadata must not contain raw credentials")
	}
	if len(e.EncPayload) == 0 {
		builder.Add(
			"secretValues",
			"required",
			"at least one encrypted credential value is required",
		)
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

func (rc resourceCredential) FindActiveForResource(
	ctx context.Context,
	db storage.Executor,
	resourceID, credentialID uuid.UUID,
	systemManaged bool,
) (ResourceCredentialEntity, error) {
	var entity ResourceCredentialEntity
	err := db.NewSelect().
		Model(&entity).
		Join("JOIN resources AS resource ON resource.id = resource_credentials.resource_id AND resource.archived_at IS NULL").
		Where("resource_credentials.id = ?", credentialID).
		Where("resource_credentials.resource_id = ?", resourceID).
		Where("resource_credentials.archived_at IS NULL").
		Where("resource.system_managed = ?", systemManaged).
		Limit(1).
		Scan(ctx)
	return entity, err
}

func (resourceCredential) ActiveAdministratorCount(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (int, error) {
	return db.NewSelect().
		TableExpr("resource_credentials").
		Where("resource_id = ?", resourceID).
		Where("metadata ->> 'purpose' = 'administrator'").
		Where("archived_at IS NULL").
		Count(ctx)
}

func (resourceCredential) LockActiveApplicationsForDatabase(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	databaseName string,
) ([]ResourceCredentialEntity, error) {
	credentials := make([]ResourceCredentialEntity, 0)
	err := db.NewSelect().
		Model(&credentials).
		Where("resource_id = ?", resourceID).
		Where("metadata ->> 'purpose' = 'application'").
		Where("lower(metadata ->> 'database') = lower(?)", strings.TrimSpace(databaseName)).
		Where("archived_at IS NULL").
		OrderExpr("created_at, id").
		For("UPDATE").
		Scan(ctx)
	return credentials, err
}

func (resourceCredential) FindActiveApplicationForDatabase(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	databaseName string,
) (ResourceCredentialEntity, error) {
	var credential ResourceCredentialEntity
	err := db.NewSelect().
		Model(&credential).
		Where("resource_id = ?", resourceID).
		Where("metadata ->> 'purpose' = 'application'").
		Where("lower(metadata ->> 'database') = lower(?)", strings.TrimSpace(databaseName)).
		Where("archived_at IS NULL").
		OrderExpr("created_at, id").
		Limit(1).
		Scan(ctx)
	return credential, err
}

type CreateResourceCredentialData struct {
	Name       string
	Username   sql.NullString
	Metadata   json.RawMessage
	EncPayload []byte
	Digest     []byte
	ArchivedAt sql.NullTime
	ResourceID uuid.UUID
}

func (rc resourceCredential) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceCredentialData,
) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Username:   data.Username,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		Digest:     data.Digest,
		ArchivedAt: data.ArchivedAt,
		ResourceID: data.ResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"resource-credential:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name),
		entity.ID,
		db.NewSelect().
			Model((*ResourceCredentialEntity)(nil)).
			Where("resource_id = ?", entity.ResourceID).
			Where("lower(name) = ?", strings.ToLower(entity.Name)),
		"name",
		"an active credential already uses this name",
	); err != nil {
		return ResourceCredentialEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}

type UpdateResourceCredentialData struct {
	ID         uuid.UUID
	UpdatedAt  time.Time
	Name       string
	Username   sql.NullString
	Metadata   json.RawMessage
	EncPayload []byte
	Digest     []byte
	ArchivedAt sql.NullTime
	ResourceID uuid.UUID
}

func (rc resourceCredential) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceCredentialData,
) (ResourceCredentialEntity, error) {
	entity := ResourceCredentialEntity{
		ID:         data.ID,
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Username:   data.Username,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		Digest:     data.Digest,
		ArchivedAt: data.ArchivedAt,
		ResourceID: data.ResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"resource-credential:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name),
		entity.ID,
		db.NewSelect().
			Model((*ResourceCredentialEntity)(nil)).
			Where("resource_id = ?", entity.ResourceID).
			Where("lower(name) = ?", strings.ToLower(entity.Name)),
		"name",
		"an active credential already uses this name",
	); err != nil {
		return ResourceCredentialEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("username").
		Column("metadata").
		Column("enc_payload").
		Column("digest").
		Column("archived_at").
		Column("resource_id").
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
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Username:   data.Username,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		Digest:     data.Digest,
		ArchivedAt: data.ArchivedAt,
		ResourceID: data.ResourceID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"resource-credential:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name),
		entity.ID,
		db.NewSelect().
			Model((*ResourceCredentialEntity)(nil)).
			Where("resource_id = ?", entity.ResourceID).
			Where("lower(name) = ?", strings.ToLower(entity.Name)),
		"name",
		"an active credential already uses this name",
	); err != nil {
		return ResourceCredentialEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("username = excluded.username").
		Set("metadata = excluded.metadata").
		Set("enc_payload = excluded.enc_payload").
		Set("digest = excluded.digest").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceCredentialEntity{}, err
	}

	return entity, nil
}
