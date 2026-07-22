-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_restores (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    resource_backup_id UUID NOT NULL REFERENCES resource_backups (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    source_binding_id UUID REFERENCES resource_bindings (id) ON DELETE RESTRICT,
    target_binding_id UUID REFERENCES resource_bindings (id) ON DELETE RESTRICT,
    target_installation_id UUID REFERENCES resource_installations (id) ON DELETE RESTRICT,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_restores;
-- +goose StatementEnd
