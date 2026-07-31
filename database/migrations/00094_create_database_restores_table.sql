-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_restores (
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
    database_id UUID NOT NULL REFERENCES databases (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_restores;
-- +goose StatementEnd
