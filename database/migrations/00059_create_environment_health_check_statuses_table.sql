-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_health_check_statuses (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    state TEXT NOT NULL,
    status_code INTEGER,
    duration_ms INTEGER NOT NULL,
    error TEXT,
    observed_at TIMESTAMPTZ NOT NULL,

    health_check_id UUID NOT NULL REFERENCES environment_health_checks (id) ON DELETE RESTRICT,
    environment_target_id UUID REFERENCES environment_targets (id) ON DELETE RESTRICT,
    instance_id UUID REFERENCES instances (id) ON DELETE RESTRICT,
    release_id UUID REFERENCES releases (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_health_check_statuses;
-- +goose StatementEnd
