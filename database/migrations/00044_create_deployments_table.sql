-- +goose Up
-- +goose StatementBegin
CREATE TABLE deployments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE RESTRICT,
    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE RESTRICT,
    attempt INTEGER NOT NULL,
    strategy JSONB NOT NULL,
    runtime_configuration JSONB NOT NULL,
    status TEXT NOT NULL,
    current_step TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE deployments;
-- +goose StatementEnd
