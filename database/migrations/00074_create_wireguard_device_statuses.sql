-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_device_statuses (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    latest_handshake_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL,
    error TEXT,

    wireguard_device_id UUID NOT NULL REFERENCES wireguard_devices (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_device_statuses;
-- +goose StatementEnd
