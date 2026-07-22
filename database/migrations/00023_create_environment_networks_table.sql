-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_networks (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    private_network_id UUID NOT NULL REFERENCES private_networks (id) ON DELETE RESTRICT,
    role TEXT NOT NULL,
    removed_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_networks;
-- +goose StatementEnd
