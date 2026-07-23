-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_installation_statuses (
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT,
    state TEXT NOT NULL,
    installed_version TEXT,
    service_state TEXT NOT NULL,
    health TEXT NOT NULL,
    source TEXT NOT NULL,
    health_reason TEXT,
    details JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    resource_installation_id UUID NOT NULL PRIMARY KEY REFERENCES resource_installations (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_installation_statuses;
-- +goose StatementEnd
