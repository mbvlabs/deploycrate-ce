-- +goose Up
-- +goose StatementBegin
CREATE TABLE change_items (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    action TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    previous_value JSONB,
    requested_value JSONB,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE change_items;
-- +goose StatementEnd
