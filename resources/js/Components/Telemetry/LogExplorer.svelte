<script lang="ts">
  import SearchIcon from "@lucide/svelte/icons/search";
  import { untrack } from "svelte";
  import LogEntry from "@/Components/TelemetryLogEntry.svelte";
  import * as Card from "@/Components/ui/card";
  import { Input } from "@/Components/ui/input";
  import type {
    TelemetryLog,
    TelemetryLogSnapshot,
    TelemetryRange,
  } from "./telemetry.types";

  let {
    active,
    endpoint,
    range,
    rangeLabel,
    live,
    title,
    description,
    headingId,
    searchId,
    searchPlaceholder,
    source,
    emptyMessage,
    filteredEmptyMessage,
    loadingMessage,
    reconnectingMessage,
    unavailableMessage,
    showSlot = false,
    ontrace,
  }: {
    active: boolean;
    endpoint: string;
    range: TelemetryRange;
    rangeLabel: string;
    live: boolean;
    title: string;
    description: string;
    headingId?: string;
    searchId: string;
    searchPlaceholder: string;
    source: (log: TelemetryLog) => string;
    emptyMessage: string;
    filteredEmptyMessage: (search: string) => string;
    loadingMessage: string;
    reconnectingMessage: string;
    unavailableMessage: string;
    showSlot?: boolean;
    ontrace: (traceId: string) => void;
  } = $props();

  let logs = $state.raw<TelemetryLog[]>([]);
  let cursor = $state("");
  let loaded = $state(false);
  let connectionError = $state("");
  let searchInput = $state("");
  let search = $state("");
  let queryKey = $state("");
  let following = $state(true);
  let viewport = $state<HTMLDivElement>();

  const rangeSeconds = $derived(
    { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 }[range],
  );

  const level = (log: TelemetryLog) => {
    if (log.severity) return log.severity.toUpperCase();
    if (log.severityNumber >= 17) return "ERROR";
    if (log.severityNumber >= 13) return "WARN";
    if (log.severityNumber >= 9) return "INFO";
    return "DEBUG";
  };

  const status = (log: TelemetryLog) => {
    if (log.severityNumber >= 17) return "error";
    if (log.severityNumber >= 13) return "warning";
    return level(log).toLowerCase();
  };

  const context = (log: TelemetryLog) =>
    Object.entries(log.attributes ?? {})
      .filter(
        ([key, value]) =>
          value &&
          !key.startsWith("code.") &&
          (key !== "trace_id" || !log.traceId) &&
          (key !== "span_id" || !log.spanId) &&
          ![
            "path",
            "url.path",
            "http.target",
            "http.route",
            "http.response.status_code",
            "http.status_code",
          ].includes(key),
      )
      .sort(([left], [right]) => left.localeCompare(right));

  const message = (log: TelemetryLog) => {
    const value = log.message.trim();
    if (!(value.startsWith("{") || value.startsWith("["))) return log.message;
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return log.message;
    }
  };

  const short = (value: string) => value.slice(0, 8);

  async function loadLogs(
    selectedRange: TelemetryRange,
    selectedSearch: string,
    signal: AbortSignal,
  ) {
    const url = new URL(endpoint, window.location.origin);
    url.searchParams.set("range", selectedRange);
    if (selectedSearch) url.searchParams.set("search", selectedSearch);
    const currentCursor = untrack(() => cursor);
    if (currentCursor) url.searchParams.set("after", currentCursor);
    const response = await window.fetch(url, {
      cache: "no-store",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal,
    });
    if (!response.ok) throw new Error(`Telemetry logs returned ${response.status}`);

    const snapshot = (await response.json()) as TelemetryLogSnapshot;
    const cutoff = Date.now() - rangeSeconds * 1000;
    logs = [...logs, ...snapshot.logs]
      .filter((log) => new Date(log.occurredAt).getTime() >= cutoff)
      .slice(-2000);
    cursor = snapshot.nextCursor;
    loaded = true;
    connectionError = "";
    return snapshot;
  }

  function updateFollow() {
    if (!viewport) return;
    following =
      viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 48;
  }

  $effect(() => {
    const value = searchInput.trim();
    const timer = window.setTimeout(() => (search = value), 300);
    return () => window.clearTimeout(timer);
  });

  $effect(() => {
    const nextQueryKey = active ? `${endpoint}:${range}:${search}` : "";
    if (nextQueryKey === queryKey) return;
    queryKey = nextQueryKey;
    logs = [];
    cursor = "";
    loaded = false;
    connectionError = "";
    following = true;
  });

  $effect(() => {
    logs.length;
    if (!following) return;
    const frame = window.requestAnimationFrame(() => {
      viewport?.scrollTo({ top: viewport.scrollHeight });
    });
    return () => window.cancelAnimationFrame(frame);
  });

  $effect(() => {
    if (!active || !queryKey) return;
    const selectedRange = range;
    const selectedSearch = search;
    const shouldPoll = live;
    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 2000;

    async function poll() {
      try {
        const snapshot = await loadLogs(
          selectedRange,
          selectedSearch,
          abortController.signal,
        );
        if (abortController.signal.aborted) return;
        retryDelay = 2000;
        if (shouldPoll)
          timer = window.setTimeout(poll, snapshot.hasMore ? 0 : retryDelay);
      } catch {
        if (abortController.signal.aborted) return;
        connectionError = shouldPoll
          ? reconnectingMessage
          : unavailableMessage;
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

<Card.Root>
  <Card.Header>
    <Card.Title id={headingId}>{title}</Card.Title>
    <Card.Description>{description}</Card.Description>
  </Card.Header>
  <Card.Content>
    <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <label class="grid w-full max-w-xl gap-1.5 text-xs font-medium" for={searchId}>
        <span class="text-muted-foreground">Search logs</span>
        <span class="relative">
          <SearchIcon
            class="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            id={searchId}
            type="search"
            maxlength={256}
            bind:value={searchInput}
            placeholder={searchPlaceholder}
            class="pl-8"
          />
        </span>
      </label>
      <p class="shrink-0 text-xs text-muted-foreground">
        {logs.length} {logs.length === 1 ? "entry" : "entries"}
        {#if live}<span class="ml-2 text-success">● Live polling</span>{/if}
      </p>
    </div>
    {#if connectionError}
      <p class="mb-3 text-xs text-warning">{connectionError}</p>
    {/if}
    <div
      bind:this={viewport}
      onscroll={updateFollow}
      class="max-h-[42rem] min-h-48 overflow-auto border border-border bg-muted/10"
    >
      {#each logs as log (log.id)}
        <LogEntry
          occurredAt={log.occurredAt}
          message={message(log)}
          status={status(log)}
          statusLabel={level(log)}
          source={source(log)}
          metadata={[
            ...(log.processReplica || log.processName || log.processKind
              ? [
                  {
                    label: "Process",
                    value:
                      log.processReplica || log.processName || log.processKind,
                  },
                ]
              : []),
            {
              label: "Path",
              value: log.requestPath || "Unavailable",
              mono: true,
            },
            ...(log.responseCode
              ? [
                  {
                    label: "Response",
                    value: String(log.responseCode),
                    mono: true,
                  },
                ]
              : []),
            ...(showSlot && log.slot
              ? [{ label: "Slot", value: log.slot, mono: true }]
              : []),
            ...(log.source
              ? [
                  {
                    label: "Source",
                    value: `${log.source}${log.line ? `:${log.line}` : ""}`,
                    mono: true,
                  },
                ]
              : []),
            ...(log.instance
              ? [{ label: "Instance", value: log.instance, mono: true }]
              : []),
            ...(log.spanId
              ? [{ label: "Span", value: short(log.spanId), mono: true }]
              : []),
          ]}
          attributes={context(log)}
          traceId={log.traceId}
          ontrace={() => ontrace(log.traceId)}
        />
      {:else}
        <p class="p-4 text-sm text-muted-foreground">
          {loaded
            ? search
              ? filteredEmptyMessage(search)
              : emptyMessage
            : loadingMessage}
        </p>
      {/each}
    </div>
  </Card.Content>
</Card.Root>
