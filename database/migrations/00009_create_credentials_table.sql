-- +goose Up
-- +goose StatementBegin
CREATE TABLE credentials (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    metadata JSONB NOT NULL,
    enc_payload BYTEA NOT NULL,
    verified_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE credentials;
-- +goose StatementEnd
