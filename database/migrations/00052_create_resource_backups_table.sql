-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_backups (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    trigger_type TEXT NOT NULL,
    strategy TEXT NOT NULL,
    driver TEXT NOT NULL,
    format TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    size_bytes BIGINT,
    digest BYTEA,
    verified_at TIMESTAMPTZ,
    error TEXT,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_policy_id UUID NOT NULL REFERENCES backup_policies (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    resource_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    resource_volume_id UUID REFERENCES resource_volumes (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_backups;
-- +goose StatementEnd
