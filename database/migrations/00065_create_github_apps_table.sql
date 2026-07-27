-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_apps (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT,
    instance_id UUID NOT NULL,
    external_id BIGINT NOT NULL,
    client_id TEXT NOT NULL,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    owner_id BIGINT NOT NULL,
    owner_login TEXT NOT NULL,
    owner_type TEXT NOT NULL CHECK (owner_type IN ('User', 'Organization')),
    html_url TEXT NOT NULL,
    permissions JSONB NOT NULL,
    events JSONB NOT NULL,
    verified_at TIMESTAMPTZ
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_apps;
-- +goose StatementEnd
