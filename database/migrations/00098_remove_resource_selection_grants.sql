-- +goose Up
-- +goose StatementBegin
DROP TABLE IF EXISTS resource_application_grants;
DROP TABLE IF EXISTS resource_environment_grants;
ALTER TABLE resources DROP COLUMN IF EXISTS sharing_scope;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE resources ADD COLUMN sharing_scope TEXT NOT NULL DEFAULT 'global';
ALTER TABLE resources ALTER COLUMN sharing_scope DROP DEFAULT;

CREATE TABLE resource_environment_grants (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT
);

CREATE TABLE resource_application_grants (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    application_id UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT
);
-- +goose StatementEnd
