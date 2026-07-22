-- +goose Up
-- +goose StatementBegin
CREATE TABLE backup_destinations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT,
    provider TEXT NOT NULL,
    endpoint TEXT,
    region TEXT,
    bucket TEXT NOT NULL,
    prefix TEXT,
    force_path_style BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_destinations;
-- +goose StatementEnd
