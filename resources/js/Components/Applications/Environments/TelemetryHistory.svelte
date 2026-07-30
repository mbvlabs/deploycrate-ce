<script lang="ts">
  type TelemetryPoint = {
    observedAt: string
    cpuCores: number
    memoryBytes: number
    cpuAvailable: boolean
    memoryAvailable: boolean
  }

  let {
    points,
    formatCPU,
    formatMemory,
  }: {
    points: TelemetryPoint[]
    formatCPU: (value: number) => string
    formatMemory: (value: number) => string
  } = $props()

  let hoveredIndex = $state<number | null>(null)
  const left = 82
  const right = 718
  const top = 18
  const bottom = 178
  const bucketCount = 12
  const historyWindowMilliseconds = 24 * 60 * 60 * 1000
  const timeFormatter = new Intl.DateTimeFormat(undefined, { hour: 'numeric' })
  const rangeFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })

  const chart = $derived.by(() => {
    const timestamps = points
      .map((point) => new Date(point.observedAt).getTime())
      .filter((timestamp) => Number.isFinite(timestamp))
    const end = timestamps.length ? Math.max(...timestamps) : Date.now()
    const start = end - historyWindowMilliseconds
    const duration = historyWindowMilliseconds / bucketCount
    const samples = points
      .map((point) => ({ ...point, timestamp: new Date(point.observedAt).getTime() }))
      .filter((point) => Number.isFinite(point.timestamp) && point.timestamp >= start && point.timestamp <= end)
      .sort((a, b) => a.timestamp - b.timestamp)
    const buckets = Array.from({ length: bucketCount }, (_, index) => {
      const bucketStart = start + (index * duration)
      const bucketEnd = bucketStart + duration
      const bucketSamples = samples.filter((point) => (
        point.timestamp >= bucketStart
        && (index === bucketCount - 1 ? point.timestamp <= bucketEnd : point.timestamp < bucketEnd)
      ))
      const cpuSample = bucketSamples.findLast((point) => point.cpuAvailable)
      const memorySample = bucketSamples.findLast((point) => point.memoryAvailable)
      return {
        start: bucketStart,
        end: bucketEnd,
        x: left + (index / (bucketCount - 1)) * (right - left),
        cpu: cpuSample?.cpuCores ?? null,
        memory: memorySample?.memoryBytes ?? null,
      }
    })
    const cpuValues = buckets.flatMap((bucket) => bucket.cpu === null ? [] : [bucket.cpu])
    const memoryValues = buckets.flatMap((bucket) => bucket.memory === null ? [] : [bucket.memory])
    return {
      buckets,
      cpuMaximum: Math.max(0.01, ...cpuValues) * 1.1,
      memoryMaximum: Math.max(1, ...memoryValues) * 1.1,
      available: cpuValues.length > 0 || memoryValues.length > 0,
    }
  })

  const yFor = (value: number, maximum: number) => bottom - (Math.max(0, value) / maximum) * (bottom - top)
  const pathFor = (metric: 'cpu' | 'memory', maximum: number) => chart.buckets.map((bucket, index) => {
    const value = bucket[metric]
    if (value === null) return ''
    return `${index === 0 || chart.buckets[index - 1][metric] === null ? 'M' : 'L'} ${bucket.x.toFixed(1)} ${yFor(value, maximum).toFixed(1)}`
  }).filter(Boolean).join(' ')
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
      <h3 class="text-sm font-semibold">CPU and memory usage</h3>
      <p class="mt-1 text-xs text-muted-foreground">Active container usage over the last 24 hours</p>
    </div>
    <div class="flex flex-wrap gap-x-4 gap-y-2 text-[11px] text-muted-foreground">
      <span class="flex items-center gap-2"><span class="size-2" style:background="var(--chart-1)"></span>CPU</span>
      <span class="flex items-center gap-2"><span class="size-2" style:background="var(--chart-2)"></span>Memory</span>
    </div>
  </div>

  {#if chart.available}
    <div class="relative mt-4">
      <svg
        viewBox="0 0 800 220"
        class="h-auto w-full touch-none"
        role="img"
        aria-label="CPU and memory usage over the last 24 hours"
        onpointerenter={hover}
        onpointermove={hover}
        onpointerleave={() => (hoveredIndex = null)}
      >
        {#each [0, 0.5, 1] as ratio}
          {@const y = bottom - ratio * (bottom - top)}
          <line x1={left} x2={right} y1={y} y2={y} stroke="currentColor" stroke-width="1" class="text-border" />
          <text x={left - 10} y={y + 4} text-anchor="end" class="fill-muted-foreground text-[11px]">{formatCPU(chart.cpuMaximum * ratio)}</text>
          <text x={right + 10} y={y + 4} text-anchor="start" class="fill-muted-foreground text-[11px]">{formatMemory(chart.memoryMaximum * ratio)}</text>
        {/each}

        <path d={pathFor('cpu', chart.cpuMaximum)} fill="none" stroke="var(--chart-1)" stroke-width="2.5" vector-effect="non-scaling-stroke" />
        <path d={pathFor('memory', chart.memoryMaximum)} fill="none" stroke="var(--chart-2)" stroke-width="2.5" vector-effect="non-scaling-stroke" />

        {#if hoveredIndex !== null}
          {@const bucket = chart.buckets[hoveredIndex]}
          <line x1={bucket.x} x2={bucket.x} y1={top} y2={bottom} stroke="currentColor" stroke-width="1" class="text-foreground/40" />
          {#if bucket.cpu !== null}<circle cx={bucket.x} cy={yFor(bucket.cpu, chart.cpuMaximum)} r="4" fill="var(--chart-1)" />{/if}
          {#if bucket.memory !== null}<circle cx={bucket.x} cy={yFor(bucket.memory, chart.memoryMaximum)} r="4" fill="var(--chart-2)" />{/if}
        {/if}

        {#each chart.buckets as bucket, index}
          {#if index === 0 || index === Math.floor((bucketCount - 1) / 2) || index === bucketCount - 1}
            <text x={bucket.x} y="210" text-anchor={index === 0 ? 'start' : index === bucketCount - 1 ? 'end' : 'middle'} class="fill-muted-foreground text-[12px]">{timeFormatter.format(bucket.end)}</text>
          {/if}
        {/each}
      </svg>

      {#if hoveredIndex !== null}
        {@const bucket = chart.buckets[hoveredIndex]}
        <div class="pointer-events-none absolute right-2 top-2 z-20 min-w-52 border border-border bg-background/95 px-3 py-2 text-xs shadow-xl">
          <p class="font-medium">{bucketLabel(hoveredIndex)}</p>
          <div class="mt-2 space-y-1.5">
            <div class="flex items-center justify-between gap-5"><span class="flex items-center gap-2 text-muted-foreground"><span class="size-2" style:background="var(--chart-1)"></span>CPU</span><span class="font-mono tabular-nums">{bucket.cpu === null ? 'Unavailable' : formatCPU(bucket.cpu)}</span></div>
            <div class="flex items-center justify-between gap-5"><span class="flex items-center gap-2 text-muted-foreground"><span class="size-2" style:background="var(--chart-2)"></span>Memory</span><span class="font-mono tabular-nums">{bucket.memory === null ? 'Unavailable' : formatMemory(bucket.memory)}</span></div>
          </div>
        </div>
      {/if}
    </div>
  {:else}
    <p class="mt-6 py-16 text-center text-sm text-muted-foreground">Historical telemetry is not available yet.</p>
  {/if}
</article>
