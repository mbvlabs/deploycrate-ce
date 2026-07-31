-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_cluster_nodes (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    desired_state TEXT NOT NULL,
    archived_at TIMESTAMPTZ,
    database_cluster_id UUID NOT NULL REFERENCES database_clusters (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_cluster_nodes;
-- +goose StatementEnd
