-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_dns_bindings (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    last_error TEXT,
    applied_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,

    resource_endpoint_id UUID NOT NULL REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    dns_zone_id UUID NOT NULL REFERENCES dns_zones (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_dns_bindings;
-- +goose StatementEnd
