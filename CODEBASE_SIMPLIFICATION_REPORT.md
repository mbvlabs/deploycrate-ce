# Codebase Simplification and Deletion Report

Reviewed: 2026-08-05  
Target: current working tree in `/home/mbv/work/deploycrate-ce`

## Executive summary

The codebase contains several large, closed deletion seams. The best first pass is not a refactor. It is removal of unreachable subsystems and generated APIs with no callers.

The strongest candidates are:

1. Remove uncalled model CRUD methods and pagination types. About 6,000 lines sit in repeated model tails, including 52 unused `All` methods, 52 unused `Paginate` methods and result types, 48 unused `Upsert` methods, and 50 unused `Destroy` methods.
2. Delete 33 unused generated factory files totaling 4,439 lines. Generate a factory only when a test or seed needs it.
3. Delete the legacy Templ and Datastar delivery stack. `internal/hypermedia/` and `views/` alone total 2,638 lines and have no inbound package references.
4. Delete seven closed dead model families. Their model and factory code totals 2,806 lines, though much of this overlaps items 1 and 2.
5. Delete four unused Svelte UI families and unused wrappers inside live families. This removes roughly 1,875 lines and two frontend dependencies.
6. Squash 81 PostgreSQL and 10 ClickHouse development migrations into current baselines. The existing migration history is 2,622 lines.
7. Remove smaller dormant paths, pass-through services, generated helper APIs, and stale documentation.

Because several estimates overlap, they should not be added directly. A conservative first two waves can remove roughly 15,000 to 20,000 lines without deleting an active product feature. Larger product choices could remove several thousand more.

## Audit basis

- Inventoried about 104,000 lines across roughly 800 source and project files.
- Checked Go package imports with `go list`, receiver-qualified model call sites, Fx providers and invokes, controller route registration, River workers and job producers, Svelte imports, package dependencies, generated artifacts, migrations, and documentation.
- Verified the current tree with `go vet ./...` using an isolated build cache.
- Did not edit application code, run tests, or alter the existing dirty working tree.
- Current Caddy, Resource, and telemetry UI changes were treated as user-owned work and were not classified as disposable.

## Confirmed candidates

| Priority | Candidate | Evidence and deletion scope | Risk |
| --- | --- | --- | --- |
| P0 | Remove blanket model CRUD | Repeated blocks begin at examples such as [`models/application.go:184`](models/application.go#L184) and [`models/resource.go:309`](models/resource.go#L309). There are 52 unused `All`, 52 unused `Paginate`, 48 unused `Upsert`, and 50 unused `Destroy` methods. Only `Job.Paginate`, seven receiver-specific `Upsert` methods, and three `Destroy` methods are called. Remove the uncalled methods and 52 unused pagination result types. | Low, after regeneration policy is fixed |
| P0 | Delete unused factories | Thirty-three files in [`models/factories/`](models/factories) have no caller outside that package and total 4,439 lines. Retain factories used by seeds and the existing tests. Remove unused plural `Create...s` helpers from retained files too. | Low to medium |
| P0 | Delete legacy Templ and Datastar stack | [`internal/hypermedia/`](internal/hypermedia) has 1,535 lines and no inbound import. [`views/`](views) has 1,103 lines and no inbound import. The live app initializes Inertia in [`cmd/app/main.go:44`](cmd/app/main.go#L44) and resolves Svelte pages in [`resources/js/app.ts:8`](resources/js/app.ts#L8). Delete both directories, legacy CSS and Datastar assets, their routes and handlers, and [`controllers/cache.go`](controllers/cache.go). Keep email Templ files and `ViteBuild`. | Low |
| P0 | Delete closed dead domain subgraphs | The unreachable families are `EnvironmentHealthCheck`, `EnvironmentHealthCheckStatus`, `ChangeTaskAttempt`, `ChangeLog`, `OutboxEvent`, `NetworkAccessRule`, and `NetworkAccessRuleApplication`, declared around [`models/model.go:44`](models/model.go#L44). No service, controller, queue, command, other model, or raw table query uses them. Delete model, singleton, factory, and schema definitions after the outbox decision below. | Medium because some may represent planned work |
| P0 | Delete dead development release downloader | [`internal/setup/development_release.go:17`](internal/setup/development_release.go#L17) contains two unexported download functions with zero callers. Delete the 126-line file and its unused environment behavior. | Very low |
| P1 | Delete unused UI component families | No imports exist for `drawer`, `pagination`, `select`, or `tabs` under [`resources/js/Components/ui/`](resources/js/Components/ui). They total 875 lines. Removing `drawer` also removes `vaul-svelte`; `@internationalized/date` has no source imports and can also leave [`package.json`](package.json). | Low |
| P1 | Trim unused wrappers in live UI families | Unused barrel exports cover parts of alert-dialog, breadcrumb, button-group, collapsible, dialog, dropdown-menu, field, input-group, input-otp, native-select, scroll-area, sidebar, table, and sheet. The wrappers total about 1,000 lines. Remove `skeleton` after its only sidebar consumer goes. | Low |
| P1 | Remove unreachable registration and confirmation | [`controllers/confirmations.go:18`](controllers/confirmations.go#L18) is not provided or invoked by [`controllers/controller.go:13`](controllers/controller.go#L13). No registration controller exists. If closed registration is intentional, delete the confirmation controller, [`services/registration.go`](services/registration.go), confirmation routes, `Auth/ConfirmEmail.svelte`, and verification email template. | Medium to high product risk |
| P1 | Remove marketing email pipeline | `SendMarketingEmailArgs` has no producer. All references are its job, worker, email types, provider methods, and Fx registration at [`queue/workers.go:12`](queue/workers.go#L12). Delete that closed pipeline and the generic marketing tutorial in the README. Transactional reset-password email remains live. | Low if marketing is not planned |
| P1 | Prune generated validation surface | Application callers use only `Required`, `MinLen`, `MaxLen`, `OneOf`, and direct `Add`. Unused roots start at [`internal/validation/rules.go:22`](internal/validation/rules.go#L22), with matching unused conversion helpers in [`internal/validation/helpers.go:75`](internal/validation/helpers.go#L75). Expected removal is 300 to 350 lines. Also stop generating 26 `Validate() error { return nil }` methods. | Low, generator recurrence must be addressed |
| P1 | Shrink queue APIs | [`internal/storage/queue.go:13`](internal/storage/queue.go#L13) defines six insertion methods, but only `Insert` and `InsertTx` are called. Remove four bulk methods and pass-throughs in [`queue/queue.go:135`](queue/queue.go#L135). Also remove duplicate `Processor.Shutdown`, unused `DeleteJobTx`, and duplicate retry naming. | Low |
| P1 | Remove pass-through services | [`services/telemetry_identity.go:11`](services/telemetry_identity.go#L11) only forwards four methods. [`services/clickhouse_resource.go:10`](services/clickhouse_resource.go#L10) constructs a client, ignores context, and cannot return its declared error. Provide the underlying types directly and delete repeated impossible error branches. | Low to medium |
| P1 | Remove dead telemetry scaffolding | No-op exporters in [`telemetry/trace_exporters.go:61`](telemetry/trace_exporters.go#L61) and [`telemetry/metric_exporters.go:66`](telemetry/metric_exporters.go#L66), plus unused logging and health helpers, have no callers. Direct confirmed deletion is about 100 lines. | Very low |
| P1 | Remove unused routing and server generality | `RouteWithSerialID`, `RouteWithSlug`, `QueryParam`, and `JsExpr` are unused in [`internal/routing/routes.go`](internal/routing/routes.go); routing interfaces are used only for compile assertions. [`internal/server/server.go:19`](internal/server/server.go#L19) has unused options, ignored parameters, and a shutdowner list always passed as `nil`. | Low, generated source may need adjustment |
| P1 | Remove duplicate dependency version | Only `config/auth.go` and `config/aws_ses.go` still use `github.com/caarlos0/env/v10`; all other config uses v11. Move those two files to v11 and delete the v10 module and sums. | Low |
| P1 | Squash development migrations | There are 81 PostgreSQL SQL migrations and 10 ClickHouse SQL migrations, totaling 2,622 lines. The repository explicitly does not require backward compatibility during development. Replace them with current PostgreSQL and ClickHouse baselines and reset development databases. | Medium operational risk for existing local data |
| P1 | Prune stale README manual | [`README.md:350`](README.md#L350) onward contains hundreds of lines of generic Andurel, Templ, Datastar, Product scaffold, test, and marketing-email tutorials that do not describe the active application. Retain DeployCrate operations and link to Andurel documentation. Expected deletion is 700 to 900 lines. | Medium on the exact cut boundary |

## Additional small deletions

- Delete unused `ResourceCredentialSummary` in [`models/resource_detail.go:118`](models/resource_detail.go#L118).
- Delete unused `FlashWarning` in [`router/cookies/flash.go:37`](router/cookies/flash.go#L37).
- Delete unused `ApplicationSetup.Create` in [`services/application_setup.go:159`](services/application_setup.go#L159).
- Delete unused `InstallApplicationBinary` in [`internal/setup/persistence.go:322`](internal/setup/persistence.go#L322).
- Delete unused `CookieKey`, `ActorKey`, `SafeExtractContext`, and write-only `SessionCookieKey` under [`internal/request/`](internal/request).
- Delete the unused `insertOnly` field and constructor parameter from [`controllers/pages.go:21`](controllers/pages.go#L21).
- Replace duplicate `Processor.RunJobNow` and `Processor.RestartJob` at [`queue/queue.go:57`](queue/queue.go#L57) with one retry operation.
- Standardize or remove the undocumented bootstrap `host-resource-access` branch at [`cmd/bootstrap/main.go:57`](cmd/bootstrap/main.go#L57). The live app binary already owns the active dispatcher. Confirm no external installer invokes the bootstrap alias first.
- Delete `mise.toml` if `tuicr` is only a personal convenience. Confirm the external release process before deleting `scripts/install.sh`.

## Product decisions that unlock larger deletion

These are not safe mechanical deletions. They need MBV direction because they remove behavior or choose between competing architectures.

### 1. Outbox and JetStream versus River

The outbox code is completely dormant, while [`CONSTRAINTS.md:260`](CONSTRAINTS.md#L260) still requires outbox events and JetStream delivery. Current workflows instead insert River jobs transactionally. Choose one direction:

- If River is the durable delivery mechanism, delete the outbox model, table, factory, singleton, and the obsolete JetStream constraints.
- If JetStream remains a product requirement, the outbox is incomplete work and should not be presented as a deletion.

### 2. Resource topology actions

The local product vision says creation establishes one active installation, one volume, and an optional mount, while Show should observe and operate that topology. The current Resource Show still exposes generic add, edit, and archive actions around [`resources/js/Pages/Resources/Show.svelte:769`](resources/js/Pages/Resources/Show.svelte#L769), backed by six HTTP routes and about 250 service lines starting at [`services/resource_management.go:2361`](services/resource_management.go#L2361).

Removing generic topology mutation after creation would shrink the 1,000-line page, controller, routes, and service while preserving internal creation behavior.

### 3. Self-update ownership

The in-process updater spans more than 2,000 lines in [`services/self_update.go`](services/self_update.go), [`services/self_update_deployment.go`](services/self_update_deployment.go), R2 download code, release config, systemd slots, Caddy switching, status persistence, and rollback. If deployment can be owned externally, this is the largest single feature deletion. Risk is high because it removes a product capability.

### 4. Two metric pipelines

OTLP metrics already land in ClickHouse, while a second periodic path reads Prometheus and writes `metric_rollups` through [`services/metric_rollup.go`](services/metric_rollup.go), a worker, a Prometheus client, ClickHouse methods, and a separate table. If OTLP data can power system and Resource views with the same identity enrichment and backup semantics, more than 1,000 lines can go.

### 5. Flash behavior

Global Inertia flash sharing is commented out in [`internal/inertia/render.go:56`](internal/inertia/render.go#L56). Many controllers write flash cookies whose destination pages never expose them. Either restore one global shared prop and delete controller-specific mappings, or delete ineffective flash writes. Centralizing is likely the better user experience, but it is a behavior choice.

### 6. Telemetry UI convergence

Five chart components total 624 lines and overlap heavily. A configurable donut can replace `System/UsageDonut.svelte`, and one multi-series history component may replace one or two specialized histories. Likely deletion is 69 to 352 lines. These files currently contain user changes, so revisit after that work settles.

### 7. Dated audit document

[`DOMAIN_LOGIC_REVIEW.md`](DOMAIN_LOGIC_REVIEW.md) is an 876-line snapshot dated 2026-08-02. Move durable invariants into `CONSTRAINTS.md`, then delete the snapshot unless someone owns regular regeneration.

## Consolidations worth doing only after deletions

- Collapse the private telemetry option/exporter framework in [`telemetry/options.go`](telemetry/options.go). It is internal, has one production implementation per interface, and could lose another 250 to 350 lines.
- Deduplicate the PostgreSQL wrappers in [`database/database.go:26`](database/database.go#L26) and [`internal/storage/psql.go:43`](internal/storage/psql.go#L43), while retaining application tracing.
- Register River workers in one grouped path instead of 14 constructors, 14 invokes, and repeated one-line `Register` methods in [`queue/workers.go`](queue/workers.go).
- Add a slice registration method to the router only if it deletes the repeated controller `AddRoute` and `errors.Join` ceremony without creating a more abstract routing language.
- Revisit duplicated system and normal Resource DTO trees in [`controllers/system_resource_props.go`](controllers/system_resource_props.go) and [`controllers/resource_props.go`](controllers/resource_props.go).

## Recommended deletion order

1. Zero-behavior files and packages: development downloader, unused UI families, dead helpers, dead telemetry scaffolding, and pass-through aliases.
2. Legacy rendering slice: `internal/hypermedia`, `views`, legacy assets/routes/cache, and related request metadata.
3. Closed application slices: marketing email and, after confirmation, registration and the seven dead domain families.
4. Generated surface: uncalled model CRUD, unused factories, validators, routing variants, and queue bulk methods. Fix generation policy in the same change.
5. Development history: squash migrations and prune the generic README.
6. Product choices: outbox, Resource mutation surface, self-update, metric pipelines, and dated review artifacts.

Before implementing any wave, agree the affected test cases with MBV as required by the repository instructions, then use red/green testing at the highest practical layer.

## Keep

- Keep [`resources/js/routes.ts`](resources/js/routes.ts). It is generated but actively imported.
- Keep `assets/dist/` in the production build flow. It is ignored locally, but startup embeds its Vite manifest.
- Keep generated email `*_templ.go` files while reset-password email uses Templ.
- Keep `ViteBuild` in `controllers/assets.go`.
- Keep factories currently used by seeds and tests.
- Keep Resource volume and mount models because the creation flow still needs them, even if generic post-creation endpoints are removed.

## Working-tree caveat

The report reviews the current working tree, including user changes, but makes no recommendation to delete modified Caddy or Resource work. The new `services/resource_caddy_route.go` appears to repeat validation between `ValidateResourcePublication` and `SyncResourcePublication`, but that should be reassessed only after the current work settles.
