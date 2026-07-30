-- +goose Up
-- +goose StatementBegin
CREATE TABLE backups (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    trigger_type TEXT NOT NULL,
    strategy TEXT NOT NULL,
    driver TEXT NOT NULL,
    format TEXT NOT NULL,
    artifact_reference TEXT NOT NULL,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    size_bytes BIGINT,
    digest BYTEA,
    verified_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ,
    pruned_at TIMESTAMPTZ,
    error TEXT,
    target_type TEXT NOT NULL,
    format_version TEXT NOT NULL,
    provider_metadata JSONB NOT NULL,
    producer_version TEXT NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_policy_id UUID NOT NULL REFERENCES backup_policies (id) ON DELETE RESTRICT,
    resource_id UUID REFERENCES resources (id) ON DELETE RESTRICT,
    environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    resource_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    resource_volume_id UUID REFERENCES resource_volumes (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,

    CONSTRAINT backups_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_installation_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL)
    ),
    CONSTRAINT backups_driver_check CHECK (
        (target_type = 'server' AND strategy = 'filesystem' AND driver = 'restic' AND format = 'restic') OR
        (target_type = 'resource' AND strategy = 'logical' AND driver = 'postgresql' AND format = 'tar.age')
    ),
    CONSTRAINT backups_trigger_check CHECK (trigger_type IN ('installer', 'schedule', 'manual', 'pre_restore')),
    CONSTRAINT backups_status_check CHECK (
        status IN ('pending', 'running', 'uploaded', 'verified', 'verification_failed', 'failed', 'pruned')
    ),
    CONSTRAINT backups_policy_scheduled_slot_unique UNIQUE (backup_policy_id, scheduled_at)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backups;
-- +goose StatementEnd
