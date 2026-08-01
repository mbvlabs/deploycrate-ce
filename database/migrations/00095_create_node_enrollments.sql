-- +goose Up
-- +goose StatementBegin
ALTER TABLE server_ssh_credentials
    ALTER COLUMN enc_private_key DROP NOT NULL,
    ADD COLUMN enc_private_key_passphrase BYTEA,
    ADD COLUMN host_key_confirmed_at TIMESTAMPTZ;

CREATE UNIQUE INDEX server_ssh_credentials_server_id_unique
    ON server_ssh_credentials (server_id);

ALTER TABLE servers ADD CONSTRAINT worker_server_capabilities_check CHECK (
    kind <> 'worker' OR (
        capabilities @> '{"telemetry":true}'::jsonb AND
        (
            capabilities @> '{"build":true}'::jsonb OR
            capabilities @> '{"runtime":true}'::jsonb OR
            capabilities @> '{"resource":true}'::jsonb OR
            capabilities @> '{"database":true}'::jsonb OR
            capabilities @> '{"repository":true}'::jsonb
        )
    )
);

CREATE TABLE node_enrollments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    state TEXT NOT NULL,
    current_step TEXT NOT NULL,
    error TEXT,
    host_fingerprint TEXT NOT NULL,
    allocated_address TEXT NOT NULL,
    installer_version TEXT NOT NULL,
    job_id BIGINT,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,

    CONSTRAINT node_enrollments_state_check CHECK (
        state IN ('awaiting_confirmation', 'queued', 'installing', 'verifying', 'ready', 'failed')
    )
);

CREATE UNIQUE INDEX node_enrollments_active_server_unique
    ON node_enrollments (server_id)
    WHERE state NOT IN ('ready', 'failed');

CREATE UNIQUE INDEX node_enrollments_allocated_address_unique
    ON node_enrollments (allocated_address);

CREATE VIEW wireguard_address_reservations AS
    SELECT host(private_address) AS private_address FROM wireguard_peers
    UNION
    SELECT host(private_address) AS private_address FROM wireguard_devices
    UNION
    SELECT allocated_address AS private_address FROM node_enrollments;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW wireguard_address_reservations;
DROP TABLE node_enrollments;
ALTER TABLE servers DROP CONSTRAINT worker_server_capabilities_check;
DROP INDEX server_ssh_credentials_server_id_unique;
DELETE FROM server_ssh_credentials WHERE enc_private_key IS NULL;
ALTER TABLE server_ssh_credentials
    DROP COLUMN host_key_confirmed_at,
    DROP COLUMN enc_private_key_passphrase,
    ALTER COLUMN enc_private_key SET NOT NULL;
-- +goose StatementEnd
