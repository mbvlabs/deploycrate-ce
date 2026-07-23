-- +goose Up
-- +goose StatementBegin
CREATE TABLE network_access_rules (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    protocol TEXT NOT NULL,
    destination_address TEXT NOT NULL,
    destination_port INTEGER NOT NULL,
    action TEXT NOT NULL,
    archived_at TIMESTAMPTZ,

    environment_resource_id UUID NOT NULL REFERENCES environment_resources (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE network_access_rules;
-- +goose StatementEnd
