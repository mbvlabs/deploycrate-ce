<script lang="ts">
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import * as Table from "@/Components/ui/table";
  import type { TraceSpan } from "./telemetry.types";

  let {
    traceId,
    endpoint,
    showDatabaseDetails = false,
  }: {
    traceId: string;
    endpoint: string;
    showDatabaseDetails?: boolean;
  } = $props();

  let spans = $state.raw<TraceSpan[]>([]);
  let loading = $state(false);
  let error = $state("");

  const databaseSpans = $derived(
    spans.filter(
      (span) => databaseSystem(span) !== "" || databaseQueryText(span) !== "",
    ),
  );

  const formatDuration = (nanoseconds: number) => {
    const milliseconds = nanoseconds / 1_000_000;
    return milliseconds < 1
      ? `${(milliseconds * 1000).toFixed(0)} µs`
      : `${milliseconds.toFixed(1)} ms`;
  };

  const stamp = (value: string) =>
    value ? new Date(value).toLocaleString() : "Unknown";

  function databaseSystem(span: TraceSpan) {
    return (
      span.spanAttributes?.["db.system.name"] ??
      span.spanAttributes?.["db.system"] ??
      ""
    );
  }

  function databaseQueryText(span: TraceSpan) {
    return (
      span.spanAttributes?.["db.query.text"] ??
      span.spanAttributes?.["db.statement"] ??
      ""
    );
  }

  $effect(() => {
    const selectedTraceId = traceId;
    const selectedEndpoint = endpoint;
    spans = [];
    error = "";
    loading = false;
    if (!selectedTraceId || !selectedEndpoint) return;

    loading = true;
    const abortController = new AbortController();

    async function load() {
      try {
        const response = await window.fetch(selectedEndpoint, {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
          signal: abortController.signal,
        });
        if (!response.ok) throw new Error(`Trace returned ${response.status}`);
        const snapshot = (await response.json()) as { spans: TraceSpan[] };
        if (!abortController.signal.aborted) spans = snapshot.spans;
      } catch {
        if (!abortController.signal.aborted)
          error = "This trace could not be loaded.";
      } finally {
        if (!abortController.signal.aborted) loading = false;
      }
    }

    void load();
    return () => abortController.abort();
  });
</script>

{#if loading}
  <p class="py-6 text-sm text-muted-foreground">Loading trace...</p>
{:else if error}
  <p class="py-6 text-sm text-destructive">{error}</p>
{:else if spans.length}
  <div class="overflow-x-auto border border-border">
    <Table.Root class="min-w-[920px] text-xs">
      <Table.Header>
        <Table.Row>
          <Table.Head>Started</Table.Head>
          <Table.Head>Service</Table.Head>
          <Table.Head>Span</Table.Head>
          <Table.Head>Span ID / parent</Table.Head>
          <Table.Head>Duration</Table.Head>
          <Table.Head>Status</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each spans as span (span.spanId)}
          <Table.Row>
            <Table.Cell class="whitespace-nowrap">{stamp(span.startedAt)}</Table.Cell>
            <Table.Cell>
              <p class="font-medium">{span.serviceName}</p>
              <p class="mt-1 text-muted-foreground">{span.kind || span.scope}</p>
            </Table.Cell>
            <Table.Cell class="font-medium">{span.name}</Table.Cell>
            <Table.Cell class="font-mono">
              <p>{span.spanId}</p>
              <p class="mt-1 text-muted-foreground">
                {span.parentSpanId || "root"}
              </p>
            </Table.Cell>
            <Table.Cell>{formatDuration(span.durationNs)}</Table.Cell>
            <Table.Cell>
              <StatusBadge status={span.statusCode || "unset"} />
              {#if span.statusMessage}
                <p class="mt-1 text-muted-foreground">{span.statusMessage}</p>
              {/if}
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </div>

  {#if showDatabaseDetails && databaseSpans.length}
    <section class="mt-6 space-y-3" aria-labelledby="trace-database-spans">
      <div>
        <h3 id="trace-database-spans" class="text-sm font-semibold">
          Database spans
        </h3>
        <p class="mt-1 text-xs text-muted-foreground">
          Query text is shown only when emitted by application instrumentation.
        </p>
      </div>
      {#each databaseSpans as span (span.spanId)}
        <article class="border border-border bg-muted/10 p-3">
          <header class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
            <span class="font-medium">{databaseSystem(span) || "database"}</span>
            <span class="text-muted-foreground">{span.name}</span>
            <span class="font-mono text-muted-foreground">
              {formatDuration(span.durationNs)}
            </span>
            <StatusBadge status={span.statusCode || "unset"} />
          </header>
          {#if databaseQueryText(span)}
            <pre
              class="mt-3 max-h-48 overflow-auto whitespace-pre-wrap break-words border-l-2 border-primary/40 bg-background/60 p-3 font-mono text-xs leading-5"
            >{databaseQueryText(span)}</pre>
          {:else}
            <p class="mt-3 text-xs text-muted-foreground">
              Query text was not emitted with this span.
            </p>
          {/if}
        </article>
      {/each}
    </section>
  {/if}
{:else}
  <p class="py-6 text-sm text-muted-foreground">
    No spans were retained for this trace.
  </p>
{/if}
