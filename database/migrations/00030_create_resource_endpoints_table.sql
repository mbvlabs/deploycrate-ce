-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_endpoints (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    role TEXT NOT NULL,
    address TEXT NOT NULL,
    port INTEGER NOT NULL,
    protocol TEXT NOT NULL,
    tls_mode TEXT NOT NULL,
    settings JSONB NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    private_network_id UUID REFERENCES private_networks (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_endpoints;
-- +goose StatementEnd
