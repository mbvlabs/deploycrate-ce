-- +goose Up
-- +goose StatementBegin
CREATE TABLE server_statuses (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE server_statuses;
-- +goose StatementEnd
