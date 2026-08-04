-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_secrets (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    key TEXT NOT NULL,
    enc_value BYTEA NOT NULL,
    digest BYTEA NOT NULL,
    source_type TEXT NOT NULL,
    source_id UUID NOT NULL,
    archived_at TIMESTAMPTZ,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_secrets;
-- +goose StatementEnd
