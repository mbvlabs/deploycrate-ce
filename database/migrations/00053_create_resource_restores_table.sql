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
    cutover_at TIMESTAMPTZ,
    rolled_back_at TIMESTAMPTZ,
    error TEXT,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_id UUID NOT NULL REFERENCES backups (id) ON DELETE RESTRICT,
    safety_backup_id UUID REFERENCES backups (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    target_installation_id UUID NOT NULL REFERENCES resource_installations (id) ON DELETE RESTRICT,

    CONSTRAINT resource_restores_status_check CHECK (
        status IN ('pending', 'safety_backup', 'restoring', 'completed', 'rolled_back', 'failed')
    )
);

CREATE UNIQUE INDEX resource_restores_active_installation_unique
    ON resource_restores (target_installation_id)
    WHERE status IN ('pending', 'safety_backup', 'restoring');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_restores;
-- +goose StatementEnd
