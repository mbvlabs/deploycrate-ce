<script lang="ts">
  import { page } from "@inertiajs/svelte";
  import SearchIcon from "@lucide/svelte/icons/search";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import TelemetryHistory from "@/Components/System/TelemetryHistory.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as Table from "@/Components/ui/table";
  import { cn } from "@/lib/utils";
  import { routes } from "@/routes";
  import type {
    ApplicationTelemetry,
    OpenTelemetryLog,
    OpenTelemetryLogSnapshot,
    TelemetryRange,
    TraceSpan,
  } from "@/Pages/Applications/Environments/show.types";

  type OpenTelemetryView = "insights" | "logs" | "traces";
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

  let logs = $state<OpenTelemetryLog[]>([]);
  let logCursor = $state("");
  let logsLoaded = $state(false);
  let logConnectionError = $state("");
  let logSearchInput = $state("");
  let logSearch = $state("");
  let logQueryKey = $state("");
  let followingLogs = $state(true);
  let logViewport = $state<HTMLDivElement>();
  let traceDialogOpen = $state(false);
  let selectedTraceId = $state("");
  let traceSpans = $state<TraceSpan[]>([]);
  let traceLoading = $state(false);
  let traceError = $state("");

  const activeView = $derived.by<OpenTelemetryView>(() => {
    const view = new URLSearchParams($page.url.split("?")[1] ?? "").get("view");
    return view === "logs" || view === "traces" ? view : "insights";
  });
  const telemetryHref = (view: OpenTelemetryView) => {
    const query = new URLSearchParams({
      source: "opentelemetry",
      view,
      range: telemetryRange,
    });
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
  const logLevel = (log: OpenTelemetryLog) => {
    if (log.severity) return log.severity.toUpperCase();
    if (log.severityNumber >= 17) return "ERROR";
    if (log.severityNumber >= 13) return "WARN";
    if (log.severityNumber >= 9) return "INFO";
    return "DEBUG";
  };
  const logStatus = (log: OpenTelemetryLog) => {
    if (log.severityNumber >= 17) return "error";
    if (log.severityNumber >= 13) return "warning";
    return logLevel(log).toLowerCase();
  };
  const logContext = (log: OpenTelemetryLog) =>
    Object.entries(log.attributes ?? {})
      .filter(
        ([key, value]) =>
          value &&
          !key.startsWith("code.") &&
          (key !== "trace_id" || !log.traceId) &&
          (key !== "span_id" || !log.spanId),
      )
      .sort(([left], [right]) => left.localeCompare(right));
  const logMessage = (log: OpenTelemetryLog) => {
    const message = log.message.trim();
    if (!(message.startsWith("{") || message.startsWith("[")))
      return log.message;
    try {
      return JSON.stringify(JSON.parse(message), null, 2);
    } catch {
      return log.message;
    }
  };

  async function loadLogs(
    range: TelemetryRange,
    search: string,
    signal: AbortSignal,
  ) {
    const endpoint = new URL(
      routes.environmentTelemetryLogs(applicationId, environmentId),
      window.location.origin,
    );
    endpoint.searchParams.set("range", range);
    if (search) endpoint.searchParams.set("search", search);
    if (logCursor) endpoint.searchParams.set("after", logCursor);
    const response = await window.fetch(endpoint, {
      cache: "no-store",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal,
    });
    if (!response.ok)
      throw new Error(`OpenTelemetry logs returned ${response.status}`);
    const snapshot = (await response.json()) as OpenTelemetryLogSnapshot;
    const cutoff = Date.now() - rangeSeconds * 1000;
    logs = [...logs, ...snapshot.logs]
      .filter((log) => new Date(log.occurredAt).getTime() >= cutoff)
      .slice(-2000);
    logCursor = snapshot.nextCursor;
    logsLoaded = true;
    logConnectionError = "";
    return snapshot;
  }

  async function loadTrace(traceId: string) {
    selectedTraceId = traceId;
    traceDialogOpen = true;
    traceSpans = [];
    traceError = "";
    traceLoading = true;
    try {
      const response = await window.fetch(
        routes.environmentTelemetryTrace(applicationId, environmentId, traceId),
        {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
        },
      );
      if (!response.ok) throw new Error(`Trace returned ${response.status}`);
      traceSpans = ((await response.json()) as { spans: TraceSpan[] }).spans;
    } catch {
      traceError = "This trace could not be loaded.";
    } finally {
      traceLoading = false;
    }
  }

  function closeTrace() {
    selectedTraceId = "";
    traceSpans = [];
    traceError = "";
  }

  function updateLogFollow() {
    if (!logViewport) return;
    followingLogs =
      logViewport.scrollHeight -
        logViewport.scrollTop -
        logViewport.clientHeight <
      48;
  }

  $effect(() => {
    const search = logSearchInput.trim();
    const timer = window.setTimeout(() => (logSearch = search), 300);
    return () => window.clearTimeout(timer);
  });

  $effect(() => {
    const nextQueryKey =
      activeView === "logs" ? `${telemetryRange}:${logSearch}` : "";
    if (nextQueryKey === logQueryKey) return;
    logQueryKey = nextQueryKey;
    logs = [];
    logCursor = "";
    logsLoaded = false;
    logConnectionError = "";
    followingLogs = true;
  });

  $effect(() => {
    logs.length;
    if (!followingLogs) return;
    const frame = window.requestAnimationFrame(() => {
      logViewport?.scrollTo({ top: logViewport.scrollHeight });
    });
    return () => window.cancelAnimationFrame(frame);
  });

  $effect(() => {
    if (activeView !== "logs" || !logQueryKey) return;
    const range = telemetryRange;
    const search = logSearch;
    const shouldPoll = live;
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 2000;

    async function poll() {
      try {
        const snapshot = await loadLogs(range, search, abortController.signal);
        if (abortController.signal.aborted) return;
        retryDelay = 2000;
        if (shouldPoll)
          timer = window.setTimeout(poll, snapshot.hasMore ? 0 : retryDelay);
      } catch {
        if (abortController.signal.aborted) return;
        logConnectionError = shouldPoll
          ? "Reconnecting to the OpenTelemetry log stream..."
          : "OpenTelemetry logs are temporarily unavailable.";
        if (!shouldPoll) return;
        retryDelay = Math.min(retryDelay * 2, 10000);
        timer = window.setTimeout(poll, retryDelay);
      }
    }

    timer = window.setTimeout(poll, 0);
    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  });
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

    {#if telemetry.database?.available || databaseHistory.length}
      <TelemetryHistory
        label="Database activity"
        description="Instrumented database operations and errors"
        series={databaseActivitySeries}
        formatValue={formatPerSecond}
        windowSeconds={rangeSeconds}
      />
    {/if}
  </div>
{/if}

{#if activeView === "logs"}
  <Card.Root>
    <Card.Header>
      <Card.Title>Application logs</Card.Title>
      <Card.Description
        >Structured OpenTelemetry logs from {rangeLabel}. Search across
        messages, attributes, trace IDs, span IDs, and services.</Card.Description
      >
    </Card.Header>
    <Card.Content>
      <div
        class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between"
      >
        <label
          class="grid w-full max-w-xl gap-1.5 text-xs font-medium"
          for="environment-otel-log-search"
        >
          <span class="text-muted-foreground">Search logs</span>
          <span class="relative">
            <SearchIcon
              class="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              id="environment-otel-log-search"
              type="search"
              maxlength="256"
              bind:value={logSearchInput}
              placeholder="Search messages, attributes, traces, or services"
              class="pl-8"
            />
          </span>
        </label>
        <p class="shrink-0 text-xs text-muted-foreground">
          {logs.length}
          {logs.length === 1 ? "entry" : "entries"}
          {#if live}<span class="ml-2 text-success">● Live polling</span>{/if}
        </p>
      </div>
      {#if logConnectionError}
        <p class="mb-3 text-xs text-warning">{logConnectionError}</p>
      {/if}
      <div
        bind:this={logViewport}
        onscroll={updateLogFollow}
        class="max-h-[42rem] min-h-48 overflow-auto border border-border bg-muted/10"
      >
        {#each logs as log (log.id)}
          <article
            class="border-b border-border p-3 last:border-b-0 hover:bg-muted/20"
          >
            <header
              class="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground"
            >
              <StatusBadge status={logStatus(log)} label={logLevel(log)} />
              <time
                datetime={log.occurredAt}
                title={new Date(log.occurredAt).toISOString()}
                >{stamp(log.occurredAt)}</time
              >
              <span aria-hidden="true">·</span>
              <span>{log.service || log.processName || "application"}</span>
              {#if log.processKind}
                <span aria-hidden="true">·</span><span
                  >{log.processKind}{log.processReplica
                    ? ` ${log.processReplica}`
                    : ""}</span
                >
              {/if}
              {#if log.traceId}
                <span aria-hidden="true">·</span><span>trace</span>
                <Button
                  variant="link"
                  size="xs"
                  class="h-auto p-0 font-mono text-[10px]"
                  onclick={() => loadTrace(log.traceId)}
                  >{short(log.traceId)}</Button
                >
              {/if}
              {#if log.spanId}
                <span aria-hidden="true">·</span><span class="font-mono"
                  >span {short(log.spanId)}</span
                >
              {/if}
            </header>
            <pre
              class={cn(
                "mt-2 whitespace-pre-wrap break-words border-l-2 border-transparent pl-3 font-mono text-xs leading-5",
                {
                  "border-warning":
                    log.severityNumber >= 13 && log.severityNumber < 17,
                  "border-destructive": log.severityNumber >= 17,
                },
              )}>{logMessage(log)}</pre>
            {#if logContext(log).length}
              <dl
                class="mt-3 grid gap-x-5 gap-y-1 border-t border-border/70 pt-2 text-[10px] sm:grid-cols-2 xl:grid-cols-3"
              >
                {#each logContext(log) as [key, value] (key)}
                  <div
                    class="grid min-w-0 grid-cols-[minmax(5rem,auto)_1fr] gap-2"
                  >
                    <dt
                      class="truncate font-mono text-muted-foreground"
                      title={key}
                    >
                      {key}
                    </dt>
                    <dd class="break-all font-mono text-foreground/80">
                      {value}
                    </dd>
                  </div>
                {/each}
              </dl>
            {/if}
          </article>
        {:else}
          <p class="p-4 text-sm text-muted-foreground">
            {logsLoaded
              ? logSearch
                ? `No logs in ${rangeLabel} match “${logSearch}”.`
                : `No OpenTelemetry logs were collected in ${rangeLabel}.`
              : "Loading OpenTelemetry logs..."}
          </p>
        {/each}
      </div>
    </Card.Content>
  </Card.Root>
{/if}

{#if activeView === "traces"}
  <Card.Root>
    <Card.Header>
      <Card.Title>Recent traces</Card.Title>
      <Card.Description
        >Up to 100 environment traces from {rangeLabel}. Select a trace to
        inspect all correlated spans.</Card.Description
      >
    </Card.Header>
    <Card.Content>
      {#if recentTraces.length}
        <div class="overflow-x-auto border border-border">
          <Table.Root class="min-w-[780px] text-xs">
            <Table.Header>
              <Table.Row>
                <Table.Head>Started</Table.Head><Table.Head
                  >Root span</Table.Head
                >
                <Table.Head>Duration</Table.Head><Table.Head>Spans</Table.Head>
                <Table.Head>Errors</Table.Head><Table.Head>Trace</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each recentTraces as trace (trace.traceId)}
                <Table.Row>
                  <Table.Cell class="whitespace-nowrap"
                    >{stamp(trace.startedAt)}</Table.Cell
                  >
                  <Table.Cell class="font-medium"
                    >{trace.rootSpanName || "Unknown root span"}</Table.Cell
                  >
                  <Table.Cell>{formatSpanDuration(trace.durationNs)}</Table.Cell
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
                      onclick={() => loadTrace(trace.traceId)}
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
              >Traces will appear after instrumented requests are sampled.</Empty.Description
            >
          </Empty.Header>
        </Empty.Root>
      {/if}
    </Card.Content>
  </Card.Root>
{/if}

<Dialog.Root
  bind:open={traceDialogOpen}
  onOpenChange={(open) => {
    if (!open) closeTrace();
  }}
>
  <Dialog.Content class="sm:max-w-6xl">
    <Dialog.Header>
      <Dialog.Title>Trace {selectedTraceId}</Dialog.Title>
      <Dialog.Description
        >Correlated OpenTelemetry spans across every service that contributed to
        this environment trace.</Dialog.Description
      >
    </Dialog.Header>
    {#if traceLoading}
      <p class="py-6 text-sm text-muted-foreground">Loading trace...</p>
    {:else if traceError}
      <p class="py-6 text-sm text-destructive">{traceError}</p>
    {:else if traceSpans.length}
      <div class="overflow-x-auto border border-border">
        <Table.Root class="min-w-[920px] text-xs">
          <Table.Header>
            <Table.Row>
              <Table.Head>Started</Table.Head><Table.Head>Service</Table.Head>
              <Table.Head>Span</Table.Head><Table.Head
                >Span ID / parent</Table.Head
              >
              <Table.Head>Duration</Table.Head><Table.Head>Status</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each traceSpans as span (span.spanId)}
              <Table.Row>
                <Table.Cell class="whitespace-nowrap"
                  >{stamp(span.startedAt)}</Table.Cell
                >
                <Table.Cell>
                  <p class="font-medium">{span.serviceName}</p>
                  <p class="mt-1 text-muted-foreground">
                    {span.kind || span.scope}
                  </p>
                </Table.Cell>
                <Table.Cell class="font-medium">{span.name}</Table.Cell>
                <Table.Cell class="font-mono">
                  <p>{span.spanId}</p>
                  <p class="mt-1 text-muted-foreground">
                    {span.parentSpanId || "root"}
                  </p>
                </Table.Cell>
                <Table.Cell>{formatSpanDuration(span.durationNs)}</Table.Cell>
                <Table.Cell>
                  <StatusBadge status={span.statusCode || "unset"} />
                  {#if span.statusMessage}<p class="mt-1 text-muted-foreground">
                      {span.statusMessage}
                    </p>{/if}
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {:else}
      <p class="py-6 text-sm text-muted-foreground">
        No spans were retained for this trace.
      </p>
    {/if}
  </Dialog.Content>
</Dialog.Root>
