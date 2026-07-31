-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_credentials (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    username TEXT,
    metadata JSONB NOT NULL,
    enc_payload BYTEA NOT NULL,
    digest BYTEA NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_credentials;
-- +goose StatementEnd
