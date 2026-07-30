<script lang="ts">
  let {
    label,
    primaryLabel,
    secondaryLabel,
    primary,
    secondary,
    centerValue,
    centerLabel,
    formatValue,
    available = true,
  }: {
    label: string
    primaryLabel: string
    secondaryLabel: string
    primary: number
    secondary: number
    centerValue: number
    centerLabel: string
    formatValue: (value: number) => string
    available?: boolean
  } = $props()

  const total = $derived(primary + secondary)
  const primaryPercent = $derived(available && total > 0 ? Math.min(100, Math.max(0, (primary / total) * 100)) : 0)
  const secondaryPercent = $derived(available && total > 0 ? 100 - primaryPercent : 0)
</script>

<article class="border border-border bg-card/35 p-5">
  <div class="flex items-center justify-between gap-4">
    <h3 class="text-sm font-semibold">{label}</h3>
    <span class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
      {available ? `${primaryLabel} / ${secondaryLabel}` : 'Unavailable'}
    </span>
  </div>

  <div class="mt-5 grid grid-cols-[8.5rem_1fr] items-center gap-6">
    <div class="relative size-[8.5rem]">
      <svg viewBox="0 0 120 120" class="size-full -rotate-90" role="img" aria-label={`${label}: ${available ? formatValue(centerValue) : 'Unavailable'}`}>
        <circle cx="60" cy="60" r="46" pathLength="100" fill="none" stroke="currentColor" stroke-width="12" class="text-muted" />
        {#if available && total > 0}
          <circle
            cx="60"
            cy="60"
            r="46"
            pathLength="100"
            fill="none"
            stroke="var(--chart-1)"
            stroke-width="12"
            stroke-dasharray={`${primaryPercent} ${secondaryPercent}`}
          />
          <circle
            cx="60"
            cy="60"
            r="46"
            pathLength="100"
            fill="none"
            stroke="var(--chart-2)"
            stroke-width="12"
            stroke-dasharray={`${secondaryPercent} ${primaryPercent}`}
            stroke-dashoffset={-primaryPercent}
          />
        {/if}
      </svg>
      <div class="absolute inset-0 flex flex-col items-center justify-center px-3 text-center">
        {#if available}
          <span class="text-base font-semibold tracking-tight">{formatValue(centerValue)}</span>
          <span class="mt-1 text-[10px] uppercase tracking-[0.14em] text-muted-foreground">{centerLabel}</span>
        {:else}
          <span class="text-xs text-muted-foreground">No data</span>
        {/if}
      </div>
    </div>

    <dl class="space-y-4">
      <div>
        <dt class="flex items-center gap-2 text-xs text-muted-foreground">
          <span class="size-2" style:background="var(--chart-1)"></span>
          {primaryLabel}
        </dt>
        <dd class="mt-1 text-sm font-medium">{available ? formatValue(primary) : 'Unavailable'}</dd>
      </div>
      <div>
        <dt class="flex items-center gap-2 text-xs text-muted-foreground">
          <span class="size-2" style:background="var(--chart-2)"></span>
          {secondaryLabel}
        </dt>
        <dd class="mt-1 text-sm font-medium">{available ? formatValue(secondary) : 'Unavailable'}</dd>
      </div>
    </dl>
  </div>
</article>
