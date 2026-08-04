# Application-Layer Constraints

This document defines domain constraints enforced by DeployCrate application code instead of PostgreSQL constraints.

Rules local to one entity belong in `models/`. Rules involving multiple records, transactions, external systems, task creation, or outbox publication belong in `services/`. Every entry point, including the dashboard, CLI, webhooks, telemetry processors, jobs, and seeds, must use the same model or service rules.

## Global invariants

- Every persisted record has a generated stable identifier. Identifiers are never reused.
- Every referenced identifier resolves to the expected record type. Cross-record relationships described below must agree on their environment, resource, installation, server, or target ownership.
- Active names, slugs, aliases, addresses, and other logical identifiers are unique within the scope stated below.
- Concurrent operations that enforce application-level uniqueness or sequencing must serialize competing writes in a transaction.
- `created_at` never changes. Update, observation, start, finish, cancellation, archival, removal, verification, and publication timestamps follow lifecycle order.
- Archived or removed records remain available to historical changes, releases, deployments, backups, restores, and telemetry correlation, but cannot be selected for new desired state.
- Immutable records are never edited after creation. Corrections create new records.
- String-backed kinds, modes, roles, states, and actions are validated against centrally defined application values and transition rules.
- JSONB payloads are validated by the owning model according to their kind, provider, driver, format, or schema version.
- JSONB written to logs, events, revisions, results, and observed state never contains plaintext credentials or secret values.
- Durable state changes, their tasks, and their outbox events are written in one transaction.

## Users, tokens, and credentials

- User email addresses are normalized before comparison and identify one active user.
- A token belongs to one user. Its scope contains only supported permissions.
- Expired or revoked tokens cannot authenticate. `last_used_at` only moves forward.
- Token digests and credential payloads are never returned after creation.
- A credential's provider determines the accepted metadata and encrypted payload shape.
- Archived credentials cannot be selected for new sources, servers, backup destinations, or resource operations.

## Applications and environments

- An environment belongs to one application.
- Application slugs are unique within the deployment's application scope.
- Environment slugs are unique within an application.
- Deleting an application permanently removes every owned environment, source, domain, secret, desired-state revision, build, release, deployment, route, and attachment after external workload cleanup succeeds.
- Deleting an environment permanently removes the same Environment-owned graph and deletes its application when no environments remain.
- Shared Resources, Servers, credentials, registries, private networks, backup policies, and backup artifacts are not owned by an application deletion. Durable backup and restore changes are rehomed to the system Environment before deletion.

## Sources, source events, builds, and releases

- An environment source belongs to its environment, and any credential selected for it supports the source provider.
- The application defines whether an environment has one active source or several named sources and rejects ambiguous source selection.
- A source event belongs to one source. A provider event identity is processed once for that source.
- A processed source event is immutable. Reprocessing creates or updates processing state without changing the received payload.
- A build's source, change, and environment refer to the same environment.
- A build captures the source revision and complete build configuration used for that attempt.
- Build state transitions move forward through the supported build lifecycle. A terminal build is immutable.
- A release belongs to one environment and is immutable.
- When a release comes from a build, the build succeeded and belongs to the same environment and source.
- Release artifact reference and digest identify the exact deployable artifact and never change.
- A release selected by a change belongs to that change's environment.

## GitHub Apps, installations, and repositories

- The GitHub connection uses the validated control-plane `INSTANCE_ID`. It never creates another instance identity.
- At most one active GitHub App is allowed for the control plane. Competing setup completions serialize in a transaction because this rule is not enforced by a unique index.
- A `github_app` credential has versioned non-secret metadata and a purpose-bound encrypted payload. The private key, webhook secret, client secret, App JWTs, and installation tokens never enter operational JSONB, logs, Inertia props, or River arguments.
- Setup state is bound to one administrator, one purpose, and the current control plane. It expires and can complete only once.
- GitHub installation and repository identity follows stable provider IDs. Account logins, repository names, URLs, permissions, and events are mutable projections.
- Repository removal and installation suspension retain bindings and history but block new source selection and builds.
- A GitHub environment-source binding connects a `git` and `github` source to one active repository from one active, non-suspended installation.
- A complete repository synchronization applies all fetched pages atomically. An incomplete provider fetch never marks unseen repositories removed.
- Webhook bypasses use exact route equality for `/webhooks/github`. Signature verification covers the bounded raw body and happens before JSON decoding or persistence.
- Delivery IDs are provider-wide idempotency identities. A matching push creates at most one source event and pending build for each active source.
- Source events, changes, pending builds, and uniquely keyed River build jobs are inserted in one transaction.
- The build job contains only the build ID. Build configuration and mutable source state remain in PostgreSQL.
- The Buildpacks context is relative and cannot traverse outside the repository. Buildpacks settings contain no secret values.

## Environment state revisions and changes

- An environment state revision belongs to one environment and the committed change that created it.
- Revision `state` is a complete canonical desired-state snapshot, not a patch.
- Revision state stores secret identifiers and keyed digests only. It never stores plaintext or encrypted secret values.
- Environment state revisions are immutable.
- Change sequence increases monotonically within an environment.
- A change's base and result revisions belong to the same environment as the change.
- A normal committed change starts from the environment's latest committed result revision.
- A corrective change may restore an earlier revision, but it still creates a new result revision and history entry.
- `corrects_change_id` identifies a change from the same environment.
- `correction_context` follows the supported schema and records the telemetry evidence reference, evaluation window, affected identities, and selected action when telemetry caused the correction.
- Change items are audit differences between the base and requested state. Environment state revisions remain the source of truth.
- `previous_value` and `requested_value` use one documented representation per subject and action. They are not interpreted differently by separate entry points.
- A requested change may be cancelled before commitment. A committed desired-state transition is not erased or cancelled; reversing it requires another change.
- The desired release is the release selected by the latest committed, non-cancelled change that selects a release.
- AI agents act as ordinary authenticated users through user tokens. They do not receive a separate durable actor identity.

## Environment targets and reconciliation

- An environment target belongs to one environment and one server.
- At most one active target represents a given environment and server placement unless the placement strategy explicitly names distinct target roles.
- Detaching a target stops new work from being scheduled there but preserves deployment and instance history.
- Every revision referenced by an environment target state belongs to the target's environment.
- `desired_revision_id` is a projection of the environment's latest committed result revision, not an independent source of desired state.
- `applying_revision_id` identifies the revision currently being reconciled on that target.
- `applied_revision_id` changes only after the target successfully applies the complete revision.
- `observed_state` is sanitized, contains no secrets, and records enough information to detect material drift from the applied revision.
- Older observations cannot overwrite newer target state.
- Partial application is derived from target states for the same environment change or revision.
- A target is drifted when its observed material state differs from its applied revision, even when no task is running.

## Servers and WireGuard

- A server has one active SSH credential configuration and one active WireGuard peer identity.
- WireGuard public keys and allocated overlay addresses identify one active peer or network attachment.
- All managed servers participate in the single DeployCrate WireGuard overlay.
- The control plane and Nodes use `10.99.0.0/17`; user devices use `10.99.128.0/17`.
- Every Node has a direct `/32` WireGuard peer for every other active Node, and Node peer state never includes user devices.
- Every Node must expose its declared WireGuard UDP endpoint to every other Node.
- Server and WireGuard status records are current projections. Older observations cannot replace newer observations.
- Archiving a server prevents new placements while preserving target, deployment, backup, and telemetry history.

## Logical private networks and access

- A private network has one CIDR and an explicit lifecycle owner.
- Active logical network CIDRs and routing configuration must be compatible with the shared WireGuard overlay.
- Every allocated address belongs to its private network CIDR and identifies one active attachment within that network.
- An environment network membership connects an environment only to a network it is permitted to use.
- An environment target network belongs to a target whose environment is a member of that network.
- The target's server is attached to every network required by that target.
- `environment_target` is the runtime network-isolation boundary. Containers and native workloads for different targets must not share unrestricted network namespaces or policy identities.
- An access rule's environment, target, dependency, endpoint, and network describe one coherent source-to-destination permission.
- A dependency can reach only its selected endpoint and required protocol and port through the declared logical network.
- User devices peer only with the control plane. A Resource grant creates an exact device-address, destination-address, protocol, and port route through the control plane, plus a matching destination-Node firewall rule.
- The control plane does not provide unrestricted forwarding between the device and Node address pools.
- Sharing a server or resource never grants access to unrelated environments, endpoints, or services.
- Applied and observed access state cannot claim success until the responsible target or server reports the intended rule.

## Resources, Docker topology, and Environment attachments

- A Resource is the stable identity and lifecycle boundary for an independently managed Docker workload.
- `resource_type` is a validated enum with `database`, `cache`, and `service` values.
- Resource `configuration` is secret-free JSONB interpreted by `resource_type`. It owns the engine, optional engine version, and logical engine-specific definitions such as database names.
- A Resource has no required owner Environment and may exist with zero connections.
- Active Resource names and slugs are globally unique without regard to case. `system_managed` Resources are immutable through user Resource workflows.
- Every user-created Resource starts with a Docker Resource Installation. Docker image, container, restart, placement, registry credential, and port mapping details belong to that installation.
- Resource Volumes and mounts provide durable storage to Resource Installations.
- A Resource Endpoint belongs only to its Resource. It describes how a consumer interacts with the Resource and never owns or identifies an installation.
- Resource health checks belong to the Resource and may select an endpoint and credential. They do not select an installation.
- An Environment consumes a Resource only through an active `environment_resources` attachment that selects one Resource Endpoint and an optional application credential.
- Resource attachment and detachment are owned by Environment setup and edit workflows. Resource workflows report only attachments whose Environment and Application are active.
- Resource configuration owns default Environment secret names. An attachment may store Resource-managed key overrides, edited only from the Resource screen, plus the last reconciled effective names.
- Resource default changes reconcile every active attachment that inherits the changed role. Attachment overrides reconcile only that attachment. Endpoint and credential value changes reconcile every active attachment selecting that dependency using its effective names.
- Attachment changes validate endpoint ownership, application credential ownership, selected runtime placement, and private-network reachability in one transaction.
- Administrator credentials are never selectable by Environment attachments and are never injected into Environment secrets.

## Database Resources and credentials

- A database Resource is a Docker Resource whose configuration selects a database engine.
- PostgreSQL's implicit `postgres` database is maintenance state and is not listed as a logical application database in Resource configuration.
- Creating a database Resource requires one administrator credential with metadata purpose `administrator`. It becomes the engine superuser and its encrypted payload remains on the Resource.
- A database Resource has at most one active administrator credential.
- Creating a Resource does not create an application database or application user.
- Logical database creation is an explicit operation. The service connects using the administrator credential, creates the database in the engine, and then records its non-secret definition in Resource configuration.
- An application credential has metadata purpose `application` and selects exactly one configured logical database.
- For PostgreSQL, application credential creation and rotation reconcile the LOGIN role and database privileges in code before desired credential state commits.
- Database names and application principal names are unique within their Resource scopes.
- Database telemetry is attributed to Resource and Resource Installation identity.

## Registry Resources

- OCI registries are service Resources with `configuration.engine = registry`. A DeployCrate-operated Registry has a Docker Resource Installation; a registered external Registry has only its interaction endpoint.
- Registry provider configuration remains in its typed backing where required. Resource owns identity, lifecycle, endpoint, access credential, and health.
- A selectable Registry Resource has one active access credential with push and pull capability. The encrypted secret exists only in `resource_credentials`.
- Buildpack configuration selects a Registry Resource and stores its image repository path separately. Environment Source owns source control only.
- Build and deployment records retain the selected Registry Resource endpoint and credential identity for audit without snapshotting secret material.

## Resource and database health

- Resource access health owns `resource_id` directly. Endpoint and credential references are optional and must belong to that Resource.
- PostgreSQL access checks select a configured database and application credential and verify access with `SELECT 1`.
- Docker installation health remains an observation of the Resource Installation and is distinct from consumer access health.
- Current health status is separate from historical telemetry. Older observations cannot replace newer status.

## Environment secrets

- Environment secret keys are normalized to uppercase, match `[A-Z_][A-Z0-9_]*`, and are unique without regard to case among active rows in one Environment.
- `PORT` and any active Resource-projected keys are reserved from user ownership.
- Secret values are encrypted with purpose-bound AES-256-GCM before persistence and are never stored in desired-state JSON, build or deployment configuration JSON, changes, jobs, logs, telemetry, or Inertia props.
- Secret digests use a server-key-derived, context-bound HMAC-SHA256 over Environment ID, normalized key, and plaintext so a same-value rotation is detected as a no-op.
- Secret ciphertext, digest, source type, source ID, and Environment ID are immutable. The secret row UUID and keyed digest identify the exact immutable value.
- Secret ownership is either `user` with the initiating User ID or `environment_resource` with the EnvironmentResource connection ID.
- User workflows cannot mutate Resource-managed secret rows. Resource reconciliation cannot silently take ownership of an active user key.
- Rotation archives the active row and creates a new immutable row in one transaction. Archived rows are retained for rollback.
- Archival creates a committed Environment change and complete state revision that omits the key. Historical revisions continue resolving their exact archived row.
- Desired-state revisions contain only secret ID, key, full keyed digest, source type, and source ID. Resolution verifies every descriptor against its Environment-owned database row before decryption.
- Secret values are decrypted only in memory for Resource projection or the exact deployment revision. Root and Docker-socket administrator visibility into a running container environment is an accepted security boundary.

## Deployments and instances

- A deployment's change, release, target, and runtime configuration all belong to the same environment.
- Environment workloads are directly managed Docker containers. Docker Compose and generated per-Environment systemd units are not deployment state stores.
- Each workload container and bridge network is identified by stable ownership labels. Names alone never establish ownership.
- Workload images use immutable registry digests, restart with `unless-stopped`, and publish application ports only to dynamic loopback host ports.
- One-replica replacement is blue-green. The previous serving container remains available until candidate health, Caddy switching, and public verification succeed.
- A deployment applies the result revision selected by its change.
- Deployment attempt numbers increase within the same change, release, and target execution.
- Deployment status follows the supported lifecycle. Start and finish timestamps reflect the actual execution window.
- A successful deployment means its target reached the requested revision, not merely that its task returned successfully.
- Partial deployment is derived across the target deployments belonging to the same change and revision.
- An instance belongs to its deployment, release, and environment target, and those identities agree.
- Runtime external identifiers identify one instance within the owning runtime and target.
- Instance observations only update current state when they are newer than the stored observation.
- Removing or replacing an instance preserves its deployment and release identity.

## Domains and Caddy routing

- Hostnames are normalized before comparison and identify one active environment domain within the managed routing scope.
- An environment has at most one active primary domain.
- A Caddy route's target, domain, release, and backend instances belong to the same environment.
- A route backend instance is reachable from the route's target and represents a release allowed by the active rollout or correction.
- Backend weights and route configuration follow the supported routing strategy.
- Routing changes are performed through change tasks and retain enough history to restore the previous backend set.
- A route is marked applied only after the responsible Caddy instance acknowledges the intended configuration.
- Older routing observations cannot overwrite newer observed state.

## Health checks and observations

- A health check belongs to one environment. Any dependency it checks belongs to the same environment.
- Health-check method, URL, timing, and expected response are valid for the selected check kind.
- A health status target, instance, and release belong to the health check's environment.
- When a health result concerns a dependency or binding, telemetry carries the dependency, resource, installation, endpoint, and binding identities available at execution time.
- PostgreSQL stores desired health-check definitions and current projections only.
- ClickHouse stores historical health results and high-volume telemetry.
- Older health observations cannot replace newer current projections.
- Health alone does not change desired state. Detection logic creates an explicit corrective change with evidence and causation.

## Change tasks and logs

- A task belongs to one change. Its target, server, and subject are compatible with that change's environment and operation.
- A parent task belongs to the same change and cannot create a dependency cycle.
- A task runs only after its required parent work succeeds or the task kind explicitly handles parent failure.
- Idempotency keys identify one logical side effect within their task kind and subject scope.
- `attempt_count` increases for every claimed execution, including redeliveries after an uncertain outcome.
- `available_at` determines when a pending or retryable task can be claimed.
- `last_heartbeat_at` moves forward only while an attempt is active.
- A running task with an expired heartbeat is recoverable according to its task kind and idempotency policy.
- Retrying a task reuses the task identity and idempotency key.
- A terminal task's input and result are immutable. A correction or explicit retry creates a new lifecycle transition rather than rewriting history.
- Task status, timestamps, result, and error agree with the supported lifecycle.
- Change logs never contain secrets. Detailed high-volume logs are written to ClickHouse; PostgreSQL retains only durable operational and audit messages.

## Backup destinations, policies, backups, and restores

- A backup destination's credential supports its provider and remains active while policies or pending operations use it.
- A backup policy has one unambiguous target: a Server filesystem or a Resource target described by validated JSONB.
- Logical database policies target a Resource plus the database name stored in target JSONB. The executing Resource Installation is resolved at execution time and recorded as provenance.
- Backup strategy and driver are explicit. Server filesystem archives and logical database backups are never treated as interchangeable artifacts.
- Schedule, retention, format, verification, and provider settings are validated by the policy model.
- A backup's policy, Resource or Server target, execution provenance, strategy, driver, and destination describe one coherent backup scope.
- Successful backups record the immutable object location, format, size, digest, and completion time available from the provider.
- Backup verification records what was verified and when. Verification failure does not rewrite a successful upload as if no artifact exists.
- Backup records and artifact identities are immutable after completion.
- A Resource restore's source backup and target Resource database are compatible in engine, format, and scope.
- A restore never mutates or consumes its source backup.
- Restore orchestration creates a mandatory safety backup, restores through staging, reconciles access credentials, cuts over the target Database, verifies access, and rolls back on failure.
- At most one active restore may operate on a Resource database. Restoring one Database cannot overwrite unrelated Databases in the same Resource.
- A restore is complete only after cutover and verification succeed.
- Backup and restore state changes are executed through change tasks and retain their operation windows.

## Outbox events and JetStream delivery

- Every event has a stable identifier reused across publication retries.
- Aggregate type and identifier name the durable control-plane record whose transition produced the event.
- Correlation identifies the broader workflow. Causation identifies the event, change, or task that directly caused this event according to the documented envelope convention.
- Event type and schema version determine the payload shape.
- Event payloads contain stable identifiers and sanitized data, never credentials or secret values.
- An outbox event is inserted in the same transaction as the state transition it announces.
- Event identity, aggregate identity, type, version, causation, correlation, payload, and occurrence time are immutable.
- Publication attempts increase monotonically. A successful JetStream acknowledgement records publication time.
- An uncertain publish outcome retries with the same event identifier so consumers can process idempotently.
- JetStream transports events and tasks but is never the sole durable record of desired state, changes, task intent, deployments, corrections, backups, restores, or publication intent.

## PostgreSQL and ClickHouse identity contract

- Telemetry emitted to ClickHouse includes every stable identity available at observation time: environment, change, revision, release, deployment, target, server, instance, resource, installation, endpoint, dependency, binding, and change task.
- Telemetry includes observation time and the relevant operation start and finish window or task identity.
- ClickHouse records can always resolve their stable identifiers back to retained PostgreSQL control-plane history.
- PostgreSQL retains desired state, current projections, causal history, correction context, task recovery state, backup and restore history, and outbox intent.
- ClickHouse retains historical telemetry, health results, logs, metrics, traces, and high-volume observations.
