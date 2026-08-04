-- +goose Up
-- +goose StatementBegin
CREATE TABLE environment_target_states (
    id SERIAL NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    observed_state JSONB,
    state TEXT NOT NULL,
    observed_at TIMESTAMPTZ,

    environment_target_id UUID NOT NULL REFERENCES environment_targets (id) ON DELETE CASCADE,
    desired_revision_id UUID REFERENCES environment_state_revisions (id) ON DELETE SET NULL,
    applying_revision_id UUID REFERENCES environment_state_revisions (id) ON DELETE SET NULL,
    applied_revision_id UUID REFERENCES environment_state_revisions (id) ON DELETE SET NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE environment_target_states;
-- +goose StatementEnd
