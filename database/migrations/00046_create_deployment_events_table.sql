-- +goose Up
-- +goose StatementBegin
CREATE TABLE deployment_events (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    sequence BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    status TEXT,
    step TEXT,
    message TEXT NOT NULL,
    metadata JSONB NOT NULL,
    error TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,

    deployment_id UUID NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    change_task_attempt_id UUID REFERENCES change_task_attempts (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE deployment_events;
-- +goose StatementEnd
