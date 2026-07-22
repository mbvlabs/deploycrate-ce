-- +goose Up
-- +goose StatementBegin
CREATE TABLE network_access_rules (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    private_network_id UUID NOT NULL REFERENCES private_networks (id) ON DELETE RESTRICT,
    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    resource_endpoint_id UUID NOT NULL REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    dependency_id UUID NOT NULL REFERENCES environment_dependencies (id) ON DELETE RESTRICT,
    protocol TEXT NOT NULL,
    destination_address TEXT NOT NULL,
    destination_port INTEGER NOT NULL,
    action TEXT NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE network_access_rules;
-- +goose StatementEnd
