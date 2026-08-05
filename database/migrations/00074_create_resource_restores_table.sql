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
    target JSONB NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_id UUID NOT NULL REFERENCES backups (id) ON DELETE RESTRICT,
    safety_backup_id UUID REFERENCES backups (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_restores;
-- +goose StatementEnd
