-- +goose Up
-- +goose StatementBegin
CREATE TABLE deployments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    attempt INTEGER NOT NULL,
    strategy JSONB NOT NULL,
    process_configuration JSONB NOT NULL,
    status TEXT NOT NULL,
    current_step TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error TEXT,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE deployments;
-- +goose StatementEnd
