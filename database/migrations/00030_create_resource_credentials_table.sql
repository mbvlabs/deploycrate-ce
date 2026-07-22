-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_credentials (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    username TEXT,
    metadata JSONB NOT NULL,
    enc_payload BYTEA NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_credentials;
-- +goose StatementEnd
