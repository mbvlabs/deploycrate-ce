-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_logs (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    occurred_at TIMESTAMPTZ NOT NULL,
    level TEXT NOT NULL,
    step TEXT,
    message TEXT NOT NULL,
    metadata JSONB NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    change_task_id UUID REFERENCES change_tasks (id) ON DELETE CASCADE,
    change_task_attempt_id UUID REFERENCES change_task_attempts (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_logs;
-- +goose StatementEnd
