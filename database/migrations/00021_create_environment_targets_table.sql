-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_targets (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    attached_at TIMESTAMPTZ NOT NULL,
    detached_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_targets;
-- +goose StatementEnd
