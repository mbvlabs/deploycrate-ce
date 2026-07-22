-- +goose Up
-- +goose StatementBegin
CREATE TABLE backup_policies (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_binding_id UUID REFERENCES resource_bindings (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    schedule TEXT NOT NULL,
    retention JSONB NOT NULL,
    format TEXT NOT NULL,
    verification JSONB NOT NULL,
    settings JSONB NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_policies;
-- +goose StatementEnd
