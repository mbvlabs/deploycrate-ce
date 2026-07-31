-- +goose Up
-- +goose StatementBegin
CREATE TABLE registry_resources (
    resource_id UUID NOT NULL PRIMARY KEY REFERENCES resources (id) ON DELETE RESTRICT,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    provider TEXT NOT NULL,
    configuration JSONB NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE registry_resources;
-- +goose StatementEnd
