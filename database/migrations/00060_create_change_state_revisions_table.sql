-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_state_revisions (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    environment_state_revision_id UUID NOT NULL REFERENCES environment_state_revisions (id) ON DELETE RESTRICT,
    role TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_state_revisions;
-- +goose StatementEnd
