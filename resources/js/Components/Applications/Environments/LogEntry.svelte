<script lang="ts">
  import { Button } from "@/Components/ui/button";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { cn } from "@/lib/utils";

  type Metadata = {
    label: string;
    value: string;
    mono?: boolean;
  };

  let {
    occurredAt,
    message,
    status,
    statusLabel,
    source,
    metadata = [],
    attributes = [],
    traceId = "",
    ontrace,
  }: {
    occurredAt: string;
    message: string;
    status: string;
    statusLabel: string;
    source: string;
    metadata?: Metadata[];
    attributes?: Array<[string, string]>;
    traceId?: string;
    ontrace?: () => void;
  } = $props();

  const timestamp = $derived(
    occurredAt ? new Date(occurredAt).toLocaleString() : "Unknown time",
  );
  const shortTraceId = $derived(traceId ? traceId.slice(0, 8) : "");
</script>

<article class="border-b border-border last:border-b-0 hover:bg-muted/15">
  <div class="grid gap-3 p-3 lg:grid-cols-[11rem_minmax(0,1fr)] lg:p-4">
    <header class="space-y-2">
      <div class="flex flex-wrap items-center gap-2">
        <StatusBadge {status} label={statusLabel} />
        <time
          class="font-mono text-[10px] text-muted-foreground"
          datetime={occurredAt}
          title={occurredAt ? new Date(occurredAt).toISOString() : undefined}
        >
          {timestamp}
        </time>
      </div>
      <p class="break-words text-xs font-medium leading-5">{source}</p>
    </header>

    <div class="min-w-0">
      <pre
        class={cn(
          "whitespace-pre-wrap break-words border-l-2 border-border pl-3 font-mono text-xs leading-5 text-foreground",
          {
            "border-warning": status === "warning",
            "border-destructive": status === "error",
          },
        )}>{message}</pre>

      {#if metadata.length > 0 || traceId}
        <dl class="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 text-[10px]">
          {#each metadata as item (`${item.label}:${item.value}`)}
            <div class="flex min-w-0 items-baseline gap-1.5">
              <dt
                class="shrink-0 font-medium uppercase tracking-[0.1em] text-muted-foreground"
              >
                {item.label}
              </dt>
              <dd
                class={cn("break-all text-foreground/80", {
                  "font-mono": item.mono,
                })}
              >
                {item.value}
              </dd>
            </div>
          {/each}
          {#if traceId}
            <div class="flex items-baseline gap-1.5">
              <dt
                class="font-medium uppercase tracking-[0.1em] text-muted-foreground"
              >
                Trace
              </dt>
              <dd>
                <Button
                  variant="link"
                  size="xs"
                  class="h-auto p-0 font-mono text-[10px]"
                  onclick={ontrace}
                >
                  {shortTraceId}
                </Button>
              </dd>
            </div>
          {/if}
        </dl>
      {/if}

      {#if attributes.length > 0}
        <details class="mt-3 border-t border-border/70 pt-2">
          <summary
            class="w-fit cursor-pointer select-none text-[10px] font-medium text-muted-foreground hover:text-foreground"
          >
            {attributes.length} structured {attributes.length === 1
              ? "attribute"
              : "attributes"}
          </summary>
          <dl
            class="mt-2 grid gap-x-5 gap-y-1.5 text-[10px] sm:grid-cols-2 xl:grid-cols-3"
          >
            {#each attributes as [key, value] (key)}
              <div class="grid min-w-0 grid-cols-[minmax(5rem,auto)_1fr] gap-2">
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
        </details>
      {/if}
    </div>
  </div>
</article>
