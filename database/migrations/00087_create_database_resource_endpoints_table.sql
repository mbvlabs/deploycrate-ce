-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_resource_endpoints (
    resource_endpoint_id UUID NOT NULL PRIMARY KEY REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    database_cluster_endpoint_id UUID NOT NULL REFERENCES database_cluster_endpoints (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_resource_endpoints;
-- +goose StatementEnd
