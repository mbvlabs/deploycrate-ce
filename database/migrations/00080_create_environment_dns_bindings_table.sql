-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_dns_bindings (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    generation BIGINT NOT NULL,
    applied_generation BIGINT NOT NULL DEFAULT 0,
    last_error TEXT,
    adoption_confirmed_at TIMESTAMPTZ,
    deploy_after_apply BOOLEAN NOT NULL DEFAULT FALSE,
    deployment_actor_id UUID,
    deployment_dispatched_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,

    environment_domain_id UUID NOT NULL REFERENCES environment_domains (id) ON DELETE CASCADE,
    dns_zone_id UUID NOT NULL REFERENCES dns_zones (id) ON DELETE RESTRICT,

    deployment_trigger_type TEXT NOT NULL DEFAULT 'user',
    deployment_reference TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_dns_bindings;
-- +goose StatementEnd
