-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_environment_grants (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    environment_id UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_environment_grants;
-- +goose StatementEnd
