-- +goose Up
-- +goose StatementBegin
CREATE TABLE instances (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    slot TEXT NOT NULL,
    replica_key TEXT NOT NULL,
    state TEXT NOT NULL,
    ports JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    removed_at TIMESTAMPTZ,

    deployment_id UUID NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE instances;
-- +goose StatementEnd
