-- +goose Up
-- +goose StatementBegin
CREATE TABLE github_webhook_deliveries (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    delivery_id TEXT NOT NULL,
    event TEXT NOT NULL,
    action TEXT,
    installation_external_id BIGINT,
    repository_external_id BIGINT,
    body_digest BYTEA NOT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    error TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_webhook_deliveries;
-- +goose StatementEnd
