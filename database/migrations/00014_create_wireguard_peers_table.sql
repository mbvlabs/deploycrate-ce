-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_peers (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    public_key TEXT NOT NULL,
    enc_private_key BYTEA NOT NULL,
    private_address INET NOT NULL,
    endpoint TEXT,
    listen_port INTEGER NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL,
    retired_at TIMESTAMPTZ,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_peers;
-- +goose StatementEnd
