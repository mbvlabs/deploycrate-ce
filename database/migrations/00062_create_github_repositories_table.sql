-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_repositories (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    removed_at TIMESTAMPTZ,
    github_installation_id UUID NOT NULL REFERENCES github_installations (id) ON DELETE RESTRICT,
    external_id BIGINT NOT NULL,
    node_id TEXT NOT NULL,
    owner_login TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    default_branch TEXT NOT NULL,
    visibility TEXT NOT NULL,
    html_url TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_repositories;
-- +goose StatementEnd
