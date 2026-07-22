-- +goose Up
-- +goose StatementBegin
CREATE TABLE applications (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE applications;
-- +goose StatementEnd
