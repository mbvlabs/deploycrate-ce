-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_task_attempts (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    attempt INTEGER NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    result JSONB,
    error TEXT,

    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_task_attempts;
-- +goose StatementEnd
