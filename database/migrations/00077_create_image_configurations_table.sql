-- +goose Up
-- +goose StatementBegin
CREATE TABLE image_configurations (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_source_id UUID NOT NULL REFERENCES environment_sources (id) ON DELETE CASCADE,
    registry_resource_id UUID NOT NULL REFERENCES registry_resources (resource_id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE image_configurations;
-- +goose StatementEnd
