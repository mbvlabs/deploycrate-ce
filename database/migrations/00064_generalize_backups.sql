-- +goose Up
-- +goose StatementBegin
ALTER TABLE backup_destinations
    ADD CONSTRAINT backup_destinations_provider_check
        CHECK (provider IN ('s3', 'r2')),
    ADD CONSTRAINT backup_destinations_bucket_check
        CHECK (length(btrim(bucket)) > 0),
    ADD CONSTRAINT backup_destinations_endpoint_check
        CHECK (endpoint IS NULL OR endpoint ~ '^https://[^/[:space:]]+'),
    ADD CONSTRAINT backup_destinations_region_check
        CHECK ((provider = 's3' AND region IS NOT NULL AND length(btrim(region)) > 0) OR
               (provider = 'r2' AND endpoint IS NOT NULL AND region = 'auto')),
    ADD CONSTRAINT backup_destinations_prefix_check
        CHECK (prefix IS NULL OR (prefix = btrim(prefix, '/') AND prefix !~ '(^|/)\.\.?(/|$)'));

ALTER TABLE backup_policies
    ALTER COLUMN resource_id DROP NOT NULL,
    ADD COLUMN target_type TEXT NOT NULL DEFAULT 'resource',
    ADD COLUMN server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,
    ADD COLUMN next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN last_scheduled_at TIMESTAMPTZ;

ALTER TABLE backup_policies
    ALTER COLUMN target_type DROP DEFAULT,
    ALTER COLUMN next_run_at DROP DEFAULT,
    ADD CONSTRAINT backup_policies_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL)
    ),
    ADD CONSTRAINT backup_policies_driver_check CHECK (
        (target_type = 'server' AND strategy = 'filesystem' AND driver = 'restic' AND format = 'restic') OR
        (target_type = 'resource' AND strategy = 'logical' AND driver = 'postgresql' AND format = 'tar.age')
    );

ALTER TABLE resource_backups RENAME TO backups;
ALTER TABLE backups RENAME COLUMN object_key TO artifact_reference;
ALTER TABLE backups
    ALTER COLUMN resource_id DROP NOT NULL,
    ADD COLUMN target_type TEXT NOT NULL DEFAULT 'resource',
    ADD COLUMN server_id UUID REFERENCES servers (id) ON DELETE RESTRICT,
    ADD COLUMN scheduled_at TIMESTAMPTZ,
    ADD COLUMN format_version TEXT NOT NULL DEFAULT '1',
    ADD COLUMN provider_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN uploaded_at TIMESTAMPTZ,
    ADD COLUMN pruned_at TIMESTAMPTZ,
    ADD COLUMN producer_version TEXT NOT NULL DEFAULT 'unknown';

UPDATE backups SET scheduled_at = requested_at WHERE scheduled_at IS NULL;

ALTER TABLE backups
    ALTER COLUMN target_type DROP DEFAULT,
    ALTER COLUMN scheduled_at SET NOT NULL,
    ALTER COLUMN format_version DROP DEFAULT,
    ALTER COLUMN provider_metadata DROP DEFAULT,
    ALTER COLUMN producer_version DROP DEFAULT,
    ADD CONSTRAINT backups_target_check CHECK (
        (target_type = 'server' AND server_id IS NOT NULL AND resource_id IS NULL AND
         environment_resource_id IS NULL AND resource_installation_id IS NULL AND resource_volume_id IS NULL) OR
        (target_type = 'resource' AND server_id IS NULL AND resource_id IS NOT NULL)
    ),
    ADD CONSTRAINT backups_driver_check CHECK (
        (target_type = 'server' AND strategy = 'filesystem' AND driver = 'restic' AND format = 'restic') OR
        (target_type = 'resource' AND strategy = 'logical' AND driver = 'postgresql' AND format = 'tar.age')
    ),
    ADD CONSTRAINT backups_trigger_check CHECK (trigger_type IN ('installer', 'schedule', 'manual')),
    ADD CONSTRAINT backups_status_check CHECK (
        status IN ('pending', 'running', 'uploaded', 'verified', 'verification_failed', 'failed', 'pruned')
    ),
    ADD CONSTRAINT backups_policy_scheduled_slot_unique UNIQUE (backup_policy_id, scheduled_at);

ALTER TABLE resource_restores RENAME COLUMN resource_backup_id TO backup_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- This migration is intentionally forward-only. Reversing it after a server
-- backup exists would either discard that data or violate the old resource-only
-- constraints.
DO $$
BEGIN
    RAISE EXCEPTION 'migration 00064 is forward-only';
END
$$;
-- +goose StatementEnd
