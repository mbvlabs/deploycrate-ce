<script lang="ts">
  import AppWindowIcon from "@lucide/svelte/icons/app-window";
  import ArrowRightIcon from "@lucide/svelte/icons/arrow-right";
  import DatabaseIcon from "@lucide/svelte/icons/database";
  import GitCommitHorizontalIcon from "@lucide/svelte/icons/git-commit-horizontal";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import RocketIcon from "@lucide/svelte/icons/rocket";
  import ServerIcon from "@lucide/svelte/icons/server";
  import { Link } from "@inertiajs/svelte";

  import StatusBadge from "@/Components/StatusBadge.svelte";
  import UsageHistory from "@/Components/System/UsageHistory.svelte";
  import UsageDonut from "@/Components/System/UsageDonut.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Metrics = {
    applications: number;
    environments: number;
    deployments: number;
    activeDeployments: number;
    successfulDeployments: number;
    finishedDeployments: number;
    deploymentSuccess: number;
    resources: number;
    nodes: number;
  };

  type DeploymentActivity = {
    day: string;
    total: number;
    succeeded: number;
    failed: number;
  };

  type Deployment = {
    id: string;
    applicationId: string;
    applicationName: string;
    environmentId: string;
    environmentName: string;
    environmentKind: string;
    status: string;
    currentStep: string;
    sourceRevision: string;
    createdAt: string;
  };

  type Application = {
    id: string;
    name: string;
    slug: string;
    environmentCount: number;
    deploymentCount: number;
    latestDeploymentStatus: string;
    latestDeploymentAt: string;
  };

  type Dashboard = {
    metrics: Metrics;
    deploymentActivity: DeploymentActivity[];
    recentDeployments: Deployment[];
    applications: Application[];
  };

  type SystemTelemetry = {
    available: boolean;
    observedAt: string;
    cpu: { used: number; free: number };
    memory: { used: number; free: number };
    storage: { used: number; free: number };
    memoryHistory: Array<{ observedAt: string; used: number; free: number }>;
    storageHistory: Array<{ observedAt: string; used: number; free: number }>;
  };

  let {
    auth,
    dashboard,
    telemetry,
  }: {
    auth: { email: string };
    dashboard: Dashboard;
    telemetry: SystemTelemetry;
  } = $props();

  const maxActivity = $derived(
    Math.max(1, ...dashboard.deploymentActivity.map((point) => point.total)),
  );
  const dayFormatter = new Intl.DateTimeFormat(undefined, { weekday: "short" });
  const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });

  function activityHeight(value: number) {
    if (value === 0) return 4;
    return Math.max(12, Math.round((value / maxActivity) * 112));
  }

  function dayLabel(day: string) {
    return dayFormatter.format(new Date(`${day}T12:00:00Z`));
  }

  function deploymentTime(value: string) {
    return dateTimeFormatter.format(new Date(value));
  }

  function revisionLabel(value: string) {
    return value ? value.slice(0, 8) : "manual";
  }

  function formatBytes(value: number) {
    if (!Number.isFinite(value) || value < 0) return "Unavailable";
    if (value === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const index = Math.min(
      Math.floor(Math.log(value) / Math.log(1024)),
      units.length - 1,
    );
    return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
  }

  const formatPercent = (value: number) => `${value.toFixed(1)}%`;
</script>

<svelte:head>
  <title>Dashboard</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-6 lg:space-y-8">
    <section aria-labelledby="host-usage-heading">
      <div class="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1
            id="host-usage-heading"
            class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground"
          >
            Host usage
          </h1>
          <p class="mt-1 text-xs text-muted-foreground">
            Latest ClickHouse telemetry for the system server
          </p>
        </div>
        {#if telemetry.available}
          <p class="text-xs text-muted-foreground">
            Observed {new Date(telemetry.observedAt).toLocaleString()}
          </p>
        {/if}
      </div>
      <div class="grid gap-3 lg:grid-cols-3">
        <UsageDonut
          label="CPU"
          used={telemetry.cpu.used}
          free={telemetry.cpu.free}
          formatValue={formatPercent}
          available={telemetry.available}
        />
        <UsageDonut
          label="Memory"
          used={telemetry.memory.used}
          free={telemetry.memory.free}
          formatValue={formatBytes}
          available={telemetry.available}
        />
        <UsageDonut
          label="Storage"
          used={telemetry.storage.used}
          free={telemetry.storage.free}
          formatValue={formatBytes}
          available={telemetry.available}
        />
      </div>
      <div class="mt-3 grid gap-3 xl:grid-cols-2">
        <UsageHistory
          label="Memory usage"
          points={telemetry.memoryHistory ?? []}
        />
        <UsageHistory
          label="Storage usage"
          points={telemetry.storageHistory ?? []}
        />
      </div>
    </section>

    <section
      class="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(18rem,0.75fr)]"
    >
      <Card.Root class="min-w-0">
        <Card.Header class="border-b border-border">
          <Card.Action
            ><span
              class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground"
              >7 day pulse</span
            ></Card.Action
          >
          <Card.Title>Deployment velocity</Card.Title>
          <Card.Description
            >Releases created across all application environments.</Card.Description
          >
        </Card.Header>
        <Card.Content>
          <div
            class="flex h-44 items-end gap-2 sm:gap-3"
            role="img"
            aria-label="Deployment activity over the last seven days"
          >
            {#each dashboard.deploymentActivity as point (point.day)}
              <div
                class="group flex min-w-0 flex-1 flex-col items-center gap-2"
              >
                <div
                  class="invisible text-[10px] font-medium tabular-nums group-hover:visible"
                >
                  {point.total}
                </div>
                <div class="flex h-28 w-full max-w-14 items-end bg-muted/45">
                  <div
                    class="w-full bg-primary transition-[height,opacity] duration-300 group-hover:opacity-80"
                    style:height={`${activityHeight(point.total)}px`}
                    title={`${point.total} deployments, ${point.succeeded} succeeded, ${point.failed} failed`}
                  ></div>
                </div>
                <span class="text-[10px] text-muted-foreground"
                  >{dayLabel(point.day)}</span
                >
              </div>
            {/each}
          </div>
          <div
            class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4 text-xs"
          >
            <p class="text-muted-foreground">
              {dashboard.metrics.deployments === 0
                ? "Your deployment history will appear here."
                : `${dashboard.metrics.deployments} deployments recorded`}
            </p>
            <span class="flex items-center gap-2"
              ><span class="size-2 bg-primary"></span>Total deployments</span
            >
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action
            ><StatusBadge
              status={dashboard.metrics.nodes > 0 ? "ready" : "pending"}
              label={dashboard.metrics.nodes > 0 ? "Available" : "Needs a node"}
            /></Card.Action
          >
          <Card.Title>Platform capacity</Card.Title>
          <Card.Description
            >Infrastructure available to your workloads.</Card.Description
          >
        </Card.Header>
        <Card.Content class="grid gap-3">
          <Link
            href={routes.nodes()}
            class="group flex items-center gap-4 border border-border bg-background/50 p-4 hover:border-primary/50"
          >
            <span
              class="grid size-10 place-items-center border border-border text-muted-foreground group-hover:text-primary"
              ><ServerIcon class="size-4" /></span
            >
            <div class="min-w-0 flex-1">
              <p class="font-mono text-xl font-semibold tabular-nums">
                {dashboard.metrics.nodes}
              </p>
              <p class="text-xs text-muted-foreground">Compute nodes</p>
            </div>
            <ArrowRightIcon class="size-4 text-muted-foreground" />
          </Link>
          <Link
            href={routes.resources()}
            class="group flex items-center gap-4 border border-border bg-background/50 p-4 hover:border-primary/50"
          >
            <span
              class="grid size-10 place-items-center border border-border text-muted-foreground group-hover:text-primary"
              ><DatabaseIcon class="size-4" /></span
            >
            <div class="min-w-0 flex-1">
              <p class="font-mono text-xl font-semibold tabular-nums">
                {dashboard.metrics.resources}
              </p>
              <p class="text-xs text-muted-foreground">Managed resources</p>
            </div>
            <ArrowRightIcon class="size-4 text-muted-foreground" />
          </Link>
        </Card.Content>
        <Card.Footer class="border-t border-border bg-muted/25 py-3">
          <p
            class="text-[10px] uppercase tracking-[0.16em] text-muted-foreground"
          >
            Ready for the next workload
          </p>
        </Card.Footer>
      </Card.Root>
    </section>

    <section class="grid gap-6 xl:grid-cols-2">
      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action>
            <Button size="sm" variant="ghost">
              {#snippet child({ props })}<Link
                  {...props}
                  href={routes.environments()}
                  >All environments<ArrowRightIcon
                    data-icon="inline-end"
                  /></Link
                >{/snippet}
            </Button>
          </Card.Action>
          <Card.Title>Latest deployments</Card.Title>
          <Card.Description
            >The freshest activity across your environments.</Card.Description
          >
        </Card.Header>
        <Card.Content class="p-0">
          {#if dashboard.recentDeployments.length === 0}
            <div class="grid min-h-64 place-items-center p-8 text-center">
              <div>
                <span
                  class="mx-auto grid size-10 place-items-center border border-dashed border-border text-muted-foreground"
                  ><RocketIcon class="size-4" /></span
                >
                <p class="mt-4 text-sm font-medium">No deployments yet</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  Your first release will light up this feed.
                </p>
              </div>
            </div>
          {:else}
            <div class="divide-y divide-border">
              {#each dashboard.recentDeployments as deployment (deployment.id)}
                <Link
                  href={routes.environmentShow(
                    deployment.applicationId,
                    deployment.environmentId,
                  )}
                  class="group flex items-center gap-3 px-4 py-3 hover:bg-muted/35"
                >
                  <span
                    class="grid size-8 shrink-0 place-items-center border border-border bg-background text-muted-foreground group-hover:text-primary"
                    ><GitCommitHorizontalIcon class="size-3.5" /></span
                  >
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium">
                      {deployment.applicationName}
                      <span class="text-muted-foreground"
                        >/ {deployment.environmentName}</span
                      >
                    </p>
                    <p
                      class="mt-1 truncate font-mono text-[10px] text-muted-foreground"
                    >
                      {revisionLabel(deployment.sourceRevision)} · {deploymentTime(
                        deployment.createdAt,
                      )}
                    </p>
                  </div>
                  <StatusBadge status={deployment.status} />
                </Link>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action>
            <Button size="sm" variant="ghost">
              {#snippet child({ props })}<Link
                  {...props}
                  href={routes.applications()}
                  >All applications<ArrowRightIcon
                    data-icon="inline-end"
                  /></Link
                >{/snippet}
            </Button>
          </Card.Action>
          <Card.Title>Applications</Card.Title>
          <Card.Description
            >Your most recently active services.</Card.Description
          >
        </Card.Header>
        <Card.Content class="p-0">
          {#if dashboard.applications.length === 0}
            <div class="grid min-h-64 place-items-center p-8 text-center">
              <div>
                <span
                  class="mx-auto grid size-10 place-items-center border border-dashed border-border text-muted-foreground"
                  ><AppWindowIcon class="size-4" /></span
                >
                <p class="mt-4 text-sm font-medium">The runway is clear</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  Create an application and start shipping.
                </p>
                <Button class="mt-4" size="sm">
                  {#snippet child({ props })}<Link
                      {...props}
                      href={routes.applicationNew()}
                      ><PlusIcon />New application</Link
                    >{/snippet}
                </Button>
              </div>
            </div>
          {:else}
            <div class="divide-y divide-border">
              {#each dashboard.applications as application (application.id)}
                <Link
                  href={routes.applicationShow(application.id)}
                  class="group flex items-center gap-3 px-4 py-3 hover:bg-muted/35"
                >
                  <span
                    class="grid size-8 shrink-0 place-items-center border border-border bg-primary/5 text-primary"
                    ><AppWindowIcon class="size-3.5" /></span
                  >
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium">
                      {application.name}
                    </p>
                    <p class="mt-1 truncate text-[10px] text-muted-foreground">
                      {application.environmentCount}
                      {application.environmentCount === 1
                        ? "environment"
                        : "environments"} · {application.deploymentCount} deployments
                    </p>
                  </div>
                  {#if application.latestDeploymentStatus}
                    <StatusBadge status={application.latestDeploymentStatus} />
                  {:else}
                    <span class="text-[10px] text-muted-foreground"
                      >Ready to deploy</span
                    >
                  {/if}
                </Link>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    </section>
  </div>
</DashboardLayout>
