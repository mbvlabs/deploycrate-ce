<script lang="ts">
  import { Link } from "@inertiajs/svelte";
  import PlayIcon from "@lucide/svelte/icons/play";
  import SquareIcon from "@lucide/svelte/icons/square";
  import { onMount } from "svelte";
  import * as Alert from "@/Components/ui/alert";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import { Textarea } from "@/Components/ui/textarea";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Resource = {
    id: string;
    name: string;
    engine: string;
    resourceType: string;
    systemManaged: boolean;
  };
  type Database = {
    name: string;
    encoding: string;
    collation: string;
    credentialName: string;
    credentialUser: string;
  };
  type CatalogColumn = {
    name: string;
    dataType: string;
    nullable: boolean;
    hasDefault: boolean;
  };
  type CatalogRelation = {
    schema: string;
    name: string;
    kind: string;
    columns: CatalogColumn[];
  };
  type Catalog = { relations: CatalogRelation[]; truncated: boolean };
  type QueryResult = {
    columns: Array<{ name: string; dataType: string }>;
    rows: Array<Array<string | null>>;
    commandTag: string;
    rowCount: number;
    truncated: boolean;
    executionMs: number;
  };
  type QueryFailure = {
    message: string;
    code?: string;
    detail?: string;
    hint?: string;
    position?: number;
  };
  type QueryHistoryEntry = {
    id: string;
    sql: string;
    executedAt: string;
  };

  let {
    auth,
    resource,
    database,
    catalog,
  }: {
    auth: { email: string };
    resource: Resource;
    database: Database;
    catalog: Catalog;
  } = $props();

  let sql = $state("SELECT *\nFROM information_schema.tables\nLIMIT 100;");
  let running = $state(false);
  let result = $state.raw<QueryResult | null>(null);
  let queryError = $state.raw<QueryFailure | null>(null);
  let selectedRelation = $state("");
  let selectedHistoryID = $state("");
  let queryHistory = $state.raw<QueryHistoryEntry[]>([]);
  let queryAbortController: AbortController | undefined;
  const queryHistoryKey = $derived(
    `deploycrate:database-editor:${resource.id}:${database.name}:history`,
  );

  const schemas = $derived.by(() => {
    const grouped: Record<string, CatalogRelation[]> = {};
    for (const relation of catalog.relations) {
      grouped[relation.schema] = [
        ...(grouped[relation.schema] ?? []),
        relation,
      ];
    }
    return Object.entries(grouped).map(([name, relations]) => ({
      name,
      relations,
    }));
  });
  const displayedRows = $derived.by(() => {
    const current = result;
    if (!current) return [];
    return current.rows.map((row) =>
      row.map((value, columnIndex) => ({
        column: current.columns[columnIndex],
        value,
      })),
    );
  });

  onMount(() => {
    try {
      const stored = JSON.parse(
        window.localStorage.getItem(queryHistoryKey) ?? "[]",
      ) as unknown;
      if (!Array.isArray(stored)) return;
      queryHistory = stored
        .filter((entry): entry is QueryHistoryEntry =>
          Boolean(
            entry &&
            typeof entry === "object" &&
            "id" in entry &&
            typeof entry.id === "string" &&
            "sql" in entry &&
            typeof entry.sql === "string" &&
            "executedAt" in entry &&
            typeof entry.executedAt === "string",
          ),
        )
        .slice(0, 25);
    } catch {
      queryHistory = [];
    }
  });

  function queryURL() {
    return routes.resourceDatabaseEditorQuery(
      resource.id,
      encodeURIComponent(database.name),
    );
  }

  function queryFailure(value: unknown): QueryFailure {
    if (typeof value === "string") return { message: value };
    if (value && typeof value === "object") {
      const failure = value as Partial<QueryFailure>;
      return {
        message: failure.message || "The query could not be executed",
        code: failure.code,
        detail: failure.detail,
        hint: failure.hint,
        position: failure.position,
      };
    }
    return { message: "The query could not be executed" };
  }

  function quoteIdentifier(value: string) {
    return `"${value.replaceAll('"', '""')}"`;
  }

  function chooseRelation() {
    if (!selectedRelation) return;
    try {
      const [schema, relation] = JSON.parse(selectedRelation) as [
        string,
        string,
      ];
      sql = `SELECT *\nFROM ${quoteIdentifier(schema)}.${quoteIdentifier(relation)}\nLIMIT 100;`;
      result = null;
      queryError = null;
    } finally {
      selectedRelation = "";
    }
  }

  function chooseHistoryQuery() {
    const entry = queryHistory.find(
      (candidate) => candidate.id === selectedHistoryID,
    );
    if (entry) {
      sql = entry.sql;
      result = null;
      queryError = null;
    }
    selectedHistoryID = "";
  }

  function historyLabel(entry: QueryHistoryEntry) {
    const statement = entry.sql.replaceAll(/\s+/g, " ").trim();
    const preview =
      statement.length > 72 ? `${statement.slice(0, 72)}…` : statement;
    return `${new Date(entry.executedAt).toLocaleString()} — ${preview}`;
  }

  function rememberQuery(statement: string) {
    const normalized = statement.trim();
    const entry: QueryHistoryEntry = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
      sql: normalized,
      executedAt: new Date().toISOString(),
    };
    queryHistory = [
      entry,
      ...queryHistory.filter((candidate) => candidate.sql !== normalized),
    ].slice(0, 25);
    try {
      window.localStorage.setItem(
        queryHistoryKey,
        JSON.stringify(queryHistory),
      );
    } catch {
      // Query execution should still work when browser storage is unavailable.
    }
  }

  async function executeQuery() {
    if (running || !sql.trim()) return;
    rememberQuery(sql);
    const abortController = new AbortController();
    queryAbortController = abortController;
    running = true;
    result = null;
    queryError = null;
    try {
      const response = await window.fetch(queryURL(), {
        method: "POST",
        cache: "no-store",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ sql }),
        signal: abortController.signal,
      });
      const payload = (await response.json().catch(() => ({}))) as
        QueryResult | { error?: unknown };
      if (!response.ok) {
        queryError = queryFailure("error" in payload ? payload.error : null);
        return;
      }
      result = payload as QueryResult;
    } catch (error) {
      if (abortController.signal.aborted) return;
      queryError = queryFailure(
        error instanceof Error ? error.message : undefined,
      );
    } finally {
      if (queryAbortController === abortController) {
        queryAbortController = undefined;
        running = false;
      }
    }
  }

  function cancelQuery() {
    queryAbortController?.abort();
    queryAbortController = undefined;
    running = false;
  }

  function handleEditorKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void executeQuery();
    }
  }
</script>

<DashboardLayout email={auth.email} resourceNavigation={resource} fullWidth>
  <div class="space-y-6">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Read-only database editor
        </p>
        <h1 class="mt-3 text-3xl font-semibold">{database.name}</h1>
        <p class="mt-2 text-sm text-muted-foreground">
          {resource.name} · Connected as
          <span class="font-mono">{database.credentialUser}</span>
          via {database.credentialName}
        </p>
      </div>
      <Button variant="outline">
        {#snippet child({ props })}
          <Link {...props} href={routes.resourceDatabases(resource.id)}
            >Back to databases</Link
          >
        {/snippet}
      </Button>
    </header>

    <Alert.Root>
      <Alert.Title>PostgreSQL-enforced read-only mode</Alert.Title>
      <Alert.Description>
        Every statement runs in a read-only transaction with execution and
        result limits. Changes are rejected by PostgreSQL and never committed.
      </Alert.Description>
    </Alert.Root>

    <div class="min-w-0 space-y-4">
      <Card.Root>
        <Card.Header>
          <Card.Action>
            {#if running}
              <Button size="sm" variant="outline" onclick={cancelQuery}>
                <SquareIcon /> Cancel
              </Button>
            {:else}
              <Button size="sm" onclick={executeQuery} disabled={!sql.trim()}>
                <PlayIcon /> Run
              </Button>
            {/if}
          </Card.Action>
          <Card.Title>SQL</Card.Title>
          <Card.Description>
            Run one statement with Ctrl/⌘ + Enter.
          </Card.Description>
        </Card.Header>
        <Card.Content class="space-y-4">
          <div class="grid gap-3 lg:grid-cols-2">
            <label class="grid gap-1.5 text-xs font-medium">
              Schema relation
              <NativeSelect.Root
                bind:value={selectedRelation}
                onchange={chooseRelation}
                class="w-full"
                aria-label="Choose a schema relation"
              >
                <NativeSelect.Option value=""
                  >Choose a table or view…</NativeSelect.Option
                >
                {#each schemas as schema (schema.name)}
                  <NativeSelect.OptGroup label={schema.name}>
                    {#each schema.relations as relation (`${schema.name}.${relation.name}`)}
                      <NativeSelect.Option
                        value={JSON.stringify([schema.name, relation.name])}
                      >
                        {relation.name} · {relation.kind} · {relation.columns
                          .length} columns
                      </NativeSelect.Option>
                    {/each}
                  </NativeSelect.OptGroup>
                {/each}
              </NativeSelect.Root>
            </label>
            <label class="grid gap-1.5 text-xs font-medium">
              Recent queries
              <NativeSelect.Root
                bind:value={selectedHistoryID}
                onchange={chooseHistoryQuery}
                class="w-full"
                disabled={queryHistory.length === 0}
                aria-label="Choose a recent query"
              >
                <NativeSelect.Option value="">
                  {queryHistory.length === 0
                    ? "No queries saved in this browser"
                    : "Choose a recent query…"}
                </NativeSelect.Option>
                {#each queryHistory as entry (entry.id)}
                  <NativeSelect.Option value={entry.id}>
                    {historyLabel(entry)}
                  </NativeSelect.Option>
                {/each}
              </NativeSelect.Root>
            </label>
          </div>
          {#if catalog.truncated}
            <p class="text-xs text-muted-foreground">
              The relation picker is limited to the first 5,000 visible columns.
            </p>
          {/if}
          <Textarea
            bind:value={sql}
            onkeydown={handleEditorKeydown}
            spellcheck="false"
            class="min-h-48 resize-y font-mono text-xs leading-5"
            aria-label="SQL statement"
          />
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Result</Card.Title>
          <Card.Description>
            {#if running}
              Executing query…
            {:else if queryError}
              The query returned an error.
            {:else if result}
              {result.rowCount}
              {result.rowCount === 1 ? "row" : "rows"} ·
              {result.executionMs} ms{result.commandTag
                ? ` · ${result.commandTag}`
                : ""}
            {:else}
              Query results will appear here.
            {/if}
          </Card.Description>
        </Card.Header>
        {#if running}
          <Card.Content class="flex items-center gap-2 py-6 text-sm">
            <Spinner /> Executing query…
          </Card.Content>
        {:else if queryError}
          <Card.Content>
            <Alert.Root variant="destructive">
              <Alert.Title>
                {queryError.code
                  ? `${queryError.code}: `
                  : ""}{queryError.message}
              </Alert.Title>
              {#if queryError.detail || queryError.hint || queryError.position}
                <Alert.Description>
                  {queryError.detail ||
                    queryError.hint ||
                    `Error near character ${queryError.position}`}
                </Alert.Description>
              {/if}
            </Alert.Root>
          </Card.Content>
        {:else if result}
          <Card.Content>
            {#if result.columns.length === 0}
              <p class="text-sm text-muted-foreground">
                The statement returned no columns.
              </p>
            {:else}
              <div class="max-h-[34rem] overflow-auto border border-border">
                <Table.Root>
                  <Table.Header class="sticky top-0 z-10 bg-background">
                    <Table.Row>
                      {#each result.columns as column (column)}
                        <Table.Head>
                          <span class="block font-mono">{column.name}</span>
                          <span
                            class="block text-[10px] font-normal text-muted-foreground"
                            >{column.dataType}</span
                          >
                        </Table.Head>
                      {/each}
                    </Table.Row>
                  </Table.Header>
                  <Table.Body>
                    {#each displayedRows as row (row)}
                      <Table.Row>
                        {#each row as cell (cell.column)}
                          <Table.Cell
                            class="max-w-96 overflow-hidden text-ellipsis font-mono text-xs"
                            title={cell.value ?? "NULL"}
                          >
                            {#if cell.value === null}
                              <span class="italic text-muted-foreground"
                                >NULL</span
                              >
                            {:else}
                              {cell.value}
                            {/if}
                          </Table.Cell>
                        {/each}
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              </div>
            {/if}
            {#if result.truncated}
              <p class="mt-3 text-xs text-muted-foreground">
                Results were truncated at 500 rows or 2 MB.
              </p>
            {/if}
          </Card.Content>
        {:else}
          <Card.Content
            class="flex min-h-48 items-center justify-center border-t border-dashed border-border text-sm text-muted-foreground"
          >
            Run a query to see its result.
          </Card.Content>
        {/if}
      </Card.Root>
    </div>
  </div>
</DashboardLayout>
