-- +goose Up
-- +goose StatementBegin
CREATE TABLE build_logs (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    sequence BIGINT NOT NULL,
    stream TEXT NOT NULL,
    message TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,

    build_id UUID NOT NULL REFERENCES builds (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE build_logs;
-- +goose StatementEnd
