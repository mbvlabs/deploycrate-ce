-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_target_networks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE RESTRICT,
    private_network_id UUID NOT NULL REFERENCES private_networks (id) ON DELETE RESTRICT,
    address INET NOT NULL,
    driver TEXT NOT NULL,
    external_id TEXT,
    configuration JSONB NOT NULL,
    state TEXT NOT NULL,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    error TEXT,
    removed_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_target_networks;
-- +goose StatementEnd
