-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_health_check_statuses (
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    status_code INTEGER,
    latency_ms INTEGER,
    message TEXT,
    consecutive_successes INTEGER NOT NULL,
    consecutive_failures INTEGER NOT NULL,
    details JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    health_check_id UUID NOT NULL PRIMARY KEY REFERENCES resource_health_checks (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_health_check_statuses;
-- +goose StatementEnd
