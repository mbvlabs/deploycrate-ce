-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_installations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,
    mode TEXT NOT NULL,
    driver TEXT NOT NULL,
    desired_version TEXT,
    configuration JSONB NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_installations;
-- +goose StatementEnd
