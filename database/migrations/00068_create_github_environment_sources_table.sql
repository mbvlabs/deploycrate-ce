-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_environment_sources (
    environment_source_id UUID NOT NULL PRIMARY KEY REFERENCES environment_sources (id) ON DELETE CASCADE,
    github_repository_id UUID NOT NULL REFERENCES github_repositories (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_environment_sources;
-- +goose StatementEnd
