-- +goose Up
-- +goose StatementBegin
CREATE TABLE caddy_route_backends (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    caddy_route_id UUID NOT NULL REFERENCES caddy_routes (id) ON DELETE RESTRICT,
    instance_id UUID NOT NULL REFERENCES instances (id) ON DELETE RESTRICT,
    weight INTEGER NOT NULL,
    removed_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE caddy_route_backends;
-- +goose StatementEnd
