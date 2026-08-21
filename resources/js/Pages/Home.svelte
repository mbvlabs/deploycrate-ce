<script lang="ts">
  import ActivityIcon from "@lucide/svelte/icons/activity";
  import AppWindowIcon from "@lucide/svelte/icons/app-window";
  import ArrowRightIcon from "@lucide/svelte/icons/arrow-right";
  import CalendarDaysIcon from "@lucide/svelte/icons/calendar-days";
  import GitBranchIcon from "@lucide/svelte/icons/git-branch";
  import GitCommitHorizontalIcon from "@lucide/svelte/icons/git-commit-horizontal";
  import RocketIcon from "@lucide/svelte/icons/rocket";
  import TimerIcon from "@lucide/svelte/icons/timer";
  import { Link } from "@inertiajs/svelte";

  import StatusBadge from "@/Components/StatusBadge.svelte";
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
  };

  type ApplicationTelemetryPoint = {
    observedAt: string;
    requestsPerSecond: number;
    p95DurationMs: number;
  };

  type ApplicationTelemetry = {
    available: boolean;
    observedAt: string;
    requestsPerSecond: number;
    history: ApplicationTelemetryPoint[];
  };

  const emptyApplicationTelemetry: ApplicationTelemetry = {
    available: false,
    observedAt: "",
    requestsPerSecond: 0,
    history: [],
  };

  let {
    auth,
    dashboard,
    telemetry,
    applicationTelemetry = emptyApplicationTelemetry,
  }: {
    auth: { email: string };
    dashboard: Dashboard;
    telemetry: SystemTelemetry;
    applicationTelemetry?: ApplicationTelemetry;
  } = $props();

  const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  const timeFormatter = new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  });

  const deploymentValues = $derived(
    dashboard.deploymentActivity.map((point) => point.total),
  );
  const requestValues = $derived(
    applicationTelemetry.history.map((point) => point.requestsPerSecond),
  );
  const latencyValues = $derived(
    applicationTelemetry.history.map((point) => point.p95DurationMs),
  );
  const deploymentPath = $derived(sparklinePath(deploymentValues));
  const requestPath = $derived(sparklinePath(requestValues));
  const latencyPath = $derived(sparklinePath(latencyValues));
  const p95DurationMs = $derived(
    applicationTelemetry.history.at(-1)?.p95DurationMs ?? 0,
  );
  const hostUsage = $derived([
    { label: "CPU", percent: usagePercent(telemetry.cpu) },
    { label: "Memory", percent: usagePercent(telemetry.memory) },
    { label: "Storage", percent: usagePercent(telemetry.storage) },
  ]);
  function sparklinePath(values: number[], width = 112, height = 34) {
    if (values.length < 2) return "";
    const maximum = Math.max(1, ...values);
    return values
      .map((value, index) => {
        const x = (index / (values.length - 1)) * width;
        const y = height - (value / maximum) * (height - 6) - 3;
        return `${index === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
      })
      .join(" ");
  }

  function usagePercent(usage: { used: number; free: number }) {
    const total = usage.used + usage.free;
    if (!telemetry.available || total <= 0) return 0;
    return Math.min(100, Math.max(0, (usage.used / total) * 100));
  }

  function usageTone(value: number) {
    if (value >= 90) return "bg-destructive";
    if (value >= 75) return "bg-warning";
    return "bg-primary";
  }

  function deploymentTime(value: string) {
    return dateTimeFormatter.format(new Date(value));
  }

  function deploymentStatus(value: string) {
    if (value === "succeeded") return "succeeded";
    if (["queued", "running", "cancelling", "in_progress"].includes(value))
      return "in_progress";
    return "failed";
  }

  function deploymentDestination(deployment: Deployment) {
    const path = routes.environmentReleases(
      deployment.applicationId,
      deployment.environmentId,
    );
    return `${path}?deployment=${encodeURIComponent(deployment.id)}`;
  }

  function revisionLabel(value: string) {
    return value ? value.slice(0, 8) : "manual";
  }

  function applicationInitial(name: string) {
    return name.trim().charAt(0).toUpperCase() || "A";
  }

  function formatRate(value: number) {
    if (!applicationTelemetry.available) return "—";
    return value >= 100 ? value.toFixed(0) : value.toFixed(2);
  }
</script>

<svelte:head>
  <title>Overview</title>
</svelte:head>

<DashboardLayout email={auth.email} fullWidth>
  <div class="space-y-5 lg:space-y-6">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">Overview</h1>
        <p class="mt-1 text-sm text-muted-foreground">
          Everything running across your infrastructure.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div
          class="flex h-8 items-center gap-2 border border-border bg-background px-3 text-xs text-muted-foreground"
        >
          <CalendarDaysIcon class="size-3.5" />
          Last 7 days
        </div>
        <Button size="sm">
          {#snippet child({ props })}<Link
              {...props}
              href={routes.applicationNew()}>New application</Link
            >{/snippet}
        </Button>
      </div>
    </header>

    <section
      class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
      aria-label="Platform summary"
    >
      <Card.Root class="min-h-40 justify-between">
        <Card.Header>
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <span class="grid size-8 place-items-center border border-border">
                <AppWindowIcon class="size-4 text-muted-foreground" />
              </span>
              <Card.Title>Applications</Card.Title>
            </div>
            <span
              class="border border-border px-2 py-0.5 text-[10px] text-muted-foreground"
              >{dashboard.metrics.environments} envs</span
            >
          </div>
        </Card.Header>
        <Card.Content>
          <p class="font-mono text-4xl font-semibold tabular-nums">
            {dashboard.metrics.applications}
          </p>
          <p class="mt-2 text-xs text-muted-foreground">active services</p>
        </Card.Content>
      </Card.Root>

      <Card.Root class="min-h-40 justify-between">
        <Card.Header>
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <span class="grid size-8 place-items-center border border-border">
                <RocketIcon class="size-4 text-muted-foreground" />
              </span>
              <Card.Title>Deployments</Card.Title>
            </div>
            <span
              class="border border-primary/30 bg-primary/10 px-2 py-0.5 text-[10px] text-primary"
              >{dashboard.metrics.deploymentSuccess.toFixed(0)}% ok</span
            >
          </div>
        </Card.Header>
        <Card.Content class="flex items-end justify-between gap-5">
          <div>
            <p class="font-mono text-4xl font-semibold tabular-nums">
              {dashboard.metrics.deployments}
            </p>
            <p class="mt-2 text-xs text-muted-foreground">
              {dashboard.metrics.activeDeployments} active now
            </p>
          </div>
          <svg
            class="h-9 w-28 text-primary/70"
            viewBox="0 0 112 34"
            aria-hidden="true"
          >
            {#if deploymentPath}<path
                d={deploymentPath}
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              />{/if}
          </svg>
        </Card.Content>
      </Card.Root>

      <Card.Root class="min-h-40 justify-between">
        <Card.Header>
          <div class="flex items-center gap-3">
            <span class="grid size-8 place-items-center border border-border">
              <ActivityIcon class="size-4 text-muted-foreground" />
            </span>
            <Card.Title>Request rate</Card.Title>
          </div>
        </Card.Header>
        <Card.Content class="flex items-end justify-between gap-5">
          <div>
            <p class="font-mono text-4xl font-semibold tabular-nums">
              {formatRate(applicationTelemetry.requestsPerSecond)}<span
                class="ml-1 text-base text-muted-foreground">req/s</span
              >
            </p>
            <p class="mt-2 text-xs text-muted-foreground">
              DeployCrate CE traffic
            </p>
          </div>
          <svg
            class="h-9 w-28 text-primary/70"
            viewBox="0 0 112 34"
            aria-hidden="true"
          >
            {#if requestPath}<path
                d={requestPath}
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              />{/if}
          </svg>
        </Card.Content>
      </Card.Root>

      <Card.Root class="min-h-40 justify-between">
        <Card.Header>
          <div class="flex items-center gap-3">
            <span class="grid size-8 place-items-center border border-border">
              <TimerIcon class="size-4 text-muted-foreground" />
            </span>
            <Card.Title>P95 response</Card.Title>
          </div>
        </Card.Header>
        <Card.Content class="flex items-end justify-between gap-5">
          <div>
            <p class="font-mono text-4xl font-semibold tabular-nums">
              {applicationTelemetry.available
                ? p95DurationMs.toFixed(0)
                : "—"}<span class="ml-1 text-base text-muted-foreground"
                >ms</span
              >
            </p>
            <p class="mt-2 text-xs text-muted-foreground">system application</p>
          </div>
          <svg
            class="h-9 w-28 text-primary/70"
            viewBox="0 0 112 34"
            aria-hidden="true"
          >
            {#if latencyPath}<path
                d={latencyPath}
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              />{/if}
          </svg>
        </Card.Content>
      </Card.Root>
    </section>

    <section
      class="grid gap-5 xl:grid-cols-[minmax(0,1.45fr)_minmax(22rem,1fr)]"
    >
      <Card.Root class="min-w-0">
        <Card.Header class="border-b border-border">
          <Card.Title
            >Deployments <span class="ml-1 text-muted-foreground"
              >{dashboard.metrics.deployments}</span
            ></Card.Title
          >
        </Card.Header>
        <Card.Content class="p-0">
          {#if dashboard.recentDeployments.length === 0}
            <div class="grid min-h-80 place-items-center p-8 text-center">
              <div>
                <span
                  class="mx-auto grid size-10 place-items-center border border-dashed border-border text-muted-foreground"
                >
                  <RocketIcon class="size-4" />
                </span>
                <p class="mt-4 text-sm font-medium">No deployments yet</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  Your first release will appear here.
                </p>
              </div>
            </div>
          {:else}
            <div class="divide-y divide-border">
              {#each dashboard.recentDeployments as deployment (deployment.id)}
                <Link
                  href={deploymentDestination(deployment)}
                  class="group grid gap-3 px-5 py-4 hover:bg-muted/35 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"
                >
                  <span
                    class="grid size-8 place-items-center text-muted-foreground"
                  >
                    <GitCommitHorizontalIcon class="size-4" />
                  </span>
                  <div class="min-w-0">
                    <p class="truncate text-sm font-semibold">
                      {deployment.applicationName}
                    </p>
                    <p
                      class="mt-2 flex items-center gap-2 text-[10px] text-muted-foreground"
                    >
                      <span
                        class="inline-flex items-center gap-1 border border-border px-1.5 py-0.5"
                        ><GitBranchIcon
                          class="size-3"
                        />{deployment.environmentName}</span
                      >
                      <span class="border border-border px-1.5 py-0.5 font-mono"
                        >{revisionLabel(deployment.sourceRevision)}</span
                      >
                    </p>
                  </div>
                  <div class="flex flex-col items-start gap-2 sm:items-end">
                    <StatusBadge status={deploymentStatus(deployment.status)} />
                    <time
                      datetime={deployment.createdAt}
                      class="text-xs tabular-nums text-muted-foreground"
                    >
                      {deploymentTime(deployment.createdAt)}
                    </time>
                  </div>
                </Link>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header class="border-b border-border">
          <Card.Action>
            <span
              class="flex items-center gap-1.5 text-xs text-muted-foreground"
            >
              <span
                class={`size-1.5 rounded-full ${telemetry.available ? "bg-success" : "bg-muted-foreground"}`}
              ></span>
              {telemetry.available ? "all reachable" : "telemetry unavailable"}
            </span>
          </Card.Action>
          <Card.Title
            >Fleet <span class="ml-1 text-muted-foreground"
              >{dashboard.metrics.nodes}</span
            ></Card.Title
          >
        </Card.Header>
        <Card.Content class="p-0">
          <div class="px-5 py-5">
            <div class="flex items-center justify-between gap-4">
              <div class="flex items-center gap-2">
                <span
                  class={`size-2 rounded-full ${telemetry.available ? "bg-success" : "bg-muted-foreground"}`}
                ></span>
                <p class="text-sm font-semibold">Registered nodes</p>
              </div>
              {#if telemetry.available}
                <time
                  datetime={telemetry.observedAt}
                  class="text-[10px] text-muted-foreground"
                >
                  observed {timeFormatter.format(
                    new Date(telemetry.observedAt),
                  )}
                </time>
              {/if}
            </div>
            <div
              class="mt-6 grid gap-5 sm:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3"
            >
              {#each hostUsage as item (item.label)}
                <div>
                  <div
                    class="mb-2 flex items-center justify-between gap-2 text-[10px]"
                  >
                    <span class="uppercase tracking-wider text-muted-foreground"
                      >{item.label}</span
                    >
                    <span class="font-mono tabular-nums"
                      >{telemetry.available
                        ? `${item.percent.toFixed(0)}%`
                        : "—"}</span
                    >
                  </div>
                  <div class="h-1.5 bg-muted">
                    <div
                      class={`h-full ${usageTone(item.percent)}`}
                      style:width={`${item.percent}%`}
                    ></div>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        </Card.Content>
      </Card.Root>
    </section>

    <Card.Root>
      <Card.Header class="border-b border-border">
        <Card.Action>
          <Button size="sm" variant="ghost">
            {#snippet child({ props })}<Link
                {...props}
                href={routes.applications()}
                >View all<ArrowRightIcon data-icon="inline-end" /></Link
              >{/snippet}
          </Button>
        </Card.Action>
        <Card.Title
          >Applications <span class="ml-1 text-muted-foreground"
            >{dashboard.metrics.applications}</span
          ></Card.Title
        >
      </Card.Header>
      <Card.Content class="p-0">
        {#if dashboard.applications.length === 0}
          <div class="grid min-h-56 place-items-center p-8 text-center">
            <div>
              <p class="text-sm font-medium">No applications yet</p>
              <p class="mt-1 text-xs text-muted-foreground">
                Create an application and start shipping.
              </p>
              <Button class="mt-4" size="sm">
                {#snippet child({ props })}<Link
                    {...props}
                    href={routes.applicationNew()}>New application</Link
                  >{/snippet}
              </Button>
            </div>
          </div>
        {:else}
          <div class="divide-y divide-border">
            {#each dashboard.applications as application (application.id)}
              <Link
                href={routes.applicationShow(application.id)}
                class="group grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 px-5 py-4 hover:bg-muted/35"
              >
                <span
                  class="grid size-9 place-items-center bg-primary/15 font-mono text-sm font-semibold text-primary"
                  >{applicationInitial(application.name)}</span
                >
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold">
                    {application.name}
                  </p>
                  <p class="mt-1 truncate text-xs text-muted-foreground">
                    {application.environmentCount}
                    {application.environmentCount === 1
                      ? "environment"
                      : "environments"} · {application.deploymentCount} deployments
                  </p>
                </div>
                {#if application.latestDeploymentStatus}
                  <StatusBadge status={application.latestDeploymentStatus} />
                {:else}
                  <span class="text-xs text-muted-foreground"
                    >Ready to deploy</span
                  >
                {/if}
              </Link>
            {/each}
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
