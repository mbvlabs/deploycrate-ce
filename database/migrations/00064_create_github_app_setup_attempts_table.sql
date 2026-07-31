-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_app_setup_attempts (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    instance_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    purpose TEXT NOT NULL,
    state_prefix TEXT NOT NULL,
    state_digest BYTEA NOT NULL,
    owner_type TEXT,
    owner_login TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_app_setup_attempts;
-- +goose StatementEnd
