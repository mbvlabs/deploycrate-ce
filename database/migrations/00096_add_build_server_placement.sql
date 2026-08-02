-- +goose Up
-- +goose StatementBegin
ALTER TABLE buildpack_configurations
    ADD COLUMN server_id UUID REFERENCES servers (id) ON DELETE RESTRICT;

UPDATE buildpack_configurations AS buildpack
SET server_id = server.id
FROM servers AS server
WHERE server.kind = 'self_hosted'
  AND server.archived_at IS NULL
  AND buildpack.server_id IS NULL;

ALTER TABLE buildpack_configurations
    ALTER COLUMN server_id SET NOT NULL;

CREATE INDEX buildpack_configurations_server_id_idx
    ON buildpack_configurations (server_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX buildpack_configurations_server_id_idx;
ALTER TABLE buildpack_configurations DROP COLUMN server_id;
-- +goose StatementEnd
