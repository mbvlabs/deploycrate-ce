-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_processes (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    command TEXT,
    arguments JSONB NOT NULL,
    replicas INTEGER NOT NULL,
    container_port INTEGER,
    health_path TEXT,
    timeout_seconds INTEGER,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_processes;
-- +goose StatementEnd
