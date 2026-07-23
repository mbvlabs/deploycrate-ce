-- +goose Up
-- +goose StatementBegin
CREATE TABLE servers (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,

    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    kind TEXT NOT NULL,
    capabilities JSONB NOT NULL,
    operating_system TEXT,
    distribution TEXT,
    distribution_version TEXT,
    architecture TEXT,
    package_manager TEXT,
    init_system TEXT,
    ipv4_address text NOT NULL,
    ipv6_address text NOT NULL,
	is_configured boolean NOT NULL,

    address TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE servers;
-- +goose StatementEnd
