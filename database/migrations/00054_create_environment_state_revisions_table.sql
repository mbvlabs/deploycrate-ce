-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_state_revisions (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    state JSONB NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_state_revisions;
-- +goose StatementEnd
