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
    status TEXT NOT NULL CHECK (status IN ('received', 'processing', 'processed', 'ignored', 'failed')),
    error TEXT
);

CREATE INDEX github_webhook_deliveries_provider_lookup
    ON github_webhook_deliveries (installation_external_id, repository_external_id, received_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE github_webhook_deliveries;
-- +goose StatementEnd
