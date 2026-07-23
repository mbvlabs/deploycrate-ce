-- +goose Up
-- +goose StatementBegin
CREATE TABLE wireguard_peer_statuses (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    latest_handshake_at TIMESTAMPTZ,
    error TEXT,
    observed_at TIMESTAMPTZ NOT NULL,

    wireguard_peer_id UUID NOT NULL REFERENCES wireguard_peers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE wireguard_peer_statuses;
-- +goose StatementEnd
