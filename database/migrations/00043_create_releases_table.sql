-- +goose Up
-- +goose StatementBegin
CREATE TABLE releases (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    environment_source_id UUID REFERENCES environment_sources (id) ON DELETE RESTRICT,
    build_id UUID REFERENCES builds (id) ON DELETE RESTRICT,
    created_by_change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    version TEXT,
    source_revision TEXT,
    artifact_reference TEXT NOT NULL,
    artifact_digest BYTEA NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE releases;
-- +goose StatementEnd
