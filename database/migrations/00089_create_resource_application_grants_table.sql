-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_application_grants (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ,
    resource_id UUID NOT NULL REFERENCES resources (id) ON DELETE RESTRICT,
    application_id UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_application_grants;
-- +goose StatementEnd
