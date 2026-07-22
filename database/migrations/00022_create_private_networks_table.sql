-- +goose Up
-- +goose StatementBegin
CREATE TABLE private_networks (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    name TEXT NOT NULL,
    cidr CIDR NOT NULL,
    scope TEXT NOT NULL,
    owner_environment_id UUID REFERENCES environments (id) ON DELETE RESTRICT,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE private_networks;
-- +goose StatementEnd
