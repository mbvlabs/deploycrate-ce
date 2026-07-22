-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_bindings (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_endpoint_id UUID NOT NULL REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    environment_dependency_id UUID NOT NULL REFERENCES environment_dependencies (id) ON DELETE RESTRICT,
    provisioning_mode TEXT NOT NULL,
    secret_management_mode TEXT NOT NULL,
    kind TEXT NOT NULL,
    external_database TEXT,
    external_principal TEXT,
    configuration JSONB NOT NULL,
    status TEXT NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_bindings;
-- +goose StatementEnd
