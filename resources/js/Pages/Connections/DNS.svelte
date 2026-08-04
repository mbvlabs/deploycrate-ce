<script lang="ts">
  import CloudIcon from '@lucide/svelte/icons/cloud'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import { router, useForm } from '@inertiajs/svelte'

  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import { Input } from '@/Components/ui/input'
  import { Spinner } from '@/Components/ui/spinner'
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
  let activeAction = $state('')
  let tokens = $state<Record<string, string>>({})
  let archiveOpen = $state(false)
  let archiveTarget = $state<Connection | null>(null)
  let archiveError = $state('')

  function createConnection(event: SubmitEvent) {
    event.preventDefault()
    $createForm.post(routes.dnsConnectionCreate(), { onSuccess: () => $createForm.reset() })
  }

  function sync(connection: Connection) {
    activeAction = `sync:${connection.id}`
    router.post(routes.dnsConnectionSync(connection.id), {}, { onFinish: () => activeAction = '' })
  }

  function rotate(connection: Connection) {
    const token = tokens[connection.id]?.trim()
    if (!token || activeAction) return
    activeAction = `rotate:${connection.id}`
    router.patch(routes.dnsConnectionTokenUpdate(connection.id), { token }, {
      onSuccess: () => tokens[connection.id] = '',
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
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Connections</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">DNS</h1>
      <p class="mt-4 text-sm leading-6 text-muted-foreground">Connect named Cloudflare accounts with durable account-owned tokens and synchronize the zones available to Environments.</p>
    </section>

    <Card.Root class="max-w-2xl">
      <Card.Header><CloudIcon class="mb-2 size-7 text-primary" /><Card.Title>Add Cloudflare connection</Card.Title><Card.Description>Use an account-owned API token with Zone Read and DNS Write access. The token is encrypted and never displayed again.</Card.Description></Card.Header>
      <Card.Content>
        <form class="grid gap-5" onsubmit={createConnection}>
          <FormField label="Connection name" error={$createForm.errors.name}><Input bind:value={$createForm.name} placeholder="Production Cloudflare" required /></FormField>
          <FormField label="Cloudflare Account ID" error={$createForm.errors.accountId}><Input bind:value={$createForm.accountId} minlength="32" maxlength="32" placeholder="023e105f4ecef8ad9ca31a8372d0c353" autocomplete="off" required /><p class="mt-2 text-xs text-muted-foreground">Find this on the Cloudflare account overview page.</p></FormField>
          <FormField label="Account-owned API token" error={$createForm.errors.token}><Input type="password" bind:value={$createForm.token} autocomplete="new-password" placeholder="cfat_..." required /></FormField>
          <Button type="submit" disabled={$createForm.processing} aria-busy={$createForm.processing}>{#if $createForm.processing}<Spinner />{/if}Connect Cloudflare</Button>
        </form>
      </Card.Content>
    </Card.Root>

    <section class="space-y-4">
      <div><h2 class="text-xl font-semibold">Connected accounts</h2><p class="mt-1 text-sm text-muted-foreground">Zones are cached locally and refreshed explicitly.</p></div>
      {#if connections.length === 0}
        <Empty.Root class="border border-border"><Empty.Header><Empty.Media variant="icon"><CloudIcon /></Empty.Media><Empty.Title>No DNS connections</Empty.Title><Empty.Description>Add Cloudflare to automate Environment A records.</Empty.Description></Empty.Header></Empty.Root>
      {:else}
        <div class="grid gap-4 lg:grid-cols-2">
          {#each connections as connection (connection.id)}
            <Card.Root>
              <Card.Header><Card.Action><StatusBadge status="connected" /></Card.Action><Card.Title>{connection.name}</Card.Title><Card.Description>{connection.activeZones} active zone{connection.activeZones === 1 ? '' : 's'} · {connection.bindingCount} managed domain{connection.bindingCount === 1 ? '' : 's'}</Card.Description></Card.Header>
              <Card.Content class="grid gap-4 text-sm sm:grid-cols-2">
                <div class="sm:col-span-2"><p class="text-xs text-muted-foreground">Cloudflare Account ID</p><p class="mt-1 break-all font-mono text-xs">{connection.accountId}</p></div>
                <div><p class="text-xs text-muted-foreground">Verified</p><p class="mt-1">{dateLabel(connection.verifiedAt)}</p></div>
                <div><p class="text-xs text-muted-foreground">Last synchronized</p><p class="mt-1">{dateLabel(connection.lastSyncedAt)}</p></div>
                <div class="sm:col-span-2"><p class="mb-2 text-xs text-muted-foreground">Rotate API token</p><div class="flex gap-2"><Input type="password" value={tokens[connection.id] ?? ''} oninput={(event) => tokens[connection.id] = event.currentTarget.value} autocomplete="new-password" placeholder="New API token" /><Button variant="outline" disabled={Boolean(activeAction) || !(tokens[connection.id]?.trim())} onclick={() => rotate(connection)}>{#if activeAction === `rotate:${connection.id}`}<Spinner />{/if}Rotate</Button></div></div>
              </Card.Content>
              <Card.Footer class="flex-wrap gap-2 border-t border-border"><Button size="sm" variant="outline" disabled={Boolean(activeAction)} onclick={() => sync(connection)}>{#if activeAction === `sync:${connection.id}`}<Spinner />{:else}<RefreshCwIcon />{/if}Sync zones</Button><Button size="sm" variant="destructive" disabled={Boolean(activeAction)} onclick={() => askArchive(connection)}>Archive</Button><span class="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground"><ShieldCheckIcon class="size-4" />Account-owned token</span></Card.Footer>
            </Card.Root>
          {/each}
        </div>
      {/if}
    </section>
  </div>
  <ConfirmActionDialog bind:open={archiveOpen} title={`Archive ${archiveTarget?.name ?? 'connection'}?`} description="Move every managed Environment domain to another connection or manual DNS first." confirmLabel="Archive" destructive processing={activeAction.startsWith('archive:')} error={archiveError} onconfirm={archive} />
</DashboardLayout>
