<script lang="ts">
  import { Link, router } from "@inertiajs/svelte";

  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  const jobStates = [
    "available",
    "running",
    "scheduled",
    "retryable",
    "pending",
    "completed",
    "discarded",
    "cancelled",
  ] as const;
  type JobState = (typeof jobStates)[number];
  type JobSummary = {
    id: number;
    state: JobState;
    attempt: number;
    maxAttempts: number;
    attemptedAt: string | null;
    createdAt: string;
    finalizedAt: string | null;
    scheduledAt: string;
    priority: number;
    kind: string;
    queue: string;
  };
  type Pagination = {
    page: number;
    pageSize: number;
    totalCount: number;
    totalPages: number;
  };
  type Filters = { state: string; search: string };
  type Stats = { total: number; byState: Record<JobState, number> };

  let {
    auth,
    items,
    pagination,
    filters,
    stats,
  }: {
    auth: { email: string };
    items: JobSummary[];
    pagination: Pagination;
    filters: Filters;
    stats: Stats;
  } = $props();

  const activeCount = $derived(
    stats.byState.available +
      stats.byState.running +
      stats.byState.scheduled +
      stats.byState.retryable +
      stats.byState.pending,
  );
  const attentionCount = $derived(
    stats.byState.discarded + stats.byState.cancelled,
  );
  const firstItem = $derived(
    pagination.totalCount === 0
      ? 0
      : (pagination.page - 1) * pagination.pageSize + 1,
  );
  const lastItem = $derived(
    Math.min(pagination.page * pagination.pageSize, pagination.totalCount),
  );
  const hasFilters = $derived(Boolean(filters.search || filters.state));
  let refreshing = $state(false);

  function timestamp(value: string | null) {
    if (!value) return "Not yet";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  function pageHref(page: number) {
    const params = new URLSearchParams({ page: String(page) });
    if (filters.search) params.set("search", filters.search);
    if (filters.state) params.set("state", filters.state);
    return `${routes.systemTasks()}?${params.toString()}`;
  }

  function applyFilters(event: SubmitEvent) {
    event.preventDefault();
    const form = new FormData(event.currentTarget as HTMLFormElement);
    const search = String(form.get("search") ?? "").trim();
    const state = String(form.get("state") ?? "");
    router.get(
      routes.systemTasks(),
      { search: search || undefined, state: state || undefined },
      { replace: true },
    );
  }

  function refresh() {
    refreshing = true;
    router.reload({
      only: ["items", "pagination", "stats"],
      onFinish: () => (refreshing = false),
    });
  }

  $effect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        router.reload({
          only: ["items", "pagination", "stats"],
          preserveScroll: true,
        });
      }
    }, 4000);
    return () => window.clearInterval(timer);
  });
</script>

<svelte:head><title>System Tasks</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header
      class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
    >
      <div class="max-w-3xl">
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          System
        </p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">System Tasks</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Inspect River queue execution, including scheduled work, active tasks,
          retries, and failures.
        </p>
      </div>
      <Button
        variant="outline"
        disabled={refreshing}
        aria-busy={refreshing}
        onclick={refresh}
        >{#if refreshing}<Spinner />{/if}Refresh</Button
      >
    </header>

    <section
      aria-label="Queue totals"
      class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
    >
      {#each [{ label: "All tasks", value: stats.total, description: "Retained River records" }, { label: "Active", value: activeCount, description: "Queued, scheduled, or running" }, { label: "Running", value: stats.byState.running, description: "Currently being worked" }, { label: "Needs attention", value: attentionCount, description: "Discarded or cancelled" }] as stat (stat.label)}
        <Card.Root>
          <Card.Header
            ><Card.Description>{stat.label}</Card.Description></Card.Header
          >
          <Card.Content
            ><p class="text-4xl font-semibold tabular-nums">{stat.value}</p>
            <p class="mt-2 text-xs text-muted-foreground">
              {stat.description}
            </p></Card.Content
          >
        </Card.Root>
      {/each}
    </section>

    <Card.Root>
      <Card.Header>
        <Card.Title>Queue</Card.Title>
        <Card.Description>
          {#if pagination.totalCount === 0}
            {hasFilters
              ? "No tasks match these filters."
              : "No System Tasks have been enqueued yet."}
          {:else}
            Showing {firstItem}-{lastItem} of {pagination.totalCount} matching tasks.
            Updates every four seconds.
          {/if}
        </Card.Description>
      </Card.Header>
      <Card.Content class="space-y-5">
        <form
          class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem_auto_auto]"
          onsubmit={applyFilters}
        >
          <Input
            name="search"
            value={filters.search}
            placeholder="Search by ID, kind, or queue"
            aria-label="Search System Tasks"
          />
          <NativeSelect.Root
            class="w-full"
            name="state"
            value={filters.state}
            aria-label="Filter by state"
          >
            <NativeSelect.Option value="">All states</NativeSelect.Option>
            {#each jobStates as state}<NativeSelect.Option value={state}
                >{state}</NativeSelect.Option
              >{/each}
          </NativeSelect.Root>
          <Button type="submit">Filter</Button>
          {#if hasFilters}<Button href={routes.systemTasks()} variant="ghost"
              >Clear</Button
            >{/if}
        </form>

        {#if items.length}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[760px]">
              <Table.Header>
                <Table.Row
                  ><Table.Head>Task</Table.Head><Table.Head>State</Table.Head
                  ><Table.Head>Queue</Table.Head><Table.Head
                    >Attempts</Table.Head
                  ><Table.Head>Last activity</Table.Head></Table.Row
                >
              </Table.Header>
              <Table.Body>
                {#each items as item (item.id)}
                  <Table.Row>
                    <Table.Cell
                      ><Link
                        class="font-medium text-primary hover:underline"
                        href={routes.systemTask(item.id)}>{item.kind}</Link
                      >
                      <p
                        class="mt-1 font-mono text-[11px] text-muted-foreground"
                      >
                        #{item.id}
                      </p></Table.Cell
                    >
                    <Table.Cell><StatusBadge status={item.state} /></Table.Cell>
                    <Table.Cell class="font-mono text-xs"
                      >{item.queue}</Table.Cell
                    >
                    <Table.Cell class="tabular-nums"
                      >{item.attempt} / {item.maxAttempts}</Table.Cell
                    >
                    <Table.Cell class="text-muted-foreground"
                      >{timestamp(
                        item.finalizedAt ?? item.attemptedAt ?? item.createdAt,
                      )}</Table.Cell
                    >
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {:else}
          <Empty.Root class="border border-dashed border-border py-10">
            <Empty.Header
              ><Empty.Title>No tasks to display</Empty.Title><Empty.Description
                >{hasFilters
                  ? "Try changing or clearing the filters."
                  : "Tasks will appear here when background work is enqueued."}</Empty.Description
              ></Empty.Header
            >
          </Empty.Root>
        {/if}

        {#if pagination.totalPages > 1}
          <div class="flex items-center justify-between gap-3">
            <p class="text-xs text-muted-foreground">
              Page {pagination.page} of {pagination.totalPages}
            </p>
            <div class="flex gap-2">
              <Button
                href={pageHref(pagination.page - 1)}
                variant="outline"
                disabled={pagination.page <= 1}>Previous</Button
              >
              <Button
                href={pageHref(pagination.page + 1)}
                variant="outline"
                disabled={pagination.page >= pagination.totalPages}>Next</Button
              >
            </div>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
