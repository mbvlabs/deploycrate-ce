-- +goose Up
-- +goose StatementBegin
CREATE TABLE resource_binding_credentials (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    resource_binding_id UUID NOT NULL REFERENCES resource_bindings (id) ON DELETE RESTRICT,
    resource_credential_id UUID NOT NULL REFERENCES resource_credentials (id) ON DELETE RESTRICT,
    generation INTEGER NOT NULL,
    state TEXT NOT NULL,
    activated_at TIMESTAMPTZ,
    retired_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE resource_binding_credentials;
-- +goose StatementEnd
