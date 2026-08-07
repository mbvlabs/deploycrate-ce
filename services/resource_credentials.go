package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
)

var ErrResourceCredentialUnavailable = errors.New("Resource credential is unavailable")

type RevealedResourceCredential struct {
	ID       uuid.UUID         `json:"id"`
	Name     string            `json:"name"`
	Username string            `json:"username"`
	Values   map[string]string `json:"values"`
}

type ResourceCredentials struct {
	db       storage.Pool
	config   config.Config
	identity Identity
}

func NewResourceCredentials(
	db storage.Pool,
	cfg config.Config,
	identity Identity,
) *ResourceCredentials {
	return &ResourceCredentials{db: db, config: cfg, identity: identity}
}

func (service *ResourceCredentials) RevealSystem(
	ctx context.Context,
	resourceID, credentialID, userID uuid.UUID,
	password string,
) (RevealedResourceCredential, error) {
	return service.reveal(ctx, resourceID, credentialID, userID, password, true)
}

func (service *ResourceCredentials) RevealManaged(
	ctx context.Context,
	resourceID, credentialID, userID uuid.UUID,
	password string,
) (RevealedResourceCredential, error) {
	return service.reveal(ctx, resourceID, credentialID, userID, password, false)
}

func (service *ResourceCredentials) reveal(
	ctx context.Context,
	resourceID, credentialID, userID uuid.UUID,
	password string,
	systemManaged bool,
) (RevealedResourceCredential, error) {
	if err := service.identity.VerifyUserPassword(ctx, userID, password); err != nil {
		return RevealedResourceCredential{}, err
	}

	var row struct {
		ID         uuid.UUID      `bun:"id"`
		Name       string         `bun:"name"`
		Username   sql.NullString `bun:"username"`
		EncPayload []byte         `bun:"enc_payload"`
	}
	err := service.db.Executor().NewSelect().
		TableExpr("resource_credentials AS credential").
		ColumnExpr("credential.id, credential.name, credential.username, credential.enc_payload").
		Join("JOIN resources AS resource ON resource.id = credential.resource_id AND resource.archived_at IS NULL").
		Where("resource.id = ?", resourceID).
		Where("resource.system_managed = ?", systemManaged).
		Where("credential.id = ?", credentialID).
		Where("credential.archived_at IS NULL").
		Limit(1).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return RevealedResourceCredential{}, ErrResourceCredentialUnavailable
	}
	if err != nil {
		return RevealedResourceCredential{}, fmt.Errorf("load Resource credential: %w", err)
	}

	plaintext, err := secretcrypto.DecryptForPurpose(
		row.EncPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return RevealedResourceCredential{}, errors.New("Resource credential cannot be decrypted")
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 ||
		len(payload.Values) == 0 {
		return RevealedResourceCredential{}, errors.New("Resource credential payload is invalid")
	}

	return RevealedResourceCredential{
		ID:       row.ID,
		Name:     row.Name,
		Username: strings.TrimSpace(row.Username.String),
		Values:   payload.Values,
	}, nil
}
