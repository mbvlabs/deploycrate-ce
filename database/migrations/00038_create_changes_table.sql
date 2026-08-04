-- +goose Up
-- +goose StatementBegin
CREATE TABLE changes (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    trigger_type TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id UUID,
    cause_system TEXT,
    cause_reference TEXT,
    correlation_id UUID NOT NULL,
    correction_context JSONB,
    summary TEXT NOT NULL,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    error TEXT,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    corrects_change_id UUID REFERENCES changes (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE changes;
-- +goose StatementEnd
