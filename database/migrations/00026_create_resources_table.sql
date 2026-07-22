-- +goose Up
-- +goose StatementBegin
CREATE TABLE resources (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    owner_environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    kind TEXT NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resources;
-- +goose StatementEnd
