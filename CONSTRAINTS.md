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
- An archived application cannot receive new environments or changes.
- An archived environment cannot receive new desired-state changes, builds, deployments, bindings, routes, or secrets.
- Archival preserves releases, changes, deployments, backups, and telemetry identities.

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
- Sharing a server or resource never grants access to unrelated environments, endpoints, or services.
- Applied and observed access state cannot claim success until the responsible target or server reports the intended rule.

## Resources, grants, connections, and directly installed topology

- A Resource is a stable consumption contract. It owns name, slug, kind, management mode, sharing scope, published endpoints, access credentials, access health, and Environment connections.
- Resource kind is persisted. Category is derived from the Resource kind catalog and is never persisted independently.
- A Resource has no required owner Environment and may exist with zero connections.
- Active Resource names and slugs are globally unique without regard to case. `system_managed` Resources are immutable through user Resource workflows.
- An Environment consumes a Resource only through an active `environment_resources` Resource Connection.
- `environment` scope requires an active grant for the selected Environment. `application` scope requires an active grant for the Environment's Application. `global` scope requires no grant.
- Environment grants and Application grants are mutually exclusive according to sharing scope. A restricted Resource may have zero grants.
- Scope changes and grant revocation serialize with connection changes and are blocked when an active connection would become ineligible.
- Connection creation validates selection eligibility, endpoint ownership, credential ownership, and private-network reachability in one transaction.
- Generic directly installed Resources may own Docker Resource Installations, stable Resource Volumes, mounts, and installation-backed endpoints.
- Database-backed Resources never own generic Resource Installations. Their producer topology belongs to Database Cluster Nodes and typed Node Installations.
- An endpoint belongs to its Resource. A generic installation reference, when present, belongs to the same Resource. A database-backed endpoint maps explicitly to a Cluster Endpoint.
- A Resource credential belongs to its Resource and represents consumer access, never Database Cluster administration.

## Database Clusters, Nodes, Databases, and database-backed Resources

- A Database Cluster is the operational boundary for one engine installation. A single-node installation is still a Cluster.
- Managed Clusters require a desired installation method of `docker` or `native`. External Clusters have registered endpoints and no managed Nodes or Node Installations.
- A Cluster credential owns administrator access. Its secret payload is purpose-bound, encrypted, and never duplicated into Resource credentials or JSON metadata.
- A Cluster Node belongs to one Cluster and one Server. Exactly one active primary Node exists per active Cluster.
- Node storage owns durable storage identity and the canonical data path. Replacing an installation method preserves Node and storage identity.
- A Node Installation owns common desired, observed, service, version, health, and Server state. Exactly one active typed Docker or native detail record describes its driver-specific installation contract.
- Docker installation details own image, digest, container identity, restart policy, optional Registry Resource access, port mappings, and mount targets.
- Native installation details own package, requested version, system service, configuration path, and validated service settings. Host commands accept validated identifiers and never place plaintext credentials in arguments, output, or persisted settings.
- A Database belongs to exactly one Cluster. Its active logical name is unique within that Cluster.
- One logical Database maps to exactly one Resource through `database_resources`. Database and Resource identities survive Node moves and installation-method replacement.
- Database creation and deprovisioning use the Cluster administrator workflow. Deprovisioning is blocked by an active Resource, Resource Connection, or Database restore.
- Archiving a Cluster requires all Databases to be archived and all managed Nodes to be retired.
- Database process telemetry is attributed to Database Cluster, Cluster Node, and Node Installation, never to an arbitrary database Resource.

## Registry Resources

- Managed and external OCI registries are Resources with `kind = registry` and exactly one typed `registry_resources` backing record.
- The typed backing stores provider configuration only. Resource owns identity, lifecycle, endpoint, access credential, health, and sharing policy.
- A selectable Registry Resource has one active access credential with push and pull capability. The encrypted secret exists only in `resource_credentials`.
- Buildpack configuration selects a Registry Resource and stores its image repository path separately. Environment Source owns source control only.
- Build and deployment records retain the selected Registry Resource endpoint and credential identity for audit without snapshotting secret material.

## Resource and database health

- Resource access health owns `resource_id` directly. Endpoint, credential, and generic installation references are optional but must belong to that Resource.
- Database Resource access checks require the published endpoint and Resource credential and do not require a generic Resource Installation.
- PostgreSQL access health resolves the backing Database and verifies application access with `SELECT 1`.
- Cluster, Cluster Endpoint, Node, and Node Installation operational health belongs to typed database health records.
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
- A backup policy has one unambiguous target: a Server filesystem or a logical Database.
- Logical backup policies target stable Database identity. The executing Cluster, Node, and Node Installation are resolved at execution time and recorded only as provenance.
- Backup strategy and driver are explicit. Server filesystem archives and logical database backups are never treated as interchangeable artifacts.
- Schedule, retention, format, verification, and provider settings are validated by the policy model.
- A backup's policy, Database or Server target, execution provenance, strategy, driver, and destination describe one coherent backup scope.
- Successful backups record the immutable object location, format, size, digest, and completion time available from the provider.
- Backup verification records what was verified and when. Verification failure does not rewrite a successful upload as if no artifact exists.
- Backup records and artifact identities are immutable after completion.
- A Database restore's source backup and target Database are compatible in engine, format, and scope.
- A restore never mutates or consumes its source backup.
- Restore orchestration creates a mandatory safety backup, restores through staging, reconciles access credentials, cuts over the target Database, verifies access, and rolls back on failure.
- At most one active restore may operate on a Database. Restoring one Database cannot overwrite unrelated Databases in the same Cluster.
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
