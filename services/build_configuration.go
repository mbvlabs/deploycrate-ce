package services

import (
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
)

type buildSnapshot struct {
	SchemaVersion              int             `json:"schema_version"`
	SourceEventID              uuid.UUID       `json:"source_event_id"`
	EnvironmentStateRevisionID uuid.UUID       `json:"environment_state_revision_id"`
	Repository                 string          `json:"repository"`
	Reference                  string          `json:"reference"`
	SourceRevision             string          `json:"source_revision"`
	ContextPath                string          `json:"context_path"`
	BuilderReference           *string         `json:"builder_reference"`
	ImageRepository            string          `json:"image_repository"`
	ContainerRegistryID        uuid.UUID       `json:"container_registry_id"`
	RegistryEndpoint           string          `json:"registry_endpoint"`
	Settings                   json.RawMessage `json:"settings"`
	BPGOTargets                string          `json:"bp_go_targets"`
}

func marshalBuildSnapshot(snapshot buildSnapshot) (json.RawMessage, error) {
	return json.Marshal(snapshot)
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
