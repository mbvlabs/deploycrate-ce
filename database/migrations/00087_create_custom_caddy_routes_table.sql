-- +goose Up
-- +goose StatementBegin
CREATE TABLE custom_caddy_routes (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    hostname TEXT NOT NULL,
    origin_address TEXT NOT NULL,
    origin_port INTEGER NOT NULL,
    origin_protocol TEXT NOT NULL,
    origin_tls_mode TEXT NOT NULL,
    health_path TEXT NOT NULL,
    state TEXT NOT NULL,
    last_error TEXT,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE custom_caddy_routes;
-- +goose StatementEnd
