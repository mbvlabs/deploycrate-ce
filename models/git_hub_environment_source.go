package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubEnvironmentSourceEntity struct {
	bun.BaseModel       `bun:"table:github_environment_sources,alias:github_environment_sources"`
	EnvironmentSourceID uuid.UUID `bun:"environment_source_id,pk,type:uuid"                                json:"environmentSourceId"`
	GitHubRepositoryID  uuid.UUID `bun:"github_repository_id,type:uuid"                                    json:"githubRepositoryId"`
	CreatedAt           time.Time `bun:"created_at"                                                        json:"createdAt"`
	UpdatedAt           time.Time `bun:"updated_at"                                                        json:"updatedAt"`
}
