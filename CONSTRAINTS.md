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
- Revision state stores secret identifiers, generations, and digests only. It never stores plaintext or encrypted secret values.
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

## Container resources, installations, volumes, endpoints, dependencies, and bindings

- A resource is a stable logical supporting service delivered by a container image. Application release images remain part of the release and deployment lifecycle, not the resource lifecycle.
- DeployCrate CE resource installations are Docker containers only. Native packages, externally managed services, clusters, and orchestration providers are outside the CE resource model.
- The desired image reference, optional resolved digest, container name, restart policy, registry credential, configuration, and server placement belong to the installation.
- Container configuration stores credential and secret references only. It never stores plaintext or encrypted secret values.
- Installations, volumes, endpoints, credentials, dependencies, and bindings cannot disagree about their resource identity.
- A resource's owner environment controls its lifecycle. Archiving the owner environment does not destroy a resource while active dependencies or bindings still use it.
- An installation belongs to one resource.
- An installation selects one Docker-capable server. Its image reference and digest determine the desired container artifact.
- Resource installation status is a current projection. Older observations cannot replace newer observations.
- A resource volume is stable durable storage owned by one resource and placed on one server.
- A volume mount connects a resource volume only to an installation of the same resource and records its absolute container mount path and access mode.
- Replacing a container installation does not replace its resource volume. A new mount explicitly attaches the stable volume to the replacement installation.
- An endpoint belongs to its resource and, when installation-backed, to an installation of that same resource.
- An endpoint attached to a private network uses an address reachable through that network.
- A resource credential belongs to its resource and, when installation-specific, to an installation of the same resource.
- A dependency belongs to one environment and names the resource capability that environment needs.
- A dependency endpoint belongs to the dependency's resource. Its network is available to both the environment and endpoint.
- Dependency aliases are unique among active dependencies in an environment.
- A binding belongs to one dependency. Its resource and endpoint agree with the dependency.
- The application defines whether a dependency permits one active binding or several named bindings and rejects ambiguous access provisioning.
- Managed and self-managed provisioning and secret modes have separate, explicit model workflows.
- Binding status follows the supported provisioning lifecycle and never skips required cleanup or verification steps.

## Shared PostgreSQL resources and binding credentials

- One PostgreSQL resource represents one cluster, regardless of how many environment databases it contains.
- Each environment-specific database and principal is owned through an explicit resource binding.
- Database and principal names identify the intended objects within their cluster and are not reused across unrelated active bindings unless sharing is explicitly requested.
- A binding cannot reference a database, principal, endpoint, installation, or credential from another resource.
- Managed credential rotation creates a new credential generation. It never overwrites the previous generation in place.
- A managed credential generation belongs to one binding and projects to identified environment secrets.
- Managed environment secrets identify their binding source and generation and cannot be edited directly.
- Self-managed mode references user-controlled secret material and never silently replaces it with generated credentials.
- Retiring a credential generation occurs only after dependent deployments stop using it or an explicitly supported overlap window ends.
- Archiving a binding retires its managed credentials and secrets through a corrective change and cleanup tasks.
- Resource changes that affect bound environments create changes for those environments before updating managed secrets or deployments.

## Environment secrets

- Active secret keys are unique within an environment.
- Secret values are encrypted before persistence and are never stored in JSONB configuration, logs, task results, or outbox payloads.
- A digest may be used for change detection but cannot be used to recover the value.
- Secret ownership is explicit through its source type and source identifier.
- Self-managed secrets are changed only through user-authorized environment changes.
- Managed binding secrets are changed only through the binding credential lifecycle.
- Rotation creates a new secret generation or record and preserves the prior generation long enough for rollback and safe rollout.
- Archival stops future injection but preserves historical identity for revisions and telemetry correlation.

## Deployments and instances

- A deployment's change, release, target, and runtime configuration all belong to the same environment.
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
- A backup policy has one unambiguous scope: a resource or a resource binding.
- A binding-scoped policy belongs to the same resource as the binding.
- A volume backup policy identifies one volume belonging to its resource. A logical backup policy targets the resource or binding and does not claim a volume as its data boundary.
- Backup strategy and driver are explicit. Volume archives and service-aware logical backups are never treated as interchangeable artifacts.
- Schedule, retention, format, verification, and provider settings are validated by the policy model.
- A backup's policy, resource, binding, installation, volume, strategy, driver, and destination describe one coherent backup scope.
- Successful backups record the immutable object location, format, size, digest, and completion time available from the provider.
- Backup verification records what was verified and when. Verification failure does not rewrite a successful upload as if no artifact exists.
- Backup records and artifact identities are immutable after completion.
- A restore's source backup and target binding or installation are compatible in resource kind, format, and scope.
- A restore never mutates or consumes its source backup.
- Restoring one environment binding on a shared cluster cannot overwrite unrelated bindings.
- A restore is complete only after provider execution and restoration verification succeed.
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
