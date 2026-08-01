-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_resources (
    resource_id UUID NOT NULL PRIMARY KEY REFERENCES resources (id) ON DELETE RESTRICT,
    database_cluster_id UUID NOT NULL REFERENCES database_clusters (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_resources;
-- +goose StatementEnd
