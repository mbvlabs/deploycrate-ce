-- +goose Up
-- +goose StatementBegin
CREATE TABLE caddy_routes (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    state TEXT NOT NULL,
    applied_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,

    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE CASCADE,
    environment_domain_id UUID NOT NULL REFERENCES environment_domains (id) ON DELETE CASCADE,
    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE caddy_routes;
-- +goose StatementEnd
