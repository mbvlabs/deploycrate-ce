package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type GitHubWebhookDeliveryEntity struct {
	bun.BaseModel          `bun:"table:github_webhook_deliveries,alias:github_webhook_deliveries"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"                                                 json:"id"`
	CreatedAt              time.Time       `bun:"created_at"                                                      json:"createdAt"`
	UpdatedAt              time.Time       `bun:"updated_at"                                                      json:"updatedAt"`
	DeliveryID             string          `bun:"delivery_id"                                                     json:"deliveryId"`
	Event                  string          `bun:"event"                                                           json:"event"`
	Action                 sql.NullString  `bun:"action"                                                          json:"action"`
	InstallationExternalID sql.NullInt64   `bun:"installation_external_id"                                        json:"installationExternalId"`
	RepositoryExternalID   sql.NullInt64   `bun:"repository_external_id"                                          json:"repositoryExternalId"`
	BodyDigest             []byte          `bun:"body_digest"                                                     json:"-"`
	Payload                json.RawMessage `bun:"payload,type:jsonb"                                              json:"payload"`
	ReceivedAt             time.Time       `bun:"received_at"                                                     json:"receivedAt"`
	ProcessedAt            sql.NullTime    `bun:"processed_at"                                                    json:"processedAt"`
	Status                 string          `bun:"status"                                                          json:"status"`
	Error                  sql.NullString  `bun:"error"                                                           json:"error"`
}
