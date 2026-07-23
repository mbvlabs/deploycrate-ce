-- +goose Up
-- +goose StatementBegin
CREATE TABLE source_events (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    external_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    source_revision TEXT,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    error TEXT,

    environment_source_id UUID NOT NULL REFERENCES environment_sources (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE source_events;
-- +goose StatementEnd
