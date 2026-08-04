-- +goose Up
-- +goose StatementBegin
CREATE TABLE runtime_configurations (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    runtime TEXT NOT NULL,
    command TEXT,
    arguments JSONB,
    replicas INTEGER NOT NULL,
    ports JSONB NOT NULL,
    resource_limits JSONB NOT NULL,
    restart_policy TEXT NOT NULL,
    settings JSONB NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE runtime_configurations ;
-- +goose StatementEnd
