<script lang="ts">
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import { Link, router } from "@inertiajs/svelte";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import FormField from "@/Components/FormField.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
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
  type Zone = { id: string; name: string; status: string };

  let {
    auth,
    connection,
    zones,
  }: { auth: { email: string }; connection: Connection; zones: Zone[] } =
    $props();
  let activeAction = $state("");
  let rotateOpen = $state(false);
  let rotateToken = $state("");
  let rotateError = $state("");
  let archiveOpen = $state(false);
  let archiveError = $state("");

  function sync() {
    activeAction = "sync";
    router.post(
      routes.dnsConnectionSync(connection.id),
      {},
      { preserveScroll: true, onFinish: () => (activeAction = "") },
    );
  }

  function openRotate() {
    rotateToken = "";
    rotateError = "";
    rotateOpen = true;
  }

  function rotate(event: SubmitEvent) {
    event.preventDefault();
    if (!rotateToken.trim() || activeAction) return;
    activeAction = "rotate";
    rotateError = "";
    router.patch(
      routes.dnsConnectionTokenUpdate(connection.id),
      { token: rotateToken },
      {
        preserveScroll: true,
        onSuccess: () => {
          rotateOpen = false;
          rotateToken = "";
        },
        onError: (errors) =>
          (rotateError =
            Object.values(errors).map(String).join("\n") ||
            "The API token could not be rotated."),
        onFinish: () => (activeAction = ""),
      },
    );
  }

  function askArchive() {
    archiveError = "";
    archiveOpen = true;
  }

  function archive() {
    if (activeAction) return;
    activeAction = "archive";
    router.delete(routes.dnsConnectionDestroy(connection.id), {
      onError: (errors) =>
        (archiveError = Object.values(errors).map(String).join("\n")),
      onFinish: () => (activeAction = ""),
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

<svelte:head><title>{connection.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <Link
        class="text-xs text-muted-foreground hover:text-foreground"
        href={routes.dnsConnections()}>DNS</Link
      >
      <div class="mt-3 flex flex-wrap items-center gap-3">
        <h1 class="text-3xl font-semibold tracking-tight">
          {connection.name}
        </h1>
        <StatusBadge status="connected" />
      </div>
      <p class="mt-3 font-mono text-sm text-muted-foreground">
        {connection.accountId}
      </p>
    </header>

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Action
          ><div class="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={Boolean(activeAction)}
              onclick={sync}
              >{#if activeAction === "sync"}<Spinner />{:else}<RefreshCwIcon
                />{/if}Sync zones</Button
            ><Button
              size="sm"
              variant="outline"
              disabled={Boolean(activeAction)}
              onclick={openRotate}>Rotate token</Button
            >
          </div></Card.Action
        ><Card.Title>Connection details</Card.Title><Card.Description
          >Cloudflare account-owned API token connection used for Environment A
          records.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        <dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <dt class="text-xs text-muted-foreground">Cloudflare account</dt>
            <dd class="mt-1 break-all font-mono text-xs">
              {connection.accountId}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Verified</dt>
            <dd class="mt-1">{dateLabel(connection.verifiedAt)}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Last synchronized</dt>
            <dd class="mt-1">{dateLabel(connection.lastSyncedAt)}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Active zones</dt>
            <dd class="mt-1">{connection.activeZones}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Managed domains</dt>
            <dd class="mt-1">{connection.bindingCount}</dd>
          </div>
        </dl>
      </Card.Content>
    </Card.Root>

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Title>Zones</Card.Title><Card.Description
          >{zones.length} zone{zones.length === 1 ? "" : "s"} cached from Cloudflare.
          Sync to refresh the list.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        {#if zones.length === 0}
          <Empty.Root class="border border-dashed border-border py-8"
            ><Empty.Header
              ><Empty.Title>No zones</Empty.Title><Empty.Description
                >Sync the connection to cache the zones exposed by the API
                token.</Empty.Description
              ></Empty.Header
            ></Empty.Root
          >
        {:else}
          <div class="overflow-x-auto border border-border">
            <Table.Root>
              <Table.Header
                ><Table.Row
                  ><Table.Head>Zone</Table.Head><Table.Head>Status</Table.Head
                  ></Table.Row
                ></Table.Header
              >
              <Table.Body>
                {#each zones as zone (zone.id)}
                  <Table.Row>
                    <Table.Cell class="font-medium">{zone.name}</Table.Cell>
                    <Table.Cell><StatusBadge status={zone.status} /></Table.Cell
                    >
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Title>Archive connection</Card.Title><Card.Description
          >The connection will no longer be available for Environment domains.
          Move managed domains to another connection or manual DNS first.</Card.Description
        ></Card.Header
      >
      <Card.Footer class="border-t border-border"
        ><Button
          variant="destructive"
          disabled={Boolean(activeAction)}
          onclick={askArchive}>Archive connection</Button
        ></Card.Footer
      >
    </Card.Root>
  </div>

  <Dialog.Root bind:open={rotateOpen}>
    <Dialog.Content showCloseButton={activeAction !== "rotate"}>
      <form class="grid gap-5" onsubmit={rotate}>
        <Dialog.Header
          ><Dialog.Title>Rotate API token</Dialog.Title><Dialog.Description
            >Replace the account-owned API token for {connection.name}. The new
            token is verified before it replaces the current credential.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="New account-owned API token"
          ><Input
            type="password"
            bind:value={rotateToken}
            autocomplete="new-password"
            required
            disabled={activeAction === "rotate"}
          /></FormField
        >
        {#if rotateError}<p
            class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
            role="alert"
          >
            {rotateError}
          </p>{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={activeAction === "rotate"}
            onclick={() => (rotateOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={!rotateToken.trim() || activeAction === "rotate"}
            >{#if activeAction === "rotate"}<Spinner />{/if}Rotate token</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={archiveOpen}
    title={`Archive ${connection.name}?`}
    description="Move every managed Environment domain to another connection or manual DNS first."
    confirmLabel="Archive"
    destructive
    processing={activeAction === "archive"}
    error={archiveError}
    onconfirm={archive}
  />
</DashboardLayout>
