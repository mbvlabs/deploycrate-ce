-- +goose Up
-- +goose StatementBegin
ALTER TABLE environment_dns_bindings
    ADD COLUMN deployment_trigger_type TEXT NOT NULL DEFAULT 'user',
    ADD COLUMN deployment_reference TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE environment_dns_bindings
    DROP COLUMN deployment_reference,
    DROP COLUMN deployment_trigger_type;
-- +goose StatementEnd
