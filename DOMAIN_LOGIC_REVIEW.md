# DeployCrate CE Domain Logic Review Guide

This document is a map for a human review of the current application. It describes what the code appears to do, where each responsibility lives, and which invariants deserve deliberate verification. It is not a claim that the implementation is correct.

Snapshot reviewed: 2026-08-02.

## How to use this guide

Review in dependency order. A mistake in an earlier area can invalidate assumptions in several later workflows.

```text
1. Runtime wiring and persistence
              |
              v
2. Identity and authorization
              |
              v
3. Change, revision, and queue model
              |
              v
4. Bootstrap, nodes, and networking
              |
              v
5. Applications and environments
              |
              v
6. Secrets, resources, and databases
              |
              v
7. GitHub, builds, and deployments
              |
              v
8. Telemetry, backups, and self-update
```

For each section:

- Confirm the stated behavior matches the product intent.
- Follow the main flow through the listed files.
- Check every transaction boundary, retry boundary, and external side effect.
- Check that failure and cancellation leave durable state recoverable.
- Check authorization at both the route and service boundary.
- Record a test scenario for each invariant before changing implementation.

## System-wide domain map

```text
User / GitHub webhook / periodic scheduler
                    |
                    v
             HTTP controllers
                    |
                    v
        Services coordinate workflows
          |          |           |
          |          |           +----> provider clients
          |          |                  SSH, Docker, Caddy,
          |          |                  GitHub, S3, PostgreSQL
          |          |
          |          +----------------> River jobs in PostgreSQL
          |                                |
          |                                v
          |                           queue workers
          |
          v
  Models and PostgreSQL
  durable intent, snapshots, status, identity
          |
          +-------------------------------> ClickHouse
                                            logs and metric rollups
```

The application follows the Andurel layer split:

- `models/` owns entities, validation, state transitions, and queries.
- `services/` owns transactions, cross-model orchestration, and external effects.
- `controllers/` owns HTTP parsing, authorization middleware selection, and rendering.
- `queue/` owns River job definitions, workers, periodic scheduling, and retry policy.
- `clients/` owns provider and host adapters.
- `cmd/app/` owns the running process lifecycle.
- `cmd/bootstrap/` and `internal/setup/` own fresh control-plane installation.

## Review hotspots found during mapping

These are starting questions, not completed defect findings.

- [ ] **Coverage:** only seven Go test files currently exist. Build execution, deployment execution, environment setup, secrets, resources, node enrollment, telemetry rollups, backups, restores, GitHub webhooks, and self-update do not have direct workflow tests.
- [ ] **Authorization model:** application and environment mutations use `AuthOnly`, as does starting a self-update. Resource, database, node, Caddy, and GitHub administration generally use `AdminOnly`. Confirm this is intentional for every authenticated account.
- [ ] **Closed registration:** registration and confirmation service/controller code exists, but `Confirmations` is not registered in `controllers.Module`, and no registration controller is wired. Confirm whether this is deliberate dead code.
- [ ] **Generated validation:** several central generated entities have empty `Validate` methods, including `ApplicationEntity`, `ChangeEntity`, and `OutboxEventEntity`. Their invariants therefore depend on services and database constraints.
- [ ] **Dormant outbox:** the outbox table, model, and factory exist, but no publisher or workflow uses them. Confirm whether transactional River insertion has replaced the outbox design.
- [ ] **String state machines:** many lifecycle states and steps are stored as free-form strings. Review transition guards and database constraints together.
- [ ] **External effects around transactions:** several workflows persist intent, perform host or provider effects, then persist results. Verify restart reconciliation and compensation for every gap.

## 1. Runtime composition and application startup

### What it does

The main process initializes Inertia, constructs the dependency graph with `fx`, applies migrations, reconciles workload state, seeds and starts River, starts the HTTP server, and ensures an initial backup exists for every active policy. Shutdown stops the HTTP server, queue processor, and telemetry exporters.

```text
process start
    |
    +--> load embedded Inertia assets
    +--> construct config, DB, telemetry, queue, services, controllers, router
    +--> apply PostgreSQL migrations under session lock
    +--> reconcile persisted workload state with Docker
    +--> seed backup schedule jobs
    +--> start River workers
    +--> start HTTP server
    `--> enqueue missing initial backups
```

### Important files

- [`cmd/app/main.go`](cmd/app/main.go): process entry point, commands, startup ordering, shutdown.
- [`services/service.go`](services/service.go): service dependency registration.
- [`controllers/controller.go`](controllers/controller.go): controller registration and reachable HTTP surfaces.
- [`queue/queue.go`](queue/queue.go): River clients, queues, concurrency, and stuck-job rescue threshold.
- [`queue/workers.go`](queue/workers.go): worker registration.
- [`database/database.go`](database/database.go): PostgreSQL and Bun construction.
- [`database/migrations.go`](database/migrations.go): migration locking and execution.
- [`router/router.go`](router/router.go): global middleware order and HTTP instrumentation.
- [`config/config.go`](config/config.go): configuration composition and process globals.

### Review checklist

- [ ] Confirm startup ordering is safe when migrations, reconciliation, queue seeding, or initial backups fail.
- [ ] Confirm `RescueStuckJobsAfter: 13h` is compatible with every worker timeout.
- [ ] Confirm queue concurrency of one for builds, deployments, backups, and node enrollment matches throughput and serialization requirements.
- [ ] Confirm shutdown deadlines are long enough for in-flight external operations.
- [ ] Confirm configuration parsing failures should panic rather than return structured startup errors.

## 2. Bootstrap and initial control-plane topology

### What it does

Bootstrap is a separate temporary CLI. It validates a fresh Debian host, collects configuration through a terminal UI, saves resumable state, and executes idempotent setup steps. Host scripts install and configure users, SSH CAs, WireGuard, Docker, Caddy, PostgreSQL, ClickHouse, Prometheus, the OpenTelemetry Collector, Buildpacks, application slots, and host hardening.

After migrations, `BootstrapService` creates the initial domain topology in one PostgreSQL transaction. Caddy reconciliation occurs after the transaction and is verified before handoff.

```text
bootstrap install or resume
          |
          v
preflight + process lock + saved config/state
          |
          v
ordered Step.Check / Step.Apply loop
          |
          +--> host packages and identities
          +--> SSH CA and WireGuard
          +--> Docker, telemetry, Buildpacks, PostgreSQL
          +--> application release and blue slot
          +--> migrations and administrator
          |
          v
BootstrapService transaction
          |
          +--> system Application + Environment + Server
          +--> private network + target
          +--> PostgreSQL, ClickHouse, registry Resources
          +--> optional backup policies
          +--> Change + Release + Deployment + Instance
          `--> domain + Caddy route/backend
          |
          v
reconcile and verify Caddy route -> credential handoff -> cleanup/reboot
```

### Important files

- [`cmd/bootstrap/main.go`](cmd/bootstrap/main.go): CLI modes, install/resume rules, locks, and handoff.
- [`cmd/bootstrap/setup_operations.go`](cmd/bootstrap/setup_operations.go): bridge from host setup to database-backed services.
- [`internal/setup/steps.go`](internal/setup/steps.go): ordered host mutations and pinned component versions.
- [`internal/setup/runner.go`](internal/setup/runner.go): resume semantics, step checks, state transitions, and secret-redacted shell execution.
- [`internal/setup/state.go`](internal/setup/state.go): per-step durable state.
- [`internal/setup/persistence.go`](internal/setup/persistence.go): installation status and cleanup state.
- [`internal/setup/scripts/`](internal/setup/scripts/): actual host configuration behavior.
- [`services/bootstrap.go`](services/bootstrap.go): initial domain graph and system Resource construction.
- [`services/caddy_route.go`](services/caddy_route.go): route reconciliation and verification.

### Review checklist

- [ ] For every setup step, verify `Check` proves the same postcondition that `Apply` creates.
- [ ] Verify resume is safe after interruption at every boundary, especially SSH hardening, slot activation, and secret cleanup.
- [ ] Verify the database transaction cannot record topology that the host did not actually create.
- [ ] Verify all generated secrets are redacted from terminal output, logs, environment dumps, and saved state as intended.
- [ ] Verify pinned downloads authenticate both bytes and metadata for production release sources.
- [ ] Verify local and external PostgreSQL produce equivalent domain topology.
- [ ] Verify the bootstrap Caddy route can be reconciled after a committed topology transaction fails to reach Caddy.

## 3. Identity, sessions, and authorization

### What it does

Passwords use Argon2id with a per-password salt and an application pepper. Authentication tries the current pepper, then configured previous peppers, and rehashes on a successful old-pepper login. Email verification and reset tokens are stored only as keyed hashes. Browser authentication uses encrypted and authenticated cookie sessions, CSRF middleware, session recovery, and route-level authorization middleware.

```text
login form
   |
   v
Identity.AuthenticateUser
   |
   +--> find normalized email
   +--> Argon2id with current or previous pepper
   +--> optional password rehash
   `--> require verified email
   |
   v
encrypted cookie session
   |
   +--> AuthOnly: any authenticated user
   `--> AdminOnly: authenticated administrator
```

### Important files

- [`models/user.go`](models/user.go): password hashing, email normalization, user persistence.
- [`models/token.go`](models/token.go): random token generation, keyed storage hash, expiry checks.
- [`services/identity.go`](services/identity.go): identity dependencies and pepper rotation.
- [`services/authentication.go`](services/authentication.go): login rules.
- [`services/registration.go`](services/registration.go): user creation and email verification flow.
- [`services/reset_password.go`](services/reset_password.go): reset request and password replacement.
- [`controllers/sessions.go`](controllers/sessions.go): login/logout HTTP flow.
- [`router/cookies/session.go`](router/cookies/session.go): invalid session recovery.
- [`router/middleware/auth.go`](router/middleware/auth.go): authorization and IP rate limiting.
- [`router/router.go`](router/router.go): session, CSRF, CORS, recovery, and logging middleware order.

### Review checklist

- [ ] Decide and document which actions require admin access versus ordinary authentication.
- [ ] Verify session renewal, logout invalidation, cookie rotation, and stale user/admin changes.
- [ ] Verify in-memory per-process IP rate limiting is adequate for the deployment model.
- [ ] Verify token attempts, token supersession, expiry cleanup, and replay behavior.
- [ ] Confirm registration/confirmation code is intentionally unreachable or wire it deliberately later.
- [ ] Review the password parameters and minimum password policy against the production threat model.

## 4. Changes, desired-state revisions, and durable jobs

### What it does

`Change` is the durable audit and workflow envelope for environment-affecting actions. Environment configuration is copied into immutable JSON revisions. Builds and deployments point back to changes and revisions, so asynchronous work uses a snapshot rather than mutable current configuration. River jobs are inserted in the same PostgreSQL transaction as the domain records they execute.

```text
user, webhook, installer, or scheduler intent
                    |
                    v
                 Change
        sequence + actor + cause + status
             /                    \
            v                      v
EnvironmentStateRevision       ChangeTask
desired configuration snapshot durable operation status
            |                      |
            +----------+-----------+
                       v
              River job inserted in tx
                       |
                       v
                worker claims work
                       |
                       v
          result status + events + observed state
```

### Important files

- [`models/change.go`](models/change.go): change identity, sequencing, and status updates.
- [`models/environment_state_revision.go`](models/environment_state_revision.go): strict JSON schema, canonicalization, and secret descriptors.
- [`models/change_task.go`](models/change_task.go): task state and idempotency records.
- [`models/change_state_revision.go`](models/change_state_revision.go): requested/result revision association.
- [`models/environment_target_state.go`](models/environment_target_state.go): desired, applying, applied, and observed target state.
- [`queue/queue.go`](queue/queue.go): transactional insertion adapter and worker configuration.
- [`queue/jobs/`](queue/jobs/): job payloads, uniqueness, attempts, queues, and scheduling.
- [`queue/telemetry.go`](queue/telemetry.go): spans around worker execution.

### Review checklist

- [ ] Verify `Change.NextSequence` is concurrency-safe for every caller. It reads `MAX(sequence) + 1` without an explicit lock in the model.
- [ ] Verify job uniqueness options allow valid retries and prevent duplicate side effects.
- [ ] Verify every transactional domain write uses `InsertTx` when queueing dependent work.
- [ ] Verify each worker is idempotent when River retries after the external effect succeeded but result persistence failed.
- [ ] Define allowed state transitions centrally or confirm database constraints cover them.
- [ ] Decide whether the unused outbox model should be removed or implemented.

## 5. Applications and environment setup

### What it does

An `Application` groups one or more `Environment` records. Application creation also configures a GitHub-backed source and Buildpacks settings. Environment completion selects a runtime server, private network, domain, resource connections, secrets, and runtime settings. It commits an initial desired-state revision and queues the first build in one transaction.

```text
Application
    |
    +--> Environment
            |
            +--> GitHub EnvironmentSource
            +--> BuildpackConfiguration
            +--> RuntimeConfiguration
            +--> EnvironmentTarget -> Server
            +--> EnvironmentNetwork -> PrivateNetwork
            +--> EnvironmentDomain
            +--> EnvironmentResource connections
            +--> EnvironmentSecret values
            `--> latest committed EnvironmentStateRevision
                              |
                              v
                         initial Build job
```

The revision is the deployable contract. Mutable tables still provide identity and operational metadata, but deployment validates them against the exact revision. Application deletion removes external routes, workloads, jobs, and build caches before one database transaction deletes the Application-owned graph. Resource and Server records remain shared infrastructure.

### Important files

- [`services/application_setup.go`](services/application_setup.go): application, source, registry, and build-server setup.
- [`services/environment_setup.go`](services/environment_setup.go): setup transaction, edits, deployability, manual builds, redeploys, retries, and deletion.
- [`services/build_configuration.go`](services/build_configuration.go): immutable build snapshot format.
- [`models/application.go`](models/application.go): ordinary versus system application rules.
- [`models/environment.go`](models/environment.go): setup-complete and deployability queries.
- [`models/environment_state_revision.go`](models/environment_state_revision.go): desired-state contract.
- [`controllers/applications.go`](controllers/applications.go): application HTTP surface.
- [`controllers/environments.go`](controllers/environments.go): environment HTTP surface.

### Review checklist

- [ ] Verify an environment cannot be partially configured or configured twice under concurrency.
- [ ] Verify edits create the right requested and result revisions and do not silently mutate deployable history.
- [ ] Verify deleted applications/environments cannot still build, deploy, receive webhooks, or retain public routes.
- [ ] Verify build-server, runtime-server, Resource, and network placement capability checks.
- [ ] Verify environment deletion ordering and compensation across Docker, Caddy, jobs, and database records.
- [ ] Verify all environment mutation routes should be available to every authenticated user.

## 6. Secrets and credentials

### What it does

Secrets are encrypted with AES-256-GCM using the session encryption key. Most application credential families use purpose-specific associated data so their ciphertext cannot be decrypted as another family. Backup destination credentials and the bootstrap WireGuard private key currently use the generic v1 envelope. A keyed digest supports no-op detection and immutable descriptors without embedding plaintext in environment revisions.

There are several secret families with different ownership and lifecycle rules.

```text
SESSION_ENCRYPTION_KEY
          |
          +--> environment secret ciphertext
          |       purpose: environment-secret/v1
          |
          +--> Resource credential payload
          |       purpose: resource-credential/v1
          |
          +--> Database Cluster credential
          |       purpose: database-cluster-credential/v1
          |
          +--> GitHub App credential
          |       purpose: github-app-credential
          |
          +--> node bootstrap SSH key/passphrase
          `--> backup destination credential, generic v1 envelope

EnvironmentStateRevision stores:
  secret ID + key + owner + HMAC digest
                     |
                     v
deployment resolves exact row -> verifies descriptor -> decrypts just in time
```

### Important files

- [`internal/secretcrypto/secretcrypto.go`](internal/secretcrypto/secretcrypto.go): cipher format, nonce generation, purpose binding.
- [`services/environment_secrets.go`](services/environment_secrets.go): creation, rotation, archive, revision commit, and resolution.
- [`models/environment_secret.go`](models/environment_secret.go): secret identity and normalized key rules.
- [`models/environment_state_revision.go`](models/environment_state_revision.go): secret descriptor verification.
- [`services/resource_management.go`](services/resource_management.go): Resource credential encryption and rotation projection.
- [`services/database_clusters.go`](services/database_clusters.go): cluster administrator credentials.
- [`services/github_connection.go`](services/github_connection.go): GitHub App secrets.
- [`services/backup_executor.go`](services/backup_executor.go): backup credential decryption.

### Review checklist

- [ ] Confirm using the session encryption key as the general master key is intentional and document rotation and recovery.
- [ ] Verify every encryption purpose is stable, unique, and reconstructable after restore.
- [ ] Decide whether generic-envelope backup and bootstrap WireGuard credentials should receive separate purpose bindings.
- [ ] Verify plaintext and derived command scripts cannot reach logs, errors, persisted job args, or telemetry.
- [ ] Verify secret rotation creates a new immutable value and a revision before old values become unusable.
- [ ] Verify Resource credential rotation updates every dependent environment revision atomically.
- [ ] Verify backups contain enough key material for recovery without making compromise of one artifact sufficient.

## 7. GitHub connection and source events

### What it does

An administrator creates a private GitHub App through the manifest flow, installs it, and synchronizes repositories. App credentials are encrypted locally. Webhooks are size-limited, HMAC-verified, deduplicated by delivery ID, sanitized before storage, and mapped to active environment sources. A matching push creates a source event, change, immutable build snapshot, pending build, and River job in one transaction.

```text
GitHub push webhook
       |
       +--> body size limit
       +--> HMAC SHA-256 verification
       +--> delivery advisory lock + deduplication
       +--> match installation/repository/reference
       |
       v
SourceEvent -> Change -> Build snapshot -> pending Build -> River job
                                                            |
                                                            v
                                                      BuildExecution
```

### Important files

- [`services/github_connection.go`](services/github_connection.go): App registration, installation, repository sync, credential rotation, API auth.
- [`services/github_webhook.go`](services/github_webhook.go): verification, delivery state, matching, and build creation.
- [`clients/github/client.go`](clients/github/client.go): GitHub App JWTs, installation tokens, API limits, redirects, archive download.
- [`models/git_hub_webhook_delivery.go`](models/git_hub_webhook_delivery.go): deduplication and delivery lifecycle.
- [`models/git_hub_environment_source.go`](models/git_hub_environment_source.go): environment-to-repository association.
- [`controllers/github_connections.go`](controllers/github_connections.go): admin setup routes and public webhook endpoint.

### Review checklist

- [ ] Verify replay, concurrent duplicate delivery, failed delivery retry, and GitHub redelivery behavior.
- [ ] Verify archived or suspended installations cannot start builds.
- [ ] Verify repository rename, removal, permission reduction, and branch deletion behavior.
- [ ] Verify exact commit resolution and archive redirect allowlisting.
- [ ] Verify sanitized webhook storage contains enough diagnostic data but no unnecessary sensitive content.
- [ ] Verify one push matching multiple environments behaves correctly within a single transaction.

## 8. Build pipeline

### What it does

A build consumes an immutable snapshot that names the exact source revision, environment revision, registry, Buildpacks settings, and selected build server. The worker downloads and safely extracts the GitHub archive, validates the context, runs Cloud Native Buildpacks locally or over SSH, publishes the image, resolves its immutable digest, creates a release, and queues deployment.

```text
pending Build
    |
    v
claim + mark Change/Build running
    |
    +--> validate immutable build snapshot
    +--> require selected Server build capability
    +--> download exact GitHub commit archive
    +--> safe extraction + context validation
    +--> authenticate to registry
    +--> Pack build and publish
    +--> resolve immutable sha256 image reference
    |
    v
transaction: Build succeeded + Release + Deployment + Instance + deploy job
```

### Important files

- [`services/build_execution.go`](services/build_execution.go): build state machine, workspace security, local/remote Pack execution, logs, completion.
- [`services/build_configuration.go`](services/build_configuration.go): snapshot serialization.
- [`clients/buildpacks/client.go`](clients/buildpacks/client.go): local Pack behavior and pinned images.
- [`clients/registry/client.go`](clients/registry/client.go): registry login and digest resolution.
- [`services/server_execution.go`](services/server_execution.go): certificate-authenticated remote execution.
- [`models/build.go`](models/build.go): build transition persistence.
- [`models/build_log.go`](models/build_log.go): durable bounded logs.
- [`models/release.go`](models/release.go): immutable artifact record.
- [`queue/build_source.go`](queue/build_source.go): timeout, retry, permanent failure behavior.
- [`queue/jobs/build.go`](queue/jobs/build.go): uniqueness and attempt policy.

### Review checklist

- [ ] Verify archive path, symlink, size, decompression, and workspace escape defenses.
- [ ] Verify cancellation terminates local and remote child processes and leaves state retryable.
- [ ] Verify a retry after image publication cannot create conflicting releases or deployments.
- [ ] Verify registry credentials are cleaned up locally and remotely on every exit path.
- [ ] Verify build logs cannot contain credentials and remain correctly ordered under concurrent stdout/stderr writes.
- [ ] Verify remote build architecture selection and pinned Buildpack/run-image compatibility.

## 9. Deployment, workloads, and Caddy routing

### What it does

A deployment combines a successful release with an exact environment revision and target. It resolves revision secrets, composes platform and Resource variables, reconciles a per-environment Docker network, and creates or adopts a labeled candidate container. It then adds the candidate to Caddy, reconciles the route, checks internal health, switches traffic when replacing an existing instance, verifies the public path, commits observed state, and removes old containers and backends. On the first deployment, the newly created backend begins at weight 100 before the explicit internal health loop, so Caddy's own active health behavior is part of the safety boundary.

```text
queued Deployment + candidate Instance
              |
              v
claim and load coherent Release/Target/Revision/Runtime
              |
              +--> resolve and verify revision secrets
              +--> reconcile Docker network and Resource attachments
              +--> run or adopt labeled candidate container
              +--> add/reconcile Caddy backend
              +--> internal health check
              +--> switch candidate to 100, previous to 0 when replacing
              +--> public health check
              |
        +-----+-----+
        |           |
      success     failure
        |           |
apply revision   restore previous traffic when possible
mark serving     mark failed and remove candidate
cleanup old
```

### Important files

- [`services/deployment_execution.go`](services/deployment_execution.go): end-to-end deployment and rollback logic.
- [`services/workload_execution.go`](services/workload_execution.go): local and remote workload operations.
- [`services/container_execution.go`](services/container_execution.go): container runtime adapter selection.
- [`clients/container/workload.go`](clients/container/workload.go): workload labels, ports, networks, and inspection.
- [`services/caddy_route.go`](services/caddy_route.go): route construction, backend weights, reconciliation, verification.
- [`models/deployment.go`](models/deployment.go): deployment status and system-update checkpoints.
- [`models/deployment_event.go`](models/deployment_event.go): ordered progress history.
- [`models/instance.go`](models/instance.go): candidate and serving instance state.
- [`queue/deploy_release.go`](queue/deploy_release.go): worker retry and terminal failure behavior.

### Review checklist

- [ ] Verify retry behavior at every external boundary: container created, backend added, traffic switched, public check passed, old container removed.
- [ ] Verify adoption requires enough immutable ownership labels to prevent controlling unrelated containers.
- [ ] Verify Resource container network attachment and pruning cannot affect another environment.
- [ ] Verify health checks cannot be redirected or made to access an unsafe target.
- [ ] Verify first-deployment traffic safety while its backend is already at weight 100 and the explicit internal health loop has not yet completed.
- [ ] Verify public-check rollback when there are zero, one, or several previous instances.
- [ ] Verify cleanup warnings are eventually reconciled, not only recorded.
- [ ] Verify only one deployment can be active for an environment target under concurrency.

## 10. Nodes, SSH trust, and private networking

### What it does

Users provision machines outside DeployCrate. Node enrollment probes and pins the initial host key, encrypts temporary root/bootstrap credentials, allocates a WireGuard address under a transaction advisory lock, then waits for explicit fingerprint confirmation. A one-attempt River job installs the node, joins it to WireGuard, verifies telemetry and short-lived SSH CA access, disables root/password SSH, persists capability and network state, and erases temporary credentials.

```text
user-provisioned Debian server
          |
          +--> probe public SSH host key
          +--> save encrypted temporary credential
          +--> user confirms fingerprint
          |
          v
node enrollment job
          |
          +--> run temporary bootstrap over public SSH
          +--> install users, Docker, WireGuard, telemetry, optional Buildpacks
          +--> apply peer on control plane
          +--> verify SSH CA access over 10.99.0.0/16
          +--> disable root and password SSH
          `--> clear bootstrap secret and mark Server configured
```

Ongoing remote operations use a five-minute SSH user certificate and the pinned host key over the WireGuard address.

### Important files

- [`services/node_enrollment.go`](services/node_enrollment.go): enrollment state machine and trust transition.
- [`internal/nodeinstall/manifest.go`](internal/nodeinstall/manifest.go): strict remote install contract.
- [`internal/nodeinstall/runner.go`](internal/nodeinstall/runner.go): node-side execution and lock.
- [`internal/nodeinstall/scripts/node-install.sh`](internal/nodeinstall/scripts/node-install.sh): actual node mutation.
- [`services/wireguard.go`](services/wireguard.go): address allocation and deterministic desired peers.
- [`services/ssh_ca.go`](services/ssh_ca.go): short-lived user certificates and host certificates.
- [`services/server_execution.go`](services/server_execution.go): trusted remote command path.
- [`models/server_capabilities.go`](models/server_capabilities.go): placement capability contract.
- [`models/node_enrollment.go`](models/node_enrollment.go): durable enrollment transitions.
- [`queue/node_enrollment.go`](queue/node_enrollment.go): background execution.

### Review checklist

- [ ] Verify host-key continuity before and after node installation and after IP or DNS changes.
- [ ] Verify a failed trust transition cannot strand the server without a valid recovery path.
- [ ] Verify the one-attempt worker plus explicit retry UI is intentional.
- [ ] Verify temporary private keys and passphrases are erased only after durable CA access succeeds.
- [ ] Verify WireGuard address reservations include servers and user devices without collisions.
- [ ] Verify capability declarations match what the node installer actually installed.
- [ ] Verify remote shell argument construction and streamed inputs are safe for every user-controlled value.

## 11. Managed Resources and private access

### What it does

A `Resource` is a deployable dependency such as PostgreSQL, MySQL, Redis, ClickHouse, an OCI registry, HTTP, or TCP. It may be managed by DeployCrate or external. Separate records describe installation, endpoint, credential, volume, mount, health check, and Environment attachments.

```text
Resource
   |
   +--> ResourceInstallation -> Server -> container runtime
   |          `--> mounts -> ResourceVolume
   +--> ResourceEndpoint -> optional PrivateNetwork
   +--> ResourceCredential -> encrypted secret values
   +--> ResourceHealthCheck -> latest expiring status
   `--> EnvironmentResource attachment
               |
               +--> Resource-managed key overrides
               `--> projected secrets in desired-state revision
```

Private device access creates a WireGuard device, Resource grant, host listener, and firewall rule. The client configuration is returned only when a new device is created.

### Important files

- [`models/resource_kind.go`](models/resource_kind.go): supported kinds, protocols, ports, credentials, and health checks.
- [`services/resource_management.go`](services/resource_management.go): Resource CRUD, deployment, credentials, and dependency checks.
- [`services/environment_setup.go`](services/environment_setup.go): Environment-owned Resource attachment workflow.
- [`services/resource_health.go`](services/resource_health.go): due-check selection, probes, thresholds, and expiring status.
- [`services/resource_private_access.go`](services/resource_private_access.go): device enrollment, listeners, firewall rules, revoke, observe.
- [`services/registry_resources.go`](services/registry_resources.go): external registry specialization.
- [`models/resource.go`](models/resource.go): Resource identity and management mode.
- [`controllers/resources.go`](controllers/resources.go): admin Resource surface.
- [`internal/resourceaccess/`](internal/resourceaccess/): host-side listener and firewall commands.

### Review checklist

- [ ] Verify Environment attachment changes preserve Resource endpoint, credential, placement, and network invariants.
- [ ] Verify Resource defaults and attachment-specific key overrides reconcile only their affected Environments and retain secret deployment state.
- [ ] Verify create/update/archive dependency checks are repeated inside a transaction where concurrency matters.
- [ ] Verify host effects are compensated when database persistence fails, and vice versa.
- [ ] Verify container names, port mappings, mount paths, image references, and server placement cannot escape managed scope.
- [ ] Verify credential projection cannot overwrite platform or user variables.
- [ ] Verify private listener/firewall cleanup after partial enrollment, revoke, Resource archive, and restart.
- [ ] Verify health error messages cannot expose credentials or connection details.

## 12. Database Clusters and published databases

### What it does

Database operations are modeled separately from generic Resources. A `DatabaseCluster` owns administrator credentials, operational endpoints, nodes, storage, and Docker/native installations. An individual `Database` can then be published as a Resource, with a Resource endpoint and application credential suitable for environment connections.

```text
DatabaseCluster
   |
   +--> administrator credential
   +--> operational endpoint
   +--> DatabaseClusterNode -> Server
   |          |
   |          +--> DatabaseNodeStorage
   |          `--> DatabaseNodeInstallation
   |                    +--> Docker installation
   |                    `--> native installation
   |
   `--> Database
          |
          v
      published Resource
          +--> Resource endpoint <-> cluster endpoint
          +--> application credential
          `--> environment/application grants
```

### Important files

- [`services/database_clusters.go`](services/database_clusters.go): cluster creation, placement, provisioning, publish, database lifecycle, archive.
- [`models/database_cluster.go`](models/database_cluster.go): cluster, node, storage, installation, credential, and endpoint entities.
- [`models/database.go`](models/database.go): logical database and Resource backing links.
- [`clients/postgresql/client.go`](clients/postgresql/client.go): PostgreSQL create, drop, readiness, and credential operations.
- [`clients/nativeinstall/client.go`](clients/nativeinstall/client.go): native installation adapter.
- [`controllers/database_clusters.go`](controllers/database_clusters.go): admin cluster surface.
- [`services/database_backups.go`](services/database_backups.go): per-database policy management.

### Review checklist

- [ ] Confirm which engines and installation methods are truly supported versus only present in catalog/schema.
- [ ] Verify provisioning is idempotent after container/native installation succeeds but status persistence fails.
- [ ] Verify operational cluster credentials and published application credentials have separate least-privilege roles.
- [ ] Verify database creation plus Resource publication is atomic or safely compensating.
- [ ] Verify deprovision and archive rules protect active connections, backups, and restores.
- [ ] Verify remote Server database operations use the same trust and capability checks as builds and workloads.

## 13. Telemetry, logs, and health

### What it does

Application logs, metrics, and traces use OpenTelemetry. Structured `slog` output always goes to stdout and can also go to an OTLP log endpoint. Metrics and traces are exported through configured OTLP endpoints. HTTP and River jobs create spans.

Host telemetry is a separate data path. Prometheus scrapes host, platform, and container metrics. A periodic service queries one-minute aggregate statistics, resolves trusted DeployCrate identity labels against PostgreSQL, rejects contradictory samples, and inserts seven-day rollups into ClickHouse. Logs flow through the collector into ClickHouse and are queried by system or environment identity.

```text
application slog + traces + metrics
              |
              v
      OpenTelemetry Collector
              |
              +--> ClickHouse logs
              `--> configured OTLP backends

node-exporter + cAdvisor + service metrics
              |
              v
          Prometheus raw data
              |
       periodic rollup query
              |
       identity verification in PostgreSQL
              |
              v
       ClickHouse metric rollups
              |
              v
     System and Environment screens
```

### Important files

- [`telemetry/telemetry.go`](telemetry/telemetry.go): providers, exporters, resource attributes, shutdown.
- [`telemetry/http.go`](telemetry/http.go): instrumented HTTP clients.
- [`router/router.go`](router/router.go): inbound HTTP tracing and exclusions.
- [`queue/telemetry.go`](queue/telemetry.go): River spans.
- [`services/metric_rollup.go`](services/metric_rollup.go): PromQL definitions, identity resolution, ClickHouse insertion, reads.
- [`services/environment_logs.go`](services/environment_logs.go): environment log filtering and cursor contract.
- [`services/system_logs.go`](services/system_logs.go): system log filtering and cursor contract.
- [`services/system_health.go`](services/system_health.go): live service, listener, network, target, disk, and active-slot checks.
- [`services/resource_health.go`](services/resource_health.go): Resource probe state machine.
- [`clients/clickhouse/client.go`](clients/clickhouse/client.go): log and rollup storage queries.
- [`internal/setup/scripts/otel-collector.sh`](internal/setup/scripts/otel-collector.sh) and [`internal/setup/scripts/prometheus.sh`](internal/setup/scripts/prometheus.sh): actual collection topology.

### Review checklist

- [ ] Verify label identity cannot be spoofed by user-controlled containers or stale database rows.
- [ ] Verify rollup formulas, units, counters versus gauges, time windows, and missing-sample semantics.
- [ ] Verify partial query failure does not produce misleading aggregate displays.
- [ ] Verify log queries enforce environment isolation and cursor ordering under equal timestamps.
- [ ] Verify sensitive attributes, HTTP headers, URL parameters, and command errors are filtered.
- [ ] Verify seven-day ClickHouse retention and 24-hour Prometheus retention match backup and diagnostic needs.
- [ ] Verify system health commands work under the exact production service account and sudo policy.

## 14. Backups, verification, retention, and restores

### What it does

Active policies are represented as durable River schedule jobs. A schedule or manual action creates a `Change`, `ChangeTask`, and pending `Backup`, then queues execution. Server backups use Restic. Managed local PostgreSQL backups create encrypted logical artifacts. Successful upload queues independent verification, and successful verification queues retention.

Database restore is a two-worker workflow. Preparation requires a verified source and creates a fresh safety backup. Only after that backup verifies does apply begin. Apply restores into a staging database, reconciles credentials, verifies it, blocks connections, renames the current database to a rollback name, activates the staged database, verifies again, and either removes the rollback copy or restores it.

```text
policy schedule/manual/startup
          |
          v
pending Backup -> execute -> uploaded -> verify -> verified -> retention -> pruned
                       |                    |
                       +--> object storage  `--> delete expired artifacts

restore request
      |
      v
prepare worker -> safety backup -> normal verify pipeline
                                      |
                                      v
                                  apply worker
                                      |
      verified source -> staging DB -> validate -> cutover -> verify
                                                |              |
                                                | failure      | success
                                                v              v
                                          rollback original  delete rollback DB
```

### Important files

- [`services/backup_scheduler.go`](services/backup_scheduler.go): schedule locking, initial/manual backups, transactional jobs.
- [`services/backup_executor.go`](services/backup_executor.go): scope validation, credential handling, dispatch, upload transition.
- [`services/server_backup.go`](services/server_backup.go): Restic server backup.
- [`services/database_backup.go`](services/database_backup.go): PostgreSQL logical backup.
- [`services/backup_verifier.go`](services/backup_verifier.go): artifact verification and next job.
- [`services/backup_retention.go`](services/backup_retention.go): expiry selection and pruning.
- [`services/database_restore_workflow.go`](services/database_restore_workflow.go): request, safety backup, prepare/apply state machine.
- [`services/database_restore.go`](services/database_restore.go): staging, cutover, interruption recovery, rollback.
- [`services/database_artifact.go`](services/database_artifact.go): artifact download and decryption.
- [`queue/jobs/backup.go`](queue/jobs/backup.go): serialized backup queue and retry policy.
- [`models/backup_policy.go`](models/backup_policy.go), [`models/backup.go`](models/backup.go), and [`models/database_restore.go`](models/database_restore.go): durable lifecycle state.

### Review checklist

- [ ] Verify schedule replacement cannot leave multiple effective future jobs.
- [ ] Verify backup execution and verification are idempotent after object upload or read succeeds but persistence fails.
- [ ] Verify server backup inclusion/exclusion captures every durable dependency and no transient or secret source that should be excluded.
- [ ] Verify artifact encryption, digest, producer version, metadata, and restore compatibility rules.
- [ ] Verify retention never prunes a backup referenced by an active restore or required recovery policy.
- [ ] Verify safety backup failure and interrupted cutover always produce an actionable durable state.
- [ ] Exercise every restore interruption point, especially after each database rename and connection toggle.
- [ ] Verify backups are restorable on a clean host, not only verifiable in place.

## 15. Self-update

### What it does

Self-update is a process-local background workflow guarded by a host file lock and durable deployment records. It downloads the next binary and checksum, validates the checksum and reported version, creates `Change`, `Release`, `Deployment`, `Instance`, and Caddy backend records, migrates with the target binary, points the inactive blue/green slot at it, starts and verifies that slot, switches Caddy, verifies the public path, changes boot enablement, and stops the old slot.

Each irreversible phase stores a checkpoint. Stopping the old slot also stops the process running the update, so durable finalization normally occurs when the target process starts and reconciles the unresolved deployment. A healthy target already serving traffic is completed; other states are rolled back to the previously active slot.

```text
authenticated user starts update
             |
             v
host lock + download + checksum/version verification
             |
             v
durable Change/Release/Deployment/Instance/backend
             |
             v
migrate -> install inactive slot -> start -> internal health
             |
             v
switch Caddy -> public health -> enable new boot slot
             |
             v
stop old slot -> commit new persisted topology

startup with unresolved checkpoint
             |
     +-------+-------+
     |               |
target healthy      anything else
and serving            |
     |                 v
complete records    restore old traffic/slot/service
```

### Important files

- [`services/self_update.go`](services/self_update.go): host lock, download, verification, blue/green effects, recovery, status.
- [`services/self_update_deployment.go`](services/self_update_deployment.go): durable records, checkpoints, events, final topology commit.
- [`services/caddy_route.go`](services/caddy_route.go): normal route domain used by persisted system topology.
- [`models/application.go`](models/application.go): query for the active system slot and route.
- [`models/deployment.go`](models/deployment.go): unresolved update and checkpoint storage.
- [`config/releases.go`](config/releases.go): release source resolution.
- [`clients/cloudflare/r2.go`](clients/cloudflare/r2.go): artifact download.
- [`controllers/self_updates.go`](controllers/self_updates.go): start/status HTTP routes.

### Review checklist

- [ ] Decide whether starting a system update should require `AdminOnly` rather than `AuthOnly`.
- [ ] Verify checksum authenticity, not only checksum equality, for every production channel.
- [ ] Verify database migrations are compatible with rollback to the old binary.
- [ ] Verify every checkpoint accurately follows the external effect it claims.
- [ ] Simulate process termination after every checkpoint and during every Caddy/systemd operation.
- [ ] Verify failure to persist the final topology cannot leave the new slot serving with old database state.
- [ ] Verify release pruning cannot remove a slot target or recovery artifact.

## End-to-end scenarios to review first

These scenarios cross the most boundaries and should expose incorrect assumptions early.

1. **Fresh install, crash, resume:** interrupt after each bootstrap phase, then resume and verify host state, database topology, Caddy, secrets, and first backups.
2. **First application deployment:** create GitHub App, application, environment, managed PostgreSQL Resource, user secret, initial build, and deployment to a worker node.
3. **Webhook race:** deliver the same push concurrently, redeliver it after a partial build failure, then verify one intended release/deployment history.
4. **Failed public cutover:** make the candidate internally healthy but publicly unhealthy and verify Caddy, containers, instances, target state, and events all return to a coherent state.
5. **Resource credential rotation:** rotate a database credential while a build or deployment is queued and verify immutable revision behavior.
6. **Worker trust failure:** interrupt node enrollment before and after SSH hardening and confirm recovery without accepting a changed host key.
7. **Backup disaster recovery:** restore a server backup to a clean host, then restore a managed database and verify applications can reconnect.
8. **Restore crash matrix:** terminate the process before and after staging creation, each rename, connection reopening, and final verification.
9. **Self-update crash matrix:** terminate the active slot after each checkpoint and verify startup reconciliation chooses completion or rollback correctly.
10. **Authorization audit:** exercise every mutating route as unauthenticated, ordinary authenticated, and administrator users.

## Review record template

Copy this block beneath a concept as it is reviewed.

```text
Reviewer:
Date:
Product intent confirmed: yes / no / needs decision

Invariants checked:
-

Failure and retry cases checked:
-

Security boundaries checked:
-

Evidence or tests:
-

Findings:
- [severity] description

Decision and follow-up:
-
```
