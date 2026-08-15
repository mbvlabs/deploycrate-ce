<script lang="ts">
  import CloudIcon from "@lucide/svelte/icons/cloud";
  import { router, useForm } from "@inertiajs/svelte";

  import FormField from "@/Components/FormField.svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type NullableTime =
    | { Time?: string; Valid?: boolean; time?: string; valid?: boolean }
    | string
    | null;
  type Connection = {
    id: string;
    name: string;
    provider: string;
    accountId: string;
    verifiedAt: NullableTime;
    lastSyncedAt: NullableTime;
    activeZones: number;
    bindingCount: number;
  };

  let {
    auth,
    connections,
  }: { auth: { email: string }; connections: Connection[] } = $props();
  const createForm = useForm({ name: "", accountId: "", token: "" });
  let createDialogOpen = $state(false);

  function openCreateDialog() {
    $createForm.reset();
    createDialogOpen = true;
  }

  function createConnection(event: SubmitEvent) {
    event.preventDefault();
    $createForm.post(routes.dnsConnectionCreate(), {
      preserveScroll: true,
      onSuccess: () => {
        createDialogOpen = false;
        $createForm.reset();
      },
      onError: () => (createDialogOpen = true),
    });
  }

  function dateLabel(value: NullableTime) {
    const raw =
      typeof value === "string" ? value : (value?.Time ?? value?.time);
    if (!raw) return "Never";
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(raw));
  }
</script>

<svelte:head><title>DNS connections</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Connections"
      title="DNS"
      description="Connect named Cloudflare accounts with durable account-owned tokens and synchronize the zones available to Environments."
    >
      {#snippet actions()}<Button type="button" onclick={openCreateDialog}
          >Add DNS connection</Button
        >{/snippet}
    </PageHeader>

    {#if connections.length === 0}
      <Empty.Root class="border border-dashed border-border py-14">
        <Empty.Header
          ><Empty.Media variant="icon"><CloudIcon /></Empty.Media><Empty.Title
            >No DNS connections</Empty.Title
          ><Empty.Description
            >Add Cloudflare to automate Environment A records.</Empty.Description
          ></Empty.Header
        >
      </Empty.Root>
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Connection</Table.Head><Table.Head>Usage</Table.Head
              ><Table.Head>Last synchronized</Table.Head></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each connections as connection (connection.id)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.dnsConnectionShow(connection.id))}
              >
                <Table.Cell class="font-medium">{connection.name}</Table.Cell>
                <Table.Cell
                  >{connection.activeZones} zone{connection.activeZones === 1
                    ? ""
                    : "s"} · {connection.bindingCount} domain{connection.bindingCount ===
                  1
                    ? ""
                    : "s"}</Table.Cell
                >
                <Table.Cell class="whitespace-nowrap"
                  >{dateLabel(connection.lastSyncedAt)}</Table.Cell
                >
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {/if}
  </div>

  <Dialog.Root bind:open={createDialogOpen}>
    <Dialog.Content
      class="sm:max-w-xl"
      showCloseButton={!$createForm.processing}
    >
      <form class="grid gap-5" onsubmit={createConnection}>
        <Dialog.Header
          ><Dialog.Title>Add Cloudflare connection</Dialog.Title
          ><Dialog.Description
            >Use an account-owned API token with Zone Read and DNS Write access.
            The token is encrypted and never displayed again.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="Connection name" error={$createForm.errors.name}
          ><Input
            bind:value={$createForm.name}
            placeholder="Production Cloudflare"
            required
            disabled={$createForm.processing}
          /></FormField
        >
        <FormField
          label="Cloudflare Account ID"
          error={$createForm.errors.accountId}
          ><Input
            bind:value={$createForm.accountId}
            minlength={32}
            maxlength={32}
            placeholder="023e105f4ecef8ad9ca31a8372d0c353"
            autocomplete="off"
            required
            disabled={$createForm.processing}
          />
          <p class="mt-2 text-xs text-muted-foreground">
            Find this on the Cloudflare account overview page.
          </p></FormField
        >
        <FormField
          label="Account-owned API token"
          error={$createForm.errors.token}
          ><Input
            type="password"
            bind:value={$createForm.token}
            autocomplete="new-password"
            placeholder="cfat_..."
            required
            disabled={$createForm.processing}
          /></FormField
        >
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$createForm.processing}
            onclick={() => (createDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$createForm.processing}
            aria-busy={$createForm.processing}
            >{#if $createForm.processing}<Spinner />{/if}Connect Cloudflare</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
