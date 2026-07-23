-- +goose Up
-- +goose StatementBegin
CREATE TABLE environments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    kind TEXT NOT NULL,
    webhook_token_prefix TEXT,  -- TODO: not sure
    webhook_token_digest BYTEA, -- TODO: not sure
    archived_at TIMESTAMPTZ,

    application_id UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environments;
-- +goose StatementEnd
