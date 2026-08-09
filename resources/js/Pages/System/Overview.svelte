<script lang="ts">
  import BoxIcon from "@lucide/svelte/icons/box";
  import ContainerIcon from "@lucide/svelte/icons/container";
  import DownloadIcon from "@lucide/svelte/icons/download";
  import PowerIcon from "@lucide/svelte/icons/power";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import RotateCcwIcon from "@lucide/svelte/icons/rotate-ccw";
  import SquareTerminalIcon from "@lucide/svelte/icons/square-terminal";
  import Trash2Icon from "@lucide/svelte/icons/trash-2";
  import { Link, router, useForm } from "@inertiajs/svelte";

  import * as Accordion from "@/Components/ui/accordion";
  import * as Alert from "@/Components/ui/alert";
  import { Button } from "@/Components/ui/button";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import * as ScrollArea from "@/Components/ui/scroll-area";
  import { Separator } from "@/Components/ui/separator";
  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type SystemOverview = {
    applicationName: string;
    applicationSlug: string;
    environmentName: string;
    environmentKind: string;
    serverName: string;
    serverAddress: string;
    serverStatus: string;
    serverCapabilities: Record<string, unknown>;
    operatingSystem: string;
    distribution: string;
    distributionVersion: string;
    architecture: string;
    networkName: string;
    networkDriver: string;
    networkState: string;
    releaseVersion: string;
    artifactReference: string;
    deploymentStatus: string;
    deploymentStep: string;
    activeSlot: string;
    activeService: string;
    activeState: string;
    activePort: number;
    domain: string;
    routeExternalId: string;
    routeState: string;
    observedAt: string;
  };

  type SystemResource = {
    id: string;
    name: string;
    resourceType: string;
    engine: string;
    bindingAlias: string;
    credentialSource: string;
    hasCredential: boolean;
    endpointName: string;
    endpointRole: string;
    address: string;
    port: number;
    protocol: string;
    tlsMode: string;
    external: boolean;
    hasInstallation: boolean;
    imageReference: string;
    containerName: string;
    restartPolicy: string;
    volume: string;
    bind: string;
  };

  type SystemHealth = {
    ok: boolean;
    checkedAt: string;
    checks: Array<{
      name: string;
      ok: boolean;
      detail: string;
    }>;
  };

  type BackupHealthPolicy = {
    policyId: string;
    targetType: string;
    schedule: string;
    provider: string;
    bucket: string;
    prefix: string;
    lastStatus: string;
    lastError: string;
    lastSuccessfulAt: string | null;
    lastVerifiedAt: string | null;
    lastSizeBytes: number;
    activeOrRetrying: boolean;
  };

  type ServerContainer = {
    id: string;
    name: string;
    image: string;
    state: string;
    status: string;
    ports: string;
  };

  type ServerImage = {
    id: string;
    repository: string;
    tag: string;
    size: string;
  };

  type ServerUpdate = {
    name: string;
    installed: string;
    available: string;
  };

  type ServerUpdateState = {
    rebootRequired: boolean;
    total: number;
    updates: ServerUpdate[];
  };

  const capabilityOptions = [
    ["build", "Build"],
    ["runtime", "Runtime"],
    ["resource", "Resource"],
    ["database", "Database"],
    ["repository", "Repository"],
  ] as const;

  let {
    auth,
    system,
    resources,
    health,
    backups,
    containers,
    images,
    updates,
    containerLogs,
    containerLogsFor,
  }: {
    auth: { email: string };
    system: SystemOverview;
    resources: SystemResource[];
    health: SystemHealth;
    backups: BackupHealthPolicy[];
    containers: ServerContainer[];
    images: ServerImage[];
    updates: ServerUpdateState | null;
    containerLogs?: string;
    containerLogsFor?: string;
  } = $props();
  let openSections = $state([
    "network",
    "runtime",
    "resource",
    "backups",
    "deployments",
  ]);

  const stateLabel = (value: string) =>
    value ? value.replaceAll("_", " ") : "Unknown";
  const checkLabel = (value: string) =>
    stateLabel(value).replace(/\b\w/g, (letter) => letter.toUpperCase());
  const versionLabel = (version: string) =>
    version ? `v${version.replace(/^v/, "")}` : "Development build";
  const credentialSourceLabel = (source: string) =>
    source === "app_env" ? "Application environment" : stateLabel(source);
  const artifactAge = (value: string | null) => {
    if (!value) return "No verified backup";
    const hours = Math.max(
      0,
      Math.floor((Date.now() - new Date(value).getTime()) / 3_600_000),
    );
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  };
  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value < 0) return "Unavailable";
    if (value === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const index = Math.min(
      Math.floor(Math.log(value) / Math.log(1024)),
      units.length - 1,
    );
    return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
  };
  const capabilityValue = (value: unknown): string => {
    if (typeof value === "boolean") return value ? "Available" : "Unavailable";
    if (typeof value === "string") return checkLabel(value);
    if (typeof value === "number") return String(value);
    if (Array.isArray(value)) return value.map(capabilityValue).join(", ");
    if (value && typeof value === "object") {
      return Object.entries(value as Record<string, unknown>)
        .map(
          ([key, nestedValue]) =>
            `${checkLabel(key)}: ${capabilityValue(nestedValue)}`,
        )
        .join(" · ");
    }
    return "Unknown";
  };
  const capabilityEntries = $derived(
    Object.entries(system.serverCapabilities ?? {}),
  );
  const platformLabel = $derived(
    [system.distribution, system.distributionVersion, system.architecture]
      .filter(Boolean)
      .join(" ") ||
      system.operatingSystem ||
      "Unknown",
  );

  let busy = $state("");
  let logsOpen = $state(false);
  let logBusy = $state("");
  let removeContainer: ServerContainer | null = $state(null);
  let removeContainerOpen = $state(false);
  let removeImage: ServerImage | null = $state(null);
  let removeImageOpen = $state(false);
  let rebootOpen = $state(false);
  let rebooting = $state(false);
  let pruneOpen = $state(false);
  let pruning = $state(false);
  let pruneScopes = $state({ containers: true, images: true, volumes: false });
  let updatesBusy = $state("");
  let capabilitiesEditing = $state(false);

  const capabilitiesForm = useForm(() => ({
    build: capabilityEnabled("build"),
    runtime: capabilityEnabled("runtime"),
    resource: capabilityEnabled("resource"),
    database: capabilityEnabled("database"),
    repository: capabilityEnabled("repository"),
  }));

  function capabilityEnabled(key: string): boolean {
    return system.serverCapabilities?.[key] === true;
  }

  function containerActions(
    container: ServerContainer,
  ): Record<string, boolean> {
    return {
      start: container.state !== "running",
      stop: container.state === "running",
      restart: container.state === "running",
      remove: true,
    };
  }

  function perform(operation: string, data: Record<string, string>) {
    busy = `${operation}:${data.container ?? data.reference ?? ""}`;
    router.post(routes.systemHostContainersControl(), data, {
      only: ["containers", "flash"],
      onFinish: () => (busy = ""),
    });
  }

  function containerControl(operation: string, container: ServerContainer) {
    if (operation === "remove") {
      removeContainer = container;
      removeContainerOpen = true;
      return;
    }
    perform(operation, { operation, container: container.name });
  }

  function confirmRemoveContainer() {
    if (!removeContainer) return;
    const container = removeContainer;
    removeContainer = null;
    removeContainerOpen = false;
    perform("remove", { operation: "remove", container: container.name });
  }

  function fetchLogs(container: ServerContainer) {
    logBusy = container.name;
    router.post(
      routes.systemHostContainersLogs(),
      { container: container.name, tail: 200 },
      {
        only: ["containerLogs", "containerLogsFor", "flash"],
        onFinish: () => (logBusy = ""),
      },
    );
  }

  function openRemoveImage(image: ServerImage) {
    removeImage = image;
    removeImageOpen = true;
  }

  function confirmRemoveImage() {
    if (!removeImage) return;
    const image = removeImage;
    removeImage = null;
    removeImageOpen = false;
    busy = `image:${image.repository}:${image.tag}`;
    router.post(
      routes.systemHostImagesRemove(),
      {
        reference:
          image.repository === "<none>"
            ? image.id
            : `${image.repository}:${image.tag}`,
      },
      {
        only: ["images", "flash"],
        onFinish: () => (busy = ""),
      },
    );
  }

  function confirmReboot() {
    rebooting = true;
    router.post(routes.systemHostReboot(), {}, {
      only: ["flash"],
      onFinish: () => {
        rebooting = false;
        rebootOpen = false;
      },
    });
  }

  const pruneEnabled = $derived(
    Object.values(pruneScopes).some((enabled) => enabled),
  );

  function confirmPrune() {
    const scopes = Object.entries(pruneScopes)
      .filter(([, enabled]) => enabled)
      .map(([scope]) => scope);
    if (scopes.length === 0) return;
    pruning = true;
    router.post(routes.systemHostPrune(), { scopes }, {
      only: ["containers", "images", "flash"],
      onFinish: () => {
        pruning = false;
        pruneOpen = false;
      },
    });
  }

  function applyCapabilities(event: SubmitEvent) {
    event.preventDefault();
    $capabilitiesForm.post(routes.systemHostCapabilities(), {
      only: ["system", "flash"],
      onSuccess: () => (capabilitiesEditing = false),
    });
  }

  function checkUpdates() {
    updatesBusy = "check";
    router.post(routes.systemHostUpdatesCheck(), {}, {
      only: ["updates", "flash"],
      onFinish: () => (updatesBusy = ""),
    });
  }

  function applyUpdates() {
    updatesBusy = "apply";
    router.post(routes.systemHostUpdatesApply(), {}, {
      only: ["updates", "flash"],
      onFinish: () => (updatesBusy = ""),
    });
  }

  $effect(() => {
    if (containerLogsFor) logsOpen = true;
  });
</script>

<svelte:head>
  <title>System overview</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section
      class="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between"
    >
      <div class="max-w-3xl">
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          System
        </p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">
          {system.applicationName}
        </h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          The control plane is managed as a protected system application. Its
          runtime, network, resources, and deployment state are available here
          as read-only operational data.
        </p>
      </div>
      <Button variant="outline" size="sm">
        {#snippet child({ props })}
          <Link {...props} href={routes.systemUpdate()}>Manage updates</Link>
        {/snippet}
      </Button>
    </section>

    <Accordion.Root
      type="multiple"
      bind:value={openSections}
      class="grid gap-3"
    >
      <Accordion.Item value="network" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Network</p>
              <p class="mt-1 font-normal text-muted-foreground">
                WireGuard network and public route
              </p>
            </div>
            <StatusBadge status={system.networkState} />
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">Network</dt>
              <dd class="mt-1 text-sm font-medium">
                {system.networkName || "Not configured"}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Driver</dt>
              <dd class="mt-1 text-sm capitalize">
                {system.networkDriver || "Unknown"}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Domain</dt>
              <dd class="mt-1 break-all font-mono text-xs">{system.domain}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Route state</dt>
              <dd class="mt-1"><StatusBadge status={system.routeState} /></dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Caddy route</dt>
              <dd class="mt-1 break-all font-mono text-xs">
                {system.routeExternalId}
              </dd>
            </div>
          </dl>
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="runtime" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Runtime</p>
              <p class="mt-1 font-normal text-muted-foreground">
                System identity, host, service, and health
              </p>
            </div>
            <StatusBadge
              status={health.ok ? "healthy" : "unhealthy"}
              label={health.ok ? "Healthy" : "Attention required"}
            />
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <div class="space-y-6">
            <div>
              <h3
                class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
              >
                System identity
              </h3>
              <dl
                class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4"
              >
                <div>
                  <dt class="text-muted-foreground">Application</dt>
                  <dd class="mt-1 text-sm font-medium">
                    {system.applicationName}
                  </dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Application slug</dt>
                  <dd class="mt-1 font-mono text-xs">
                    {system.applicationSlug}
                  </dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Environment</dt>
                  <dd class="mt-1 text-sm font-medium">
                    {system.environmentName}
                  </dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Environment kind</dt>
                  <dd class="mt-1 text-sm capitalize">
                    {system.environmentKind}
                  </dd>
                </div>
              </dl>
            </div>

            <Separator />

            <div>
              <h3
                class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
              >
                Host
              </h3>
              <dl
                class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4"
              >
                <div>
                  <dt class="text-muted-foreground">Server</dt>
                  <dd class="mt-1 text-sm font-medium">{system.serverName}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Server state</dt>
                  <dd class="mt-1">
                    <StatusBadge status={system.serverStatus} />
                  </dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Address</dt>
                  <dd class="mt-1 font-mono text-xs">{system.serverAddress}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Platform</dt>
                  <dd class="mt-1 text-sm">{platformLabel}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Service</dt>
                  <dd class="mt-1 font-mono text-xs">{system.activeService}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Listener</dt>
                  <dd class="mt-1 font-mono text-xs">
                    127.0.0.1:{system.activePort}
                  </dd>
                </div>
              </dl>
              <div class="mt-6">
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <h4
                    class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
                  >
                    Capabilities
                  </h4>
                  {#if !capabilitiesEditing}
                    <Button
                      variant="outline"
                      size="sm"
                      onclick={() => (capabilitiesEditing = true)}
                    >
                      Manage capabilities
                    </Button>
                  {/if}
                </div>
                {#if capabilitiesEditing}
                  <form
                    class="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5"
                    onsubmit={applyCapabilities}
                  >
                    {#each capabilityOptions as [key, label] (key)}
                      <label
                        class="flex items-center gap-3 border border-border/70 bg-muted/20 p-3 text-sm"
                      >
                        <Checkbox
                          bind:checked={$capabilitiesForm[key]}
                          disabled={capabilityEnabled(key)}
                        />
                        <span>{label}</span>
                      </label>
                    {/each}
                    <p
                      class="text-xs text-muted-foreground sm:col-span-2 xl:col-span-5"
                    >
                      Provisioned capabilities cannot be removed from a
                      configured server.
                    </p>
                    <div
                      class="flex items-center gap-2 sm:col-span-2 xl:col-span-5"
                    >
                      <Button
                        type="submit"
                        size="sm"
                        disabled={$capabilitiesForm.processing}
                        aria-busy={$capabilitiesForm.processing}
                      >
                        {#if $capabilitiesForm.processing}<Spinner />{/if}
                        Save capabilities
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        type="button"
                        onclick={() => (capabilitiesEditing = false)}
                      >
                        Cancel
                      </Button>
                    </div>
                  </form>
                {:else if capabilityEntries.length}
                  <dl class="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                    {#each capabilityEntries as [key, value] (key)}
                      <div class="border border-border/70 bg-muted/20 p-3">
                        <dt class="text-xs text-muted-foreground">
                          {checkLabel(key)}
                        </dt>
                        <dd class="mt-1 break-words text-sm font-medium">
                          {capabilityValue(value)}
                        </dd>
                      </div>
                    {/each}
                  </dl>
                {:else}
                  <p class="mt-4 text-sm text-muted-foreground">
                    No server capabilities have been reported.
                  </p>
                {/if}
              </div>
            </div>

            <Separator />

            <div>
              <div class="flex flex-wrap items-end justify-between gap-3">
                <h3
                  class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
                >
                  Host maintenance
                </h3>
                <div class="flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onclick={() => (pruneOpen = true)}
                    disabled={!!busy}
                  >
                    <BoxIcon />
                    Prune
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onclick={() => (rebootOpen = true)}
                    aria-busy={rebooting}
                    disabled={!!busy}
                  >
                    {#if rebooting}<Spinner />{/if}
                    <PowerIcon />
                    Reboot server
                  </Button>
                </div>
              </div>

              <div class="mt-4">
                <h4
                  class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
                >
                  Containers
                </h4>
                {#if containers.length === 0}
                  <Empty.Root class="mt-4 border border-dashed border-border py-8"
                    ><Empty.Header
                      ><Empty.Media variant="icon"><ContainerIcon /></Empty.Media
                      ><Empty.Title>No containers</Empty.Title
                      ><Empty.Description
                        >Containers running on this host will appear here.</Empty.Description
                      ></Empty.Header
                    ></Empty.Root
                  >
                {:else}
                  <div class="mt-4 overflow-x-auto border border-border">
                    <Table.Root class="min-w-[760px]">
                      <Table.Header>
                        <Table.Row>
                          <Table.Head>Name</Table.Head>
                          <Table.Head>Image</Table.Head>
                          <Table.Head>State</Table.Head>
                          <Table.Head>Status</Table.Head>
                          <Table.Head>Ports</Table.Head>
                          <Table.Head class="w-44"
                            ><span class="sr-only">Actions</span></Table.Head
                          >
                        </Table.Row>
                      </Table.Header>
                      <Table.Body>
                        {#each containers as container (container.id)}
                          {@const actions = containerActions(container)}
                          <Table.Row>
                            <Table.Cell class="font-medium">
                              {container.name}
                            </Table.Cell>
                            <Table.Cell class="max-w-56 break-all font-mono text-xs">
                              {container.image}
                            </Table.Cell>
                            <Table.Cell>
                              <StatusBadge status={container.state} />
                            </Table.Cell>
                            <Table.Cell class="text-xs">
                              {container.status}
                            </Table.Cell>
                            <Table.Cell class="font-mono text-xs">
                              {container.ports}
                            </Table.Cell>
                            <Table.Cell>
                              <div class="flex justify-end gap-1">
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  class="size-8"
                                  title="Start"
                                  aria-label="Start {container.name}"
                                  disabled={!actions.start || !!busy}
                                  onclick={() => containerControl("start", container)}
                                >
                                  <BoxIcon class="size-4" />
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  class="size-8"
                                  title="Stop"
                                  aria-label="Stop {container.name}"
                                  disabled={!actions.stop || !!busy}
                                  onclick={() => containerControl("stop", container)}
                                >
                                  <SquareTerminalIcon class="size-4" />
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  class="size-8"
                                  title="Restart"
                                  aria-label="Restart {container.name}"
                                  disabled={!actions.restart || !!busy}
                                  onclick={() => containerControl("restart", container)}
                                >
                                  <RotateCcwIcon class="size-4" />
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  class="size-8"
                                  title="Logs"
                                  aria-label="Show logs for {container.name}"
                                  disabled={!!logBusy}
                                  onclick={() => fetchLogs(container)}
                                >
                                  {#if logBusy === container.name}
                                    <Spinner class="size-4" />
                                  {:else}
                                    <RefreshCwIcon class="size-4" />
                                  {/if}
                                </Button>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  class="size-8 text-destructive"
                                  title="Remove"
                                  aria-label="Remove {container.name}"
                                  disabled={!!busy}
                                  onclick={() => containerControl("remove", container)}
                                >
                                  <Trash2Icon class="size-4" />
                                </Button>
                              </div>
                            </Table.Cell>
                          </Table.Row>
                        {/each}
                      </Table.Body>
                    </Table.Root>
                  </div>
                {/if}
              </div>

              <div class="mt-8">
                <h4
                  class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
                >
                  Images
                </h4>
                {#if images.length === 0}
                  <Empty.Root class="mt-4 border border-dashed border-border py-8"
                    ><Empty.Header
                      ><Empty.Media variant="icon"><BoxIcon /></Empty.Media
                      ><Empty.Title>No images</Empty.Title
                      ><Empty.Description
                        >Images pulled to this host will appear here.</Empty.Description
                      ></Empty.Header
                    ></Empty.Root
                  >
                {:else}
                  <div class="mt-4 overflow-x-auto border border-border">
                    <Table.Root class="min-w-[560px]">
                      <Table.Header>
                        <Table.Row>
                          <Table.Head>Repository</Table.Head>
                          <Table.Head>Tag</Table.Head>
                          <Table.Head>Size</Table.Head>
                          <Table.Head class="w-16"
                            ><span class="sr-only">Actions</span></Table.Head
                          >
                        </Table.Row>
                      </Table.Header>
                      <Table.Body>
                        {#each images as image (`${image.id}\u0000${image.repository}\u0000${image.tag}`)}
                          <Table.Row>
                            <Table.Cell class="max-w-96 break-all font-mono text-xs">
                              {image.repository}
                            </Table.Cell>
                            <Table.Cell class="font-mono text-xs">
                              {image.tag}
                            </Table.Cell>
                            <Table.Cell class="text-xs">{image.size}</Table.Cell>
                            <Table.Cell class="text-right">
                              <Button
                                size="icon"
                                variant="ghost"
                                class="size-8 text-destructive"
                                title="Remove image"
                                aria-label="Remove image {image.repository}:{image.tag}"
                                disabled={!!busy}
                                onclick={() => openRemoveImage(image)}
                              >
                                <Trash2Icon class="size-4" />
                              </Button>
                            </Table.Cell>
                          </Table.Row>
                        {/each}
                      </Table.Body>
                    </Table.Root>
                  </div>
                {/if}
              </div>
            </div>

            <Separator />

            <div>
              <div class="flex flex-wrap items-end justify-between gap-3">
                <h3
                  class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
                >
                  Health checks
                </h3>
                <p class="text-xs text-muted-foreground">
                  Checked {new Date(health.checkedAt).toLocaleString()}
                </p>
              </div>
              <div class="mt-4 grid gap-3 md:grid-cols-2">
                {#each health.checks as check (check.name)}
                  <div class="border border-border/70 bg-muted/20 p-3">
                    <div class="flex items-start justify-between gap-4">
                      <p class="text-sm font-medium">
                        {checkLabel(check.name)}
                      </p>
                      <StatusBadge
                        status={check.ok ? "healthy" : "failed"}
                        label={check.ok ? "Passed" : "Failed"}
                      />
                    </div>
                    <p
                      class="mt-1 break-words text-xs leading-5 text-muted-foreground"
                    >
                      {check.detail}
                    </p>
                  </div>
                {/each}
              </div>
            </div>
          </div>
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="resource" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Resources</p>
              <p class="mt-1 font-normal text-muted-foreground">
                Infrastructure resources bound to the system environment
              </p>
            </div>
            <span class="text-muted-foreground"
              >{resources.length
                ? `${resources.length} configured`
                : "Not configured"}</span
            >
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          {#if resources.length}
            <div class="space-y-4">
              {#each resources as resource (resource.id)}
                <article class="border border-border/70 bg-muted/20 p-4">
                  <div class="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <p class="text-sm font-semibold">{resource.name}</p>
                      <p class="mt-1 font-mono text-xs text-muted-foreground">
                        {resource.bindingAlias}
                      </p>
                    </div>
                    <span class="text-xs capitalize text-muted-foreground">
                      {resource.external ? "External" : "Local"} · {resource.engine}
                    </span>
                  </div>

                  <dl
                    class="mt-5 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4"
                  >
                    <div>
                      <dt class="text-muted-foreground">Resource type</dt>
                      <dd class="mt-1 text-sm capitalize">
                        {resource.resourceType}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Credential source</dt>
                      <dd class="mt-1 text-sm">
                        {credentialSourceLabel(resource.credentialSource)}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Managed credential</dt>
                      <dd class="mt-1 text-sm">
                        {resource.hasCredential
                          ? "Configured"
                          : "Application environment"}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Endpoint</dt>
                      <dd class="mt-1 text-sm font-medium">
                        {resource.endpointName}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Role</dt>
                      <dd class="mt-1 text-sm capitalize">
                        {resource.endpointRole}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Address</dt>
                      <dd class="mt-1 break-all font-mono text-xs">
                        {resource.protocol}://{resource.address}:{resource.port}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">TLS mode</dt>
                      <dd class="mt-1 text-sm">
                        {resource.tlsMode || "Not configured"}
                      </dd>
                    </div>
                    {#if resource.hasInstallation}
                      <div>
                        <dt class="text-muted-foreground">Image</dt>
                        <dd class="mt-1 break-all font-mono text-xs">
                          {resource.imageReference}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-muted-foreground">Container</dt>
                        <dd class="mt-1 font-mono text-xs">
                          {resource.containerName}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-muted-foreground">Restart policy</dt>
                        <dd class="mt-1 text-sm capitalize">
                          {stateLabel(resource.restartPolicy)}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-muted-foreground">Volume</dt>
                        <dd class="mt-1 font-mono text-xs">
                          {resource.volume || "Not configured"}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-muted-foreground">Bind</dt>
                        <dd class="mt-1 font-mono text-xs">
                          {resource.bind || "Not configured"}
                        </dd>
                      </div>
                    {/if}
                    <div class="sm:col-span-2 xl:col-span-4">
                      <dt class="text-muted-foreground">Resource ID</dt>
                      <dd class="mt-1 break-all font-mono text-xs">
                        {resource.id}
                      </dd>
                    </div>
                  </dl>
                </article>
              {/each}
            </div>
          {:else}
            <Empty.Root class="py-8"
              ><Empty.Header
                ><Empty.Title>No active Resources</Empty.Title
                ><Empty.Description
                  >Resources bound to the system Environment will appear here.</Empty.Description
                ></Empty.Header
              ></Empty.Root
            >
          {/if}
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="backups" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Backups</p>
              <p class="mt-1 font-normal text-muted-foreground">
                Independent server and database recovery artifacts
              </p>
            </div>
            <span class="capitalize text-muted-foreground">
              {backups.length
                ? backups.some((backup) => backup.activeOrRetrying)
                  ? "Active or retrying"
                  : "Configured"
                : "Not configured"}
            </span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          {#if backups.length}
            <div class="grid gap-4 lg:grid-cols-2">
              {#each backups as backup (backup.policyId)}
                <div class="border border-border/70 bg-muted/20 p-4">
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <p class="text-sm font-medium capitalize">
                        {backup.targetType}
                      </p>
                      <p class="mt-1 font-mono text-xs text-muted-foreground">
                        {backup.schedule}
                      </p>
                    </div>
                    <StatusBadge
                      status={backup.activeOrRetrying
                        ? "running"
                        : backup.lastStatus}
                      label={backup.activeOrRetrying
                        ? "Active or retrying"
                        : undefined}
                    />
                  </div>
                  <dl class="mt-4 grid gap-4 sm:grid-cols-2">
                    <div>
                      <dt class="text-xs text-muted-foreground">Destination</dt>
                      <dd class="mt-1 break-all font-mono text-xs">
                        {backup.provider.toUpperCase()} / {backup.bucket}{backup.prefix
                          ? `/${backup.prefix}`
                          : ""}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">
                        Artifact age
                      </dt>
                      <dd class="mt-1 text-sm">
                        {artifactAge(backup.lastVerifiedAt)}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">
                        Last successful
                      </dt>
                      <dd class="mt-1 text-sm">
                        {backup.lastSuccessfulAt
                          ? new Date(backup.lastSuccessfulAt).toLocaleString()
                          : "Never"}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">
                        Last verified
                      </dt>
                      <dd class="mt-1 text-sm">
                        {backup.lastVerifiedAt
                          ? new Date(backup.lastVerifiedAt).toLocaleString()
                          : "Never"}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">Size</dt>
                      <dd class="mt-1 text-sm">
                        {formatBytes(backup.lastSizeBytes)}
                      </dd>
                    </div>
                  </dl>
                  {#if backup.lastError}
                    <Alert.Root variant="destructive" class="mt-4"
                      ><Alert.Title>Backup error</Alert.Title><Alert.Description
                        >{backup.lastError}</Alert.Description
                      ></Alert.Root
                    >
                  {/if}
                </div>
              {/each}
            </div>
          {:else}
            <Empty.Root class="py-8"
              ><Empty.Header
                ><Empty.Title>No backup policies</Empty.Title><Empty.Description
                  >Backups were not configured during bootstrap.</Empty.Description
                ></Empty.Header
              ></Empty.Root
            >
          {/if}
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="deployments" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Deployments</p>
              <p class="mt-1 font-normal text-muted-foreground">
                Active release, deployment, and systemd slot
              </p>
            </div>
            <StatusBadge status={system.deploymentStatus} />
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">Release</dt>
              <dd class="mt-1 text-sm font-medium">
                {versionLabel(system.releaseVersion)}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Deployment status</dt>
              <dd class="mt-1">
                <StatusBadge status={system.deploymentStatus} />
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Current step</dt>
              <dd class="mt-1 text-sm capitalize">
                {stateLabel(system.deploymentStep)}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Observed</dt>
              <dd class="mt-1 text-sm">
                {new Date(system.observedAt).toLocaleString()}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Active slot</dt>
              <dd class="mt-1 text-sm capitalize">{system.activeSlot}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Instance state</dt>
              <dd class="mt-1"><StatusBadge status={system.activeState} /></dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Service</dt>
              <dd class="mt-1 font-mono text-xs">{system.activeService}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Listener</dt>
              <dd class="mt-1 font-mono text-xs">
                127.0.0.1:{system.activePort}
              </dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Artifact</dt>
              <dd class="mt-1 break-all font-mono text-xs">
                {system.artifactReference}
              </dd>
            </div>
          </dl>
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="host-updates" class="min-w-0 border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Host updates</p>
              <p class="mt-1 font-normal text-muted-foreground">
                Operating system package updates for this server
              </p>
            </div>
            {#if updates && updates.total > 0}
              <StatusBadge
                status={updates.rebootRequired ? "warning" : "available"}
                label={`${updates.total} available`}
              />
            {:else if updates}
              <StatusBadge status="ready" label="Up to date" />
            {:else}
              <StatusBadge status="pending" label="Not checked" />
            {/if}
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <div class="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onclick={checkUpdates}
              disabled={!!updatesBusy}
            >
              {#if updatesBusy === "check"}<Spinner />{/if}
              <RefreshCwIcon />
              Check for updates
            </Button>
            <Button
              size="sm"
              onclick={applyUpdates}
              disabled={!!updatesBusy || updates === null || updates.total === 0}
              aria-busy={updatesBusy === "apply"}
            >
              {#if updatesBusy === "apply"}<Spinner />{/if}
              <DownloadIcon />
              Apply updates
            </Button>
            {#if updates?.rebootRequired}
              <Button
                variant="outline"
                size="sm"
                onclick={() => (rebootOpen = true)}
              >
                <PowerIcon />
                Reboot to complete
              </Button>
            {/if}
          </div>

          <div class="mt-6">
            {#if updates === null}
              <Empty.Root class="border border-dashed border-border py-8"
                ><Empty.Header
                  ><Empty.Title>No check run yet</Empty.Title
                  ><Empty.Description
                    >Run a check to see which operating system package updates
                    are available.</Empty.Description
                  ></Empty.Header
                ></Empty.Root
              >
            {:else if updates.total === 0}
              <Empty.Root class="border border-dashed border-border py-8"
                ><Empty.Header
                  ><Empty.Title>No pending updates</Empty.Title
                  ><Empty.Description
                    >Check again to refresh the list of available package
                    updates.</Empty.Description
                  ></Empty.Header
                ></Empty.Root
              >
            {:else}
              <div class="overflow-x-auto border border-border">
                <Table.Root class="min-w-[560px]">
                  <Table.Header>
                    <Table.Row>
                      <Table.Head>Package</Table.Head>
                      <Table.Head>Installed</Table.Head>
                      <Table.Head>Available</Table.Head>
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {#each updates.updates as update (update.name)}
                      <Table.Row>
                        <Table.Cell class="font-mono text-xs font-medium">
                          {update.name}
                        </Table.Cell>
                        <Table.Cell class="font-mono text-xs">
                          {update.installed}
                        </Table.Cell>
                        <Table.Cell class="font-mono text-xs">
                          {update.available}
                        </Table.Cell>
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              </div>
              {#if updates.rebootRequired}
                <Alert.Root variant="default" class="mt-4"
                  ><Alert.Title>Reboot required</Alert.Title><Alert.Description
                    >The applied kernel or system libraries will not take
                    effect until the server is rebooted.</Alert.Description
                  ></Alert.Root
                >
              {/if}
            {/if}
          </div>
        </Accordion.Content>
      </Accordion.Item>
    </Accordion.Root>
  </div>

  <Dialog.Root bind:open={logsOpen}>
    <Dialog.Content class="sm:max-w-3xl">
      <Dialog.Header>
        <Dialog.Title>Container logs</Dialog.Title>
        <Dialog.Description>
          Last 200 log lines for {containerLogsFor}.
        </Dialog.Description>
      </Dialog.Header>
      <ScrollArea.Root class="max-h-96">
        <pre class="whitespace-pre-wrap bg-muted/30 p-4 font-mono text-xs leading-5">
          {containerLogs}</pre
        >
      </ScrollArea.Root>
      <Dialog.Footer
        ><Button onclick={() => (logsOpen = false)}>Close</Button></Dialog.Footer
      >
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={removeContainerOpen}
    title="Remove container"
    description={`This force-removes the container "${removeContainer?.name}". The image is kept.`}
    confirmLabel="Remove container"
    destructive
    onconfirm={confirmRemoveContainer}
  />

  <ConfirmActionDialog
    bind:open={removeImageOpen}
    title="Remove image"
    description={`Delete the image "${removeImage?.repository}:${removeImage?.tag}" from this server?`}
    confirmLabel="Remove image"
    destructive
    requiredPhrase={removeImage?.tag === "<none>" ? "remove" : ""}
    onconfirm={confirmRemoveImage}
  />

  <ConfirmActionDialog
    bind:open={rebootOpen}
    title="Reboot server"
    description={`Reboot ${system.serverName} now? Active workloads will stop until the host comes back.`}
    confirmLabel="Reboot server"
    destructive
    processing={rebooting}
    requiredPhrase="reboot"
    onconfirm={confirmReboot}
  />

  <Dialog.Root bind:open={pruneOpen}>
    <Dialog.Content>
      <Dialog.Header>
        <Dialog.Title>Prune host artifacts</Dialog.Title>
        <Dialog.Description>
          Remove unused Docker artifacts from this server. Only the selected
          scopes are cleaned.
        </Dialog.Description>
      </Dialog.Header>
      <div class="grid gap-2">
        <label
          class="flex items-start gap-3 border border-border/70 bg-muted/20 p-3 text-sm"
        >
          <Checkbox bind:checked={pruneScopes.containers} class="mt-0.5" />
          <span class="grid gap-1">
            <span class="font-medium">Stopped containers</span>
            <span class="text-xs text-muted-foreground">
              Removes stopped containers that are not running.
            </span>
          </span>
        </label>
        <label
          class="flex items-start gap-3 border border-border/70 bg-muted/20 p-3 text-sm"
        >
          <Checkbox bind:checked={pruneScopes.images} class="mt-0.5" />
          <span class="grid gap-1">
            <span class="font-medium">Unused images</span>
            <span class="text-xs text-muted-foreground">
              Removes all images not referenced by a running or stopped
              container.
            </span>
          </span>
        </label>
        <label
          class="flex items-start gap-3 border border-border/70 bg-muted/20 p-3 text-sm"
        >
          <Checkbox bind:checked={pruneScopes.volumes} class="mt-0.5" />
          <span class="grid gap-1">
            <span class="font-medium">Unused volumes</span>
            <span class="text-xs text-muted-foreground">
              Removes volumes that are not referenced by any container.
            </span>
          </span>
        </label>
      </div>
      <Dialog.Footer>
        <Dialog.Close>Cancel</Dialog.Close>
        <Button
          variant="destructive"
          disabled={!pruneEnabled || pruning}
          aria-busy={pruning}
          onclick={confirmPrune}
        >
          {#if pruning}<Spinner />{/if}
          Prune
        </Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
