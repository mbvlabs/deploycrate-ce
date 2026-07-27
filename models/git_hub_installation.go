package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubInstallationEntity struct {
	bun.BaseModel       `bun:"table:github_installations,alias:github_installations"`
	ID                  uuid.UUID       `bun:"id,pk,type:uuid" json:"id"`
	CreatedAt           time.Time       `bun:"created_at" json:"createdAt"`
	UpdatedAt           time.Time       `bun:"updated_at" json:"updatedAt"`
	ArchivedAt          sql.NullTime    `bun:"archived_at" json:"archivedAt"`
	GitHubAppID         uuid.UUID       `bun:"github_app_id,type:uuid" json:"githubAppId"`
	ExternalID          int64           `bun:"external_id" json:"externalId"`
	AccountID           int64           `bun:"account_id" json:"accountId"`
	AccountLogin        string          `bun:"account_login" json:"accountLogin"`
	AccountType         string          `bun:"account_type" json:"accountType"`
	RepositorySelection string          `bun:"repository_selection" json:"repositorySelection"`
	Permissions         json.RawMessage `bun:"permissions,type:jsonb" json:"permissions"`
	Events              json.RawMessage `bun:"events,type:jsonb" json:"events"`
	SuspendedAt         sql.NullTime    `bun:"suspended_at" json:"suspendedAt"`
	LastSyncedAt        sql.NullTime    `bun:"last_synced_at" json:"lastSyncedAt"`
}
