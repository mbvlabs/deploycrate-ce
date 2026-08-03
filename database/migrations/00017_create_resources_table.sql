-- +goose Up
-- +goose StatementBegin
CREATE TABLE resources (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    configuration JSONB NOT NULL,
    sharing_scope TEXT NOT NULL,
    system_managed BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resources;
-- +goose StatementEnd
