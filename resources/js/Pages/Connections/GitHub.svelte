<script lang="ts">
  import GithubIcon from "@lucide/svelte/icons/git-fork";
  import { Link, useForm } from "@inertiajs/svelte";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import FormField from "@/Components/FormField.svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Checkbox } from "@/Components/ui/checkbox";
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
    lastSyncedAt: NullableTime;
  };
  type Connection = {
    app: null | {
      name: string;
      slug: string;
      ownerLogin: string;
      ownerType: string;
      htmlUrl: string;
    };
    installations: Installation[];
    degraded: boolean;
    healthMessage: string;
  };

  let {
    auth,
    connection,
  }: { auth: { email: string }; connection: Connection } = $props();
  const setup = useForm({
    ownerType: "personal",
    ownerLogin: "",
    public: true,
  });
  const install = useForm({ ownerType: "personal", ownerLogin: "" });
  let setupDialogOpen = $state(false);
  let installDialogOpen = $state(false);
  let activeAction = $state("");
  let archiveDialogOpen = $state(false);
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

  function openInstallDialog() {
    $install.reset();
    installDialogOpen = true;
  }

  function startInstall(event: SubmitEvent) {
    event.preventDefault();
    $install.post(routes.gitHubInstall(), {
      onError: () => (installDialogOpen = true),
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

  function installAccount() {
    openInstallDialog();
  }

  function archiveApp() {
    if (activeAction) return;
    activeAction = "archive:app";
    archiveError = "";
    router.delete(routes.gitHubAppDestroy(), {
      preserveScroll: true,
      onSuccess: () => (archiveDialogOpen = false),
      onError: (errors) =>
        (archiveError =
          Object.values(errors).map(String).join("\n") ||
          "The GitHub App could not be archived."),
      onFinish: () => (activeAction = ""),
    });
  }
</script>

<svelte:head><title>GitHub connections</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Connections"
      title="GitHub"
      description="Install GitHub user and organization accounts to make repositories available to Applications."
    >
      {#snippet actions()}
        {#if connection.app}
          <Button
            type="button"
            disabled={Boolean(activeAction)}
            onclick={installAccount}>Install account</Button
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
            >Create a private GitHub App before installing user or organization
            accounts.</Empty.Description
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
          <dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <dt class="text-xs text-muted-foreground">App</dt>
              <dd class="mt-1 font-medium">{connection.app.name}</dd>
              <dd class="mt-1 font-mono text-[11px] text-muted-foreground">
                {connection.app.slug}
              </dd>
            </div>
            <div>
              <dt class="text-xs text-muted-foreground">Owner</dt>
              <dd class="mt-1">{connection.app.ownerLogin}</dd>
              <dd class="mt-1 capitalize text-[11px] text-muted-foreground">
                {connection.app.ownerType}
              </dd>
            </div>
            <div>
              <dt class="text-xs text-muted-foreground">Status</dt>
              <dd class="mt-1">
                <StatusBadge
                  status={connection.degraded ? "degraded" : "connected"}
                />
              </dd>
            </div>
            <div class="flex items-end justify-start gap-2 lg:justify-end">
              <Button size="sm" variant="outline"
                >{#snippet child({ props })}<a
                    {...props}
                    href={connection.app.htmlUrl}
                    target="_blank"
                    rel="noreferrer">Settings</a
                  >{/snippet}</Button
              ><Button
                size="sm"
                variant="destructive"
                disabled={Boolean(activeAction)}
                onclick={() => (archiveDialogOpen = true)}>Archive</Button
              >
            </div>
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header
          ><Card.Title>Installed accounts</Card.Title><Card.Description
            >{connection.installations.length} account installation{connection
              .installations.length === 1
              ? ""
              : "s"} available for repository sources.</Card.Description
          ></Card.Header
        >
        <Card.Content>
          {#if connection.installations.length === 0}
            <Empty.Root class="border border-dashed border-border py-12">
              <Empty.Header
                ><Empty.Media variant="icon"><GithubIcon /></Empty.Media
                ><Empty.Title>No GitHub accounts installed</Empty.Title
                ><Empty.Description
                  >Install a user or organization account to make its
                  repositories available to Applications.</Empty.Description
                ></Empty.Header
              >
            </Empty.Root>
          {:else}
            <div class="overflow-hidden border border-border">
              <Table.Root class="min-w-[860px]">
                <Table.Header
                  ><Table.Row
                    ><Table.Head>Account</Table.Head><Table.Head
                      >Type</Table.Head
                    ><Table.Head>Repository access</Table.Head><Table.Head
                      >Repositories</Table.Head
                    ><Table.Head>Status</Table.Head><Table.Head
                      >Last synchronized</Table.Head
                    ><Table.Head class="text-right">Actions</Table.Head
                    ></Table.Row
                  ></Table.Header
                >
                <Table.Body>
                  {#each connection.installations as installation (installation.id)}
                    <Table.Row>
                      <Table.Cell class="font-medium"
                        >{installation.accountLogin}</Table.Cell
                      >
                      <Table.Cell class="capitalize"
                        >{installation.accountType}</Table.Cell
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
                        ><div class="flex justify-end">
                          <Button size="sm" variant="outline"
                            >{#snippet child({ props })}<Link
                                {...props}
                                href={routes.gitHubInstallationShow(
                                  installation.id,
                                )}>View</Link
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
        <label class="flex items-start gap-3 border border-border p-4"
          ><Checkbox
            class="mt-0.5"
            bind:checked={$setup.public}
            disabled={$setup.processing}
          /><span
            ><span class="block text-sm font-medium"
              >Installable by any account (public)</span
            ><span class="mt-1 block text-xs text-muted-foreground"
              >Public apps can be installed on personal and organization
              accounts. Private apps can only be installed on the owner account.</span
            ></span
          ></label
        >
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

  <Dialog.Root bind:open={installDialogOpen}>
    <Dialog.Content class="sm:max-w-xl" showCloseButton={!$install.processing}>
      <form class="grid gap-5" onsubmit={startInstall}>
        <Dialog.Header
          ><Dialog.Title>Install GitHub account</Dialog.Title
          ><Dialog.Description
            >Choose which GitHub account to install the App on. Installing on an
            organization makes its repositories available to Applications.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="Account type" error={$install.errors.ownerType}>
          <NativeSelect.Root
            bind:value={$install.ownerType}
            class="w-full"
            disabled={$install.processing}
            ><NativeSelect.Option value="personal"
              >Personal account</NativeSelect.Option
            ><NativeSelect.Option value="organization"
              >Organization</NativeSelect.Option
            ></NativeSelect.Root
          >
        </FormField>
        {#if $install.ownerType === "organization"}<FormField
            label="Organization login"
            error={$install.errors.ownerLogin}
            ><Input
              bind:value={$install.ownerLogin}
              placeholder="acme"
              required
              disabled={$install.processing}
            /></FormField
          >{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$install.processing}
            onclick={() => (installDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$install.processing}
            aria-busy={$install.processing}
            >{#if $install.processing}<Spinner />{/if}Continue to GitHub</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={archiveDialogOpen}
    title={`Archive ${connection.app?.name ?? "GitHub App"}?`}
    description="This removes the GitHub App from DeployCrate. Active account installations or Application sources must be removed first."
    confirmLabel="Archive"
    destructive
    processing={activeAction === "archive:app"}
    error={archiveError}
    onconfirm={archiveApp}
  />
</DashboardLayout>
