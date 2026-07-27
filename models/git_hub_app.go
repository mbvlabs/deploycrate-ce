package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubAppEntity struct {
	bun.BaseModel `bun:"table:github_apps,alias:github_apps"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid" json:"id"`
	CreatedAt     time.Time       `bun:"created_at" json:"createdAt"`
	UpdatedAt     time.Time       `bun:"updated_at" json:"updatedAt"`
	ArchivedAt    sql.NullTime    `bun:"archived_at" json:"archivedAt"`
	CredentialID  uuid.UUID       `bun:"credential_id,type:uuid" json:"-"`
	InstanceID    uuid.UUID       `bun:"instance_id,type:uuid" json:"instanceId"`
	ExternalID    int64           `bun:"external_id" json:"externalId"`
	ClientID      string          `bun:"client_id" json:"clientId"`
	Slug          string          `bun:"slug" json:"slug"`
	Name          string          `bun:"name" json:"name"`
	OwnerID       int64           `bun:"owner_id" json:"ownerId"`
	OwnerLogin    string          `bun:"owner_login" json:"ownerLogin"`
	OwnerType     string          `bun:"owner_type" json:"ownerType"`
	HTMLURL       string          `bun:"html_url" json:"htmlUrl"`
	Permissions   json.RawMessage `bun:"permissions,type:jsonb" json:"permissions"`
	Events        json.RawMessage `bun:"events,type:jsonb" json:"events"`
	VerifiedAt    sql.NullTime    `bun:"verified_at" json:"verifiedAt"`
}
