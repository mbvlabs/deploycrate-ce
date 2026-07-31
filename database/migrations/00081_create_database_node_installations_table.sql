-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_node_installations (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    installation_method TEXT NOT NULL,
    desired_state TEXT NOT NULL,
    observed_state TEXT NOT NULL,
    installed_version TEXT,
    service_state TEXT NOT NULL,
    health TEXT NOT NULL,
    reason TEXT,
    observed_at TIMESTAMPTZ,
    external_runtime_id TEXT,
    archived_at TIMESTAMPTZ,
    database_cluster_node_id UUID NOT NULL REFERENCES database_cluster_nodes (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    database_node_storage_id UUID NOT NULL REFERENCES database_node_storage (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_node_installations;
-- +goose StatementEnd
