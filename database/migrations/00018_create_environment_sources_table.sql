-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_sources (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    kind TEXT NOT NULL,
    provider TEXT NOT NULL,
    repository TEXT NOT NULL,
    reference TEXT NOT NULL,
    settings JSONB NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    credential_id UUID REFERENCES credentials (id) ON DELETE RESTRICT,
    container_registry_id UUID REFERENCES container_registries (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_sources;
-- +goose StatementEnd
