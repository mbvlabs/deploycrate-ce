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
    auto_build BOOLEAN NOT NULL DEFAULT TRUE,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    credential_id UUID REFERENCES credentials (id) ON DELETE RESTRICT,
    container_registry_id UUID REFERENCES container_registries (id) ON DELETE RESTRICT
);

CREATE INDEX environment_sources_active_provider_repository
    ON environment_sources (provider, repository)
    WHERE archived_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_sources;
-- +goose StatementEnd
