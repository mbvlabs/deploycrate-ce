<script lang="ts">
  import CloudIcon from '@lucide/svelte/icons/cloud'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import { router, useForm } from '@inertiajs/svelte'

  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import PageHeader from '@/Components/PageHeader.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import * as Empty from '@/Components/ui/empty'
  import { Input } from '@/Components/ui/input'
  import { Spinner } from '@/Components/ui/spinner'
  import * as Table from '@/Components/ui/table'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type NullableTime = { Time?: string; Valid?: boolean; time?: string; valid?: boolean } | string | null
  type Connection = {
    id: string
    name: string
    provider: string
    accountId: string
    verifiedAt: NullableTime
    lastSyncedAt: NullableTime
    activeZones: number
    bindingCount: number
  }

  let { auth, connections }: { auth: { email: string }; connections: Connection[] } = $props()
  const createForm = useForm({ name: '', accountId: '', token: '' })
  let createDialogOpen = $state(false)
  let activeAction = $state('')
  let rotateOpen = $state(false)
  let rotateTarget = $state<Connection | null>(null)
  let rotateToken = $state('')
  let rotateError = $state('')
  let archiveOpen = $state(false)
  let archiveTarget = $state<Connection | null>(null)
  let archiveError = $state('')

  function openCreateDialog() {
    $createForm.reset()
    createDialogOpen = true
  }

  function createConnection(event: SubmitEvent) {
    event.preventDefault()
    $createForm.post(routes.dnsConnectionCreate(), {
      preserveScroll: true,
      onSuccess: () => { createDialogOpen = false; $createForm.reset() },
      onError: () => (createDialogOpen = true),
    })
  }

  function sync(connection: Connection) {
    activeAction = `sync:${connection.id}`
    router.post(routes.dnsConnectionSync(connection.id), {}, { preserveScroll: true, onFinish: () => activeAction = '' })
  }

  function openRotate(connection: Connection) {
    rotateTarget = connection
    rotateToken = ''
    rotateError = ''
    rotateOpen = true
  }

  function rotate(event: SubmitEvent) {
    event.preventDefault()
    if (!rotateTarget || !rotateToken.trim() || activeAction) return
    activeAction = `rotate:${rotateTarget.id}`
    rotateError = ''
    router.patch(routes.dnsConnectionTokenUpdate(rotateTarget.id), { token: rotateToken }, {
      preserveScroll: true,
      onSuccess: () => { rotateOpen = false; rotateTarget = null; rotateToken = '' },
      onError: (errors) => rotateError = Object.values(errors).map(String).join('\n') || 'The API token could not be rotated.',
      onFinish: () => activeAction = '',
    })
  }

  function askArchive(connection: Connection) {
    archiveTarget = connection
    archiveError = ''
    archiveOpen = true
  }

  function archive() {
    if (!archiveTarget || activeAction) return
    activeAction = `archive:${archiveTarget.id}`
    router.delete(routes.dnsConnectionDestroy(archiveTarget.id), {
      preserveScroll: true,
      onSuccess: () => { archiveOpen = false; archiveTarget = null },
      onError: (errors) => archiveError = Object.values(errors).map(String).join('\n'),
      onFinish: () => activeAction = '',
    })
  }

  function dateLabel(value: NullableTime) {
    const raw = typeof value === 'string' ? value : value?.Time ?? value?.time
    if (!raw) return 'Never'
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(raw))
  }
</script>

<svelte:head><title>DNS connections</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader eyebrow="Connections" title="DNS" description="Connect named Cloudflare accounts with durable account-owned tokens and synchronize the zones available to Environments.">
      {#snippet actions()}<Button type="button" onclick={openCreateDialog}>Add DNS connection</Button>{/snippet}
    </PageHeader>

    <Card.Root>
      <Card.Header><Card.Title>Cloudflare accounts</Card.Title><Card.Description>{connections.length} connected account{connections.length === 1 ? '' : 's'}. Zones are cached locally and refreshed explicitly.</Card.Description></Card.Header>
      <Card.Content>
        {#if connections.length === 0}
          <Empty.Root class="border border-dashed border-border py-12">
            <Empty.Header><Empty.Media variant="icon"><CloudIcon /></Empty.Media><Empty.Title>No DNS connections</Empty.Title><Empty.Description>Add Cloudflare to automate Environment A records.</Empty.Description></Empty.Header>
          </Empty.Root>
        {:else}
          <div class="overflow-hidden border border-border">
            <Table.Root class="min-w-[980px]">
              <Table.Header class="bg-muted/30"><Table.Row><Table.Head>Connection</Table.Head><Table.Head>Cloudflare account</Table.Head><Table.Head>Usage</Table.Head><Table.Head>Verified</Table.Head><Table.Head>Last synchronized</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each connections as connection (connection.id)}
                  <Table.Row>
                    <Table.Cell><p class="font-medium">{connection.name}</p><div class="mt-1"><StatusBadge status="connected" /></div></Table.Cell>
                    <Table.Cell class="font-mono text-xs">{connection.accountId}</Table.Cell>
                    <Table.Cell><p>{connection.activeZones} active zone{connection.activeZones === 1 ? '' : 's'}</p><p class="mt-1 text-[11px] text-muted-foreground">{connection.bindingCount} managed domain{connection.bindingCount === 1 ? '' : 's'}</p></Table.Cell>
                    <Table.Cell class="whitespace-nowrap">{dateLabel(connection.verifiedAt)}</Table.Cell>
                    <Table.Cell class="whitespace-nowrap">{dateLabel(connection.lastSyncedAt)}</Table.Cell>
                    <Table.Cell><div class="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={Boolean(activeAction)} onclick={() => sync(connection)}>{#if activeAction === `sync:${connection.id}`}<Spinner />{:else}<RefreshCwIcon />{/if}Sync</Button><Button size="sm" variant="outline" disabled={Boolean(activeAction)} onclick={() => openRotate(connection)}>Rotate token</Button><Button size="sm" variant="destructive" disabled={Boolean(activeAction)} onclick={() => askArchive(connection)}>Archive</Button></div></Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>

  <Dialog.Root bind:open={createDialogOpen}>
    <Dialog.Content class="sm:max-w-xl" showCloseButton={!$createForm.processing}>
      <form class="grid gap-5" onsubmit={createConnection}>
        <Dialog.Header><Dialog.Title>Add Cloudflare connection</Dialog.Title><Dialog.Description>Use an account-owned API token with Zone Read and DNS Write access. The token is encrypted and never displayed again.</Dialog.Description></Dialog.Header>
        <FormField label="Connection name" error={$createForm.errors.name}><Input bind:value={$createForm.name} placeholder="Production Cloudflare" required disabled={$createForm.processing} /></FormField>
        <FormField label="Cloudflare Account ID" error={$createForm.errors.accountId}><Input bind:value={$createForm.accountId} minlength="32" maxlength="32" placeholder="023e105f4ecef8ad9ca31a8372d0c353" autocomplete="off" required disabled={$createForm.processing} /><p class="mt-2 text-xs text-muted-foreground">Find this on the Cloudflare account overview page.</p></FormField>
        <FormField label="Account-owned API token" error={$createForm.errors.token}><Input type="password" bind:value={$createForm.token} autocomplete="new-password" placeholder="cfat_..." required disabled={$createForm.processing} /></FormField>
        <Dialog.Footer><Button type="button" variant="outline" disabled={$createForm.processing} onclick={() => (createDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={$createForm.processing} aria-busy={$createForm.processing}>{#if $createForm.processing}<Spinner />{/if}Connect Cloudflare</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={rotateOpen}>
    <Dialog.Content showCloseButton={!activeAction.startsWith('rotate:')}>
      <form class="grid gap-5" onsubmit={rotate}>
        <Dialog.Header><Dialog.Title>Rotate API token</Dialog.Title><Dialog.Description>Replace the account-owned API token for {rotateTarget?.name ?? 'this connection'}. The new token is verified before it replaces the current credential.</Dialog.Description></Dialog.Header>
        <FormField label="New account-owned API token"><Input type="password" bind:value={rotateToken} autocomplete="new-password" required disabled={activeAction.startsWith('rotate:')} /></FormField>
        {#if rotateError}<p class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{rotateError}</p>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={activeAction.startsWith('rotate:')} onclick={() => (rotateOpen = false)}>Cancel</Button><Button type="submit" disabled={!rotateToken.trim() || activeAction.startsWith('rotate:')}>{#if activeAction.startsWith('rotate:')}<Spinner />{/if}Rotate token</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog bind:open={archiveOpen} title={`Archive ${archiveTarget?.name ?? 'connection'}?`} description="Move every managed Environment domain to another connection or manual DNS first." confirmLabel="Archive" destructive processing={activeAction.startsWith('archive:')} error={archiveError} onconfirm={archive} />
</DashboardLayout>
