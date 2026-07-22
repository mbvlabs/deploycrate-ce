-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_items (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    action TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    previous_value JSONB,
    requested_value JSONB
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_items;
-- +goose StatementEnd
