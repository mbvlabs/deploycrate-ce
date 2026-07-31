-- +goose Up
-- +goose StatementBegin
CREATE TABLE backups (
    id UUID NOT NULL PRIMARY KEY,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    trigger_type TEXT NOT NULL,
    strategy TEXT NOT NULL,
    driver TEXT NOT NULL,
    format TEXT NOT NULL,
    artifact_reference TEXT NOT NULL,
    status TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    size_bytes BIGINT,
    digest BYTEA,
    verified_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ,
    pruned_at TIMESTAMPTZ,
    error TEXT,
    target_type TEXT NOT NULL,
    format_version TEXT NOT NULL,
    provider_metadata JSONB NOT NULL,
    producer_version TEXT NOT NULL,

    change_id UUID NOT NULL REFERENCES changes (id) ON DELETE RESTRICT,
    change_task_id UUID NOT NULL REFERENCES change_tasks (id) ON DELETE RESTRICT,
    backup_policy_id UUID NOT NULL REFERENCES backup_policies (id) ON DELETE RESTRICT,
    database_id UUID REFERENCES databases (id) ON DELETE RESTRICT,
    database_cluster_id UUID REFERENCES database_clusters (id) ON DELETE RESTRICT,
    database_cluster_node_id UUID REFERENCES database_cluster_nodes (id) ON DELETE RESTRICT,
    database_node_installation_id UUID REFERENCES database_node_installations (id) ON DELETE RESTRICT,
    backup_destination_id UUID NOT NULL REFERENCES backup_destinations (id) ON DELETE RESTRICT,
    server_id UUID REFERENCES servers (id) ON DELETE RESTRICT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE backups;
-- +goose StatementEnd
