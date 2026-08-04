-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_domains (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    hostname TEXT NOT NULL,
    is_primary BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_domains;
-- +goose StatementEnd
