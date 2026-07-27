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

type CredentialEntity struct {
	bun.BaseModel `bun:"table:credentials,alias:credentials"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	Name          string          `bun:"name"`
	Provider      string          `bun:"provider"`
	Metadata      json.RawMessage `bun:"metadata,type:jsonb"`
	EncPayload    []byte          `bun:"enc_payload"`
	VerifiedAt    sql.NullTime    `bun:"verified_at"`
	LastUsedAt    sql.NullTime    `bun:"last_used_at"`
	ArchivedAt    sql.NullTime    `bun:"archived_at"`
}

func (e *CredentialEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil {
		builder.Add("id", "required", "credential ID is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		builder.Add("name", "required", "credential name is required")
	}
	if strings.TrimSpace(e.Provider) == "" {
		builder.Add("provider", "required", "credential provider is required")
	}
	if len(e.Metadata) == 0 || !json.Valid(e.Metadata) {
		builder.Add("metadata", "invalid", "credential metadata must be valid JSON")
	}
	if len(e.EncPayload) < 2 {
		builder.Add("enc_payload", "required", "encrypted credential payload is required")
	}
	if strings.HasPrefix(e.Provider, "backup_") {
		if e.Provider != "backup_s3" && e.Provider != "backup_r2" {
			builder.Add("provider", "unsupported", "backup credential provider must be backup_s3 or backup_r2")
		}
		var metadata struct {
			InstanceID string `json:"instance_id"`
			Provider   string `json:"provider"`
			Endpoint   string `json:"endpoint"`
			Region     string `json:"region"`
			Bucket     string `json:"bucket"`
		}
		if json.Unmarshal(e.Metadata, &metadata) != nil ||
			strings.TrimSpace(metadata.InstanceID) == "" ||
			metadata.Provider != strings.TrimPrefix(e.Provider, "backup_") ||
			strings.TrimSpace(metadata.Region) == "" || strings.TrimSpace(metadata.Bucket) == "" ||
			(metadata.Provider == "r2" && strings.TrimSpace(metadata.Endpoint) == "") {
			builder.Add("metadata", "invalid", "backup credential metadata is incomplete or incompatible")
		}
	}
	if e.Provider == "github_app" {
		var metadata struct {
			SchemaVersion  int    `json:"schema_version"`
			InstanceID     string `json:"instance_id"`
			CredentialKind string `json:"credential_kind"`
		}
		unmarshalErr := json.Unmarshal(e.Metadata, &metadata)
		instanceID, parseErr := uuid.Parse(metadata.InstanceID)
		if unmarshalErr != nil ||
			metadata.SchemaVersion != 1 || instanceID == uuid.Nil || parseErr != nil ||
			metadata.CredentialKind != "github_app" {
			builder.Add("metadata", "invalid", "GitHub App credential metadata is incomplete or incompatible")
		}
	}
	if e.ArchivedAt.Valid && e.VerifiedAt.Valid && e.VerifiedAt.Time.After(e.ArchivedAt.Time) {
		builder.Add("verified_at", "invalid", "archived credentials cannot be verified after archival")
	}
	if e.ArchivedAt.Valid && e.LastUsedAt.Valid && e.LastUsedAt.Time.After(e.ArchivedAt.Time) {
		builder.Add("last_used_at", "invalid", "archived credentials cannot be used after archival")
	}
	return builder.Err()
}

func (c credential) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (CredentialEntity, error) {
	var entity CredentialEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return CredentialEntity{}, err
	}

	return entity, nil
}

type CreateCredentialData struct {
	Name       string
	Provider   string
	Metadata   json.RawMessage
	EncPayload []byte
	VerifiedAt sql.NullTime
	LastUsedAt sql.NullTime
	ArchivedAt sql.NullTime
}

func (c credential) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateCredentialData,
) (CredentialEntity, error) {
	entity := CredentialEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Provider:   data.Provider,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		VerifiedAt: data.VerifiedAt,
		LastUsedAt: data.LastUsedAt,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return CredentialEntity{}, err
	}

	return entity, nil
}

type UpdateCredentialData struct {
	ID         uuid.UUID
	UpdatedAt  time.Time
	Name       string
	Provider   string
	Metadata   json.RawMessage
	EncPayload []byte
	VerifiedAt sql.NullTime
	LastUsedAt sql.NullTime
	ArchivedAt sql.NullTime
}

func (c credential) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateCredentialData,
) (CredentialEntity, error) {
	entity := CredentialEntity{
		ID:         data.ID,
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Provider:   data.Provider,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		VerifiedAt: data.VerifiedAt,
		LastUsedAt: data.LastUsedAt,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("provider").
		Column("metadata").
		Column("enc_payload").
		Column("verified_at").
		Column("last_used_at").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return CredentialEntity{}, err
	}

	return entity, nil
}

func (c credential) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*CredentialEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (c credential) All(ctx context.Context, db storage.Executor) ([]CredentialEntity, error) {
	var entities []CredentialEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedCredentials struct {
	Credentials []CredentialEntity
	TotalCount  int64
	Page        int64
	PageSize    int64
	TotalPages  int64
}

func (c credential) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedCredentials, error) {
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
		Model(&CredentialEntity{}).Count(ctx)
	if err != nil {
		return PaginatedCredentials{}, err
	}

	entities := make([]CredentialEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedCredentials{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedCredentials{
		Credentials: entities,
		TotalCount:  int64(totalCount),
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	}, nil
}

func (c credential) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateCredentialData,
) (CredentialEntity, error) {
	entity := CredentialEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Provider:   data.Provider,
		Metadata:   data.Metadata,
		EncPayload: data.EncPayload,
		VerifiedAt: data.VerifiedAt,
		LastUsedAt: data.LastUsedAt,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return CredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("provider = excluded.provider").
		Set("metadata = excluded.metadata").
		Set("enc_payload = excluded.enc_payload").
		Set("verified_at = excluded.verified_at").
		Set("last_used_at = excluded.last_used_at").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return CredentialEntity{}, err
	}

	return entity, nil
}
