-- +goose Up
-- +goose StatementBegin
ALTER TABLE resources ADD COLUMN management_mode TEXT;

UPDATE resources AS resource
SET management_mode = CASE
    WHEN EXISTS (
        SELECT 1
        FROM resource_installations AS installation
        WHERE installation.resource_id = resource.id
          AND installation.archived_at IS NULL
    ) THEN 'managed'
    ELSE 'external'
END;

ALTER TABLE resources ALTER COLUMN management_mode SET NOT NULL;

ALTER TABLE resource_credentials ADD COLUMN digest BYTEA;
UPDATE resource_credentials SET digest = decode(repeat('00', 32), 'hex');
ALTER TABLE resource_credentials ALTER COLUMN digest SET NOT NULL;

CREATE UNIQUE INDEX resources_active_owner_name
    ON resources (owner_environment_id, lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_endpoints_active_resource_name
    ON resource_endpoints (resource_id, lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_credentials_active_resource_name
    ON resource_credentials (resource_id, lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_installations_active_server_container_name
    ON resource_installations (server_id, lower(container_name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_volumes_active_resource_name
    ON resource_volumes (resource_id, lower(name))
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_volume_mounts_active_installation_path
    ON resource_volume_mounts (resource_installation_id, mount_path)
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX resource_health_checks_active_installation_name
    ON resource_health_checks (resource_installation_id, lower(name))
    WHERE archived_at IS NULL;

ALTER TABLE resources
    ADD CONSTRAINT resources_management_mode_check
        CHECK (management_mode IN ('managed', 'external')),
    ADD CONSTRAINT resources_name_check CHECK (btrim(name) <> ''),
    ADD CONSTRAINT resources_sharing_scope_check
        CHECK (sharing_scope IN ('environment', 'application', 'global'));

ALTER TABLE resource_endpoints
    ADD CONSTRAINT resource_endpoints_name_check CHECK (btrim(name) <> ''),
    ADD CONSTRAINT resource_endpoints_port_check CHECK (port BETWEEN 1 AND 65535);

ALTER TABLE resource_credentials
    ADD CONSTRAINT resource_credentials_name_check CHECK (btrim(name) <> ''),
    ADD CONSTRAINT resource_credentials_role_check CHECK (btrim(role) <> '');

ALTER TABLE resource_installations
    ADD CONSTRAINT resource_installations_image_reference_check
        CHECK (btrim(image_reference) <> ''),
    ADD CONSTRAINT resource_installations_container_name_check
        CHECK (btrim(container_name) <> '');

ALTER TABLE resource_volumes
    ADD CONSTRAINT resource_volumes_name_check CHECK (btrim(name) <> '');

ALTER TABLE resource_volume_mounts
    ADD CONSTRAINT resource_volume_mounts_path_check
        CHECK (mount_path LIKE '/%');

ALTER TABLE resource_health_checks
    ADD CONSTRAINT resource_health_checks_name_check CHECK (btrim(name) <> ''),
    ADD CONSTRAINT resource_health_checks_interval_check CHECK (interval_seconds > 0),
    ADD CONSTRAINT resource_health_checks_timeout_check CHECK (timeout_seconds > 0),
    ADD CONSTRAINT resource_health_checks_failure_threshold_check CHECK (failure_threshold > 0),
    ADD CONSTRAINT resource_health_checks_success_threshold_check CHECK (success_threshold > 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE resource_health_checks
    DROP CONSTRAINT resource_health_checks_success_threshold_check,
    DROP CONSTRAINT resource_health_checks_failure_threshold_check,
    DROP CONSTRAINT resource_health_checks_timeout_check,
    DROP CONSTRAINT resource_health_checks_interval_check,
    DROP CONSTRAINT resource_health_checks_name_check;

ALTER TABLE resource_volume_mounts DROP CONSTRAINT resource_volume_mounts_path_check;
ALTER TABLE resource_volumes DROP CONSTRAINT resource_volumes_name_check;
ALTER TABLE resource_installations
    DROP CONSTRAINT resource_installations_container_name_check,
    DROP CONSTRAINT resource_installations_image_reference_check;
ALTER TABLE resource_credentials
    DROP CONSTRAINT resource_credentials_role_check,
    DROP CONSTRAINT resource_credentials_name_check;
ALTER TABLE resource_endpoints
    DROP CONSTRAINT resource_endpoints_port_check,
    DROP CONSTRAINT resource_endpoints_name_check;
ALTER TABLE resources
    DROP CONSTRAINT resources_sharing_scope_check,
    DROP CONSTRAINT resources_name_check,
    DROP CONSTRAINT resources_management_mode_check;

DROP INDEX resource_health_checks_active_installation_name;
DROP INDEX resource_volume_mounts_active_installation_path;
DROP INDEX resource_volumes_active_resource_name;
DROP INDEX resource_installations_active_server_container_name;
DROP INDEX resource_credentials_active_resource_name;
DROP INDEX resource_endpoints_active_resource_name;
DROP INDEX resources_active_owner_name;

ALTER TABLE resource_credentials DROP COLUMN digest;
ALTER TABLE resources DROP COLUMN management_mode;
-- +goose StatementEnd
