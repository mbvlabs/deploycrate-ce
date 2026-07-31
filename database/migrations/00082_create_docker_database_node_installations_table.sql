-- +goose Up
-- +goose StatementBegin
CREATE TABLE docker_database_node_installations (
    database_node_installation_id UUID NOT NULL PRIMARY KEY REFERENCES database_node_installations (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    image_reference TEXT NOT NULL,
    image_digest TEXT,
    container_name TEXT NOT NULL,
    restart_policy TEXT NOT NULL,
    port_mappings JSONB NOT NULL,
    configuration JSONB NOT NULL,
    registry_resource_id UUID REFERENCES registry_resources (resource_id) ON DELETE RESTRICT,
    registry_credential_id UUID REFERENCES resource_credentials (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE docker_database_node_installations;
-- +goose StatementEnd
