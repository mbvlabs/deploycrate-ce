-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_build_configurations (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    method TEXT NOT NULL,
    context_path TEXT NOT NULL,
    dockerfile_path TEXT,
    builder_reference TEXT,
    settings JSONB NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_build_configurations;
-- +goose StatementEnd
