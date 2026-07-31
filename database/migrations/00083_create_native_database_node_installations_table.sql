-- +goose Up
-- +goose StatementBegin
CREATE TABLE native_database_node_installations (
    database_node_installation_id UUID NOT NULL PRIMARY KEY REFERENCES database_node_installations (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    package_name TEXT NOT NULL,
    requested_package_version TEXT,
    service_name TEXT NOT NULL,
    configuration_path TEXT NOT NULL,
    settings JSONB NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE native_database_node_installations;
-- +goose StatementEnd
