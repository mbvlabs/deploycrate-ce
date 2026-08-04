-- +goose Up
-- +goose StatementBegin
UPDATE environment_resources AS connection
SET archived_at = COALESCE(environment.archived_at, application.archived_at),
    updated_at = CURRENT_TIMESTAMP
FROM environments AS environment
JOIN applications AS application ON application.id = environment.application_id
WHERE connection.environment_id = environment.id
  AND connection.archived_at IS NULL
  AND (environment.archived_at IS NOT NULL OR application.archived_at IS NOT NULL);

UPDATE environment_domains AS domain
SET archived_at = COALESCE(environment.archived_at, application.archived_at),
    updated_at = CURRENT_TIMESTAMP
FROM environments AS environment
JOIN applications AS application ON application.id = environment.application_id
WHERE domain.environment_id = environment.id
  AND domain.archived_at IS NULL
  AND (environment.archived_at IS NOT NULL OR application.archived_at IS NOT NULL);

ALTER TABLE environments DROP CONSTRAINT environments_application_id_fkey,
    ADD CONSTRAINT environments_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications (id) ON DELETE CASCADE;

ALTER TABLE environment_sources DROP CONSTRAINT environment_sources_environment_id_fkey,
    ADD CONSTRAINT environment_sources_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE runtime_configurations DROP CONSTRAINT runtime_configurations_environment_id_fkey,
    ADD CONSTRAINT runtime_configurations_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_secrets DROP CONSTRAINT environment_secrets_environment_id_fkey,
    ADD CONSTRAINT environment_secrets_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_targets DROP CONSTRAINT environment_targets_environment_id_fkey,
    ADD CONSTRAINT environment_targets_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE private_networks DROP CONSTRAINT private_networks_owner_environment_id_fkey,
    ADD CONSTRAINT private_networks_owner_environment_id_fkey FOREIGN KEY (owner_environment_id) REFERENCES environments (id) ON DELETE SET NULL;
ALTER TABLE environment_networks DROP CONSTRAINT environment_networks_environment_id_fkey,
    ADD CONSTRAINT environment_networks_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_resources DROP CONSTRAINT environment_resources_environment_id_fkey,
    ADD CONSTRAINT environment_resources_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_domains DROP CONSTRAINT environment_domains_environment_id_fkey,
    ADD CONSTRAINT environment_domains_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_health_checks DROP CONSTRAINT environment_health_checks_environment_id_fkey,
    ADD CONSTRAINT environment_health_checks_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE changes DROP CONSTRAINT changes_environment_id_fkey,
    ADD CONSTRAINT changes_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE builds DROP CONSTRAINT builds_environment_id_fkey,
    ADD CONSTRAINT builds_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE releases DROP CONSTRAINT releases_environment_id_fkey,
    ADD CONSTRAINT releases_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;
ALTER TABLE environment_state_revisions DROP CONSTRAINT environment_state_revisions_environment_id_fkey,
    ADD CONSTRAINT environment_state_revisions_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE CASCADE;

ALTER TABLE buildpack_configurations DROP CONSTRAINT buildpack_configurations_environment_source_id_fkey,
    ADD CONSTRAINT buildpack_configurations_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE CASCADE;
ALTER TABLE image_configurations DROP CONSTRAINT image_configurations_environment_source_id_fkey,
    ADD CONSTRAINT image_configurations_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE CASCADE;
ALTER TABLE source_events DROP CONSTRAINT source_events_environment_source_id_fkey,
    ADD CONSTRAINT source_events_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE CASCADE;
ALTER TABLE builds DROP CONSTRAINT builds_environment_source_id_fkey,
    ADD CONSTRAINT builds_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE CASCADE;
ALTER TABLE releases DROP CONSTRAINT releases_environment_source_id_fkey,
    ADD CONSTRAINT releases_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE CASCADE;

ALTER TABLE environment_target_networks DROP CONSTRAINT environment_target_networks_environment_target_id_fkey,
    ADD CONSTRAINT environment_target_networks_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_environment_target_id_fkey,
    ADD CONSTRAINT environment_target_states_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_environment_target_id_fkey,
    ADD CONSTRAINT change_tasks_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE SET NULL;
ALTER TABLE deployments DROP CONSTRAINT deployments_environment_target_id_fkey,
    ADD CONSTRAINT deployments_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;
ALTER TABLE instances DROP CONSTRAINT instances_environment_target_id_fkey,
    ADD CONSTRAINT instances_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_environment_target_id_fkey,
    ADD CONSTRAINT caddy_routes_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_environment_domain_id_fkey,
    ADD CONSTRAINT caddy_routes_environment_domain_id_fkey FOREIGN KEY (environment_domain_id) REFERENCES environment_domains (id) ON DELETE CASCADE;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_environment_target_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE CASCADE;

ALTER TABLE network_access_rules DROP CONSTRAINT network_access_rules_environment_resource_id_fkey,
    ADD CONSTRAINT network_access_rules_environment_resource_id_fkey FOREIGN KEY (environment_resource_id) REFERENCES environment_resources (id) ON DELETE CASCADE;
ALTER TABLE network_access_rule_applications DROP CONSTRAINT network_access_rule_applications_network_access_rule_id_fkey,
    ADD CONSTRAINT network_access_rule_applications_network_access_rule_id_fkey FOREIGN KEY (network_access_rule_id) REFERENCES network_access_rules (id) ON DELETE CASCADE;
DO $$
DECLARE
    existing_constraint TEXT;
BEGIN
    SELECT constraint_record.conname
    INTO existing_constraint
    FROM pg_constraint AS constraint_record
    JOIN pg_attribute AS column_record
      ON column_record.attrelid = constraint_record.conrelid
     AND column_record.attnum = ANY (constraint_record.conkey)
    WHERE constraint_record.conrelid = 'network_access_rule_applications'::regclass
      AND constraint_record.contype = 'f'
      AND column_record.attname = 'environment_target_network_id'
    LIMIT 1;

    IF existing_constraint IS NULL THEN
        RAISE EXCEPTION 'Environment target network foreign key is unavailable';
    END IF;

    EXECUTE format(
        'ALTER TABLE network_access_rule_applications DROP CONSTRAINT %I',
        existing_constraint
    );
END $$;
ALTER TABLE network_access_rule_applications
    ADD CONSTRAINT nara_environment_target_network_id_fkey FOREIGN KEY (environment_target_network_id) REFERENCES environment_target_networks (id) ON DELETE CASCADE;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_health_check_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_health_check_id_fkey FOREIGN KEY (health_check_id) REFERENCES environment_health_checks (id) ON DELETE CASCADE;

ALTER TABLE changes DROP CONSTRAINT changes_corrects_change_id_fkey,
    ADD CONSTRAINT changes_corrects_change_id_fkey FOREIGN KEY (corrects_change_id) REFERENCES changes (id) ON DELETE SET NULL;
ALTER TABLE change_items DROP CONSTRAINT change_items_change_id_fkey,
    ADD CONSTRAINT change_items_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_change_id_fkey,
    ADD CONSTRAINT change_tasks_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_parent_task_id_fkey,
    ADD CONSTRAINT change_tasks_parent_task_id_fkey FOREIGN KEY (parent_task_id) REFERENCES change_tasks (id) ON DELETE SET NULL;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_id_fkey,
    ADD CONSTRAINT change_logs_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE builds DROP CONSTRAINT builds_change_id_fkey,
    ADD CONSTRAINT builds_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE releases DROP CONSTRAINT releases_created_by_change_id_fkey,
    ADD CONSTRAINT releases_created_by_change_id_fkey FOREIGN KEY (created_by_change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE deployments DROP CONSTRAINT deployments_change_id_fkey,
    ADD CONSTRAINT deployments_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE environment_state_revisions DROP CONSTRAINT environment_state_revisions_change_id_fkey,
    ADD CONSTRAINT environment_state_revisions_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE change_releases DROP CONSTRAINT change_releases_change_id_fkey,
    ADD CONSTRAINT change_releases_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;
ALTER TABLE change_state_revisions DROP CONSTRAINT change_state_revisions_change_id_fkey,
    ADD CONSTRAINT change_state_revisions_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE CASCADE;

ALTER TABLE change_task_attempts DROP CONSTRAINT change_task_attempts_change_task_id_fkey,
    ADD CONSTRAINT change_task_attempts_change_task_id_fkey FOREIGN KEY (change_task_id) REFERENCES change_tasks (id) ON DELETE CASCADE;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_task_id_fkey,
    ADD CONSTRAINT change_logs_change_task_id_fkey FOREIGN KEY (change_task_id) REFERENCES change_tasks (id) ON DELETE CASCADE;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_task_attempt_id_fkey,
    ADD CONSTRAINT change_logs_change_task_attempt_id_fkey FOREIGN KEY (change_task_attempt_id) REFERENCES change_task_attempts (id) ON DELETE CASCADE;
ALTER TABLE deployment_events DROP CONSTRAINT deployment_events_change_task_attempt_id_fkey,
    ADD CONSTRAINT deployment_events_change_task_attempt_id_fkey FOREIGN KEY (change_task_attempt_id) REFERENCES change_task_attempts (id) ON DELETE SET NULL;

ALTER TABLE releases DROP CONSTRAINT releases_build_id_fkey,
    ADD CONSTRAINT releases_build_id_fkey FOREIGN KEY (build_id) REFERENCES builds (id) ON DELETE CASCADE;
ALTER TABLE deployments DROP CONSTRAINT deployments_release_id_fkey,
    ADD CONSTRAINT deployments_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE CASCADE;
ALTER TABLE instances DROP CONSTRAINT instances_release_id_fkey,
    ADD CONSTRAINT instances_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE CASCADE;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_release_id_fkey,
    ADD CONSTRAINT caddy_routes_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE CASCADE;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_release_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE CASCADE;
ALTER TABLE change_releases DROP CONSTRAINT change_releases_release_id_fkey,
    ADD CONSTRAINT change_releases_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE CASCADE;
ALTER TABLE deployment_events DROP CONSTRAINT deployment_events_deployment_id_fkey,
    ADD CONSTRAINT deployment_events_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES deployments (id) ON DELETE CASCADE;
ALTER TABLE instances DROP CONSTRAINT instances_deployment_id_fkey,
    ADD CONSTRAINT instances_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES deployments (id) ON DELETE CASCADE;
ALTER TABLE caddy_route_backends DROP CONSTRAINT caddy_route_backends_instance_id_fkey,
    ADD CONSTRAINT caddy_route_backends_instance_id_fkey FOREIGN KEY (instance_id) REFERENCES instances (id) ON DELETE CASCADE;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_instance_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_instance_id_fkey FOREIGN KEY (instance_id) REFERENCES instances (id) ON DELETE CASCADE;
ALTER TABLE caddy_route_backends DROP CONSTRAINT caddy_route_backends_caddy_route_id_fkey,
    ADD CONSTRAINT caddy_route_backends_caddy_route_id_fkey FOREIGN KEY (caddy_route_id) REFERENCES caddy_routes (id) ON DELETE CASCADE;

ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_desired_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_desired_revision_id_fkey FOREIGN KEY (desired_revision_id) REFERENCES environment_state_revisions (id) ON DELETE SET NULL;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_applying_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_applying_revision_id_fkey FOREIGN KEY (applying_revision_id) REFERENCES environment_state_revisions (id) ON DELETE SET NULL;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_applied_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_applied_revision_id_fkey FOREIGN KEY (applied_revision_id) REFERENCES environment_state_revisions (id) ON DELETE SET NULL;
ALTER TABLE change_state_revisions DROP CONSTRAINT change_state_revisions_environment_state_revision_id_fkey,
    ADD CONSTRAINT change_state_revisions_environment_state_revision_id_fkey FOREIGN KEY (environment_state_revision_id) REFERENCES environment_state_revisions (id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE environments DROP CONSTRAINT environments_application_id_fkey,
    ADD CONSTRAINT environments_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications (id) ON DELETE RESTRICT;

ALTER TABLE environment_sources DROP CONSTRAINT environment_sources_environment_id_fkey,
    ADD CONSTRAINT environment_sources_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE runtime_configurations DROP CONSTRAINT runtime_configurations_environment_id_fkey,
    ADD CONSTRAINT runtime_configurations_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_secrets DROP CONSTRAINT environment_secrets_environment_id_fkey,
    ADD CONSTRAINT environment_secrets_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_targets DROP CONSTRAINT environment_targets_environment_id_fkey,
    ADD CONSTRAINT environment_targets_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE private_networks DROP CONSTRAINT private_networks_owner_environment_id_fkey,
    ADD CONSTRAINT private_networks_owner_environment_id_fkey FOREIGN KEY (owner_environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_networks DROP CONSTRAINT environment_networks_environment_id_fkey,
    ADD CONSTRAINT environment_networks_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_resources DROP CONSTRAINT environment_resources_environment_id_fkey,
    ADD CONSTRAINT environment_resources_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_domains DROP CONSTRAINT environment_domains_environment_id_fkey,
    ADD CONSTRAINT environment_domains_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_health_checks DROP CONSTRAINT environment_health_checks_environment_id_fkey,
    ADD CONSTRAINT environment_health_checks_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE changes DROP CONSTRAINT changes_environment_id_fkey,
    ADD CONSTRAINT changes_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE builds DROP CONSTRAINT builds_environment_id_fkey,
    ADD CONSTRAINT builds_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE releases DROP CONSTRAINT releases_environment_id_fkey,
    ADD CONSTRAINT releases_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;
ALTER TABLE environment_state_revisions DROP CONSTRAINT environment_state_revisions_environment_id_fkey,
    ADD CONSTRAINT environment_state_revisions_environment_id_fkey FOREIGN KEY (environment_id) REFERENCES environments (id) ON DELETE RESTRICT;

ALTER TABLE buildpack_configurations DROP CONSTRAINT buildpack_configurations_environment_source_id_fkey,
    ADD CONSTRAINT buildpack_configurations_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE RESTRICT;
ALTER TABLE image_configurations DROP CONSTRAINT image_configurations_environment_source_id_fkey,
    ADD CONSTRAINT image_configurations_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE RESTRICT;
ALTER TABLE source_events DROP CONSTRAINT source_events_environment_source_id_fkey,
    ADD CONSTRAINT source_events_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE RESTRICT;
ALTER TABLE builds DROP CONSTRAINT builds_environment_source_id_fkey,
    ADD CONSTRAINT builds_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE RESTRICT;
ALTER TABLE releases DROP CONSTRAINT releases_environment_source_id_fkey,
    ADD CONSTRAINT releases_environment_source_id_fkey FOREIGN KEY (environment_source_id) REFERENCES environment_sources (id) ON DELETE RESTRICT;

ALTER TABLE environment_target_networks DROP CONSTRAINT environment_target_networks_environment_target_id_fkey,
    ADD CONSTRAINT environment_target_networks_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_environment_target_id_fkey,
    ADD CONSTRAINT environment_target_states_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_environment_target_id_fkey,
    ADD CONSTRAINT change_tasks_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE deployments DROP CONSTRAINT deployments_environment_target_id_fkey,
    ADD CONSTRAINT deployments_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE instances DROP CONSTRAINT instances_environment_target_id_fkey,
    ADD CONSTRAINT instances_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_environment_target_id_fkey,
    ADD CONSTRAINT caddy_routes_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_environment_domain_id_fkey,
    ADD CONSTRAINT caddy_routes_environment_domain_id_fkey FOREIGN KEY (environment_domain_id) REFERENCES environment_domains (id) ON DELETE RESTRICT;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_environment_target_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_environment_target_id_fkey FOREIGN KEY (environment_target_id) REFERENCES environment_targets (id) ON DELETE RESTRICT;

ALTER TABLE network_access_rules DROP CONSTRAINT network_access_rules_environment_resource_id_fkey,
    ADD CONSTRAINT network_access_rules_environment_resource_id_fkey FOREIGN KEY (environment_resource_id) REFERENCES environment_resources (id) ON DELETE RESTRICT;
ALTER TABLE network_access_rule_applications DROP CONSTRAINT network_access_rule_applications_network_access_rule_id_fkey,
    ADD CONSTRAINT network_access_rule_applications_network_access_rule_id_fkey FOREIGN KEY (network_access_rule_id) REFERENCES network_access_rules (id) ON DELETE RESTRICT;
ALTER TABLE network_access_rule_applications DROP CONSTRAINT nara_environment_target_network_id_fkey,
    ADD CONSTRAINT nara_environment_target_network_id_fkey FOREIGN KEY (environment_target_network_id) REFERENCES environment_target_networks (id) ON DELETE RESTRICT;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_health_check_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_health_check_id_fkey FOREIGN KEY (health_check_id) REFERENCES environment_health_checks (id) ON DELETE RESTRICT;

ALTER TABLE changes DROP CONSTRAINT changes_corrects_change_id_fkey,
    ADD CONSTRAINT changes_corrects_change_id_fkey FOREIGN KEY (corrects_change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE change_items DROP CONSTRAINT change_items_change_id_fkey,
    ADD CONSTRAINT change_items_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_change_id_fkey,
    ADD CONSTRAINT change_tasks_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE change_tasks DROP CONSTRAINT change_tasks_parent_task_id_fkey,
    ADD CONSTRAINT change_tasks_parent_task_id_fkey FOREIGN KEY (parent_task_id) REFERENCES change_tasks (id) ON DELETE RESTRICT;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_id_fkey,
    ADD CONSTRAINT change_logs_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE builds DROP CONSTRAINT builds_change_id_fkey,
    ADD CONSTRAINT builds_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE releases DROP CONSTRAINT releases_created_by_change_id_fkey,
    ADD CONSTRAINT releases_created_by_change_id_fkey FOREIGN KEY (created_by_change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE deployments DROP CONSTRAINT deployments_change_id_fkey,
    ADD CONSTRAINT deployments_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE environment_state_revisions DROP CONSTRAINT environment_state_revisions_change_id_fkey,
    ADD CONSTRAINT environment_state_revisions_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE change_releases DROP CONSTRAINT change_releases_change_id_fkey,
    ADD CONSTRAINT change_releases_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;
ALTER TABLE change_state_revisions DROP CONSTRAINT change_state_revisions_change_id_fkey,
    ADD CONSTRAINT change_state_revisions_change_id_fkey FOREIGN KEY (change_id) REFERENCES changes (id) ON DELETE RESTRICT;

ALTER TABLE change_task_attempts DROP CONSTRAINT change_task_attempts_change_task_id_fkey,
    ADD CONSTRAINT change_task_attempts_change_task_id_fkey FOREIGN KEY (change_task_id) REFERENCES change_tasks (id) ON DELETE RESTRICT;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_task_id_fkey,
    ADD CONSTRAINT change_logs_change_task_id_fkey FOREIGN KEY (change_task_id) REFERENCES change_tasks (id) ON DELETE RESTRICT;
ALTER TABLE change_logs DROP CONSTRAINT change_logs_change_task_attempt_id_fkey,
    ADD CONSTRAINT change_logs_change_task_attempt_id_fkey FOREIGN KEY (change_task_attempt_id) REFERENCES change_task_attempts (id) ON DELETE RESTRICT;
ALTER TABLE deployment_events DROP CONSTRAINT deployment_events_change_task_attempt_id_fkey,
    ADD CONSTRAINT deployment_events_change_task_attempt_id_fkey FOREIGN KEY (change_task_attempt_id) REFERENCES change_task_attempts (id) ON DELETE RESTRICT;

ALTER TABLE releases DROP CONSTRAINT releases_build_id_fkey,
    ADD CONSTRAINT releases_build_id_fkey FOREIGN KEY (build_id) REFERENCES builds (id) ON DELETE RESTRICT;
ALTER TABLE deployments DROP CONSTRAINT deployments_release_id_fkey,
    ADD CONSTRAINT deployments_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE RESTRICT;
ALTER TABLE instances DROP CONSTRAINT instances_release_id_fkey,
    ADD CONSTRAINT instances_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE RESTRICT;
ALTER TABLE caddy_routes DROP CONSTRAINT caddy_routes_release_id_fkey,
    ADD CONSTRAINT caddy_routes_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE RESTRICT;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_release_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE RESTRICT;
ALTER TABLE change_releases DROP CONSTRAINT change_releases_release_id_fkey,
    ADD CONSTRAINT change_releases_release_id_fkey FOREIGN KEY (release_id) REFERENCES releases (id) ON DELETE RESTRICT;
ALTER TABLE deployment_events DROP CONSTRAINT deployment_events_deployment_id_fkey,
    ADD CONSTRAINT deployment_events_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES deployments (id) ON DELETE RESTRICT;
ALTER TABLE instances DROP CONSTRAINT instances_deployment_id_fkey,
    ADD CONSTRAINT instances_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES deployments (id) ON DELETE RESTRICT;
ALTER TABLE caddy_route_backends DROP CONSTRAINT caddy_route_backends_instance_id_fkey,
    ADD CONSTRAINT caddy_route_backends_instance_id_fkey FOREIGN KEY (instance_id) REFERENCES instances (id) ON DELETE RESTRICT;
ALTER TABLE environment_health_check_statuses DROP CONSTRAINT environment_health_check_statuses_instance_id_fkey,
    ADD CONSTRAINT environment_health_check_statuses_instance_id_fkey FOREIGN KEY (instance_id) REFERENCES instances (id) ON DELETE RESTRICT;
ALTER TABLE caddy_route_backends DROP CONSTRAINT caddy_route_backends_caddy_route_id_fkey,
    ADD CONSTRAINT caddy_route_backends_caddy_route_id_fkey FOREIGN KEY (caddy_route_id) REFERENCES caddy_routes (id) ON DELETE RESTRICT;

ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_desired_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_desired_revision_id_fkey FOREIGN KEY (desired_revision_id) REFERENCES environment_state_revisions (id) ON DELETE RESTRICT;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_applying_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_applying_revision_id_fkey FOREIGN KEY (applying_revision_id) REFERENCES environment_state_revisions (id) ON DELETE RESTRICT;
ALTER TABLE environment_target_states DROP CONSTRAINT environment_target_states_applied_revision_id_fkey,
    ADD CONSTRAINT environment_target_states_applied_revision_id_fkey FOREIGN KEY (applied_revision_id) REFERENCES environment_state_revisions (id) ON DELETE RESTRICT;
ALTER TABLE change_state_revisions DROP CONSTRAINT change_state_revisions_environment_state_revision_id_fkey,
    ADD CONSTRAINT change_state_revisions_environment_state_revision_id_fkey FOREIGN KEY (environment_state_revision_id) REFERENCES environment_state_revisions (id) ON DELETE RESTRICT;
-- +goose StatementEnd
