-- +goose Up
-- +goose StatementBegin
CREATE TABLE private_networks (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    archived_at TIMESTAMPTZ,

    owner_environment_id UUID REFERENCES environments (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE private_networks;
-- +goose StatementEnd
