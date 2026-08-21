<script lang="ts">
  import RequestGeography from "@/Components/Applications/Environments/RequestGeography.svelte";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import type {
    CountryTelemetry,
    RouteTelemetry,
    TelemetryRange,
  } from "@/Pages/Applications/Environments/show.types";

  let {
    routes,
    countries,
    telemetryRange,
  }: {
    routes: RouteTelemetry[];
    countries: CountryTelemetry[];
    telemetryRange: TelemetryRange;
  } = $props();

  const rangeLabel = $derived(
    {
      "1h": "the last hour",
      "6h": "the last 6 hours",
      "24h": "the last 24 hours",
      "7d": "the last 7 days",
    }[telemetryRange],
  );
  const pageRoute = (route: RouteTelemetry) =>
    route.method === "GET" &&
    route.route.startsWith("/") &&
    !["/api/", "/assets/", "/dist/"].some((prefix) =>
      route.route.startsWith(prefix),
    ) &&
    !["/favicon.ico", "/robots.txt"].includes(route.route);
  const popularPages = $derived(
    routes
      .filter(pageRoute)
      .sort(
        (left, right) =>
          right.requests - left.requests ||
          left.route.localeCompare(right.route),
      )
      .slice(0, 8),
  );
  const pageRequests = $derived(
    popularPages.reduce((total, route) => total + route.requests, 0),
  );
</script>

<div class="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
  <RequestGeography {countries} />
  <Card.Root class="h-full gap-0 py-0">
    <Card.Header class="border-b border-border py-4">
      <Card.Title>Popular pages</Card.Title>
      <Card.Description>
        Literal GET paths ranked by requests across {rangeLabel}.
      </Card.Description>
    </Card.Header>
    <Card.Content class="p-4">
      {#if popularPages.length}
        <ol class="space-y-2">
          {#each popularPages as route (`${route.method}:${route.route}`)}
            <li class="relative overflow-hidden px-3 py-2">
              <div
                class="absolute inset-y-0 left-0 bg-muted"
                style:width={`${Math.max(
                  pageRequests > 0
                    ? (route.requests / pageRequests) * 100
                    : 0,
                  3,
                )}%`}
              ></div>
              <div class="relative flex items-center justify-between gap-3">
                <span class="truncate font-mono text-xs font-medium">
                  {route.route}
                </span>
                <span
                  class="shrink-0 text-xs tabular-nums text-muted-foreground"
                >
                  {route.requests.toLocaleString()}
                </span>
              </div>
            </li>
          {/each}
        </ol>
      {:else}
        <Empty.Root class="py-10">
          <Empty.Header>
            <Empty.Title>No page requests yet</Empty.Title>
            <Empty.Description>
              Literal paths appear here after gateway requests are processed.
            </Empty.Description>
          </Empty.Header>
        </Empty.Root>
      {/if}
    </Card.Content>
  </Card.Root>
</div>
