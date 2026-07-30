<script lang="ts">
  type TelemetryPoint = {
    observedAt: string
    value: number
  }

  type TelemetrySeries = {
    label: string
    points: TelemetryPoint[]
  }

  let {
    label,
    description,
    series,
    formatValue,
  }: {
    label: string
    description: string
    series: TelemetrySeries[]
    formatValue: (value: number) => string
  } = $props()

  let hoveredIndex = $state<number | null>(null)
  const left = 68
  const right = 780
  const top = 18
  const bottom = 178
  const bucketCount = 12
  const historyWindowMilliseconds = 24 * 60 * 60 * 1000
  const colors = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)']
  const timeFormatter = new Intl.DateTimeFormat(undefined, { hour: 'numeric' })
  const rangeFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })

  const chart = $derived.by(() => {
    const timestamps = series
      .flatMap((item) => item.points)
      .map((point) => new Date(point.observedAt).getTime())
      .filter((timestamp) => Number.isFinite(timestamp))
    const end = timestamps.length ? Math.max(...timestamps) : Date.now()
    const start = end - historyWindowMilliseconds
    const duration = historyWindowMilliseconds / bucketCount
    const buckets = Array.from({ length: bucketCount }, (_, index) => ({
      start: start + (index * duration),
      end: start + ((index + 1) * duration),
      x: left + (index / (bucketCount - 1)) * (right - left),
    }))
    const chartSeries = series.map((item, seriesIndex) => {
      const samples = item.points
        .map((point) => ({ timestamp: new Date(point.observedAt).getTime(), value: point.value }))
        .filter((point) => Number.isFinite(point.timestamp) && Number.isFinite(point.value) && point.timestamp >= start && point.timestamp <= end)
        .sort((a, b) => a.timestamp - b.timestamp)
      return {
        label: item.label,
        color: colors[seriesIndex % colors.length],
        values: buckets.map((bucket, index) => samples.findLast((point) => (
          point.timestamp >= bucket.start
          && (index === bucketCount - 1 ? point.timestamp <= bucket.end : point.timestamp < bucket.end)
        ))?.value ?? null),
      }
    })
    const values = chartSeries.flatMap((item) => item.values).filter((value): value is number => value !== null)
    const maximum = Math.max(1, ...values) * 1.1
    return { buckets, series: chartSeries, maximum, available: values.length > 0 }
  })

  const yFor = (value: number) => bottom - (Math.max(0, value) / chart.maximum) * (bottom - top)
  const pathFor = (values: Array<number | null>) => values.map((value, index) => (
    value === null
      ? ''
      : `${index === 0 || values[index - 1] === null ? 'M' : 'L'} ${chart.buckets[index].x.toFixed(1)} ${yFor(value).toFixed(1)}`
  )).filter(Boolean).join(' ')
  const bucketLabel = (index: number) => `${rangeFormatter.format(chart.buckets[index].start)} to ${rangeFormatter.format(chart.buckets[index].end)}`

  const hover = (event: PointerEvent) => {
    const bounds = (event.currentTarget as SVGSVGElement).getBoundingClientRect()
    const x = ((event.clientX - bounds.left) / bounds.width) * 800
    hoveredIndex = Math.min(bucketCount - 1, Math.max(0, Math.round(((x - left) / (right - left)) * (bucketCount - 1))))
  }
</script>

<article class="border border-border bg-card/35 p-5">
  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
    <div>
      <h3 class="text-sm font-semibold">{label}</h3>
      <p class="mt-1 text-xs text-muted-foreground">{description}</p>
    </div>
    <div class="flex flex-wrap justify-end gap-x-4 gap-y-2 text-[11px] text-muted-foreground">
      {#each chart.series as item}
        <span class="flex items-center gap-2"><span class="size-2" style:background={item.color}></span>{item.label}</span>
      {/each}
    </div>
  </div>

  {#if chart.available}
    <div class="relative mt-4">
      <svg
        viewBox="0 0 800 220"
        class="h-56 w-full touch-none"
        role="img"
        aria-label={`${label} over the last 24 hours`}
        onpointerenter={hover}
        onpointermove={hover}
        onpointerleave={() => (hoveredIndex = null)}
      >
        {#each [0, 0.5, 1] as ratio}
          {@const y = bottom - ratio * (bottom - top)}
          <line x1={left} x2={right} y1={y} y2={y} stroke="currentColor" stroke-width="1" class="text-border" />
          <text x={left - 10} y={y + 4} text-anchor="end" class="fill-muted-foreground text-[12px]">{formatValue(chart.maximum * ratio)}</text>
        {/each}

        {#each chart.series as item}
          <path d={pathFor(item.values)} fill="none" stroke={item.color} stroke-width="2.5" vector-effect="non-scaling-stroke" />
        {/each}

        {#if hoveredIndex !== null}
          <line x1={chart.buckets[hoveredIndex].x} x2={chart.buckets[hoveredIndex].x} y1={top} y2={bottom} stroke="currentColor" stroke-width="1" class="text-foreground/40" />
          {#each chart.series as item}
            {@const value = item.values[hoveredIndex]}
            {#if value !== null}
              <circle cx={chart.buckets[hoveredIndex].x} cy={yFor(value)} r="4" fill={item.color} />
            {/if}
          {/each}
        {/if}

        {#each chart.buckets as bucket, index}
          {#if index === 0 || index === Math.floor((bucketCount - 1) / 2) || index === bucketCount - 1}
            <text x={bucket.x} y="210" text-anchor={index === 0 ? 'start' : index === bucketCount - 1 ? 'end' : 'middle'} class="fill-muted-foreground text-[12px]">{timeFormatter.format(bucket.end)}</text>
          {/if}
        {/each}
      </svg>

      {#if hoveredIndex !== null}
        <div class="pointer-events-none absolute right-2 top-2 z-20 min-w-52 border border-border bg-background/95 px-3 py-2 text-xs shadow-xl">
          <p class="font-medium">{bucketLabel(hoveredIndex)}</p>
          <div class="mt-2 space-y-1.5">
            {#each chart.series as item}
              <div class="flex items-center justify-between gap-5">
                <span class="flex items-center gap-2 text-muted-foreground"><span class="size-2" style:background={item.color}></span>{item.label}</span>
                <span class="font-mono tabular-nums">{item.values[hoveredIndex] === null ? 'Unavailable' : formatValue(item.values[hoveredIndex])}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {:else}
    <p class="mt-6 py-16 text-center text-sm text-muted-foreground">Historical telemetry is not available yet.</p>
  {/if}
</article>
