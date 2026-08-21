<script lang="ts">
  import { geoNaturalEarth1, geoPath } from "d3-geo";
  import iso from "iso-3166-1";
  import { feature } from "topojson-client";
  import type { GeometryCollection, Topology } from "topojson-specification";
  import world from "world-atlas/countries-110m.json";
  import * as Card from "@/Components/ui/card";
  import type { CountryTelemetry } from "@/Pages/Applications/Environments/show.types";

  const topology = world as unknown as Topology;
  const worldCountries = feature(
    topology,
    topology.objects.countries as GeometryCollection<{ name: string }>,
  );
  const path = geoPath(
    geoNaturalEarth1().fitExtent(
      [
        [8, 8],
        [992, 492],
      ],
      worldCountries,
    ),
  );

  let { countries }: { countries: CountryTelemetry[] } = $props();

  const maximum = $derived(
    Math.max(...countries.map((country) => country.requests), 1),
  );
  const total = $derived(
    countries.reduce((sum, country) => sum + country.requests, 0),
  );
  const byCode = $derived(
    new Map(countries.map((country) => [country.code.toUpperCase(), country])),
  );
  const countryFor = (id: string | number | undefined) => {
    const code = iso.whereNumeric(String(id))?.alpha2;
    return code ? byCode.get(code) : undefined;
  };
  const countryKey = (country: (typeof worldCountries.features)[number]) =>
    country.id === undefined
      ? `name:${country.properties?.name ?? "unknown"}`
      : `id:${country.id}`;
  const countryName = (country: CountryTelemetry) =>
    iso.whereAlpha2(country.code)?.country ?? country.code;
</script>

<Card.Root class="h-full gap-0 py-0">
  <Card.Header class="border-b border-border py-4">
    <Card.Title>Request geography</Card.Title>
    <Card.Description>
      Gateway access logs resolved by country. Addresses that cannot be
      resolved are omitted.
    </Card.Description>
  </Card.Header>
  <Card.Content class="grid gap-4 p-4 lg:grid-cols-[minmax(0,2fr)_15rem]">
    <div class="relative min-h-56 overflow-hidden bg-muted/20">
      <svg
        viewBox="0 0 1000 500"
        class="h-full w-full"
        role="img"
        aria-label="Requests by country"
      >
        {#each worldCountries.features as country (countryKey(country))}
          {@const telemetry = countryFor(country.id)}
          {@const requests = telemetry?.requests ?? 0}
          <path
            d={path(country) ?? ""}
            class={requests
              ? "fill-primary stroke-background"
              : "fill-muted-foreground/15 stroke-background"}
            style:fill-opacity={requests ? 0.3 + (requests / maximum) * 0.7 : 1}
            stroke-width="0.8"
          >
            <title
              >{telemetry
                ? countryName(telemetry)
                : (country.properties?.name ?? "Unknown")}:
              {requests.toLocaleString()} requests</title
            >
          </path>
        {/each}
      </svg>
      {#if countries.length === 0}
        <div
          class="absolute inset-0 grid place-items-center bg-background/70 p-6"
        >
          <div class="max-w-sm text-center">
            <p class="text-sm font-medium">No country data yet</p>
            <p class="mt-1 text-xs text-muted-foreground">
              Geography appears after gateway requests are collected and
              resolved.
            </p>
          </div>
        </div>
      {/if}
    </div>

    <div>
      <div
        class="mb-2 flex items-center justify-between text-xs text-muted-foreground"
      >
        <span>Country</span><span>Requests</span>
      </div>
      <ol class="space-y-1">
        {#each countries.slice(0, 8) as country (country.code)}
          <li
            class="relative flex h-8 items-center justify-between gap-3 overflow-hidden px-2 text-sm"
          >
            <div
              class="absolute inset-y-0 left-0 bg-muted"
              style:width={`${Math.max((country.requests / maximum) * 74, 3)}%`}
            ></div>
            <span class="relative truncate font-medium"
              >{countryName(country)}</span
            >
            <span
              class="relative shrink-0 text-xs tabular-nums text-muted-foreground"
              >{country.requests.toLocaleString()}</span
            >
          </li>
        {/each}
      </ol>
      {#if total > 0}
        <p class="mt-3 text-xs text-muted-foreground">
          {total.toLocaleString()} geolocated requests
        </p>
      {/if}
    </div>
  </Card.Content>
</Card.Root>
