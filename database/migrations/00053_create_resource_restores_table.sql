-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_restores (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    error TEXT,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    resource_backup_id UUID NOT NULL REFERENCES resource_backups (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    source_environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    target_environment_resource_id UUID REFERENCES environment_resources (id) ON DELETE RESTRICT,
    target_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_restores;
-- +goose StatementEnd
