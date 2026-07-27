-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_device_resource_grants (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    granted_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,

    wireguard_device_id UUID NOT NULL REFERENCES wireguard_devices (id) ON DELETE RESTRICT,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    granted_by_user_id UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_device_resource_grants;
-- +goose StatementEnd
