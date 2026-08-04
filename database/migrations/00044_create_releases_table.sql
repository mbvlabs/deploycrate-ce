-- +goose Up
-- +goose StatementBegin
CREATE TABLE releases (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    version TEXT,
    source_revision TEXT,
    artifact_reference TEXT NOT NULL,
    artifact_digest BYTEA NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    environment_source_id UUID REFERENCES environment_sources (id) ON DELETE CASCADE,
    build_id UUID REFERENCES builds (id) ON DELETE CASCADE,
    created_by_change_id UUID NOT NULL REFERENCES changes (id) ON DELETE CASCADE,

    registry_resource_id UUID REFERENCES registry_resources (resource_id) ON DELETE RESTRICT,
    registry_credential_id UUID REFERENCES resource_credentials (id) ON DELETE RESTRICT,
    registry_endpoint TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE releases;
-- +goose StatementEnd
