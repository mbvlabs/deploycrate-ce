<script lang="ts">
  import { LineChart, Tooltip } from "layerchart";
  import { curveStepAfter } from "d3-shape";

  type TelemetryPoint = {
    observedAt: string;
    value: number;
  };

  type TelemetrySeries = {
    label: string;
    points: TelemetryPoint[];
    comparison?: boolean;
  };

  type ChartPoint = {
    bucketStart: number;
    bucketEnd: number;
    timestamp: number;
    values: number[];
  };

  let {
    label,
    description,
    series,
    formatValue,
    windowSeconds = 24 * 60 * 60,
    maximum,
  }: {
    label: string;
    description: string;
    series: TelemetrySeries[];
    formatValue: (value: number) => string;
    windowSeconds?: number;
    maximum?: number;
  } = $props();

  const bucketCount = 36;
  const yAxisWidth = 76;
  const colors = [
    "var(--chart-1)",
    "var(--chart-2)",
    "var(--chart-3)",
    "var(--chart-4)",
    "var(--chart-5)",
  ];
  const timeFormatter = new Intl.DateTimeFormat(undefined, { hour: "numeric" });
  const rangeFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });

  const chart = $derived.by(() => {
    const timestamps = series
      .flatMap((item) => item.points)
      .map((point) => new Date(point.observedAt).getTime())
      .filter((timestamp) => Number.isFinite(timestamp));
    const end = timestamps.length ? Math.max(...timestamps) : Date.now();
    const historyWindowMilliseconds = windowSeconds * 1000;
    const start = end - historyWindowMilliseconds;
    const duration = historyWindowMilliseconds / bucketCount;
    const buckets = Array.from({ length: bucketCount }, (_, index) => ({
      bucketStart: start + index * duration,
      bucketEnd: start + (index + 1) * duration,
      timestamp: start + (index + 0.5) * duration,
    }));
    const preparedSeries = series.map((item, seriesIndex) => {
      const samples = item.points
        .map((point) => ({
          timestamp: new Date(point.observedAt).getTime(),
          value: point.value,
        }))
        .filter(
          (point) =>
            Number.isFinite(point.timestamp) &&
            Number.isFinite(point.value) &&
            point.timestamp >= start &&
            point.timestamp <= end,
        )
        .sort((a, b) => a.timestamp - b.timestamp);
      const bucketValues = buckets.map(
        (bucket, index) =>
          samples.findLast(
            (point) =>
              point.timestamp >= bucket.bucketStart &&
              (index === bucketCount - 1
                ? point.timestamp <= bucket.bucketEnd
                : point.timestamp < bucket.bucketEnd),
          )?.value,
      );

      return {
        key: `series-${seriesIndex}`,
        label: item.label,
        color: colors[seriesIndex % colors.length],
        comparison: item.comparison === true,
        hasSamples: bucketValues.some((value) => value !== undefined),
        values: bucketValues.map((value) => value ?? 0),
      };
    });
    const values = preparedSeries.flatMap((item) => item.values);
    const chartMaximum = maximum ?? Math.max(1, ...values) * 1.1;
    const data: ChartPoint[] = buckets.map((bucket, index) => ({
      ...bucket,
      values: preparedSeries.map((item) => item.values[index]),
    }));
    const lineSeries = preparedSeries.map((item, index) => ({
      key: item.key,
      label: item.label,
      color: item.comparison ? "var(--muted-foreground)" : item.color,
      value: (point: ChartPoint) => point.values[index],
      props: {
        fill: "none",
        strokeWidth: item.comparison ? 2 : 3.5,
        dashArray: item.comparison ? "7 6" : undefined,
        curve: curveStepAfter,
      },
    }));

    return {
      data,
      series: preparedSeries,
      lineSeries,
      maximum: chartMaximum,
      available: preparedSeries.some((item) => item.hasSamples),
    };
  });

  const bucketLabel = (point: ChartPoint) =>
    `${rangeFormatter.format(point.bucketStart)} to ${rangeFormatter.format(point.bucketEnd)}`;
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-6">
  <div
    class="flex w-full flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"
  >
    <div class="sm:shrink-0">
      <h3 class="text-base font-semibold sm:whitespace-nowrap">{label}</h3>
      <p class="mt-1 text-sm leading-5 text-muted-foreground">{description}</p>
    </div>
    <div
      class="flex min-w-0 flex-wrap justify-end gap-x-5 gap-y-2 text-sm text-muted-foreground sm:flex-1"
    >
      {#each chart.series as item (item.key)}
        <span class="flex items-center gap-2">
          <span class="h-0.5 w-5" style:background={item.color}></span>
          {item.label}
        </span>
      {/each}
    </div>
  </div>

  {#if chart.available}
    <div class="mt-4 h-64 w-full min-w-0">
      <LineChart
        data={chart.data}
        x="timestamp"
        series={chart.lineSeries}
        xDomain={[chart.data[0]?.timestamp, chart.data.at(-1)?.timestamp]}
        yDomain={[0, chart.maximum]}
        height={256}
        padding={{ top: 14, right: 12, bottom: 30, left: yAxisWidth }}
        axis={true}
        rule={false}
        highlight={{
          lines: { stroke: "var(--foreground)", opacity: 0.35 },
          points: { r: 4.5, stroke: "var(--background)", strokeWidth: 2 },
          motion: false,
        }}
        tooltipContext={{ mode: "bisect-x" }}
        motion={false}
        aria-label={`${label} over the selected telemetry range`}
        props={{
          xAxis: {
            ticks: 3,
            tickMarks: false,
            rule: false,
            format: (value: number) => timeFormatter.format(value),
            classes: { tickLabel: "fill-muted-foreground font-medium" },
          },
          yAxis: {
            ticks: [0, chart.maximum / 2, chart.maximum],
            tickMarks: false,
            rule: false,
            grid: { stroke: "var(--border)" },
            format: (value: number) => formatValue(value),
            tickLabelProps: { x: -yAxisWidth, dx: 0, textAnchor: "start" },
            classes: { tickLabel: "fill-muted-foreground font-medium" },
          },
          svg: { title: `${label} over the selected telemetry range` },
        }}
      >
        {#snippet tooltip({ context })}
          <Tooltip.Root
            {context}
            x="pointer"
            y="pointer"
            portal={false}
            motion={false}
            fadeDuration={0}
            variant="none"
          >
            {#snippet children({ data }: { data: ChartPoint })}
              <div
                class="min-w-60 border border-border bg-background/95 px-4 py-3 text-sm shadow-xl"
              >
                <p class="font-medium">{bucketLabel(data)}</p>
                <div class="mt-3 space-y-2">
                  {#each chart.series as item, index (item.key)}
                    <div class="flex items-center justify-between gap-6">
                      <span
                        class="flex items-center gap-2 text-muted-foreground"
                      >
                        <span
                          class="h-0.5 w-5"
                          style:background={item.comparison
                            ? "var(--muted-foreground)"
                            : item.color}
                        ></span>
                        {item.label}
                      </span>
                      <span class="font-mono tabular-nums">
                        {formatValue(data.values[index])}
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
      Historical telemetry is not available yet.
    </p>
  {/if}
</article>
