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
    activated_at TIMESTAMPTZ,
    target_type TEXT NOT NULL,
    target JSONB NOT NULL,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_scheduled_at TIMESTAMPTZ,

    resource_id UUID REFERENCES resources (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_policies;
-- +goose StatementEnd
