-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_tasks (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    parent_task_id UUID REFERENCES change_tasks (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,
    environment_target_id UUID REFERENCES environment_targets (id) ON DELETE RESTRICT,
    kind TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    input JSONB NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL,
    available_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_tasks;
-- +goose StatementEnd
