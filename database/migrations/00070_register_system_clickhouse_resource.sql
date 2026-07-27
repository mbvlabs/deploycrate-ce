-- +goose Up
-- +goose StatementBegin
WITH system_environment AS (
    SELECT environment.id
    FROM environments AS environment
    JOIN applications AS application ON application.id = environment.application_id
    WHERE application.slug = 'deploycrate-ce'
      AND application.archived_at IS NULL
      AND environment.archived_at IS NULL
    ORDER BY environment.created_at
    LIMIT 1
)
INSERT INTO resources (
    id,
    created_at,
    updated_at,
    name,
    category,
    kind,
    sharing_scope,
    owner_environment_id
)
SELECT
    md5('deploycrate-ce-clickhouse-resource:' || environment.id::text)::uuid,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    'DeployCrate CE ClickHouse',
    'database',
    'clickhouse',
    'environment',
    environment.id
FROM system_environment AS environment
ON CONFLICT (id) DO NOTHING;

WITH system_topology AS (
    SELECT
        environment.id AS environment_id,
        target.server_id,
        md5('deploycrate-ce-clickhouse-resource:' || environment.id::text)::uuid AS resource_id
    FROM environments AS environment
    JOIN applications AS application ON application.id = environment.application_id
    JOIN LATERAL (
        SELECT environment_target.server_id
        FROM environment_targets AS environment_target
        WHERE environment_target.environment_id = environment.id
          AND environment_target.detached_at IS NULL
        ORDER BY environment_target.attached_at DESC
        LIMIT 1
    ) AS target ON TRUE
    WHERE application.slug = 'deploycrate-ce'
      AND application.archived_at IS NULL
      AND environment.archived_at IS NULL
    ORDER BY environment.created_at
    LIMIT 1
)
INSERT INTO resource_installations (
    id,
    created_at,
    updated_at,
    image_reference,
    container_name,
    restart_policy,
    configuration,
    resource_id,
    server_id
)
SELECT
    md5('deploycrate-ce-clickhouse-installation:' || topology.environment_id::text)::uuid,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    'clickhouse/clickhouse-server:25.8.28.1',
    'deploycrate-ce-clickhouse',
    'unless-stopped',
    '{"volume":"deploycrate-ce-clickhouse","bind":"127.0.0.1:8123"}'::jsonb,
    topology.resource_id,
    topology.server_id
FROM system_topology AS topology
JOIN resources AS resource ON resource.id = topology.resource_id
ON CONFLICT (id) DO NOTHING;

WITH system_environment AS (
    SELECT environment.id, network.private_network_id
    FROM environments AS environment
    JOIN applications AS application ON application.id = environment.application_id
    LEFT JOIN LATERAL (
        SELECT environment_network.private_network_id
        FROM environment_networks AS environment_network
        WHERE environment_network.environment_id = environment.id
          AND environment_network.role = 'primary'
          AND environment_network.removed_at IS NULL
        LIMIT 1
    ) AS network ON TRUE
    WHERE application.slug = 'deploycrate-ce'
      AND application.archived_at IS NULL
      AND environment.archived_at IS NULL
    ORDER BY environment.created_at
    LIMIT 1
)
INSERT INTO resource_endpoints (
    id,
    created_at,
    updated_at,
    name,
    role,
    address,
    port,
    protocol,
    tls_mode,
    settings,
    resource_id,
    resource_installation_id,
    private_network_id
)
SELECT
    md5('deploycrate-ce-clickhouse-endpoint:' || environment.id::text)::uuid,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    'ClickHouse HTTP',
    'primary',
    '127.0.0.1',
    8123,
    'http',
    'disable',
    '{"database":"deploycrate","user":"deploycrate"}'::jsonb,
    md5('deploycrate-ce-clickhouse-resource:' || environment.id::text)::uuid,
    md5('deploycrate-ce-clickhouse-installation:' || environment.id::text)::uuid,
    environment.private_network_id
FROM system_environment AS environment
JOIN resource_installations AS installation
  ON installation.id = md5(
      'deploycrate-ce-clickhouse-installation:' || environment.id::text
  )::uuid
ON CONFLICT (id) DO NOTHING;

WITH system_environment AS (
    SELECT environment.id
    FROM environments AS environment
    JOIN applications AS application ON application.id = environment.application_id
    WHERE application.slug = 'deploycrate-ce'
      AND application.archived_at IS NULL
      AND environment.archived_at IS NULL
    ORDER BY environment.created_at
    LIMIT 1
)
INSERT INTO environment_resources (
    id,
    created_at,
    updated_at,
    alias,
    configuration,
    environment_id,
    resource_id,
    resource_endpoint_id
)
SELECT
    md5('deploycrate-ce-clickhouse-binding:' || environment.id::text)::uuid,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    'telemetry',
    '{"credential_source":"app_env","password_env":"CLICKHOUSE_PASSWORD"}'::jsonb,
    environment.id,
    md5('deploycrate-ce-clickhouse-resource:' || environment.id::text)::uuid,
    md5('deploycrate-ce-clickhouse-endpoint:' || environment.id::text)::uuid
FROM system_environment AS environment
JOIN resource_endpoints AS endpoint
  ON endpoint.id = md5('deploycrate-ce-clickhouse-endpoint:' || environment.id::text)::uuid
ON CONFLICT (environment_id, alias) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM environment_resources
WHERE alias = 'telemetry'
  AND resource_id IN (
      SELECT resource.id
      FROM resources AS resource
      WHERE resource.name = 'DeployCrate CE ClickHouse'
        AND resource.kind = 'clickhouse'
  );

DELETE FROM resource_endpoints
WHERE resource_id IN (
    SELECT resource.id
    FROM resources AS resource
    WHERE resource.name = 'DeployCrate CE ClickHouse'
      AND resource.kind = 'clickhouse'
);

DELETE FROM resource_installations
WHERE resource_id IN (
    SELECT resource.id
    FROM resources AS resource
    WHERE resource.name = 'DeployCrate CE ClickHouse'
      AND resource.kind = 'clickhouse'
);

DELETE FROM resources
WHERE name = 'DeployCrate CE ClickHouse'
  AND kind = 'clickhouse';
-- +goose StatementEnd
