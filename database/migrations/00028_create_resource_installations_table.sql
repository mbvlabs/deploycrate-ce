-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_installations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    image_reference TEXT NOT NULL,
    image_digest TEXT,
    container_name TEXT NOT NULL,
    restart_policy TEXT NOT NULL,
    configuration JSONB NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    registry_credential_id UUID REFERENCES credentials (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_installations;
-- +goose StatementEnd
