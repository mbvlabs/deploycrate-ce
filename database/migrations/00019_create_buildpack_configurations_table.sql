-- +goose Up
-- +goose StatementBegin
CREATE TABLE buildpack_configurations (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    context_path TEXT NOT NULL,
    builder_reference TEXT,
    image_repository TEXT NOT NULL,
    settings JSONB NOT NULL,

    environment_source_id UUID NOT NULL REFERENCES environment_sources (id) ON DELETE RESTRICT,
    container_registry_id UUID NOT NULL REFERENCES container_registries (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE buildpack_configurations;
-- +goose StatementEnd
