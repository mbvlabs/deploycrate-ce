-- +goose Up
-- +goose StatementBegin
CREATE TABLE container_registries (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    endpoint TEXT NOT NULL,

    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE container_registries;
-- +goose StatementEnd
