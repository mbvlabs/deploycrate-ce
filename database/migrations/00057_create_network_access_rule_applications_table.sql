-- +goose Up
-- +goose StatementBegin
CREATE TABLE network_access_rule_applications (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    network_access_rule_id UUID NOT NULL REFERENCES network_access_rules (id) ON DELETE RESTRICT,
    environment_target_network_id INTEGER NOT NULL REFERENCES environment_target_networks (id) ON DELETE RESTRICT,
    driver TEXT NOT NULL,
    external_id TEXT,
    configuration JSONB NOT NULL,
    state TEXT NOT NULL,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE network_access_rule_applications;
-- +goose StatementEnd
