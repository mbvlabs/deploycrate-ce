-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_volume_mounts (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    mount_path TEXT NOT NULL,
    read_only BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ,

    resource_volume_id UUID NOT NULL REFERENCES resource_volumes (id) ON DELETE RESTRICT,
    resource_installation_id UUID NOT NULL REFERENCES resource_installations (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_volume_mounts;
-- +goose StatementEnd
