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
    account_type TEXT NOT NULL CHECK (account_type IN ('User', 'Organization')),
    repository_selection TEXT NOT NULL CHECK (repository_selection IN ('all', 'selected')),
    permissions JSONB NOT NULL,
    events JSONB NOT NULL,
    suspended_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ
);

CREATE INDEX github_installations_active_app
    ON github_installations (github_app_id)
    WHERE archived_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_installations;
-- +goose StatementEnd
