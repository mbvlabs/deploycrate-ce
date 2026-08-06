<script lang="ts">
  import GithubIcon from "@lucide/svelte/icons/git-fork";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import TriangleAlertIcon from "@lucide/svelte/icons/triangle-alert";
  import { router, useForm } from "@inertiajs/svelte";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import FormField from "@/Components/FormField.svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type NullableTime =
    | { Time?: string; Valid?: boolean; time?: string; valid?: boolean }
    | string
    | null;
  type Installation = {
    id: string;
    accountLogin: string;
    accountType: string;
    repositorySelection: string;
    repositoryCount: number;
    suspendedAt: NullableTime;
    archivedAt: NullableTime;
    lastSyncedAt: NullableTime;
    externalId: number;
  };
  type Connection = {
    app: null | {
      name: string;
      slug: string;
      ownerLogin: string;
      ownerType: string;
      htmlUrl: string;
      permissions: Record<string, string>;
      events: string[];
      verifiedAt: NullableTime;
    };
    installations: Installation[];
    degraded: boolean;
    healthMessage: string;
  };

  let {
    auth,
    connection,
  }: { auth: { email: string }; connection: Connection } = $props();
  const setup = useForm({ ownerType: "personal", ownerLogin: "" });
  let setupDialogOpen = $state(false);
  let activeAction = $state("");
  let archiveDialogOpen = $state(false);
  let archiveTarget = $state<{
    kind: "app" | "installation";
    id?: string;
    name: string;
  } | null>(null);
  let archiveError = $state("");

  function openSetupDialog() {
    $setup.reset();
    setupDialogOpen = true;
  }

  function startSetup(event: SubmitEvent) {
    event.preventDefault();
    $setup.post(routes.gitHubAppSetup(), {
      onError: () => (setupDialogOpen = true),
    });
  }

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

  function runAction(key: string, url: string) {
    if (activeAction) return;
    activeAction = key;
    router.post(
      url,
      {},
      { preserveScroll: true, onFinish: () => (activeAction = "") },
    );
  }

  function askToArchive(target: {
    kind: "app" | "installation";
    id?: string;
    name: string;
  }) {
    archiveTarget = target;
    archiveError = "";
    archiveDialogOpen = true;
  }

  function archive() {
    if (!archiveTarget || activeAction) return;
    activeAction = `archive:${archiveTarget.id ?? "app"}`;
    archiveError = "";
    const url =
      archiveTarget.kind === "app"
        ? routes.gitHubAppDestroy()
        : routes.gitHubInstallationDestroy(archiveTarget.id ?? "");
    router.delete(url, {
      preserveScroll: true,
      onSuccess: () => {
        archiveDialogOpen = false;
        archiveTarget = null;
      },
      onError: (errors) =>
        (archiveError =
          Object.values(errors).map(String).join("\n") ||
          "The connection could not be archived."),
      onFinish: () => (activeAction = ""),
    });
  }
</script>

<svelte:head><title>GitHub connection</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Connections"
      title="GitHub"
      description="Connect one private GitHub App to discover repositories and accept signed push events."
    >
      {#snippet actions()}
        {#if connection.app}
          <Button
            type="button"
            disabled={Boolean(activeAction)}
            onclick={() => runAction("install", routes.gitHubInstall())}
            >{#if activeAction === "install"}<Spinner />{/if}Install account</Button
          >
        {:else}
          <Button type="button" onclick={openSetupDialog}>Add GitHub App</Button
          >
        {/if}
      {/snippet}
    </PageHeader>

    {#if !connection.app}
      <Empty.Root class="border border-dashed border-border py-14">
        <Empty.Header
          ><Empty.Media variant="icon"><GithubIcon /></Empty.Media><Empty.Title
            >No GitHub App</Empty.Title
          ><Empty.Description
            >Create a private GitHub App to connect repositories and receive
            signed push events.</Empty.Description
          ></Empty.Header
        >
      </Empty.Root>
    {:else}
      <Card.Root>
        <Card.Header
          ><Card.Title>GitHub App</Card.Title><Card.Description
            >{connection.healthMessage}</Card.Description
          ></Card.Header
        >
        <Card.Content>
          <div class="overflow-hidden border border-border">
            <Table.Root class="min-w-[900px]">
              <Table.Header
                ><Table.Row
                  ><Table.Head>App</Table.Head><Table.Head>Owner</Table.Head
                  ><Table.Head>Permissions</Table.Head><Table.Head
                    >Events</Table.Head
                  ><Table.Head>Verified</Table.Head><Table.Head
                    >Status</Table.Head
                  ><Table.Head class="text-right">Actions</Table.Head
                  ></Table.Row
                ></Table.Header
              >
              <Table.Body
                ><Table.Row>
                  <Table.Cell
                    ><p class="font-medium">{connection.app.name}</p>
                    <p class="mt-1 font-mono text-[11px] text-muted-foreground">
                      {connection.app.slug}
                    </p></Table.Cell
                  >
                  <Table.Cell
                    ><p>{connection.app.ownerLogin}</p>
                    <p
                      class="mt-1 capitalize text-[11px] text-muted-foreground"
                    >
                      {connection.app.ownerType}
                    </p></Table.Cell
                  >
                  <Table.Cell class="font-mono text-[11px]"
                    >contents: read<br />metadata: read</Table.Cell
                  >
                  <Table.Cell class="font-mono text-[11px]"
                    >{connection.app.events.join(", ")}</Table.Cell
                  >
                  <Table.Cell class="whitespace-nowrap"
                    >{dateLabel(connection.app.verifiedAt)}</Table.Cell
                  >
                  <Table.Cell
                    ><div class="flex items-center gap-1.5">
                      {#if connection.degraded}<TriangleAlertIcon
                          class="size-4"
                        />{/if}<StatusBadge
                        status={connection.degraded ? "degraded" : "connected"}
                      />
                    </div></Table.Cell
                  >
                  <Table.Cell
                    ><div class="flex justify-end gap-2">
                      <Button size="sm" variant="outline"
                        >{#snippet child({ props })}<a
                            {...props}
                            href={connection.app.htmlUrl}
                            target="_blank"
                            rel="noreferrer">Settings</a
                          >{/snippet}</Button
                      ><Button
                        size="sm"
                        variant="outline"
                        disabled={Boolean(activeAction)}
                        onclick={() =>
                          runAction("install", routes.gitHubInstall())}
                        >{#if activeAction === "install"}<Spinner />{/if}Install
                        account</Button
                      ><Button
                        size="sm"
                        variant="destructive"
                        disabled={Boolean(activeAction)}
                        onclick={() =>
                          askToArchive({
                            kind: "app",
                            name: connection.app?.name ?? "GitHub connection",
                          })}>Archive</Button
                      >
                    </div></Table.Cell
                  >
                </Table.Row></Table.Body
              >
            </Table.Root>
          </div>
        </Card.Content>
      </Card.Root>
    {/if}

    {#if connection.app}
      <Card.Root>
        <Card.Header
          ><Card.Title>Installed accounts</Card.Title><Card.Description
            >{connection.installations.length} account installation{connection
              .installations.length === 1
              ? ""
              : "s"} with repository grants reconciled by stable GitHub IDs.</Card.Description
          ></Card.Header
        >
        <Card.Content>
          {#if connection.installations.length === 0}
            <Empty.Root class="border border-dashed border-border py-12">
              <Empty.Header
                ><Empty.Media variant="icon"><GithubIcon /></Empty.Media
                ><Empty.Title>No GitHub accounts installed</Empty.Title
                ><Empty.Description
                  >Install an account to make its repositories available to
                  Applications.</Empty.Description
                ></Empty.Header
              >
            </Empty.Root>
          {:else}
            <div class="overflow-hidden border border-border">
              <Table.Root class="min-w-[820px]">
                <Table.Header
                  ><Table.Row
                    ><Table.Head>Account</Table.Head><Table.Head
                      >Repository access</Table.Head
                    ><Table.Head>Repositories</Table.Head><Table.Head
                      >Status</Table.Head
                    ><Table.Head>Last synchronized</Table.Head><Table.Head
                      class="text-right">Actions</Table.Head
                    ></Table.Row
                  ></Table.Header
                >
                <Table.Body>
                  {#each connection.installations as installation (installation.id)}
                    <Table.Row>
                      <Table.Cell
                        ><p class="font-medium">{installation.accountLogin}</p>
                        <p
                          class="mt-1 capitalize text-[11px] text-muted-foreground"
                        >
                          {installation.accountType}
                        </p></Table.Cell
                      >
                      <Table.Cell class="capitalize"
                        >{installation.repositorySelection}</Table.Cell
                      >
                      <Table.Cell class="tabular-nums"
                        >{installation.repositoryCount}</Table.Cell
                      >
                      <Table.Cell
                        ><StatusBadge
                          status={isPresent(installation.suspendedAt)
                            ? "suspended"
                            : "active"}
                        /></Table.Cell
                      >
                      <Table.Cell class="whitespace-nowrap"
                        >{dateLabel(installation.lastSyncedAt)}</Table.Cell
                      >
                      <Table.Cell
                        ><div class="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={Boolean(activeAction)}
                            onclick={() =>
                              runAction(
                                `sync:${installation.id}`,
                                routes.gitHubInstallationSync(installation.id),
                              )}
                            >{#if activeAction === `sync:${installation.id}`}<Spinner
                              />{:else}<RefreshCwIcon />{/if}Sync</Button
                          ><Button
                            size="sm"
                            variant="outline"
                            disabled={Boolean(activeAction)}
                            onclick={() =>
                              runAction(
                                `verify:${installation.id}`,
                                routes.gitHubInstallationVerify(
                                  installation.id,
                                ),
                              )}
                            >{#if activeAction === `verify:${installation.id}`}<Spinner
                              />{/if}Verify</Button
                          ><Button
                            size="sm"
                            variant="destructive"
                            disabled={Boolean(activeAction)}
                            onclick={() =>
                              askToArchive({
                                kind: "installation",
                                id: installation.id,
                                name: installation.accountLogin,
                              })}>Archive</Button
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
    {/if}
  </div>

  <Dialog.Root bind:open={setupDialogOpen}>
    <Dialog.Content class="sm:max-w-xl" showCloseButton={!$setup.processing}>
      <form class="grid gap-5" onsubmit={startSetup}>
        <Dialog.Header
          ><Dialog.Title>Add GitHub App</Dialog.Title><Dialog.Description
            >GitHub returns the private key and webhook secret directly to
            DeployCrate. Secret values are encrypted before persistence.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="App owner" error={$setup.errors.ownerType}>
          <NativeSelect.Root
            bind:value={$setup.ownerType}
            class="w-full"
            disabled={$setup.processing}
            ><NativeSelect.Option value="personal"
              >Personal account</NativeSelect.Option
            ><NativeSelect.Option value="organization"
              >Organization</NativeSelect.Option
            ></NativeSelect.Root
          >
        </FormField>
        {#if $setup.ownerType === "organization"}<FormField
            label="Organization login"
            error={$setup.errors.ownerLogin}
            ><Input
              bind:value={$setup.ownerLogin}
              placeholder="acme"
              required
              disabled={$setup.processing}
            /></FormField
          >{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$setup.processing}
            onclick={() => (setupDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$setup.processing}
            aria-busy={$setup.processing}
            >{#if $setup.processing}<Spinner />{/if}Continue to GitHub</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={archiveDialogOpen}
    title={`Archive ${archiveTarget?.name ?? "connection"}?`}
    description="This removes the connection from DeployCrate. Applications that depend on its repositories may no longer build."
    confirmLabel="Archive"
    destructive
    processing={activeAction.startsWith("archive:")}
    error={archiveError}
    onconfirm={archive}
  />
</DashboardLayout>
