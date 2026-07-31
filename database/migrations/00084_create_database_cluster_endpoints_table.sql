-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_cluster_endpoints (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    address TEXT NOT NULL,
    port INTEGER NOT NULL,
    protocol TEXT NOT NULL,
    tls_mode TEXT NOT NULL,
    desired_state TEXT NOT NULL,
    observed_state TEXT NOT NULL,
    settings JSONB NOT NULL,
    archived_at TIMESTAMPTZ,
    database_cluster_id UUID NOT NULL REFERENCES database_clusters (id) ON DELETE RESTRICT,
    private_network_id UUID REFERENCES private_networks (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_cluster_endpoints;
-- +goose StatementEnd
