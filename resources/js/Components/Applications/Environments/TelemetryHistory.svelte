<script lang="ts">
  import { LineChart, Tooltip } from "layerchart";

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

  type ChartPoint = {
    bucketStart: number;
    bucketEnd: number;
    timestamp: number;
    values: number[];
  };

  let {
    series,
    telemetryRange = "24h",
  }: {
    series: TelemetrySeries[];
    telemetryRange: "1h" | "6h" | "24h" | "7d";
  } = $props();

  const bucketCount = 36;
  const yAxisWidth = 76;
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
      bucketStart: start + index * bucketMilliseconds,
      bucketEnd: start + (index + 1) * bucketMilliseconds,
      timestamp: start + (index + 0.5) * bucketMilliseconds,
    }));
    let colorIndex = 0;
    const preparedSeries = series.map((item) => {
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
      let hasSamples = false;
      const values = buckets.map((bucket, index) => {
        const bucketSamples = samples.filter(
          (point) =>
            point.timestamp >= bucket.bucketStart &&
            (index === bucketCount - 1
              ? point.timestamp <= bucket.bucketEnd
              : point.timestamp < bucket.bucketEnd),
        );
        if (bucketSamples.length === 0) return 0;

        hasSamples = true;
        return (
          bucketSamples.reduce((total, point) => total + point.memoryBytes, 0) /
          bucketSamples.length
        );
      });

      return {
        id: item.id,
        key: item.id,
        label: item.label,
        active,
        color,
        hasSamples,
        values,
      };
    });
    const memoryValues = preparedSeries.flatMap((item) => item.values);
    const data: ChartPoint[] = buckets.map((bucket, index) => ({
      ...bucket,
      values: preparedSeries.map((item) => item.values[index]),
    }));
    const lineSeries = preparedSeries.map((item, index) => ({
      key: item.key,
      label: item.label,
      color: item.color,
      value: (point: ChartPoint) => point.values[index],
      props: {
        fill: "none",
        strokeWidth: item.active ? 3 : 2.5,
      },
    }));

    return {
      data,
      series: preparedSeries,
      lineSeries,
      multiple: preparedSeries.length > 1,
      memoryMaximum: Math.max(1, ...memoryValues) * 1.1,
      available: preparedSeries.some((item) => item.hasSamples),
    };
  });

  const bucketLabel = (point: ChartPoint) =>
    `${rangeFormatter.format(point.bucketStart)} to ${rangeFormatter.format(point.bucketEnd)}`;
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-5">
  <div
    class="flex w-full flex-col gap-2 sm:flex-row sm:items-start sm:justify-between"
  >
    <div class="sm:shrink-0">
      <h3 class="text-sm font-semibold">Memory usage</h3>
      <p class="mt-1 text-xs text-muted-foreground">
        Total memory used by live environment containers over the {rangeLabel}
      </p>
    </div>
    {#if chart.multiple}
      <div
        class="flex min-w-0 flex-wrap justify-end gap-x-4 gap-y-1 text-xs text-muted-foreground sm:flex-1"
      >
        {#each chart.series as item (item.id)}
          <span class="flex items-center gap-2">
            <span class="size-2" style:background={item.color}></span>
            {item.label}{item.active ? " (active)" : ""}
          </span>
        {/each}
      </div>
    {/if}
  </div>

  {#if chart.available}
    <div class="mt-4 h-[13.75rem] w-full min-w-0">
      <LineChart
        data={chart.data}
        x="timestamp"
        series={chart.lineSeries}
        xDomain={[chart.data[0]?.timestamp, chart.data.at(-1)?.timestamp]}
        yDomain={[0, chart.memoryMaximum]}
        height={220}
        padding={{ top: 12, right: 10, bottom: 28, left: yAxisWidth }}
        axis={true}
        rule={false}
        highlight={{
          lines: { stroke: "var(--foreground)", opacity: 0.35 },
          points: { r: 4, stroke: "var(--background)", strokeWidth: 2 },
          motion: "none" as const,
        }}
        tooltipContext={{ mode: "bisect-x" }}
        motion="none"
        aria-label={`Memory usage over the ${rangeLabel}`}
        props={{
          xAxis: {
            ticks: 3,
            tickMarks: false,
            rule: false,
            format: (value: number) => axisFormatter.format(value),
            classes: { tickLabel: "fill-muted-foreground" },
          },
          yAxis: {
            ticks: [0, chart.memoryMaximum / 2, chart.memoryMaximum],
            tickMarks: false,
            rule: false,
            grid: { stroke: "var(--border)" },
            format: (value: number) => formatMemory(value),
            tickLabelProps: { x: -yAxisWidth, dx: 0, textAnchor: "start" },
            classes: { tickLabel: "fill-muted-foreground" },
          },
          svg: { title: `Memory usage over the ${rangeLabel}` },
        }}
      >
        {#snippet tooltip({ context })}
          <Tooltip.Root
            {context}
            x="pointer"
            y="pointer"
            portal={false}
            motion="none"
            fadeDuration={0}
            variant="none"
          >
            {#snippet children({ data }: { data: ChartPoint })}
              <div
                class="min-w-52 border border-border bg-background/95 px-3 py-2 text-xs shadow-xl"
              >
                <p class="font-medium">{bucketLabel(data)}</p>
                <div class="mt-2 space-y-1.5">
                  {#each chart.series as item, index (item.id)}
                    <div class="flex items-center justify-between gap-5">
                      <span
                        class="flex items-center gap-2 text-muted-foreground"
                      >
                        <span class="size-2" style:background={item.color}
                        ></span>
                        {chart.multiple
                          ? `${item.label}${item.active ? " (active)" : ""}`
                          : "Environment memory"}
                      </span>
                      <span class="font-mono tabular-nums">
                        {formatMemory(data.values[index])}
                      </span>
                    </div>
                  {/each}
                </div>
              </div>
            {/snippet}
          </Tooltip.Root>
        {/snippet}
      </LineChart>
    </div>
  {:else}
    <p class="mt-6 py-16 text-center text-sm text-muted-foreground">
      Historical memory telemetry is not available yet.
    </p>
  {/if}
</article>
