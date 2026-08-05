-- +goose Up
-- +goose StatementBegin
CREATE TABLE release_command_logs (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    attempt INTEGER NOT NULL,
    sequence BIGINT NOT NULL,
    stream TEXT NOT NULL,
    message TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,

    release_command_execution_id UUID NOT NULL REFERENCES release_command_executions (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE release_command_logs;
-- +goose StatementEnd
