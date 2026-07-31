-- +goose Up
-- +goose StatementBegin
CREATE TABLE databases (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    encoding TEXT,
    database_collation TEXT,
    settings JSONB NOT NULL,
    desired_state TEXT NOT NULL,
    observed_state TEXT NOT NULL,
    archived_at TIMESTAMPTZ,
    database_cluster_id UUID NOT NULL REFERENCES database_clusters (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE databases;
-- +goose StatementEnd
