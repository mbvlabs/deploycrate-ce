-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_devices (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    private_address INET NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,

    owner_user_id UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_devices;
-- +goose StatementEnd
