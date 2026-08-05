<script lang="ts">
  type TelemetryPoint = {
    observedAt: string
    memoryBytes: number
    memoryAvailable: boolean
  }

  let { points }: { points: TelemetryPoint[] } = $props()

  let hoveredIndex = $state<number | null>(null)
  const left = 82
  const right = 718
  const top = 18
  const bottom = 178
  const bucketCount = 24
  const hourMilliseconds = 60 * 60 * 1000
  const historyWindowMilliseconds = bucketCount * hourMilliseconds
  const bucketWidth = (right - left) / bucketCount
  const timeFormatter = new Intl.DateTimeFormat(undefined, { hour: 'numeric' })
  const rangeFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
  const formatMemory = (value: number) => `${(value / (1024 ** 2)).toFixed(1)} MB`

  const chart = $derived.by(() => {
    const timestamps = points
      .map((point) => new Date(point.observedAt).getTime())
      .filter((timestamp) => Number.isFinite(timestamp))
    const latest = timestamps.length ? Math.max(...timestamps) : Date.now()
    const end = Math.ceil(latest / hourMilliseconds) * hourMilliseconds
    const start = end - historyWindowMilliseconds
    const samples = points
      .map((point) => ({ ...point, timestamp: new Date(point.observedAt).getTime() }))
      .filter((point) => point.memoryAvailable && Number.isFinite(point.timestamp) && point.timestamp >= start && point.timestamp <= end)
      .sort((a, b) => a.timestamp - b.timestamp)
    const buckets = Array.from({ length: bucketCount }, (_, index) => {
      const bucketStart = start + (index * hourMilliseconds)
      const bucketEnd = bucketStart + hourMilliseconds
      const bucketSamples = samples.filter((point) => (
        point.timestamp >= bucketStart
        && (index === bucketCount - 1 ? point.timestamp <= bucketEnd : point.timestamp < bucketEnd)
      ))
      const memory = bucketSamples.length === 0
        ? null
        : bucketSamples.reduce((total, point) => total + point.memoryBytes, 0) / bucketSamples.length
      return {
        start: bucketStart,
        end: bucketEnd,
        x: left + ((index + 0.5) * bucketWidth),
        memory,
      }
    })
    const memoryValues = buckets.flatMap((bucket) => bucket.memory === null ? [] : [bucket.memory])
    return {
      buckets,
      memoryMaximum: Math.max(1, ...memoryValues) * 1.1,
      available: memoryValues.length > 0,
    }
  })

  const yFor = (value: number) => bottom - (Math.max(0, value) / chart.memoryMaximum) * (bottom - top)
  const memoryPath = $derived(chart.buckets.map((bucket, index) => {
    if (bucket.memory === null) return ''
    const command = index === 0 || chart.buckets[index - 1].memory === null ? 'M' : 'L'
    return `${command} ${bucket.x.toFixed(1)} ${yFor(bucket.memory).toFixed(1)}`
  }).filter(Boolean).join(' '))
  const bucketLabel = (index: number) => `${rangeFormatter.format(chart.buckets[index].start)} to ${rangeFormatter.format(chart.buckets[index].end)}`

  const hover = (event: PointerEvent) => {
    const bounds = (event.currentTarget as SVGSVGElement).getBoundingClientRect()
    const x = ((event.clientX - bounds.left) / bounds.width) * 800
    hoveredIndex = Math.min(bucketCount - 1, Math.max(0, Math.floor((x - left) / bucketWidth)))
  }
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-5">
  <div>
    <h3 class="text-sm font-semibold">Memory usage</h3>
    <p class="mt-1 text-xs text-muted-foreground">Average active container memory for each hour over the last 24 hours</p>
  </div>

  {#if chart.available}
    <div class="relative mt-4 w-full min-w-0">
      <svg
        viewBox="0 0 800 220"
        class="block h-auto w-full max-w-none touch-none"
        role="img"
        aria-label="Hourly memory usage over the last 24 hours"
        onpointerenter={hover}
        onpointermove={hover}
        onpointerleave={() => (hoveredIndex = null)}
      >
        {#each [0, 0.5, 1] as ratio}
          {@const y = bottom - ratio * (bottom - top)}
          <line x1={left} x2={right} y1={y} y2={y} stroke="currentColor" stroke-width="1" class="text-border" />
          <text x="0" y={y + 4} text-anchor="start" class="fill-muted-foreground text-[11px]">{formatMemory(chart.memoryMaximum * ratio)}</text>
        {/each}

        <path d={memoryPath} fill="none" stroke="var(--chart-2)" stroke-width="2.5" vector-effect="non-scaling-stroke" />

        {#each chart.buckets as bucket}
          {#if bucket.memory !== null}
            <circle cx={bucket.x} cy={yFor(bucket.memory)} r="2.5" fill="var(--chart-2)" />
          {/if}
        {/each}

        {#if hoveredIndex !== null}
          {@const bucket = chart.buckets[hoveredIndex]}
          <line x1={bucket.x} x2={bucket.x} y1={top} y2={bottom} stroke="currentColor" stroke-width="1" class="text-foreground/40" />
          {#if bucket.memory !== null}<circle cx={bucket.x} cy={yFor(bucket.memory)} r="4" fill="var(--chart-2)" />{/if}
        {/if}

        {#each chart.buckets as bucket}
          <text x={bucket.x} y="210" text-anchor="middle" class="fill-muted-foreground text-[8px]">{timeFormatter.format(bucket.start)}</text>
        {/each}
      </svg>

      {#if hoveredIndex !== null}
        {@const bucket = chart.buckets[hoveredIndex]}
        <div class="pointer-events-none absolute right-2 top-2 z-20 min-w-52 border border-border bg-background/95 px-3 py-2 text-xs shadow-xl">
          <p class="font-medium">{bucketLabel(hoveredIndex)}</p>
          <div class="mt-2 flex items-center justify-between gap-5"><span class="flex items-center gap-2 text-muted-foreground"><span class="size-2" style:background="var(--chart-2)"></span>Average memory</span><span class="font-mono tabular-nums">{bucket.memory === null ? 'Unavailable' : formatMemory(bucket.memory)}</span></div>
        </div>
      {/if}
    </div>
  {:else}
    <p class="mt-6 py-16 text-center text-sm text-muted-foreground">Historical memory telemetry is not available yet.</p>
  {/if}
</article>
