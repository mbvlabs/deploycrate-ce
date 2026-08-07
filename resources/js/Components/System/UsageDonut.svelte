<script lang="ts">
  import { PieChart } from "layerchart";

  type DonutDatum = {
    key: string;
    label: string;
    value: number;
    color: string;
  };

  let {
    label,
    used,
    total,
    formatValue,
    available = true,
  }: {
    label: string;
    used: number;
    total: number;
    formatValue: (value: number) => string;
    available?: boolean;
  } = $props();

  const usedPercent = $derived(
    available && total > 0
      ? Math.min(100, Math.max(0, (used / total) * 100))
      : 0,
  );
  const chartData = $derived.by<DonutDatum[]>(() => {
    if (!available || total <= 0) {
      return [
        {
          key: "unavailable",
          label: "Unavailable",
          value: 1,
          color: "var(--muted)",
        },
      ];
    }

    return [
      {
        key: "used",
        label: "Used",
        value: Math.min(total, Math.max(0, used)),
        color: "var(--primary)",
      },
      {
        key: "remaining",
        label: "Remaining",
        value: Math.max(0, total - used),
        color: "var(--muted)",
      },
    ];
  });
</script>

<article class="border border-border bg-card/35 p-5">
  <div class="flex items-center justify-between gap-4">
    <h2 class="text-sm font-semibold">{label}</h2>
    <span
      class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
    >
      {available ? "Used / total" : "Unavailable"}
    </span>
  </div>

  <div class="mt-5 grid grid-cols-[8.5rem_1fr] items-center gap-6">
    <div class="relative size-[8.5rem]">
      <PieChart
        data={chartData}
        key="key"
        label="label"
        value="value"
        c="color"
        innerRadius={46}
        outerRadius={58}
        width={136}
        height={136}
        padding={0}
        tooltipContext={false}
        motion={false}
        aria-label={`${label}: ${available ? `${usedPercent.toFixed(0)} percent used` : "Unavailable"}`}
        props={{
          arc: { strokeWidth: 0 },
          svg: {
            title: `${label}: ${available ? `${usedPercent.toFixed(0)} percent used` : "Unavailable"}`,
          },
        }}
      />
      <div
        class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center"
      >
        {#if available}
          <span class="text-2xl font-semibold tracking-tight">
            {usedPercent.toFixed(0)}%
          </span>
          <span
            class="mt-1 text-[10px] uppercase tracking-[0.14em] text-muted-foreground"
            >used</span
          >
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
        <dd class="mt-1 text-sm font-medium">
          {available ? formatValue(used) : "Unavailable"}
        </dd>
      </div>
      <div>
        <dt class="flex items-center gap-2 text-xs text-muted-foreground">
          <span class="size-2 bg-muted"></span>
          Total
        </dt>
        <dd class="mt-1 text-sm font-medium">
          {available ? formatValue(total) : "Unavailable"}
        </dd>
      </div>
    </dl>
  </div>
</article>
