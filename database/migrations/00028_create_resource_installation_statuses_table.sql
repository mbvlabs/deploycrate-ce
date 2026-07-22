-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_installation_statuses (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_installation_id UUID NOT NULL REFERENCES resource_installations (id) ON DELETE RESTRICT,
    external_id TEXT,
    state TEXT NOT NULL,
    installed_version TEXT,
    service_state TEXT,
    health TEXT,
    details JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_installation_statuses;
-- +goose StatementEnd
