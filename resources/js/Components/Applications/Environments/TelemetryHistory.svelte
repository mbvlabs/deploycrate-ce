<script lang="ts">
  type TelemetryPoint = {
    observedAt: string;
    memoryBytes: number;
    memoryAvailable: boolean;
  };

  type TelemetrySeries = {
    id: string;
    label: string;
    active?: boolean;
    points: TelemetryPoint[];
  };

  let {
    series,
    telemetryRange = "24h",
  }: {
    series: TelemetrySeries[];
    telemetryRange: "1h" | "6h" | "24h" | "7d";
  } = $props();

  let hoveredIndex = $state<number | null>(null);
  const left = 82;
  const right = 718;
  const top = 18;
  const bottom = 178;
  const bucketCount = 36;
  const activeColor = "var(--chart-2)";
  const containerColors = [
    "var(--chart-1)",
    "var(--chart-3)",
    "var(--chart-4)",
    "var(--chart-5)",
  ];
  const rangeSeconds = $derived(
    { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 }[telemetryRange] ??
      86400,
  );
  const rangeLabel = $derived(
    {
      "1h": "last hour",
      "6h": "last 6 hours",
      "24h": "last 24 hours",
      "7d": "last 7 days",
    }[telemetryRange] ?? "last 24 hours",
  );
  const historyWindowMilliseconds = $derived(rangeSeconds * 1000);
  const bucketMilliseconds = $derived(historyWindowMilliseconds / bucketCount);
  const rangeFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  const axisFormatter = $derived(
    new Intl.DateTimeFormat(
      undefined,
      rangeSeconds === 604800
        ? { month: "short", day: "numeric" }
        : rangeSeconds <= 6 * 3600
          ? { hour: "numeric", minute: "2-digit" }
          : { hour: "numeric" },
    ),
  );
  const formatMemory = (value: number) =>
    `${(value / 1024 ** 2).toFixed(1)} MB`;

  const chart = $derived.by(() => {
    const timestamps = series
      .flatMap((item) => item.points)
      .map((point) => new Date(point.observedAt).getTime())
      .filter((timestamp) => Number.isFinite(timestamp));
    const latest = timestamps.length ? Math.max(...timestamps) : Date.now();
    const end = Math.ceil(latest / bucketMilliseconds) * bucketMilliseconds;
    const start = end - historyWindowMilliseconds;
    const buckets = Array.from({ length: bucketCount }, (_, index) => ({
      start: start + index * bucketMilliseconds,
      end: start + (index + 1) * bucketMilliseconds,
      x: left + (index + 0.5) * ((right - left) / bucketCount),
    }));
    let colorIndex = 0;
    const chartSeries = series.map((item) => {
      const active = item.active === true || series.length === 1;
      const color = active
        ? activeColor
        : containerColors[colorIndex++ % containerColors.length];
      const samples = item.points
        .map((point) => ({
          ...point,
          timestamp: new Date(point.observedAt).getTime(),
        }))
        .filter(
          (point) =>
            point.memoryAvailable &&
            Number.isFinite(point.timestamp) &&
            point.timestamp >= start &&
            point.timestamp <= end,
        )
        .sort((a, b) => a.timestamp - b.timestamp);
      const memory = buckets.map((bucket, index) => {
        const bucketSamples = samples.filter(
          (point) =>
            point.timestamp >= bucket.start &&
            (index === bucketCount - 1
              ? point.timestamp <= bucket.end
              : point.timestamp < bucket.end),
        );
        return bucketSamples.length === 0
          ? null
          : bucketSamples.reduce(
              (total, point) => total + point.memoryBytes,
              0,
            ) / bucketSamples.length;
      });
      return { id: item.id, label: item.label, active, color, memory };
    });
    const memoryValues = chartSeries.flatMap((item) =>
      item.memory.flatMap((value) => (value === null ? [] : [value])),
    );
    return {
      buckets,
      series: chartSeries,
      multiple: chartSeries.length > 1,
      memoryMaximum: Math.max(1, ...memoryValues) * 1.1,
      available: memoryValues.length > 0,
    };
  });

  const yFor = (value: number) =>
    bottom - (Math.max(0, value) / chart.memoryMaximum) * (bottom - top);
  const pathFor = (memory: Array<number | null>) =>
    memory
      .map((value, index) => {
        if (value === null) return "";
        const command = index === 0 || memory[index - 1] === null ? "M" : "L";
        return `${command} ${chart.buckets[index].x.toFixed(1)} ${yFor(value).toFixed(1)}`;
      })
      .filter(Boolean)
      .join(" ");
  const bucketLabel = (index: number) =>
    `${rangeFormatter.format(chart.buckets[index].start)} to ${rangeFormatter.format(chart.buckets[index].end)}`;

  const hover = (event: PointerEvent) => {
    const bounds = (
      event.currentTarget as SVGSVGElement
    ).getBoundingClientRect();
    const x = ((event.clientX - bounds.left) / bounds.width) * 800;
    hoveredIndex = Math.min(
      bucketCount - 1,
      Math.max(0, Math.floor(((x - left) / (right - left)) * bucketCount)),
    );
  };
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-5">
  <div
    class="flex w-full flex-col gap-2 sm:flex-row sm:items-start sm:justify-between"
  >
    <div class="sm:shrink-0">
      <h3 class="text-sm font-semibold">Memory usage</h3>
      <p class="mt-1 text-xs text-muted-foreground">
        Average container memory over the {rangeLabel}
      </p>
    </div>
    {#if chart.multiple}
      <div
        class="flex min-w-0 flex-wrap justify-end gap-x-4 gap-y-1 text-xs text-muted-foreground sm:flex-1"
      >
        {#each chart.series as item (item.id)}
          <span class="flex items-center gap-2"
            ><span class="size-2" style:background={item.color}
            ></span>{item.label}{item.active ? " (active)" : ""}</span
          >
        {/each}
      </div>
    {/if}
  </div>

  {#if chart.available}
    <div class="relative mt-4 w-full min-w-0">
      <svg
        viewBox="0 0 800 220"
        class="block h-auto w-full max-w-none touch-none"
        role="img"
        aria-label={`Memory usage over the ${rangeLabel}`}
        onpointerenter={hover}
        onpointermove={hover}
        onpointerleave={() => (hoveredIndex = null)}
      >
        {#each [0, 0.5, 1] as ratio}
          {@const y = bottom - ratio * (bottom - top)}
          <line
            x1={left}
            x2={right}
            y1={y}
            y2={y}
            stroke="currentColor"
            stroke-width="1"
            class="text-border"
          />
          <text
            x="0"
            y={y + 4}
            text-anchor="start"
            class="fill-muted-foreground text-[11px]"
            >{formatMemory(chart.memoryMaximum * ratio)}</text
          >
        {/each}

        {#each chart.series as item (item.id)}
          <path
            d={pathFor(item.memory)}
            fill="none"
            stroke={item.color}
            stroke-width="2.5"
            vector-effect="non-scaling-stroke"
          />
          {#each item.memory as value, index}
            {#if value !== null}
              <circle
                cx={chart.buckets[index].x}
                cy={yFor(value)}
                r="2.5"
                fill={item.color}
              />
            {/if}
          {/each}
        {/each}

        {#if hoveredIndex !== null}
          {@const bucket = chart.buckets[hoveredIndex]}
          <line
            x1={bucket.x}
            x2={bucket.x}
            y1={top}
            y2={bottom}
            stroke="currentColor"
            stroke-width="1"
            class="text-foreground/40"
          />
          {#each chart.series as item (item.id)}
            {@const value = item.memory[hoveredIndex]}
            {#if value !== null}
              <circle cx={bucket.x} cy={yFor(value)} r="4" fill={item.color} />
            {/if}
          {/each}
        {/if}

        {#each chart.buckets as bucket, index}
          {#if index === 0 || index === Math.floor((bucketCount - 1) / 2) || index === bucketCount - 1}
            <text
              x={bucket.x}
              y="210"
              text-anchor={index === 0
                ? "start"
                : index === bucketCount - 1
                  ? "end"
                  : "middle"}
              class="fill-muted-foreground text-[8px]"
              >{axisFormatter.format(bucket.end)}</text
            >
          {/if}
        {/each}
      </svg>

      {#if hoveredIndex !== null}
        <div
          class="pointer-events-none absolute right-2 top-2 z-20 min-w-52 border border-border bg-background/95 px-3 py-2 text-xs shadow-xl"
        >
          <p class="font-medium">{bucketLabel(hoveredIndex)}</p>
          <div class="mt-2 space-y-1.5">
            {#each chart.series as item (item.id)}
              {@const value = item.memory[hoveredIndex]}
              <div class="flex items-center justify-between gap-5">
                <span class="flex items-center gap-2 text-muted-foreground"
                  ><span class="size-2" style:background={item.color}
                  ></span>{chart.multiple
                    ? `${item.label}${item.active ? " (active)" : ""}`
                    : "Average memory"}</span
                ><span class="font-mono tabular-nums"
                  >{value === null ? "Unavailable" : formatMemory(value)}</span
                >
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {:else}
    <p class="mt-6 py-16 text-center text-sm text-muted-foreground">
      Historical memory telemetry is not available yet.
    </p>
  {/if}
</article>
