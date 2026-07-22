-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_backups (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_policy_id UUID NOT NULL REFERENCES backup_policies (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    resource_binding_id UUID REFERENCES resource_bindings (id) ON DELETE RESTRICT,
    resource_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    trigger_type TEXT NOT NULL,
    format TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    size_bytes BIGINT,
    digest BYTEA,
    verified_at TIMESTAMPTZ,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_backups;
-- +goose StatementEnd
