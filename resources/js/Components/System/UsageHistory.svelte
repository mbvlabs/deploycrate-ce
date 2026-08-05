<script lang="ts">
  type UsagePoint = {
    observedAt: string
    used: number
    free: number
  }

  let { label, points }: {
    label: string
    points: UsagePoint[]
  } = $props()
  let hoveredIndex = $state<number | null>(null)
  let tooltipPosition = $state({ left: 0, top: 0 })
  let chartElement: HTMLDivElement

  const left = 52
  const right = 784
  const top = 18
  const bottom = 180
  const pointCount = 12
  const historyWindowMilliseconds = 24 * 60 * 60 * 1000
  const bucketDurationMilliseconds = historyWindowMilliseconds / pointCount
  const bucketFormatter = new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
  const pointTimeFormatter = new Intl.DateTimeFormat(undefined, { hour: 'numeric' })

  const chart = $derived.by(() => {
    const end = Date.now()
    const start = end - historyWindowMilliseconds
    const samples = points
      .map((point) => ({
        ...point,
        timestamp: new Date(point.observedAt).getTime(),
        percentage: point.used + point.free > 0 ? (point.used / (point.used + point.free)) * 100 : 0,
      }))
      .filter((point) => Number.isFinite(point.timestamp) && point.timestamp >= start && point.timestamp <= end)
      .sort((a, b) => a.timestamp - b.timestamp)

    return Array.from({ length: pointCount }, (_, index) => {
      const bucketStart = start + (index * bucketDurationMilliseconds)
      const bucketEnd = bucketStart + bucketDurationMilliseconds
      const sample = samples.findLast((candidate) => (
        candidate.timestamp >= bucketStart
        && (index === pointCount - 1 ? candidate.timestamp <= bucketEnd : candidate.timestamp < bucketEnd)
      ))
      const percentage = Math.min(100, Math.max(0, sample?.percentage ?? 0))
      return {
        bucketStart,
        bucketEnd,
        percentage,
        timeLabel: pointTimeFormatter.format(bucketEnd),
        x: left + (index / (pointCount - 1)) * (right - left),
        y: bottom - (percentage / 100) * (bottom - top),
      }
    })
  })
  const hoveredPoint = $derived(hoveredIndex === null ? null : chart[hoveredIndex])

  const formatBucket = (start: number, end: number) => `${bucketFormatter.format(start)} to ${bucketFormatter.format(end)}`

  const showPoint = (index: number, event: PointerEvent | FocusEvent) => {
    const target = (event.currentTarget as SVGCircleElement).getBoundingClientRect()
    const container = chartElement.getBoundingClientRect()
    tooltipPosition = {
      left: target.left + (target.width / 2) - container.left,
      top: target.top + (target.height / 2) - container.top,
    }
    hoveredIndex = index
  }

  const tooltipTransform = (index: number, y: number) => {
    const horizontal = index < 2 ? '0%' : index > pointCount - 3 ? '-100%' : '-50%'
    const vertical = y < 70 ? '0.75rem' : 'calc(-100% - 0.75rem)'
    return `translate(${horizontal}, ${vertical})`
  }
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-5">
  <div class="flex items-center justify-between gap-4">
    <h2 class="text-base font-semibold">{label}</h2>
    <p class="text-sm font-medium text-foreground/70">Last 24 hours</p>
  </div>
  <div bind:this={chartElement} class="relative mt-4 w-full min-w-0">
    <svg viewBox="0 0 800 220" class="block h-56 w-full max-w-none" role="img" aria-label={`${label} over the last 24 hours`}>
      {#each [0, 50, 100] as percentage}
        {@const y = bottom - (percentage / 100) * (bottom - top)}
        <line x1={left} x2={right} y1={y} y2={y} stroke="currentColor" stroke-width="1" class="text-border" />
        <text x="0" y={y + 5} text-anchor="start" class="fill-foreground text-[16px] font-medium opacity-80">{percentage}%</text>
      {/each}
      <polyline
        points={chart.map((point) => `${point.x.toFixed(1)},${point.y.toFixed(1)}`).join(' ')}
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        vector-effect="non-scaling-stroke"
        class="text-primary"
      />
      {#each chart as point, index}
        <circle
          cx={point.x}
          cy={point.y}
          r={hoveredIndex === index ? 5.5 : 3.5}
          fill="currentColor"
          class="text-primary transition-all"
        />
        <circle
          cx={point.x}
          cy={point.y}
          r="18"
          fill="transparent"
          class="cursor-crosshair outline-none focus:stroke-primary"
          stroke="transparent"
          stroke-width="2"
          role="button"
          tabindex="0"
          aria-label={`${formatBucket(point.bucketStart, point.bucketEnd)}, ${point.percentage.toFixed(1)}%`}
          onpointerenter={(event) => showPoint(index, event)}
          onpointermove={(event) => showPoint(index, event)}
          onpointerleave={() => (hoveredIndex = hoveredIndex === index ? null : hoveredIndex)}
          onfocus={(event) => showPoint(index, event)}
          onblur={() => (hoveredIndex = hoveredIndex === index ? null : hoveredIndex)}
        />
      {/each}
      {#each chart as point, index}
        <text
          x={point.x}
          y="208"
          text-anchor={index === 0 ? 'start' : index === pointCount - 1 ? 'end' : 'middle'}
          class="fill-foreground text-[14px] font-medium opacity-80"
        >{point.timeLabel}</text>
      {/each}
    </svg>

    {#if hoveredPoint && hoveredIndex !== null}
      <div
        class="pointer-events-none absolute z-20 grid min-w-44 gap-1.5 border border-border/70 bg-background px-3 py-2 text-xs shadow-xl"
        style:left={`${tooltipPosition.left}px`}
        style:top={`${tooltipPosition.top}px`}
        style:transform={tooltipTransform(hoveredIndex, hoveredPoint.y)}
      >
        <p class="whitespace-nowrap font-medium">{formatBucket(hoveredPoint.bucketStart, hoveredPoint.bucketEnd)}</p>
        <div class="flex items-center justify-between gap-5">
          <span class="flex items-center gap-2 text-muted-foreground">
            <span class="size-2 bg-primary"></span>
            {label}
          </span>
          <span class="font-mono font-medium tabular-nums">{hoveredPoint.percentage.toFixed(1)}%</span>
        </div>
      </div>
    {/if}
  </div>
</article>
