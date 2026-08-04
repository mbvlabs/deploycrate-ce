-- +goose Up
-- +goose StatementBegin
CREATE TABLE dns_connections (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    verified_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,

    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT,

    CONSTRAINT dns_connections_provider_check CHECK (provider = 'cloudflare')
);

CREATE UNIQUE INDEX dns_connections_active_name_unique
    ON dns_connections (lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX dns_connections_active_credential_unique
    ON dns_connections (credential_id)
    WHERE archived_at IS NULL;

CREATE TABLE dns_zones (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    dns_connection_id UUID NOT NULL REFERENCES dns_connections (id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX dns_zones_connection_external_unique
    ON dns_zones (dns_connection_id, external_id);

CREATE UNIQUE INDEX dns_zones_connection_name_unique
    ON dns_zones (dns_connection_id, lower(name));

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

    CONSTRAINT environment_dns_bindings_state_check CHECK (
        state IN ('pending', 'reconciling', 'applied', 'conflict', 'failed', 'removing', 'removal_failed')
    ),
    CONSTRAINT environment_dns_bindings_generation_check CHECK (
        generation > 0 AND applied_generation >= 0 AND applied_generation <= generation
    )
);

CREATE UNIQUE INDEX environment_dns_bindings_active_domain_unique
    ON environment_dns_bindings (environment_domain_id)
    WHERE archived_at IS NULL;

CREATE TABLE environment_dns_records (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    record_type TEXT NOT NULL,
    content TEXT NOT NULL,
    observed_name TEXT NOT NULL,
    archived_at TIMESTAMPTZ,

    environment_dns_binding_id UUID NOT NULL REFERENCES environment_dns_bindings (id) ON DELETE CASCADE,
    dns_zone_id UUID NOT NULL REFERENCES dns_zones (id) ON DELETE RESTRICT,

    CONSTRAINT environment_dns_records_type_check CHECK (record_type = 'A')
);

CREATE UNIQUE INDEX environment_dns_records_binding_external_unique
    ON environment_dns_records (environment_dns_binding_id, external_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_dns_records;
DROP TABLE environment_dns_bindings;
DROP TABLE dns_zones;
DROP TABLE dns_connections;
-- +goose StatementEnd
