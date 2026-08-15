<script lang="ts">
  import { page, router } from "@inertiajs/svelte";
  import RequestGeography from "@/Components/Applications/Environments/RequestGeography.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import TelemetryHistory from "@/Components/System/TelemetryHistory.svelte";
  import LogExplorer from "@/Components/Telemetry/LogExplorer.svelte";
  import TraceDetails from "@/Components/Telemetry/TraceDetails.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import * as NativeSelect from "@/Components/ui/native-select";
  import * as Table from "@/Components/ui/table";
  import { cn } from "@/lib/utils";
  import { routes } from "@/routes";
  import type {
    ApplicationTelemetry,
    QueryTelemetry,
    RouteTelemetry,
    TelemetryRange,
  } from "@/Pages/Applications/Environments/show.types";

  type OpenTelemetryView = "insights" | "logs" | "traces" | "database";
  type TelemetryResponseClass = "" | "2xx" | "3xx" | "4xx" | "5xx";
  type ChartSeries = {
    label: string;
    points: Array<{ observedAt: string; value: number }>;
  };

  let {
    applicationId,
    environmentId,
    telemetry,
    telemetryRange,
    live,
  }: {
    applicationId: string;
    environmentId: string;
    telemetry: ApplicationTelemetry;
    telemetryRange: TelemetryRange;
    live: boolean;
  } = $props();

  let expandedSlowQueries = $state<QueryTelemetry[] | null>(null);
  let expandedSlowQueriesKey = $state("");
  let slowQueriesLoading = $state(false);
  let slowQueriesError = $state("");

  const activeView = $derived.by<OpenTelemetryView>(() => {
    const view = new URLSearchParams($page.url.split("?")[1] ?? "").get("view");
    return view === "logs" || view === "traces" || view === "database"
      ? view
      : "insights";
  });
  const focusedTraceId = $derived.by(
    () => new URLSearchParams($page.url.split("?")[1] ?? "").get("trace") ?? "",
  );
  const traceResponseClass = $derived.by<TelemetryResponseClass>(() => {
    const value = new URLSearchParams($page.url.split("?")[1] ?? "").get(
      "responseClass",
    );
    return value === "2xx" ||
      value === "3xx" ||
      value === "4xx" ||
      value === "5xx"
      ? value
      : "";
  });
  const telemetryHref = (
    view: OpenTelemetryView,
    traceId = "",
    responseClass: TelemetryResponseClass = view === "traces"
      ? traceResponseClass
      : "",
  ) => {
    const query = new URLSearchParams({
      source: "opentelemetry",
      view,
      range: telemetryRange,
    });
    if (view === "traces" && traceId) query.set("trace", traceId);
    if (view === "traces" && responseClass)
      query.set("responseClass", responseClass);
    return `${routes.environmentTelemetry(applicationId, environmentId)}?${query.toString()}`;
  };
  const rangeLabel = $derived(
    {
      "1h": "the last hour",
      "6h": "the last 6 hours",
      "24h": "the last 24 hours",
      "7d": "the last 7 days",
    }[telemetryRange],
  );
  const rangeSeconds = $derived(
    { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 }[telemetryRange],
  );
  const history = $derived(telemetry.history ?? []);
  const databaseHistory = $derived(telemetry.database?.history ?? []);
  const recentTraces = $derived(telemetry.recentTraces ?? []);
  const slowRoutes = $derived((telemetry.routes ?? []).slice(0, 20));
  const pageRoute = (route: RouteTelemetry) =>
    route.method === "GET" &&
    route.route.startsWith("/") &&
    !["/api/", "/assets/", "/dist/"].some((prefix) =>
      route.route.startsWith(prefix),
    ) &&
    !["/favicon.ico", "/robots.txt"].includes(route.route);
  const popularPages = $derived(
    (telemetry.routes ?? [])
      .filter(pageRoute)
      .sort(
        (left, right) =>
          right.requests - left.requests ||
          left.route.localeCompare(right.route),
      )
      .slice(0, 8),
  );
  const pageRequests = $derived(
    popularPages.reduce((total, route) => total + route.requests, 0),
  );
  const slowQueriesKey = $derived(
    `${applicationId}:${environmentId}:${telemetryRange}`,
  );
  const slowQueriesExpanded = $derived(
    expandedSlowQueries !== null && expandedSlowQueriesKey === slowQueriesKey,
  );
  const slowQueries = $derived(
    slowQueriesExpanded
      ? (expandedSlowQueries ?? [])
      : (telemetry.queries ?? []),
  );
  const trafficSeries = $derived<ChartSeries[]>([
    {
      label: "Requests",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.requestsPerSecond,
      })),
    },
    {
      label: "Server errors",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.serverErrorsPerSecond,
      })),
    },
    {
      label: "Client errors",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.clientErrorsPerSecond,
      })),
    },
  ]);
  const latencySeries = $derived<ChartSeries[]>([
    {
      label: "p50",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.p50DurationMs,
      })),
    },
    {
      label: "p95",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.p95DurationMs,
      })),
    },
    {
      label: "p99",
      points: history.map((point) => ({
        observedAt: point.observedAt,
        value: point.p99DurationMs,
      })),
    },
  ]);
  const databaseActivitySeries = $derived<ChartSeries[]>([
    {
      label: "Operations",
      points: databaseHistory.map((point) => ({
        observedAt: point.observedAt,
        value: point.operationsPerSecond,
      })),
    },
    {
      label: "Errors",
      points: databaseHistory.map((point) => ({
        observedAt: point.observedAt,
        value: point.errorsPerSecond,
      })),
    },
  ]);
  const databaseLatencySeries = $derived<ChartSeries[]>([
    {
      label: "p50",
      points: databaseHistory.map((point) => ({
        observedAt: point.observedAt,
        value: point.p50DurationMs,
      })),
    },
    {
      label: "p95",
      points: databaseHistory.map((point) => ({
        observedAt: point.observedAt,
        value: point.p95DurationMs,
      })),
    },
    {
      label: "p99",
      points: databaseHistory.map((point) => ({
        observedAt: point.observedAt,
        value: point.p99DurationMs,
      })),
    },
  ]);
  const formatPerSecond = (value: number) =>
    `${value.toFixed(value < 1 ? 2 : 1)}/s`;
  const formatPercent = (value: number) => `${(value * 100).toFixed(2)}%`;
  const formatDuration = (milliseconds: number) =>
    milliseconds < 1
      ? `${(milliseconds * 1000).toFixed(0)} µs`
      : `${milliseconds.toFixed(1)} ms`;
  const formatSpanDuration = (nanoseconds: number) =>
    formatDuration(nanoseconds / 1_000_000);
  const short = (value: string) => (value ? value.slice(0, 8) : "Unknown");
  const stamp = (value: string) =>
    value ? new Date(value).toLocaleString() : "Unknown";
  async function toggleSlowQueries() {
    if (slowQueriesExpanded) {
      expandedSlowQueries = null;
      expandedSlowQueriesKey = "";
      slowQueriesError = "";
      return;
    }
    if (slowQueriesLoading) return;

    const requestKey = slowQueriesKey;
    const range = telemetryRange;
    slowQueriesLoading = true;
    slowQueriesError = "";
    try {
      const endpoint = new URL(
        routes.environmentTelemetryQueries(applicationId, environmentId),
        window.location.origin,
      );
      endpoint.searchParams.set("range", range);
      const response = await window.fetch(endpoint, {
        cache: "no-store",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      });
      if (!response.ok)
        throw new Error(`Slow queries returned ${response.status}`);
      const snapshot = (await response.json()) as { queries: QueryTelemetry[] };
      if (requestKey !== slowQueriesKey) return;
      expandedSlowQueries = snapshot.queries;
      expandedSlowQueriesKey = requestKey;
    } catch {
      if (requestKey === slowQueriesKey)
        slowQueriesError = "The additional slow queries could not be loaded.";
    } finally {
      slowQueriesLoading = false;
    }
  }

  function focusTrace(traceId: string) {
    router.visit(telemetryHref("traces", traceId), {
      preserveScroll: true,
      preserveState: true,
    });
  }

  function clearTraceFocus() {
    router.visit(telemetryHref("traces"), {
      preserveScroll: true,
      preserveState: true,
    });
  }

  function filterTraces(event: Event) {
    const responseClass = (event.currentTarget as HTMLSelectElement)
      .value as TelemetryResponseClass;
    router.visit(telemetryHref("traces", "", responseClass), {
      preserveScroll: true,
      preserveState: true,
    });
  }

</script>

<nav
  class="flex flex-wrap gap-2 border-b border-border pb-3"
  aria-label="OpenTelemetry views"
>
  <Button
    size="sm"
    variant={activeView === "insights" ? "default" : "ghost"}
    href={telemetryHref("insights")}>Insights</Button
  >
  <Button
    size="sm"
    variant={activeView === "logs" ? "default" : "ghost"}
    href={telemetryHref("logs")}>Logs</Button
  >
  <Button
    size="sm"
    variant={activeView === "traces" ? "default" : "ghost"}
    href={telemetryHref("traces")}>Traces</Button
  >
  <Button
    size="sm"
    variant={activeView === "database" ? "default" : "ghost"}
    href={telemetryHref("database")}>Database</Button
  >
</nav>

{#if activeView === "insights"}
  <div class="space-y-6">
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Card.Root>
        <Card.Header
          ><Card.Title class="text-sm">Requests</Card.Title></Card.Header
        >
        <Card.Content>
          <p class="text-2xl font-semibold">
            {telemetry.available
              ? formatPerSecond(telemetry.requestsPerSecond)
              : "Unavailable"}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">Current throughput</p>
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header
          ><Card.Title class="text-sm">Server errors</Card.Title></Card.Header
        >
        <Card.Content>
          <p
            class={cn("text-2xl font-semibold", {
              "text-destructive": telemetry.serverErrorRate > 0,
            })}
          >
            {telemetry.available
              ? formatPercent(telemetry.serverErrorRate)
              : "Unavailable"}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">5xx request rate</p>
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header
          ><Card.Title class="text-sm">Client errors</Card.Title></Card.Header
        >
        <Card.Content>
          <p class="text-2xl font-semibold">
            {telemetry.available
              ? formatPercent(telemetry.clientErrorRate)
              : "Unavailable"}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">4xx request rate</p>
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header
          ><Card.Title class="text-sm">Mean latency</Card.Title></Card.Header
        >
        <Card.Content>
          <p class="text-2xl font-semibold">
            {telemetry.available
              ? formatDuration(telemetry.meanRequestDurationMs)
              : "Unavailable"}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">Recent requests</p>
        </Card.Content>
      </Card.Root>
    </div>

    <div class="grid gap-4 xl:grid-cols-2">
      <TelemetryHistory
        label="Traffic and errors"
        description={`Application request rates from ${rangeLabel}`}
        series={trafficSeries}
        formatValue={formatPerSecond}
        windowSeconds={rangeSeconds}
      />
      <TelemetryHistory
        label="Response latency"
        description="Median and tail latency by time window"
        series={latencySeries}
        formatValue={formatDuration}
        windowSeconds={rangeSeconds}
      />
    </div>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
      <RequestGeography countries={telemetry.countries ?? []} />
      <Card.Root class="h-full gap-0 py-0">
        <Card.Header class="border-b border-border py-4">
          <Card.Title>Popular pages</Card.Title>
          <Card.Description
            >GET routes ranked by requests across {rangeLabel}.</Card.Description
          >
        </Card.Header>
        <Card.Content class="p-4">
          {#if popularPages.length}
            <ol class="space-y-2">
              {#each popularPages as route (`${route.method}:${route.route}`)}
                <li class="relative overflow-hidden px-3 py-2">
                  <div
                    class="absolute inset-y-0 left-0 bg-muted"
                    style:width={`${Math.max(
                      pageRequests > 0
                        ? (route.requests / pageRequests) * 100
                        : 0,
                      3,
                    )}%`}
                  ></div>
                  <div class="relative flex items-center justify-between gap-3">
                    <span class="truncate font-mono text-xs font-medium"
                      >{route.route}</span
                    >
                    <span
                      class="shrink-0 text-xs tabular-nums text-muted-foreground"
                      >{route.requests.toLocaleString()}</span
                    >
                  </div>
                </li>
              {/each}
            </ol>
          {:else}
            <Empty.Root class="py-10">
              <Empty.Header>
                <Empty.Title>No page metrics yet</Empty.Title>
                <Empty.Description
                  >Instrumented GET routes appear here after traffic is
                  observed.</Empty.Description
                >
              </Empty.Header>
            </Empty.Root>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>Slow endpoints</Card.Title>
        <Card.Description
          >Routes ranked by p95 response time across {rangeLabel}.</Card.Description
        >
      </Card.Header>
      <Card.Content>
        {#if slowRoutes.length}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[680px] text-xs">
              <Table.Header>
                <Table.Row>
                  <Table.Head>Method</Table.Head>
                  <Table.Head>Route</Table.Head>
                  <Table.Head>Requests</Table.Head>
                  <Table.Head>Error rate</Table.Head>
                  <Table.Head>p95 latency</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#each slowRoutes as route (`${route.method}:${route.route}`)}
                  <Table.Row>
                    <Table.Cell class="font-mono"
                      >{route.method || "—"}</Table.Cell
                    >
                    <Table.Cell class="font-mono font-medium"
                      >{route.route}</Table.Cell
                    >
                    <Table.Cell
                      >{formatPerSecond(route.requestsPerSecond)}</Table.Cell
                    >
                    <Table.Cell
                      class={route.errorRate > 0
                        ? "text-destructive"
                        : undefined}
                      >{formatPercent(route.errorRate)}</Table.Cell
                    >
                    <Table.Cell
                      >{formatDuration(route.p95DurationMs)}</Table.Cell
                    >
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {:else}
          <Empty.Root class="py-10">
            <Empty.Header>
              <Empty.Title>No route metrics yet</Empty.Title>
              <Empty.Description
                >Instrumented HTTP route metrics will appear here after traffic
                is observed.</Empty.Description
              >
            </Empty.Header>
          </Empty.Root>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>
{/if}

{#if activeView === "logs"}
  <LogExplorer
    active
    endpoint={routes.environmentTelemetryLogs(applicationId, environmentId)}
    range={telemetryRange}
    {rangeLabel}
    {live}
    title="Application logs"
    description={`Structured OpenTelemetry logs from ${rangeLabel}. Search across messages, attributes, trace IDs, span IDs, and services.`}
    searchId="environment-otel-log-search"
    searchPlaceholder="Search messages, attributes, traces, or services"
    source={(log) => log.service || log.processName || "application"}
    emptyMessage={`No OpenTelemetry logs were collected in ${rangeLabel}.`}
    filteredEmptyMessage={() =>
      `No logs in ${rangeLabel} match the current filters.`}
    loadingMessage="Loading OpenTelemetry logs..."
    reconnectingMessage="Reconnecting to the OpenTelemetry log stream..."
    unavailableMessage="OpenTelemetry logs are temporarily unavailable."
    ontrace={focusTrace}
  />
{/if}

{#if activeView === "traces"}
  {#if focusedTraceId}
    <Card.Root>
      <Card.Header class="border-b border-border">
        <Card.Action>
          <Button variant="outline" size="sm" onclick={clearTraceFocus}
            >Back to traces</Button
          >
        </Card.Action>
        <Card.Title>Trace {focusedTraceId}</Card.Title>
        <Card.Description
          >Correlated spans across every service that contributed to this
          environment trace.</Card.Description
        >
      </Card.Header>
      <Card.Content>
        <TraceDetails
          traceId={focusedTraceId}
          endpoint={routes.environmentTelemetryTrace(
            applicationId,
            environmentId,
            focusedTraceId,
          )}
          showDatabaseDetails
        />
      </Card.Content>
    </Card.Root>
  {:else}
    <Card.Root>
      <Card.Header>
        <Card.Action>
          <label class="grid gap-1.5 text-xs font-medium">
            <span class="text-muted-foreground">HTTP response</span>
            <NativeSelect.Root
              class="w-44"
              value={traceResponseClass}
              onchange={filterTraces}
              aria-label="Filter traces by HTTP response code"
            >
              <NativeSelect.Option value="">All traces</NativeSelect.Option>
              <NativeSelect.Option value="2xx">2xx success</NativeSelect.Option>
              <NativeSelect.Option value="3xx">3xx redirect</NativeSelect.Option
              >
              <NativeSelect.Option value="4xx"
                >4xx client error</NativeSelect.Option
              >
              <NativeSelect.Option value="5xx"
                >5xx server error</NativeSelect.Option
              >
            </NativeSelect.Root>
          </label>
        </Card.Action>
        <Card.Title>Recent traces</Card.Title>
        <Card.Description
          >Up to 100 {traceResponseClass || "environment"} traces from {rangeLabel}.
          Select a trace to inspect all correlated spans.</Card.Description
        >
      </Card.Header>
      <Card.Content>
        {#if recentTraces.length}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[920px] text-xs">
              <Table.Header>
                <Table.Row>
                  <Table.Head>Started</Table.Head><Table.Head>Method</Table.Head
                  ><Table.Head>Route</Table.Head><Table.Head
                    >Response</Table.Head
                  >
                  <Table.Head>Duration</Table.Head><Table.Head>Spans</Table.Head
                  >
                  <Table.Head>Errors</Table.Head><Table.Head>Trace</Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#each recentTraces as trace (trace.traceId)}
                  <Table.Row>
                    <Table.Cell class="whitespace-nowrap"
                      >{stamp(trace.startedAt)}</Table.Cell
                    >
                    <Table.Cell class="font-mono"
                      >{trace.requestMethod || "—"}</Table.Cell
                    >
                    <Table.Cell class="font-mono font-medium"
                      >{trace.requestRoute ||
                        trace.rootSpanName ||
                        "Unknown"}</Table.Cell
                    >
                    <Table.Cell>
                      {#if trace.responseCode}
                        <StatusBadge
                          status={trace.responseCode >= 500
                            ? "error"
                            : trace.responseCode >= 400
                              ? "warning"
                              : "healthy"}
                          label={String(trace.responseCode)}
                        />
                      {:else}
                        —
                      {/if}
                    </Table.Cell>
                    <Table.Cell
                      >{formatSpanDuration(trace.durationNs)}</Table.Cell
                    >
                    <Table.Cell>{trace.spanCount}</Table.Cell>
                    <Table.Cell
                      class={trace.errorCount > 0
                        ? "text-destructive"
                        : undefined}>{trace.errorCount}</Table.Cell
                    >
                    <Table.Cell>
                      <Button
                        variant="link"
                        size="xs"
                        class="h-auto p-0 font-mono"
                        onclick={() => focusTrace(trace.traceId)}
                        >{short(trace.traceId)}</Button
                      >
                    </Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {:else}
          <Empty.Root class="py-12">
            <Empty.Header>
              <Empty.Title>No traces in this range</Empty.Title>
              <Empty.Description
                >{traceResponseClass
                  ? `No sampled ${traceResponseClass} HTTP requests were found.`
                  : "Traces will appear after instrumented requests are sampled."}</Empty.Description
              >
            </Empty.Header>
          </Empty.Root>
        {/if}
      </Card.Content>
    </Card.Root>
  {/if}
{/if}

{#if activeView === "database"}
  {#if telemetry.database?.available || databaseHistory.length}
    <div class="space-y-6">
      <div class="grid gap-3 sm:grid-cols-3">
        <Card.Root>
          <Card.Header
            ><Card.Title class="text-sm">Operations</Card.Title></Card.Header
          >
          <Card.Content>
            <p class="text-2xl font-semibold">
              {telemetry.database?.available
                ? formatPerSecond(telemetry.database.operationsPerSecond)
                : "Unavailable"}
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              Current database operation rate
            </p>
          </Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header
            ><Card.Title class="text-sm">Errors</Card.Title></Card.Header
          >
          <Card.Content>
            <p
              class={cn("text-2xl font-semibold", {
                "text-destructive":
                  (telemetry.database?.errorsPerSecond ?? 0) > 0,
              })}
            >
              {telemetry.database?.available
                ? formatPerSecond(telemetry.database.errorsPerSecond)
                : "Unavailable"}
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              Instrumented operation failures
            </p>
          </Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header
            ><Card.Title class="text-sm">p95 latency</Card.Title></Card.Header
          >
          <Card.Content>
            <p class="text-2xl font-semibold">
              {telemetry.database?.available
                ? formatDuration(telemetry.database.p95DurationMs)
                : "Unavailable"}
            </p>
            <p class="mt-1 text-xs text-muted-foreground">
              Tail latency for database operations
            </p>
          </Card.Content>
        </Card.Root>
      </div>

      <div class="grid gap-4 xl:grid-cols-2">
        <TelemetryHistory
          label="Operation rate"
          description={`Instrumented database operations and errors from ${rangeLabel}`}
          series={databaseActivitySeries}
          formatValue={formatPerSecond}
          windowSeconds={rangeSeconds}
        />
        <TelemetryHistory
          label="Operation latency"
          description="Median and tail latency by time window"
          series={databaseLatencySeries}
          formatValue={formatDuration}
          windowSeconds={rangeSeconds}
        />
      </div>

      <Card.Root>
        <Card.Header>
          <Card.Title>Slow queries</Card.Title>
          <Card.Description
            >Queries ranked by p95 operation time across {rangeLabel}.</Card.Description
          >
        </Card.Header>
        <Card.Content>
          {#if slowQueries.length}
            <div class="overflow-x-auto border border-border">
              <Table.Root class="min-w-[780px] text-xs">
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Database</Table.Head>
                    <Table.Head>Query</Table.Head>
                    <Table.Head>Executions</Table.Head>
                    <Table.Head>p95 latency</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each slowQueries as query (`${query.databaseSystem}:${query.operation}:${query.query}`)}
                    <Table.Row>
                      <Table.Cell class="font-mono">
                        {query.databaseSystem || "—"}{query.operation
                          ? ` · ${query.operation}`
                          : ""}
                      </Table.Cell>
                      <Table.Cell
                        class="max-w-[42rem] whitespace-pre-wrap break-words font-mono font-medium"
                      >
                        {query.query}
                      </Table.Cell>
                      <Table.Cell>{query.executions}</Table.Cell>
                      <Table.Cell
                        >{formatDuration(query.p95DurationMs)}</Table.Cell
                      >
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </div>
            {#if telemetry.moreQueries || slowQueriesExpanded}
              <div class="mt-4">
                <Button
                  variant="outline"
                  size="sm"
                  onclick={toggleSlowQueries}
                  disabled={slowQueriesLoading}
                  aria-expanded={slowQueriesExpanded}
                  >{slowQueriesExpanded
                    ? "Show top 10"
                    : slowQueriesLoading
                      ? "Loading top 25..."
                      : "Show top 25"}</Button
                >
              </div>
            {/if}
            {#if slowQueriesError}
              <p class="mt-3 text-sm text-destructive">{slowQueriesError}</p>
            {/if}
          {:else}
            <Empty.Root class="py-10">
              <Empty.Header>
                <Empty.Title>No query text yet</Empty.Title>
                <Empty.Description
                  >Slow queries appear when instrumented spans include
                  <span class="font-mono">db.query.text</span
                  >.</Empty.Description
                >
              </Empty.Header>
            </Empty.Root>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>
  {:else}
    <Empty.Root class="py-12">
      <Empty.Header>
        <Empty.Title>No database telemetry yet</Empty.Title>
        <Empty.Description
          >Database activity will appear after the application exports
          <span class="font-mono">db.client.operation.duration</span>
          metrics.</Empty.Description
        >
      </Empty.Header>
    </Empty.Root>
  {/if}
{/if}
