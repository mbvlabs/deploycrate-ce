-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_dns_records (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    record_type TEXT NOT NULL,
    content TEXT NOT NULL,
    observed_name TEXT NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_dns_binding_id UUID NOT NULL REFERENCES resource_dns_bindings (id) ON DELETE RESTRICT,
    dns_zone_id UUID NOT NULL REFERENCES dns_zones (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_dns_records;
-- +goose StatementEnd
