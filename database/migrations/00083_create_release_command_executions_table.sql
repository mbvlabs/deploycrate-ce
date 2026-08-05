-- +goose Up
-- +goose StatementBegin
CREATE TABLE release_command_executions (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    status TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    configuration JSONB NOT NULL,
    configuration_digest BYTEA NOT NULL,
    external_id TEXT,
    exit_code INTEGER,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error TEXT,
    retry_requested_by UUID REFERENCES users (id) ON DELETE SET NULL,

    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    environment_state_revision_id UUID NOT NULL REFERENCES environment_state_revisions (id) ON DELETE CASCADE,
    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE RESTRICT,
    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE release_command_executions;
-- +goose StatementEnd
