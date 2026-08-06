package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	EnvironmentSecretSourceUser     = "user"
	EnvironmentSecretSourceResource = "environment_resource"
	EnvironmentSecretKeyMaxBytes    = 128
	EnvironmentSecretValueMaxBytes  = 64 * 1024
)

var environmentSecretKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

var environmentSecretReservedKeys = map[string]struct{}{
	"PORT": {},
}

type EnvironmentSecretEntity struct {
	bun.BaseModel `bun:"table:environment_secrets,alias:environment_secrets"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Key           string       `bun:"key"`
	EncValue      []byte       `bun:"enc_value"`
	Digest        []byte       `bun:"digest"`
	SourceType    string       `bun:"source_type"`
	SourceID      uuid.UUID    `bun:"source_id,type:uuid"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
}

type EnvironmentSecretMetadata struct {
	ID           uuid.UUID  `json:"id"`
	Key          string     `json:"key"`
	DigestPrefix string     `json:"digestPrefix"`
	SourceType   string     `json:"sourceType"`
	SourceID     uuid.UUID  `json:"sourceId"`
	CreatedAt    time.Time  `json:"createdAt"`
	ArchivedAt   *time.Time `json:"archivedAt,omitempty"`
}

func NormalizeEnvironmentSecretKey(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func ValidateEnvironmentSecretKey(value string, allowReserved bool) error {
	key := NormalizeEnvironmentSecretKey(value)
	builder := validation.NewBuilder()
	builder.Required("key", key)
	if len(key) > EnvironmentSecretKeyMaxBytes {
		builder.Add("key", "max_length", "secret key must not exceed 128 bytes")
	}
	if key != "" && !environmentSecretKeyPattern.MatchString(key) {
		builder.Add("key", "format", "secret key must match [A-Z_][A-Z0-9_]*")
	}
	if _, reserved := environmentSecretReservedKeys[key]; reserved && !allowReserved {
		builder.Add("key", "reserved", "secret key is reserved by the platform")
	}
	return builder.Err()
}

func (e *EnvironmentSecretEntity) Validate() error {
	e.Key = NormalizeEnvironmentSecretKey(e.Key)
	builder := validation.NewBuilder()
	if err := ValidateEnvironmentSecretKey(e.Key, false); err != nil {
		if validationErrors, ok := validation.As(err); ok {
			for _, validationErr := range validationErrors {
				builder.Add(validationErr.Field, validationErr.Code, validationErr.Message)
			}
		}
	}
	if len(e.EncValue) == 0 {
		builder.Add("value", "required", "encrypted secret value is required")
	}
	if len(e.Digest) != 32 {
		builder.Add("value", "digest", "secret digest must be 32 bytes")
	}
	if e.SourceType != EnvironmentSecretSourceUser &&
		e.SourceType != EnvironmentSecretSourceResource {
		builder.Add("sourceType", "unsupported", "secret source type is unsupported")
	}
	if e.SourceID == uuid.Nil {
		builder.Add("sourceId", "required", "secret owner is required")
	}
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environmentId", "required", "Environment is required")
	}
	return builder.Err()
}

func (e EnvironmentSecretEntity) Sanitized() EnvironmentSecretMetadata {
	metadata := EnvironmentSecretMetadata{
		ID: e.ID, Key: e.Key, SourceType: e.SourceType,
		SourceID: e.SourceID, CreatedAt: e.CreatedAt,
	}

	if len(e.Digest) >= 8 {
		const hexDigits = "0123456789abcdef"
		prefix := make([]byte, 16)
		for index, value := range e.Digest[:8] {
			prefix[index*2] = hexDigits[value>>4]
			prefix[index*2+1] = hexDigits[value&0x0f]
		}
		metadata.DigestPrefix = "hmac-sha256:" + string(prefix)
	}

	if e.ArchivedAt.Valid {
		archivedAt := e.ArchivedAt.Time
		metadata.ArchivedAt = &archivedAt
	}

	return metadata
}

func (es environmentSecret) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentSecretEntity, error) {
	var entity EnvironmentSecretEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}
	return entity, nil
}

func (es environmentSecret) FindForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID, id uuid.UUID,
) (EnvironmentSecretEntity, error) {
	var entity EnvironmentSecretEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Where("environment_id = ?", environmentID).
		Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}
	return entity, nil
}

func (es environmentSecret) ActiveByKey(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
	key string,
) (EnvironmentSecretEntity, error) {
	var entity EnvironmentSecretEntity
	if err := db.NewSelect().Model(&entity).
		Where("environment_id = ?", environmentID).
		Where("lower(key) = ?", strings.ToLower(NormalizeEnvironmentSecretKey(key))).
		Where("archived_at IS NULL").
		Scan(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}
	return entity, nil
}

func (es environmentSecret) ActiveForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) ([]EnvironmentSecretEntity, error) {
	entities := make([]EnvironmentSecretEntity, 0)
	if err := db.NewSelect().Model(&entities).
		Where("environment_id = ?", environmentID).
		Where("archived_at IS NULL").
		OrderExpr("key ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return entities, nil
}

type CreateEnvironmentSecretData struct {
	Key           string
	EncValue      []byte
	Digest        []byte
	SourceType    string
	SourceID      uuid.UUID
	ArchivedAt    sql.NullTime
	EnvironmentID uuid.UUID
}

func (es environmentSecret) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentSecretData,
) (EnvironmentSecretEntity, error) {
	entity := EnvironmentSecretEntity{
		ID: uuid.New(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Key: data.Key, EncValue: data.EncValue,
		Digest: data.Digest, SourceType: data.SourceType, SourceID: data.SourceID,
		ArchivedAt: data.ArchivedAt, EnvironmentID: data.EnvironmentID,
	}
	if err := validation.Validate(&entity); err != nil {
		return EnvironmentSecretEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"environment-secret:"+entity.EnvironmentID.String()+":"+entity.Key,
		entity.ID,
		db.NewSelect().
			Model((*EnvironmentSecretEntity)(nil)).
			Where("environment_id = ?", entity.EnvironmentID).
			Where("lower(key) = ?", strings.ToLower(entity.Key)),
		"key",
		"an active secret already uses this key",
	); err != nil {
		return EnvironmentSecretEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentSecretEntity{}, err
	}
	return entity, nil
}

func (es environmentSecret) Archive(
	ctx context.Context,
	db storage.Executor,
	environmentID, id uuid.UUID,
) error {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*EnvironmentSecretEntity)(nil)).
		Set("archived_at = ?", now).Set("updated_at = ?", now).
		Where("id = ?", id).Where("environment_id = ?", environmentID).
		Where("archived_at IS NULL").Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrNotFound
	}
	return nil
}
