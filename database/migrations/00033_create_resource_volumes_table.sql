-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_volumes (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    driver TEXT NOT NULL,
    configuration JSONB NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_volumes;
-- +goose StatementEnd
