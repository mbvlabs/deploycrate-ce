-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_node_storage (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    driver TEXT NOT NULL,
    external_id TEXT,
    data_path TEXT NOT NULL,
    configuration JSONB NOT NULL,
    archived_at TIMESTAMPTZ,
    database_cluster_node_id UUID NOT NULL REFERENCES database_cluster_nodes (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_node_storage;
-- +goose StatementEnd
