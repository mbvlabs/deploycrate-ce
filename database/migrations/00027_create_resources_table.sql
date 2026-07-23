-- +goose Up
-- +goose StatementBegin
CREATE TABLE resources (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    category TEXT NOT NULL,
    kind TEXT NOT NULL,
    sharing_scope TEXT NOT NULL,
    archived_at TIMESTAMPTZ,

    owner_environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resources;
-- +goose StatementEnd
