-- +goose Up
-- +goose StatementBegin
CREATE TABLE image_configurations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_source_id UUID NOT NULL UNIQUE REFERENCES environment_sources (id) ON DELETE RESTRICT,
    registry_resource_id UUID NOT NULL REFERENCES registry_resources (resource_id) ON DELETE RESTRICT
);

ALTER TABLE releases
    ADD COLUMN registry_resource_id UUID REFERENCES registry_resources (resource_id) ON DELETE RESTRICT,
    ADD COLUMN registry_credential_id UUID REFERENCES resource_credentials (id) ON DELETE RESTRICT,
    ADD COLUMN registry_endpoint TEXT;

UPDATE releases AS release
SET registry_resource_id = (build.build_configuration ->> 'registry_resource_id')::UUID,
    registry_credential_id = (build.build_configuration ->> 'registry_credential_id')::UUID,
    registry_endpoint = build.build_configuration ->> 'registry_endpoint'
FROM builds AS build
WHERE build.id = release.build_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE releases
    DROP COLUMN registry_endpoint,
    DROP COLUMN registry_credential_id,
    DROP COLUMN registry_resource_id;

DROP TABLE image_configurations;
-- +goose StatementEnd
