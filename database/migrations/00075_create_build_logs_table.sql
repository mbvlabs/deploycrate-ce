-- +goose Up
-- +goose StatementBegin
ALTER TABLE builds ADD COLUMN current_step TEXT;

CREATE TABLE build_logs (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    sequence BIGINT NOT NULL,
    stream TEXT NOT NULL CHECK (stream IN ('system', 'pack')),
    message TEXT NOT NULL CHECK (octet_length(message) BETWEEN 1 AND 65536),
    occurred_at TIMESTAMPTZ NOT NULL,

    build_id UUID NOT NULL REFERENCES builds (id) ON DELETE CASCADE,

    UNIQUE (build_id, sequence)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE build_logs;
ALTER TABLE builds DROP COLUMN current_step;
-- +goose StatementEnd
