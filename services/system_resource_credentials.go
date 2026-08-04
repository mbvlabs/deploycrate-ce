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

var ErrSystemResourceCredentialUnavailable = errors.New("system Resource credential is unavailable")

type RevealedSystemResourceCredential struct {
	ID       uuid.UUID         `json:"id"`
	Name     string            `json:"name"`
	Username string            `json:"username"`
	Values   map[string]string `json:"values"`
}

type SystemResourceCredentials struct {
	db       storage.Pool
	config   config.Config
	identity Identity
}

func NewSystemResourceCredentials(db storage.Pool, cfg config.Config, identity Identity) *SystemResourceCredentials {
	return &SystemResourceCredentials{db: db, config: cfg, identity: identity}
}

func (service *SystemResourceCredentials) Reveal(
	ctx context.Context,
	resourceID, credentialID, userID uuid.UUID,
	password string,
) (RevealedSystemResourceCredential, error) {
	if err := service.identity.VerifyUserPassword(ctx, userID, password); err != nil {
		return RevealedSystemResourceCredential{}, err
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
		Join("JOIN resources AS resource ON resource.id = credential.resource_id AND resource.system_managed = TRUE AND resource.archived_at IS NULL").
		Where("resource.id = ?", resourceID).
		Where("credential.id = ?", credentialID).
		Where("credential.archived_at IS NULL").
		Limit(1).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return RevealedSystemResourceCredential{}, ErrSystemResourceCredentialUnavailable
	}
	if err != nil {
		return RevealedSystemResourceCredential{}, fmt.Errorf("load system Resource credential: %w", err)
	}

	plaintext, err := secretcrypto.DecryptForPurpose(
		row.EncPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return RevealedSystemResourceCredential{}, errors.New("system Resource credential cannot be decrypted")
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 || len(payload.Values) == 0 {
		return RevealedSystemResourceCredential{}, errors.New("system Resource credential payload is invalid")
	}

	return RevealedSystemResourceCredential{
		ID: row.ID, Name: row.Name, Username: strings.TrimSpace(row.Username.String), Values: payload.Values,
	}, nil
}
