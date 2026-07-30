-- +goose Up
-- +goose StatementBegin
ALTER TABLE backup_policies
    ADD COLUMN resource_installation_id UUID
    REFERENCES resource_installations (id) ON DELETE RESTRICT;

UPDATE backup_policies AS policy
SET resource_installation_id = endpoint.resource_installation_id,
    environment_resource_id = NULL
FROM environment_resources AS binding
JOIN resource_endpoints AS endpoint ON endpoint.id = binding.resource_endpoint_id
WHERE policy.target_type = 'resource'
  AND policy.environment_resource_id = binding.id;

ALTER TABLE backup_policies DROP CONSTRAINT backup_policies_target_check;
ALTER TABLE backup_policies
    ADD CONSTRAINT backup_policies_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_installation_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL AND
         resource_installation_id IS NOT NULL AND environment_resource_id IS NULL AND resource_volume_id IS NULL)
    );

CREATE UNIQUE INDEX backup_policies_active_resource_installation_idx
    ON backup_policies (resource_installation_id)
    WHERE archived_at IS NULL AND target_type = 'resource';

UPDATE credentials
SET metadata = metadata || jsonb_build_object(
    'schema_version', 1,
    'credential_kind', 'object_storage_backup_access'
)
WHERE provider IN ('backup_s3', 'backup_r2');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX backup_policies_active_resource_installation_idx;

ALTER TABLE backup_policies DROP CONSTRAINT backup_policies_target_check;
ALTER TABLE backup_policies
    ADD CONSTRAINT backup_policies_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL)
    );

ALTER TABLE backup_policies DROP COLUMN resource_installation_id;

UPDATE credentials
SET metadata = metadata - 'schema_version' - 'credential_kind'
WHERE provider IN ('backup_s3', 'backup_r2')
  AND metadata ->> 'credential_kind' = 'object_storage_backup_access';
-- +goose StatementEnd
