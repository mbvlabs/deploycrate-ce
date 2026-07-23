-- +goose Up
-- +goose StatementBegin
CREATE TABLE builds (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    source_revision TEXT NOT NULL,
    build_method TEXT NOT NULL,
    build_configuration JSONB NOT NULL,
    status TEXT NOT NULL,
    artifact_reference TEXT,
    artifact_digest BYTEA,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error TEXT,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    environment_source_id UUID NOT NULL REFERENCES environment_sources (id) ON DELETE RESTRICT,
    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE builds;
-- +goose StatementEnd
