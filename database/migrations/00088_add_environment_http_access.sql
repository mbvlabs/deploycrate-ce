-- +goose Up
-- +goose StatementBegin
ALTER TABLE environments
    ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'public',
    ADD COLUMN basic_auth_username TEXT NOT NULL DEFAULT '',
    ADD COLUMN basic_auth_password_hash TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE environments
    DROP COLUMN basic_auth_password_hash,
    DROP COLUMN basic_auth_username,
    DROP COLUMN access_mode;
-- +goose StatementEnd
