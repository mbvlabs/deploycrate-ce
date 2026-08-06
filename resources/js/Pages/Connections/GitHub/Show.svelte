<script lang="ts">
  import ExternalLinkIcon from "@lucide/svelte/icons/external-link";
  import GithubIcon from "@lucide/svelte/icons/git-fork";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import TriangleAlertIcon from "@lucide/svelte/icons/triangle-alert";
  import { Link, router } from "@inertiajs/svelte";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type NullableTime =
    | { Time?: string; Valid?: boolean; time?: string; valid?: boolean }
    | string
    | null;
  type App = {
    name: string;
    slug: string;
    ownerLogin: string;
    ownerType: string;
    htmlUrl: string;
  };
  type Installation = {
    id: string;
    accountLogin: string;
    accountType: string;
    repositorySelection: string;
    repositoryCount: number;
    suspendedAt: NullableTime;
    lastSyncedAt: NullableTime;
    externalId: number;
  };
  type Repository = {
    id: string;
    fullName: string;
    defaultBranch: string;
    visibility: string;
    htmlUrl: string;
    lastSyncedAt: string;
  };
  type Connection = {
    app: App;
    installation: Installation;
    repositories: Repository[];
    degraded: boolean;
    healthMessage: string;
  };

  let {
    auth,
    connection,
  }: { auth: { email: string }; connection: Connection } = $props();
  let activeAction = $state("");
  let archiveDialogOpen = $state(false);
  let archiveError = $state("");

  const installation = $derived(connection.installation);
  const repositories = $derived(connection.repositories);

  function dateLabel(value: NullableTime) {
    const raw =
      typeof value === "string" ? value : (value?.Time ?? value?.time);
    if (
      !raw ||
      (typeof value !== "string" &&
        (value?.Valid === false || value?.valid === false))
    )
      return "Never";
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(raw));
  }

  function isPresent(value: NullableTime) {
    if (typeof value === "string") return value.length > 0;
    if (!value || value.Valid === false || value.valid === false) return false;
    return Boolean(value.Time ?? value.time ?? value.Valid ?? value.valid);
  }

  function installationSettingsURL() {
    const login = encodeURIComponent(installation.accountLogin);
    if (installation.accountType === "Organization") {
      return `https://github.com/organizations/${login}/settings/installations/${installation.externalId}`;
    }
    return `https://github.com/settings/installations/${installation.externalId}`;
  }

  function runAction(action: "sync" | "verify") {
    if (activeAction) return;
    activeAction = action;
    const url =
      action === "sync"
        ? routes.gitHubInstallationSync(installation.id)
        : routes.gitHubInstallationVerify(installation.id);
    router.post(
      url,
      {},
      { preserveScroll: true, onFinish: () => (activeAction = "") },
    );
  }

  function archive() {
    if (activeAction) return;
    activeAction = "archive";
    archiveError = "";
    router.delete(routes.gitHubInstallationDestroy(installation.id), {
      preserveScroll: true,
      onSuccess: () => (archiveDialogOpen = false),
      onError: (errors) =>
        (archiveError =
          Object.values(errors).map(String).join("\n") ||
          "The GitHub installation could not be archived."),
      onFinish: () => (activeAction = ""),
    });
  }
</script>

<svelte:head><title>{installation.accountLogin}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <Link
        class="text-xs text-muted-foreground hover:text-foreground"
        href={routes.gitHubConnection()}>GitHub</Link
      >
      <div class="mt-3 flex flex-wrap items-center gap-3">
        <h1 class="text-3xl font-semibold tracking-tight">
          {installation.accountLogin}
        </h1>
        <StatusBadge
          status={isPresent(installation.suspendedAt)
            ? "suspended"
            : connection.degraded
              ? "degraded"
              : "active"}
        />
      </div>
      <p class="mt-3 text-sm text-muted-foreground">
        {connection.healthMessage}
      </p>
      <div class="mt-5 flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={Boolean(activeAction)}
          onclick={() => runAction("sync")}
          >{#if activeAction === "sync"}<Spinner />{:else}<RefreshCwIcon />{/if}Sync
          repositories</Button
        >
        <Button
          type="button"
          variant="outline"
          disabled={Boolean(activeAction)}
          onclick={() => runAction("verify")}
          >{#if activeAction === "verify"}<Spinner />{/if}Verify access</Button
        >
        <Button type="button" variant="outline"
          >{#snippet child({ props })}<a
              {...props}
              href={installationSettingsURL()}
              target="_blank"
              rel="noreferrer"><ExternalLinkIcon />Manage in GitHub</a
            >{/snippet}</Button
        >
        <Button
          type="button"
          variant="destructive"
          disabled={Boolean(activeAction)}
          onclick={() => (archiveDialogOpen = true)}>Archive</Button
        >
      </div>
    </header>

    <Card.Root class="max-w-5xl">
      <Card.Header
        ><Card.Title>Connection details</Card.Title><Card.Description
          >GitHub App installation used for repository discovery and signed push
          events.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        <dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <dt class="text-xs text-muted-foreground">Account</dt>
            <dd class="mt-1 font-medium">{installation.accountLogin}</dd>
            <dd class="mt-1 capitalize text-[11px] text-muted-foreground">
              {installation.accountType}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Repository access</dt>
            <dd class="mt-1 capitalize">{installation.repositorySelection}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Repositories</dt>
            <dd class="mt-1 tabular-nums">{installation.repositoryCount}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Last synchronized</dt>
            <dd class="mt-1">{dateLabel(installation.lastSyncedAt)}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">GitHub App</dt>
            <dd class="mt-1">{connection.app.name}</dd>
            <dd class="mt-1 font-mono text-[11px] text-muted-foreground">
              {connection.app.slug}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">App owner</dt>
            <dd class="mt-1">{connection.app.ownerLogin}</dd>
            <dd class="mt-1 capitalize text-[11px] text-muted-foreground">
              {connection.app.ownerType}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">External ID</dt>
            <dd class="mt-1 font-mono text-xs">{installation.externalId}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Status</dt>
            <dd class="mt-1 flex items-center gap-1.5">
              {#if connection.degraded}<TriangleAlertIcon class="size-4" />{/if}
              <StatusBadge
                status={isPresent(installation.suspendedAt)
                  ? "suspended"
                  : connection.degraded
                    ? "degraded"
                    : "connected"}
              />
            </dd>
          </div>
        </dl>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header
        ><Card.Action
          ><Button type="button" size="sm" variant="outline"
            >{#snippet child({ props })}<a
                {...props}
                href={installationSettingsURL()}
                target="_blank"
                rel="noreferrer"><ExternalLinkIcon />Manage repositories</a
              >{/snippet}</Button
          ></Card.Action
        ><Card.Title>Installed repositories</Card.Title><Card.Description
          >Repositories currently granted to this GitHub installation.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        {#if repositories.length === 0}
          <Empty.Root class="border border-dashed border-border py-12">
            <Empty.Header
              ><Empty.Media variant="icon"><GithubIcon /></Empty.Media
              ><Empty.Title>No repositories installed</Empty.Title
              ><Empty.Description
                >Grant repositories in GitHub, then synchronize this
                installation.</Empty.Description
              ></Empty.Header
            >
          </Empty.Root>
        {:else}
          <div class="overflow-hidden border border-border">
            <Table.Root class="min-w-[860px]">
              <Table.Header
                ><Table.Row
                  ><Table.Head>Repository</Table.Head><Table.Head
                    >Default branch</Table.Head
                  ><Table.Head>Visibility</Table.Head><Table.Head
                    >Last synchronized</Table.Head
                  ><Table.Head class="text-right">Actions</Table.Head
                  ></Table.Row
                ></Table.Header
              >
              <Table.Body>
                {#each repositories as repository (repository.id)}
                  <Table.Row>
                    <Table.Cell
                      ><p class="font-medium">{repository.fullName}</p>
                      <p class="mt-1 font-mono text-[11px] text-muted-foreground">
                        {repository.id}
                      </p></Table.Cell
                    >
                    <Table.Cell class="font-mono text-xs"
                      >{repository.defaultBranch}</Table.Cell
                    >
                    <Table.Cell class="capitalize"
                      >{repository.visibility}</Table.Cell
                    >
                    <Table.Cell class="whitespace-nowrap"
                      >{dateLabel(repository.lastSyncedAt)}</Table.Cell
                    >
                    <Table.Cell
                      ><div class="flex justify-end">
                        <Button size="sm" variant="outline"
                          >{#snippet child({ props })}<a
                              {...props}
                              href={repository.htmlUrl}
                              target="_blank"
                              rel="noreferrer">Open</a
                            >{/snippet}</Button
                        >
                      </div></Table.Cell
                    >
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>

  <ConfirmActionDialog
    bind:open={archiveDialogOpen}
    title={`Archive ${installation.accountLogin}?`}
    description="This removes the installation from DeployCrate. Applications that depend on its repositories may no longer build."
    confirmLabel="Archive"
    destructive
    processing={activeAction === "archive"}
    error={archiveError}
    onconfirm={archive}
  />
</DashboardLayout>
