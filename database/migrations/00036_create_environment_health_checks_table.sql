-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_health_checks (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    url TEXT NOT NULL,
    method TEXT NOT NULL,
    expected_status INTEGER NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    interval_seconds INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL,
    archived_at TIMESTAMPTZ,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_health_checks;
-- +goose StatementEnd
