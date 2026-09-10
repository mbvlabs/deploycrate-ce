<script lang="ts">
  import ArrowUpRightIcon from "@lucide/svelte/icons/arrow-up-right";
  import { Link, router } from "@inertiajs/svelte";
  import { untrack } from "svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as Table from "@/Components/ui/table";
  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Environment = {
    environmentId: string;
    environmentName: string;
    environmentKind: string;
    setupComplete: boolean;
    sourceType: "buildpacks" | "image";
    installationAccount: string;
    repositoryFullName: string;
    repositoryRemovedAt: unknown;
    installationSuspendedAt: unknown;
    reference: string;
    imageRepository: string;
    registryName: string;
    canPromoteToProduction: boolean;
    promotionTargetName: string;
    latestSuccessfulDeploymentId?: string;
    latestSuccessfulReleaseId?: string;
  };

  type Deployment = {
    id: string;
    environmentId: string;
    environmentName: string;
    environmentKind: string;
    status: string;
    currentStep: string;
    error: string;
    releaseId: string;
    sourceRevision: string;
    createdAt: string;
    active: boolean;
  };

  type Application = {
    id: string;
    name: string;
    slug: string;
    basicAuthUsername: string;
    hasBasicAuthDefault: boolean;
    environments: Environment[];
    deployments: Deployment[];
  };

  let {
    auth,
    application,
  }: {
    auth: { email: string };
    application: Application;
  } = $props();

  let deleteDialogOpen = $state(false);
  let deleteProcessing = $state(false);
  let promotionDialogOpen = $state(false);
  let promotionProcessing = $state(false);
  let promotionError = $state("");
  let basicAuthUsernameDraft = $state(
    untrack(() => application.basicAuthUsername || "deploycrate"),
  );
  let basicAuthPasswordDraft = $state("");
  let basicAuthProcessing = $state(false);
  let basicAuthError = $state("");
  let basicAuthPassword = $state("");
  let basicAuthPasswordOpen = $state(false);
  let clearBasicAuthOpen = $state(false);

  function generateLocalPassword() {
    const bytes = new Uint8Array(18);
    crypto.getRandomValues(bytes);
    basicAuthPasswordDraft = btoa(String.fromCharCode(...bytes))
      .replaceAll("+", "")
      .replaceAll("/", "")
      .replaceAll("=", "")
      .slice(0, 24);
  }

  async function saveBasicAuth(generate: boolean) {
    if (basicAuthProcessing) return;
    basicAuthProcessing = true;
    basicAuthError = "";
    const submittedPassword = generate ? "" : basicAuthPasswordDraft.trim();
    try {
      const response = await window.fetch(
        routes.applicationBasicAuthUpdate(application.id),
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            username: basicAuthUsernameDraft,
            password: submittedPassword,
            generate,
            clear: false,
          }),
        },
      );
      const payload = (await response.json()) as {
        username?: string;
        password?: string;
        error?: string;
      };
      if (!response.ok)
        throw new Error(
          payload.error || "Default basic authentication could not be saved",
        );
      if (payload.username) basicAuthUsernameDraft = payload.username;
      basicAuthPasswordDraft = "";
      if (payload.password || submittedPassword) {
        basicAuthPassword = payload.password || submittedPassword;
        basicAuthPasswordOpen = true;
      }
      router.reload({ only: ["application"], preserveScroll: true });
    } catch (error) {
      basicAuthError =
        error instanceof Error
          ? error.message
          : "Default basic authentication could not be saved";
    } finally {
      basicAuthProcessing = false;
    }
  }

  async function clearBasicAuth() {
    if (basicAuthProcessing) return;
    basicAuthProcessing = true;
    basicAuthError = "";
    try {
      const response = await window.fetch(
        routes.applicationBasicAuthUpdate(application.id),
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ clear: true }),
        },
      );
      const payload = (await response.json()) as { error?: string };
      if (!response.ok)
        throw new Error(
          payload.error || "Default basic authentication could not be cleared",
        );
      basicAuthUsernameDraft = "deploycrate";
      basicAuthPasswordDraft = "";
      clearBasicAuthOpen = false;
      router.reload({ only: ["application"], preserveScroll: true });
    } catch (error) {
      basicAuthError =
        error instanceof Error
          ? error.message
          : "Default basic authentication could not be cleared";
    } finally {
      basicAuthProcessing = false;
    }
  }

  const staging = $derived(
    application.environments.find(
      (environment) => environment.environmentKind === "staging",
    ) ?? null,
  );
  const production = $derived(
    application.environments.find(
      (environment) => environment.environmentKind === "production",
    ) ?? null,
  );
  const otherEnvironments = $derived(
    application.environments.filter(
      (environment) =>
        environment.environmentId !== staging?.environmentId &&
        environment.environmentId !== production?.environmentId,
    ),
  );
  function hasTimestamp(value: unknown) {
    if (typeof value === "string") return value.trim() !== "";
    if (!value || typeof value !== "object") return false;
    const timestamp = value as { Valid?: boolean; valid?: boolean };
    return Boolean(timestamp.Valid ?? timestamp.valid);
  }

  function healthy(environment: Environment) {
    return (
      environment.sourceType === "image" ||
      (!hasTimestamp(environment.repositoryRemovedAt) &&
        !hasTimestamp(environment.installationSuspendedAt))
    );
  }

  function sourceLabel(environment: Environment) {
    if (environment.sourceType === "image") {
      return [environment.registryName, environment.imageRepository]
        .filter(Boolean)
        .join(" / ");
    }
    return [environment.installationAccount, environment.repositoryFullName]
      .filter(Boolean)
      .join(" / ");
  }

  function latestDeployment(environmentId: string) {
    return (
      application.deployments.find(
        (deployment) => deployment.environmentId === environmentId,
      ) ?? null
    );
  }

  const formatTimestamp = (value: string) =>
    value ? new Date(value).toLocaleString() : "Not recorded";
  const short = (value: string) =>
    value ? value.slice(0, 12) : "Not recorded";
  const label = (value: string) =>
    value ? value.replaceAll("_", " ") : "Not recorded";

  function openEnvironment(environmentId: string) {
    router.visit(routes.environmentShow(application.id, environmentId));
  }

  function activateEnvironmentRow(event: KeyboardEvent, environmentId: string) {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    openEnvironment(environmentId);
  }

  function deleteApplication() {
    if (deleteProcessing) return;
    deleteProcessing = true;
    router.delete(routes.applicationDestroy(application.id), {
      onSuccess: () => (deleteDialogOpen = false),
      onFinish: () => (deleteProcessing = false),
    });
  }

  function askToPromote() {
    if (!staging) return;
    promotionError = "";
    promotionDialogOpen = true;
  }
  function promoteToProduction() {
    if (!staging || promotionProcessing) return;
    promotionProcessing = true;
    promotionError = "";
    router.post(
      routes.environmentPromoteToProduction(
        application.id,
        staging.environmentId,
      ),
      {},
      {
        preserveScroll: true,
        headers: { "X-Deploycrate-Return-To": "application" },
        onSuccess: () => {
          promotionDialogOpen = false;
          router.reload({ only: ["application"], preserveScroll: true });
        },
        onError: (errors) =>
          (promotionError =
            Object.values(errors).map(String).join("\n") ||
            "The release could not be promoted to production."),
        onFinish: () => (promotionProcessing = false),
      },
    );
  }
</script>

<svelte:head><title>{application.name}</title></svelte:head>

<DashboardLayout
  email={auth.email}
  applicationNavigation={{ id: application.id, name: application.name }}
>
  <div class="space-y-10">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Application
        </p>
        <h1 class="mt-3 text-3xl font-semibold">{application.name}</h1>
        <p class="mt-2 font-mono text-xs text-muted-foreground">
          {application.slug}
        </p>
      </div>
      <div class="flex gap-2">
        <Button variant="destructive" onclick={() => (deleteDialogOpen = true)}
          >Delete application</Button
        >
        <Button
          >{#snippet child({ props })}<Link
              {...props}
              href={routes.environmentNew(application.id)}>Add environment</Link
            >{/snippet}</Button
        >
      </div>
    </header>

    <Card.Root>
      <Card.Header
        ><Card.Action
          ><StatusBadge
            status={application.hasBasicAuthDefault ? "warning" : "applied"}
            label={application.hasBasicAuthDefault ? "Configured" : "None"}
          /></Card.Action
        ><Card.Title>Default basic authentication</Card.Title><Card.Description
          >Store a username and password you can apply to any Environment from
          its HTTP access card. Changing this default does not update
          Environments that already have credentials.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-4">
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-2">
            <p class="text-sm font-medium">Username</p>
            <Input
              bind:value={basicAuthUsernameDraft}
              autocomplete="off"
              aria-label="Default basic auth username"
            />
          </div>
          <div class="space-y-2">
            <p class="text-sm font-medium">Password</p>
            <Input
              bind:value={basicAuthPasswordDraft}
              type="text"
              autocomplete="off"
              placeholder={application.hasBasicAuthDefault
                ? "Leave blank to keep the current password"
                : "Enter a password or generate one"}
              aria-label="Default basic auth password"
            />
          </div>
        </div>
        <p class="text-sm text-muted-foreground">
          A generated password is shown once. DeployCrate stores only the hash.
        </p>
        {#if basicAuthError}
          <p class="text-sm text-destructive">{basicAuthError}</p>
        {/if}
        <div class="flex flex-wrap gap-2">
          <Button
            disabled={basicAuthProcessing}
            aria-busy={basicAuthProcessing}
            onclick={() => void saveBasicAuth(false)}
            >{#if basicAuthProcessing}<Spinner />{/if}Save default</Button
          >
          <Button
            variant="outline"
            disabled={basicAuthProcessing}
            onclick={generateLocalPassword}>Generate password</Button
          >
          <Button
            variant="outline"
            disabled={basicAuthProcessing}
            onclick={() => void saveBasicAuth(true)}
            >{#if basicAuthProcessing}<Spinner />{/if}Rotate password</Button
          >
          {#if application.hasBasicAuthDefault}
            <Button
              variant="outline"
              disabled={basicAuthProcessing}
              onclick={() => (clearBasicAuthOpen = true)}>Clear default</Button
            >
          {/if}
        </div>
      </Card.Content>
    </Card.Root>

    <section aria-labelledby="featured-environments-heading" class="space-y-4">
      <div>
        <h2
          id="featured-environments-heading"
          class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
        >
          Primary environments
        </h2>
      </div>
      <div class="grid gap-4 lg:grid-cols-2">
        {#each [{ kind: "staging", environment: staging }, { kind: "production", environment: production }] as featured (featured.kind)}
          {#if featured.environment}
            {@const environment = featured.environment}
            {@const deployment = latestDeployment(environment.environmentId)}
            <Link
              href={routes.environmentShow(
                application.id,
                environment.environmentId,
              )}
              class="group block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Card.Root
                class="h-full transition-colors group-hover:bg-muted/30"
              >
                <Card.Header>
                  <Card.Action
                    ><StatusBadge
                      status={deployment?.active
                        ? "serving"
                        : (deployment?.status ??
                          (environment.setupComplete ? "ready" : "pending"))}
                    /></Card.Action
                  >
                  <Card.Title class="flex items-center gap-2 capitalize"
                    >{featured.kind}<ArrowUpRightIcon
                      class="size-4 text-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5"
                    /></Card.Title
                  >
                  <Card.Description
                    >{environment.environmentName} · {sourceLabel(
                      environment,
                    )}</Card.Description
                  >
                </Card.Header>
                <Card.Content>
                  <p
                    class="text-[10px] uppercase tracking-[0.12em] text-muted-foreground"
                  >
                    Reference
                  </p>
                  <p class="mt-1 truncate font-mono text-xs">
                    {environment.reference || "Not configured"}
                  </p>
                </Card.Content>
                <Card.Footer
                  class="flex items-center justify-between gap-3 border-t border-border text-xs text-muted-foreground"
                >
                  <span
                    >{deployment
                      ? `${deployment.active ? "Serving" : label(deployment.status)} · ${short(deployment.sourceRevision || deployment.releaseId)}`
                      : "No deployments yet"}</span
                  >
                  {#if environment.environmentKind === "staging"}
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!environment.canPromoteToProduction ||
                        promotionProcessing}
                      aria-busy={promotionProcessing}
                      onclick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        askToPromote();
                      }}
                      >{#if promotionProcessing}<Spinner />{/if}Promote to
                      production</Button
                    >
                  {/if}
                </Card.Footer>
              </Card.Root>
            </Link>
          {:else}
            <Link
              href={routes.environmentNew(application.id)}
              class="group block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Card.Root
                class="h-full border-dashed bg-muted/10 transition-colors group-hover:bg-muted/30"
              >
                <Card.Header>
                  <Card.Title class="flex items-center gap-2 capitalize"
                    >{featured.kind}<ArrowUpRightIcon
                      class="size-4 text-muted-foreground"
                    /></Card.Title
                  >
                  <Card.Description
                    >No {featured.kind} environment has been configured.</Card.Description
                  >
                </Card.Header>
                <Card.Content
                  ><p class="text-sm font-medium">
                    Add {featured.kind} environment
                  </p></Card.Content
                >
              </Card.Root>
            </Link>
          {/if}
        {/each}
      </div>
    </section>

    <section aria-labelledby="other-environments-heading" class="space-y-4">
      <div>
        <h2
          id="other-environments-heading"
          class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
        >
          Other environments
        </h2>
        <p class="mt-1 text-sm text-muted-foreground">
          Every environment outside the primary staging and production pair.
        </p>
      </div>
      {#if otherEnvironments.length === 0}
        <Empty.Root class="border border-dashed border-border py-10"
          ><Empty.Header
            ><Empty.Title>No other environments</Empty.Title><Empty.Description
              >Additional environments will appear here.</Empty.Description
            ></Empty.Header
          ></Empty.Root
        >
      {:else}
        <div class="border border-border">
          <Table.Root>
            <Table.Header
              ><Table.Row
                ><Table.Head>Environment</Table.Head><Table.Head
                  >Kind</Table.Head
                ><Table.Head>Source</Table.Head><Table.Head
                  >Reference</Table.Head
                ><Table.Head>Latest deployment</Table.Head></Table.Row
              ></Table.Header
            >
            <Table.Body>
              {#each otherEnvironments as environment (environment.environmentId)}
                {@const deployment = latestDeployment(
                  environment.environmentId,
                )}
                <Table.Row
                  class="cursor-pointer"
                  tabindex={0}
                  onclick={() => openEnvironment(environment.environmentId)}
                  onkeydown={(event) =>
                    activateEnvironmentRow(event, environment.environmentId)}
                >
                  <Table.Cell
                    ><div class="font-medium">
                      {environment.environmentName}
                    </div>
                    <div class="text-xs text-muted-foreground">
                      {environment.environmentId}
                    </div></Table.Cell
                  >
                  <Table.Cell class="capitalize"
                    >{environment.environmentKind}</Table.Cell
                  >
                  <Table.Cell
                    ><StatusBadge
                      status={healthy(environment) ? "ready" : "degraded"}
                      label={environment.sourceType}
                    /></Table.Cell
                  >
                  <Table.Cell class="font-mono text-xs"
                    >{environment.reference || "Not configured"}</Table.Cell
                  >
                  <Table.Cell
                    >{#if deployment}<StatusBadge
                        status={deployment.active
                          ? "serving"
                          : deployment.status}
                      />{:else}<span class="text-muted-foreground">None</span
                      >{/if}</Table.Cell
                  >
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </div>
      {/if}
    </section>

    <section aria-labelledby="deployments-heading" class="space-y-4">
      <div>
        <h2
          id="deployments-heading"
          class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
        >
          Recent deployments
        </h2>
      </div>
      {#if application.deployments.length === 0}
        <Empty.Root class="border border-dashed border-border py-10"
          ><Empty.Header
            ><Empty.Title>No deployments yet</Empty.Title><Empty.Description
              >Deployment activity will appear here after the first release.</Empty.Description
            ></Empty.Header
          ></Empty.Root
        >
      {:else}
        <div class="border border-border">
          <Table.Root>
            <Table.Header
              ><Table.Row
                ><Table.Head>Environment</Table.Head><Table.Head
                  >Revision</Table.Head
                ><Table.Head>Status</Table.Head><Table.Head>Step</Table.Head
                ><Table.Head>Created</Table.Head></Table.Row
              ></Table.Header
            >
            <Table.Body>
              {#each application.deployments as deployment (deployment.id)}
                <Table.Row
                  class="cursor-pointer"
                  tabindex={0}
                  onclick={() => openEnvironment(deployment.environmentId)}
                  onkeydown={(event) =>
                    activateEnvironmentRow(event, deployment.environmentId)}
                >
                  <Table.Cell
                    ><div class="font-medium">{deployment.environmentName}</div>
                    <div class="text-xs capitalize text-muted-foreground">
                      {deployment.environmentKind}
                    </div></Table.Cell
                  >
                  <Table.Cell class="font-mono text-xs"
                    >{short(
                      deployment.sourceRevision || deployment.releaseId,
                    )}</Table.Cell
                  >
                  <Table.Cell
                    ><StatusBadge
                      status={deployment.active ? "serving" : deployment.status}
                    /></Table.Cell
                  >
                  <Table.Cell class="capitalize"
                    >{deployment.active
                      ? "serving"
                      : label(
                          deployment.currentStep || deployment.status,
                        )}</Table.Cell
                  >
                  <Table.Cell
                    >{formatTimestamp(deployment.createdAt)}</Table.Cell
                  >
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </div>
      {/if}
    </section>
  </div>

  <ConfirmActionDialog
    bind:open={deleteDialogOpen}
    title="Permanently delete application?"
    description={`Delete ${application.name}, every Environment, deployment record, secret, domain, and runtime workload. Shared Resources and Servers are kept. This cannot be undone.`}
    confirmLabel="Delete application"
    destructive
    processing={deleteProcessing}
    onconfirm={deleteApplication}
  />

  <ConfirmActionDialog
    bind:open={promotionDialogOpen}
    title="Promote staging to production?"
    description={`Create a new immutable production Release from the latest successful staging deployment (Release ${short(
      staging?.latestSuccessfulReleaseId ?? "",
    )}) and queue its deployment to ${
      staging?.promotionTargetName ?? "production"
    }. The staging deployment is left unchanged.`}
    confirmLabel="Promote to production"
    processing={promotionProcessing}
    error={promotionError}
    onconfirm={promoteToProduction}
  />

  <ConfirmActionDialog
    bind:open={clearBasicAuthOpen}
    title="Clear default basic authentication?"
    description="Environments that already use these credentials keep working. New Environments will no longer have a default to apply."
    confirmLabel="Clear default"
    destructive
    processing={basicAuthProcessing}
    error={basicAuthError}
    onconfirm={clearBasicAuth}
  />

  <Dialog.Root bind:open={basicAuthPasswordOpen}>
    <Dialog.Content
      ><Dialog.Header
        ><Dialog.Title>Default basic auth password</Dialog.Title><Dialog.Description
          >Copy this password now. DeployCrate stores only its hash and cannot
          show it again.</Dialog.Description
        ></Dialog.Header
      >
      <pre
        class="whitespace-pre-wrap break-all border border-border bg-muted/30 p-3 font-mono text-xs">{basicAuthPassword}</pre>
      <Dialog.Footer
        ><Button
          variant="outline"
          onclick={() => navigator.clipboard.writeText(basicAuthPassword)}
          >Copy password</Button
        ><Button onclick={() => (basicAuthPasswordOpen = false)}>Done</Button
        ></Dialog.Footer
      ></Dialog.Content
    >
  </Dialog.Root>
</DashboardLayout>
