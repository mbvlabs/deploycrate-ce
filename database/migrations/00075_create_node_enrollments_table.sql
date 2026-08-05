-- +goose Up
-- +goose StatementBegin
CREATE TABLE node_enrollments (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    state TEXT NOT NULL,
    current_step TEXT NOT NULL,
    error TEXT,
    host_fingerprint TEXT NOT NULL,
    allocated_address TEXT NOT NULL,
    installer_version TEXT NOT NULL,
    job_id BIGINT,

    server_id UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE node_enrollments;
-- +goose StatementEnd
