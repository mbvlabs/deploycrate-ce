-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_device_resource_grant_applications (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    driver TEXT NOT NULL,
    external_id TEXT,
    state TEXT NOT NULL,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    error TEXT,

    wireguard_device_resource_grant_id UUID NOT NULL REFERENCES wireguard_device_resource_grants (id) ON DELETE RESTRICT,
    resource_endpoint_id UUID NOT NULL REFERENCES resource_endpoints (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_device_resource_grant_applications;
-- +goose StatementEnd
