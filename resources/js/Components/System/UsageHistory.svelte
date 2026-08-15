<script lang="ts">
  import { LineChart, Tooltip } from "layerchart";

  type UsagePoint = {
    observedAt: string;
    used: number;
    free: number;
  };

  type ChartPoint = {
    bucketStart: number;
    bucketEnd: number;
    timestamp: number;
    percentage: number;
  };

  let {
    label,
    points,
  }: {
    label: string;
    points: UsagePoint[];
  } = $props();

  const pointCount = 12;
  const yAxisWidth = 48;
  const historyWindowMilliseconds = 24 * 60 * 60 * 1000;
  const bucketDurationMilliseconds = historyWindowMilliseconds / pointCount;
  const bucketFormatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  const pointTimeFormatter = new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
  });

  const chart = $derived.by<ChartPoint[]>(() => {
    const end = Date.now();
    const start = end - historyWindowMilliseconds;
    const samples = points
      .map((point) => ({
        ...point,
        timestamp: new Date(point.observedAt).getTime(),
        percentage:
          point.used + point.free > 0
            ? (point.used / (point.used + point.free)) * 100
            : 0,
      }))
      .filter(
        (point) =>
          Number.isFinite(point.timestamp) &&
          point.timestamp >= start &&
          point.timestamp <= end,
      )
      .sort((a, b) => a.timestamp - b.timestamp);

    return Array.from({ length: pointCount }, (_, index) => {
      const bucketStart = start + index * bucketDurationMilliseconds;
      const bucketEnd = bucketStart + bucketDurationMilliseconds;
      const sample = samples.findLast(
        (candidate) =>
          candidate.timestamp >= bucketStart &&
          (index === pointCount - 1
            ? candidate.timestamp <= bucketEnd
            : candidate.timestamp < bucketEnd),
      );

      return {
        bucketStart,
        bucketEnd,
        timestamp: bucketEnd,
        percentage: Math.min(100, Math.max(0, sample?.percentage ?? 0)),
      };
    });
  });

  const formatBucket = (start: number, end: number) =>
    `${bucketFormatter.format(start)} to ${bucketFormatter.format(end)}`;
</script>

<article class="w-full min-w-0 border border-border bg-card/35 p-5">
  <div class="flex items-center justify-between gap-4">
    <h2 class="text-base font-semibold">{label}</h2>
    <p class="text-sm font-medium text-foreground/70">Last 24 hours</p>
  </div>

  <div class="mt-4 h-56 w-full min-w-0">
    <LineChart
      data={chart}
      x="timestamp"
      y="percentage"
      xDomain={[chart[0]?.timestamp, chart.at(-1)?.timestamp]}
      yDomain={[0, 100]}
      height={224}
      padding={{ top: 12, right: 12, bottom: 28, left: yAxisWidth }}
      axis={true}
      rule={false}
      points={{
        r: 3.5,
        fill: "var(--chart-1)",
        stroke: "var(--background)",
        strokeWidth: 1.5,
      }}
      highlight={{
        lines: { stroke: "var(--foreground)", opacity: 0.35 },
        points: { r: 5, stroke: "var(--background)", strokeWidth: 2 },
        motion: "none" as const,
      }}
      tooltipContext={{ mode: "bisect-x" }}
      motion="none"
      aria-label={`${label} over the last 24 hours`}
      props={{
        spline: {
          fill: "none",
          stroke: "var(--chart-1)",
          strokeWidth: 2.5,
        },
        xAxis: {
          ticks: 3,
          tickMarks: false,
          rule: false,
          format: (value: number) => pointTimeFormatter.format(value),
          classes: { tickLabel: "fill-foreground font-medium opacity-80" },
        },
        yAxis: {
          ticks: [0, 50, 100],
          tickMarks: false,
          rule: false,
          grid: { stroke: "var(--border)" },
          format: (value: number) => `${value}%`,
          tickLabelProps: { x: -yAxisWidth, dx: 0, textAnchor: "start" },
          classes: { tickLabel: "fill-foreground font-medium opacity-80" },
        },
        svg: { title: `${label} over the last 24 hours` },
      }}
    >
      {#snippet tooltip({ context })}
        <Tooltip.Root
          {context}
          x="data"
          y="data"
          anchor="bottom"
          yOffset={10}
          portal={false}
          motion="none"
          fadeDuration={0}
          variant="none"
        >
          {#snippet children({ data }: { data: ChartPoint })}
            <div
              class="grid min-w-44 gap-1.5 border border-border/70 bg-background px-3 py-2 text-xs shadow-xl"
            >
              <p class="whitespace-nowrap font-medium">
                {formatBucket(data.bucketStart, data.bucketEnd)}
              </p>
              <div class="flex items-center justify-between gap-5">
                <span class="flex items-center gap-2 text-muted-foreground">
                  <span class="size-2 bg-primary"></span>
                  {label}
                </span>
                <span class="font-mono font-medium tabular-nums">
                  {data.percentage.toFixed(1)}%
                </span>
              </div>
            </div>
          {/snippet}
        </Tooltip.Root>
      {/snippet}
    </LineChart>
  </div>
</article>
