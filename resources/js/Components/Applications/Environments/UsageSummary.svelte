<script lang="ts">
  import ArrowDownIcon from "@lucide/svelte/icons/arrow-down";
  import ArrowUpIcon from "@lucide/svelte/icons/arrow-up";
  import MinusIcon from "@lucide/svelte/icons/minus";
  import { Link } from "@inertiajs/svelte";

  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";

  let {
    cpuCores,
    memoryBytes,
    cpuChange,
    memoryChange,
    cpuAvailable,
    memoryAvailable,
    observedAt,
    telemetryUrl,
  }: {
    cpuCores: number;
    memoryBytes: number;
    cpuChange: number | null;
    memoryChange: number | null;
    cpuAvailable: boolean;
    memoryAvailable: boolean;
    observedAt: string;
    telemetryUrl: string;
  } = $props();

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
  const formatChange = (value: number | null) =>
    value === null
      ? "Unavailable"
      : `${value > 0 ? "+" : ""}${value.toFixed(1)}%`;
</script>

<Card.Root>
  <Card.Header>
    <Card.Action>
      <Button size="sm" variant="outline">
        {#snippet child({ props })}
          <Link {...props} href={telemetryUrl}>View telemetry</Link>
        {/snippet}
      </Button>
    </Card.Action>
    <Card.Title>Resource usage</Card.Title>
    <Card.Description>
      Current CPU and memory usage for the serving container vs the last hour.
    </Card.Description>
  </Card.Header>
  <Card.Content>
    {#if !cpuAvailable && !memoryAvailable}
      <p class="text-sm text-muted-foreground">
        No telemetry is available for the active container yet.
      </p>
    {:else}
      <div class="grid gap-3 sm:grid-cols-2">
        <article class="border border-border bg-card/35 p-4">
          <p
            class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
          >
            CPU
          </p>
          <p class="mt-2 text-2xl font-semibold tracking-tight">
            {cpuAvailable ? `${cpuCores.toFixed(2)} cores` : "Unavailable"}
          </p>
          <p class="mt-1 flex items-center gap-1 text-xs">
            {#if cpuChange === null}
              <MinusIcon class="size-3.5 text-muted-foreground" />
              <span class="text-muted-foreground">vs last hour</span>
            {:else if cpuChange > 0}
              <ArrowUpIcon class="size-3.5 text-warning" />
              <span class="text-warning"
                >{formatChange(cpuChange)} vs last hour</span
              >
            {:else}
              <ArrowDownIcon class="size-3.5 text-success" />
              <span class="text-success"
                >{formatChange(cpuChange)} vs last hour</span
              >
            {/if}
          </p>
        </article>
        <article class="border border-border bg-card/35 p-4">
          <p
            class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
          >
            Memory
          </p>
          <p class="mt-2 text-2xl font-semibold tracking-tight">
            {memoryAvailable ? formatBytes(memoryBytes) : "Unavailable"}
          </p>
          <p class="mt-1 flex items-center gap-1 text-xs">
            {#if memoryChange === null}
              <MinusIcon class="size-3.5 text-muted-foreground" />
              <span class="text-muted-foreground">vs last hour</span>
            {:else if memoryChange > 0}
              <ArrowUpIcon class="size-3.5 text-warning" />
              <span class="text-warning"
                >{formatChange(memoryChange)} vs last hour</span
              >
            {:else}
              <ArrowDownIcon class="size-3.5 text-success" />
              <span class="text-success"
                >{formatChange(memoryChange)} vs last hour</span
              >
            {/if}
          </p>
        </article>
      </div>
      {#if observedAt}
        <p class="mt-3 text-xs text-muted-foreground">
          Observed {new Date(observedAt).toLocaleString()}
        </p>
      {/if}
    {/if}
  </Card.Content>
</Card.Root>
