-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_installations (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    github_app_id UUID NOT NULL REFERENCES github_apps (id) ON DELETE RESTRICT,
    external_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    account_login TEXT NOT NULL,
    account_type TEXT NOT NULL,
    repository_selection TEXT NOT NULL,
    permissions JSONB NOT NULL,
    events JSONB NOT NULL,
    suspended_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_installations;
-- +goose StatementEnd
