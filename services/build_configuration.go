package services

import (
	"database/sql"
	"deploycrate-ce/models"
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
	RegistryResourceID         uuid.UUID       `json:"registry_resource_id"`
	RegistryCredentialID       uuid.UUID       `json:"registry_credential_id"`
	RegistryEndpoint           string          `json:"registry_endpoint"`
	Settings                   json.RawMessage `json:"settings"`
	BPGOTargets                string          `json:"bp_go_targets"`
	ServerID                   uuid.UUID       `json:"server_id"`
	parsedSettings             models.BuildpackSettings
}

func marshalBuildSnapshot(snapshot buildSnapshot) (json.RawMessage, error) {
	settings, err := models.CanonicalBuildpackSettings(snapshot.Settings)
	if err != nil {
		return nil, err
	}
	snapshot.Settings = settings
	return json.Marshal(snapshot)
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
