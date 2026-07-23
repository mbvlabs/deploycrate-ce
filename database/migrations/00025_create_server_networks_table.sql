-- +goose Up
-- +goose StatementBegin
CREATE TABLE server_networks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    address INET NOT NULL,
    driver TEXT NOT NULL,
    external_id TEXT,
    configuration JSONB NOT NULL,
    state TEXT NOT NULL,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    error TEXT,
    removed_at TIMESTAMPTZ,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    private_network_id UUID NOT NULL REFERENCES private_networks (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE server_networks;
-- +goose StatementEnd
