-- +goose Up
-- +goose StatementBegin
CREATE TABLE database_clusters (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    engine TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    sharing_mode TEXT NOT NULL,
    management_mode TEXT NOT NULL,
    desired_installation_method TEXT,
    topology JSONB NOT NULL,
    maintenance_policy JSONB NOT NULL,
    archived_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE database_clusters;
-- +goose StatementEnd
