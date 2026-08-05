-- +goose Up
-- +goose StatementBegin
CREATE TABLE dns_connections (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    verified_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,

    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT,

    account_external_id TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE dns_connections;
-- +goose StatementEnd
