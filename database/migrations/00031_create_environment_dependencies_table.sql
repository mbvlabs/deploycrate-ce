-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_dependencies (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_endpoint_id UUID NOT NULL REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    private_network_id UUID REFERENCES private_networks (id) ON DELETE RESTRICT,
    alias TEXT NOT NULL,
    required BOOLEAN NOT NULL,
    secret_mapping JSONB NOT NULL,
    settings JSONB NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_dependencies;
-- +goose StatementEnd
