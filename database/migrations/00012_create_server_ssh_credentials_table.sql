-- +goose Up
-- +goose StatementBegin
CREATE TABLE server_ssh_credentials (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    username TEXT NOT NULL,
    port INTEGER NOT NULL,
    enc_private_key BYTEA NOT NULL,
    known_host_key TEXT NOT NULL,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE server_ssh_credentials;
-- +goose StatementEnd
