-- +goose Up
-- +goose StatementBegin
CREATE TABLE backup_destinations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    endpoint TEXT,
    region TEXT,
    bucket TEXT NOT NULL,
    prefix TEXT,
    force_path_style BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ,

    credential_id UUID NOT NULL REFERENCES credentials (id) ON DELETE RESTRICT,

    CONSTRAINT backup_destinations_provider_check CHECK (provider IN ('s3', 'r2')),
    CONSTRAINT backup_destinations_bucket_check CHECK (length(btrim(bucket)) > 0),
    CONSTRAINT backup_destinations_endpoint_check CHECK (endpoint IS NULL OR endpoint ~ '^https://[^/[:space:]]+'),
    CONSTRAINT backup_destinations_region_check CHECK (
        (provider = 's3' AND region IS NOT NULL AND length(btrim(region)) > 0) OR
        (provider = 'r2' AND endpoint IS NOT NULL AND region = 'auto')
    ),
    CONSTRAINT backup_destinations_prefix_check CHECK (
        prefix IS NULL OR (prefix = btrim(prefix, '/') AND prefix !~ '(^|/)\.\.?(/|$)')
    )
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backup_destinations;
-- +goose StatementEnd
