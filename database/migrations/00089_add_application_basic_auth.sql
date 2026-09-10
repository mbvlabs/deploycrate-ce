-- +goose Up
-- +goose StatementBegin
ALTER TABLE applications
    ADD COLUMN basic_auth_username TEXT NOT NULL DEFAULT '',
    ADD COLUMN basic_auth_password_hash TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE applications
    DROP COLUMN basic_auth_password_hash,
    DROP COLUMN basic_auth_username;
-- +goose StatementEnd
