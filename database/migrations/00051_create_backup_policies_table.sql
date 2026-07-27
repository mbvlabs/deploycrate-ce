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
    next_run_at TIMESTAMPTZ NOT NULL,
    last_scheduled_at TIMESTAMPTZ,

    resource_id UUID REFERENCES resources (id) ON DELETE RESTRICT,
    environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    resource_volume_id UUID REFERENCES resource_volumes (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,

    CONSTRAINT backup_policies_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL)
    ),
    CONSTRAINT backup_policies_driver_check CHECK (
        (target_type = 'server' AND strategy = 'filesystem' AND driver = 'restic' AND format = 'restic') OR
        (target_type = 'resource' AND strategy = 'logical' AND driver = 'postgresql' AND format = 'tar.age')
    )
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_policies;
-- +goose StatementEnd
