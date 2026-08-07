package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubRepositoryEntity struct {
	bun.BaseModel        `bun:"table:github_repositories,alias:github_repositories"`
	ID                   uuid.UUID    `bun:"id,pk,type:uuid"                                     json:"id"`
	CreatedAt            time.Time    `bun:"created_at"                                          json:"createdAt"`
	UpdatedAt            time.Time    `bun:"updated_at"                                          json:"updatedAt"`
	RemovedAt            sql.NullTime `bun:"removed_at"                                          json:"removedAt"`
	GitHubInstallationID uuid.UUID    `bun:"github_installation_id,type:uuid"                    json:"githubInstallationId"`
	ExternalID           int64        `bun:"external_id"                                         json:"externalId"`
	NodeID               string       `bun:"node_id"                                             json:"nodeId"`
	OwnerLogin           string       `bun:"owner_login"                                         json:"ownerLogin"`
	Name                 string       `bun:"name"                                                json:"name"`
	FullName             string       `bun:"full_name"                                           json:"fullName"`
	DefaultBranch        string       `bun:"default_branch"                                      json:"defaultBranch"`
	Visibility           string       `bun:"visibility"                                          json:"visibility"`
	HTMLURL              string       `bun:"html_url"                                            json:"htmlUrl"`
	LastSyncedAt         time.Time    `bun:"last_synced_at"                                      json:"lastSyncedAt"`
}
