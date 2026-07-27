<script lang="ts">
  let { label, used, free, formatValue, available = true }: {
    label: string
    used: number
    free: number
    formatValue: (value: number) => string
    available?: boolean
  } = $props()

  const total = $derived(used + free)
  const usedPercent = $derived(available && total > 0 ? Math.min(100, Math.max(0, (used / total) * 100)) : 0)
  const freePercent = $derived(available && total > 0 ? 100 - usedPercent : 0)
</script>

<article class="border border-border bg-card/35 p-5">
  <div class="flex items-center justify-between gap-4">
    <h2 class="text-sm font-semibold">{label}</h2>
    <span class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
      {available ? 'Used / free' : 'Unavailable'}
    </span>
  </div>

  <div class="mt-5 grid grid-cols-[8.5rem_1fr] items-center gap-6">
    <div class="relative size-[8.5rem]">
      <svg viewBox="0 0 120 120" class="size-full -rotate-90" role="img" aria-label={`${label}: ${usedPercent.toFixed(0)} percent used`}>
        <circle cx="60" cy="60" r="46" pathLength="100" fill="none" stroke="currentColor" stroke-width="12" class="text-muted" />
        {#if available}
          <circle
            cx="60"
            cy="60"
            r="46"
            pathLength="100"
            fill="none"
            stroke="currentColor"
            stroke-width="12"
            stroke-linecap="butt"
            stroke-dasharray={`${usedPercent} ${freePercent}`}
            class="text-primary"
          />
        {/if}
      </svg>
      <div class="absolute inset-0 flex flex-col items-center justify-center text-center">
        {#if available}
          <span class="text-2xl font-semibold tracking-tight">{usedPercent.toFixed(0)}%</span>
          <span class="mt-1 text-[10px] uppercase tracking-[0.14em] text-muted-foreground">used</span>
        {:else}
          <span class="text-xs text-muted-foreground">No data</span>
        {/if}
      </div>
    </div>

    <dl class="space-y-4">
      <div>
        <dt class="flex items-center gap-2 text-xs text-muted-foreground">
          <span class="size-2 bg-primary"></span>
          Used
        </dt>
        <dd class="mt-1 text-sm font-medium">{available ? formatValue(used) : 'Unavailable'}</dd>
      </div>
      <div>
        <dt class="flex items-center gap-2 text-xs text-muted-foreground">
          <span class="size-2 bg-muted"></span>
          Free
        </dt>
        <dd class="mt-1 text-sm font-medium">{available ? formatValue(free) : 'Unavailable'}</dd>
      </div>
    </dl>
  </div>
</article>
