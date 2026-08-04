-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_releases (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE CASCADE,
    release_id UUID NOT NULL REFERENCES releases (id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_releases;
-- +goose StatementEnd
