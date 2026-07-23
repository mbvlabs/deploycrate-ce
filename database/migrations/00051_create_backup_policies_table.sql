-- +goose Up
-- +goose StatementBegin
CREATE TABLE backup_policies (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    schedule TEXT NOT NULL,
    strategy TEXT NOT NULL,
    driver TEXT NOT NULL,
    retention JSONB NOT NULL,
    format TEXT NOT NULL,
    verification JSONB NOT NULL,
    settings JSONB NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    resource_volume_id UUID REFERENCES resource_volumes (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_policies;
-- +goose StatementEnd
