-- +goose Up
-- +goose StatementBegin
CREATE TABLE environments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    application_id UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    kind TEXT NOT NULL,
    webhook_token_prefix TEXT,
    webhook_token_digest BYTEA,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environments;
-- +goose StatementEnd
