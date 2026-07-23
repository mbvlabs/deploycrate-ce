-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_health_checks (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    configuration JSONB NOT NULL,
    interval_seconds INTEGER NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    failure_threshold INTEGER NOT NULL,
    success_threshold INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_installation_id UUID NOT NULL REFERENCES resource_installations (id) ON DELETE RESTRICT,
    resource_endpoint_id UUID REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    resource_credential_id UUID REFERENCES resource_credentials (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_health_checks;
-- +goose StatementEnd
