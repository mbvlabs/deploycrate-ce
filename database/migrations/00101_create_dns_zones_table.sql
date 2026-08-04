-- +goose Up
-- +goose StatementBegin
CREATE TABLE dns_zones (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    dns_connection_id UUID NOT NULL REFERENCES dns_connections (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE dns_zones;
-- +goose StatementEnd
