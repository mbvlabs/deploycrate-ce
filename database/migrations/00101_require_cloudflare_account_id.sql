-- +goose Up
-- +goose StatementBegin
ALTER TABLE dns_connections
    ADD COLUMN account_external_id TEXT NOT NULL,
    ADD CONSTRAINT dns_connections_account_external_id_check
        CHECK (account_external_id ~ '^[0-9a-f]{32}$');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE dns_connections
    DROP CONSTRAINT dns_connections_account_external_id_check,
    DROP COLUMN account_external_id;
-- +goose StatementEnd
