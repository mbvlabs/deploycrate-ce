package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubAppSetupAttemptEntity struct {
	bun.BaseModel `bun:"table:github_app_setup_attempts,alias:github_app_setup_attempts"`
	ID            uuid.UUID      `bun:"id,pk,type:uuid" json:"id"`
	CreatedAt     time.Time      `bun:"created_at" json:"createdAt"`
	UpdatedAt     time.Time      `bun:"updated_at" json:"updatedAt"`
	InstanceID    uuid.UUID      `bun:"instance_id,type:uuid" json:"instanceId"`
	UserID        uuid.UUID      `bun:"user_id,type:uuid" json:"userId"`
	Purpose       string         `bun:"purpose" json:"purpose"`
	StatePrefix   string         `bun:"state_prefix" json:"statePrefix"`
	StateDigest   []byte         `bun:"state_digest" json:"-"`
	OwnerType     sql.NullString `bun:"owner_type" json:"ownerType"`
	OwnerLogin    sql.NullString `bun:"owner_login" json:"ownerLogin"`
	ExpiresAt     time.Time      `bun:"expires_at" json:"expiresAt"`
	CompletedAt   sql.NullTime   `bun:"completed_at" json:"completedAt"`
	Error         sql.NullString `bun:"error" json:"error"`
}
