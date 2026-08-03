<script lang="ts">
  import GithubIcon from '@lucide/svelte/icons/git-fork'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert'
  import { router, useForm } from '@inertiajs/svelte'

  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type NullableTime = { Time?: string; Valid?: boolean; time?: string; valid?: boolean } | string | null

  type Installation = {
    id: string
    accountLogin: string
    accountType: string
    repositorySelection: string
    repositoryCount: number
    suspendedAt: NullableTime
    archivedAt: NullableTime
    lastSyncedAt: NullableTime
    externalId: number
  }

  type Connection = {
    app: null | {
      name: string
      slug: string
      ownerLogin: string
      ownerType: string
      htmlUrl: string
      permissions: Record<string, string>
      events: string[]
      verifiedAt: NullableTime
    }
    installations: Installation[]
    degraded: boolean
    healthMessage: string
  }

  let { auth, connection }: { auth: { email: string }; connection: Connection } = $props()
  const setup = useForm({ ownerType: 'personal', ownerLogin: '' })
  let activeAction = $state('')
  let archiveDialogOpen = $state(false)
  let archiveTarget = $state<{ kind: 'app' | 'installation'; id?: string; name: string } | null>(null)
  let archiveError = $state('')

  function startSetup(event: SubmitEvent) {
    event.preventDefault()
    $setup.post(routes.gitHubAppSetup())
  }

  function dateLabel(value: NullableTime) {
    const raw = typeof value === 'string' ? value : value?.Time ?? value?.time
    if (!raw || (typeof value !== 'string' && (value?.Valid === false || value?.valid === false))) return 'Never'
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(raw))
  }

  function isPresent(value: NullableTime) {
    if (typeof value === 'string') return value.length > 0
    if (!value || value.Valid === false || value.valid === false) return false
    return Boolean(value.Time ?? value.time ?? value.Valid ?? value.valid)
  }

  function runAction(key: string, url: string) {
    if (activeAction) return
    activeAction = key
    router.post(url, {}, { onFinish: () => (activeAction = '') })
  }

  function askToArchive(target: { kind: 'app' | 'installation'; id?: string; name: string }) {
    archiveTarget = target
    archiveError = ''
    archiveDialogOpen = true
  }

  function archive() {
    if (!archiveTarget || activeAction) return
    activeAction = `archive:${archiveTarget.id ?? 'app'}`
    archiveError = ''
    const url = archiveTarget.kind === 'app' ? routes.gitHubAppDestroy() : routes.gitHubInstallationDestroy(archiveTarget.id ?? '')
    router.delete(url, {
      onSuccess: () => { archiveDialogOpen = false; archiveTarget = null },
      onError: (errors) => (archiveError = Object.values(errors).map(String).join('\n') || 'The connection could not be archived.'),
      onFinish: () => (activeAction = ''),
    })
  }
</script>

<svelte:head><title>GitHub connection</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Connections</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">GitHub App</h1>
      <p class="mt-4 text-sm leading-6 text-muted-foreground">Connect one private GitHub App to discover repositories and accept signed push events.</p>
    </section>

    {#if !connection.app}
      <Card.Root class="max-w-2xl">
        <Card.Header>
          <GithubIcon class="mb-2 size-7 text-primary" />
          <Card.Title>Create a private GitHub App</Card.Title>
          <Card.Description>GitHub returns the private key and webhook secret directly to DeployCrate. Secret values are encrypted before persistence.</Card.Description>
        </Card.Header>
        <Card.Content>
          <form class="grid gap-5" onsubmit={startSetup}>
            <FormField label="App owner" error={$setup.errors.ownerType}>
              <NativeSelect.Root bind:value={$setup.ownerType} class="w-full">
                <NativeSelect.Option value="personal">Personal account</NativeSelect.Option>
                <NativeSelect.Option value="organization">Organization</NativeSelect.Option>
              </NativeSelect.Root>
            </FormField>
            {#if $setup.ownerType === 'organization'}
              <FormField label="Organization login" error={$setup.errors.ownerLogin}><Input bind:value={$setup.ownerLogin} placeholder="acme" required /></FormField>
            {/if}
            <Button type="submit" disabled={$setup.processing} aria-busy={$setup.processing}>{#if $setup.processing}<Spinner />{/if}Continue to GitHub</Button>
          </form>
        </Card.Content>
      </Card.Root>
    {:else}
      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class="inline-flex items-center gap-1.5 text-xs">
              {#if connection.degraded}<TriangleAlertIcon class="size-4" />{:else}<ShieldCheckIcon class="size-4" />{/if}
              <StatusBadge status={connection.degraded ? 'degraded' : 'connected'} />
            </span>
          </Card.Action>
          <Card.Title>{connection.app.name}</Card.Title>
          <Card.Description>{connection.healthMessage}</Card.Description>
        </Card.Header>
        <Card.Content class="grid gap-4 text-sm sm:grid-cols-3">
          <div><p class="text-xs text-muted-foreground">Owner</p><p class="mt-1 font-medium">{connection.app.ownerLogin} · {connection.app.ownerType}</p></div>
          <div><p class="text-xs text-muted-foreground">Permissions</p><p class="mt-1 font-mono text-xs">contents: read · metadata: read</p></div>
          <div><p class="text-xs text-muted-foreground">Events</p><p class="mt-1 font-mono text-xs">{connection.app.events.join(', ')}</p></div>
        </Card.Content>
        <Card.Footer class="flex-col items-stretch justify-between gap-3 border-t border-border sm:flex-row sm:items-center">
          <a href={connection.app.htmlUrl} target="_blank" rel="noreferrer" class="text-xs text-primary hover:underline">Open GitHub App settings</a>
          <div class="flex flex-wrap gap-2">
            <Button variant="outline" disabled={Boolean(activeAction)} onclick={() => runAction('install', routes.gitHubInstall())}>{#if activeAction === 'install'}<Spinner />{/if}Install account</Button>
            <Button variant="destructive" disabled={Boolean(activeAction)} onclick={() => askToArchive({ kind: 'app', name: connection.app?.name ?? 'GitHub connection' })}>Archive connection</Button>
          </div>
        </Card.Footer>
      </Card.Root>

      <section class="space-y-4">
        <div><h2 class="text-xl font-semibold">Installed accounts</h2><p class="mt-1 text-sm text-muted-foreground">Repository grants are reconciled by stable GitHub IDs.</p></div>
        {#if connection.installations.length === 0}
          <Empty.Root class="border border-border">
            <Empty.Header><Empty.Media variant="icon"><GithubIcon /></Empty.Media><Empty.Title>No GitHub accounts installed</Empty.Title><Empty.Description>Install an account to make its repositories available to Applications.</Empty.Description></Empty.Header>
            <Empty.Content><Button variant="outline" disabled={Boolean(activeAction)} onclick={() => runAction('install', routes.gitHubInstall())}>{#if activeAction === 'install'}<Spinner />{/if}Install account</Button></Empty.Content>
          </Empty.Root>
        {:else}
          <div class="grid gap-4 lg:grid-cols-2">
            {#each connection.installations as installation (installation.id)}
              <Card.Root>
                <Card.Header>
                  <Card.Action><StatusBadge status={isPresent(installation.suspendedAt) ? 'suspended' : 'active'} /></Card.Action>
                  <Card.Title>{installation.accountLogin}</Card.Title>
                  <Card.Description>{installation.accountType} · {installation.repositorySelection} repositories</Card.Description>
                </Card.Header>
                <Card.Content class="grid grid-cols-2 gap-4 text-sm">
                  <div><p class="text-xs text-muted-foreground">Repositories</p><p class="mt-1 text-lg font-semibold">{installation.repositoryCount}</p></div>
                  <div><p class="text-xs text-muted-foreground">Last synchronized</p><p class="mt-1 text-xs">{dateLabel(installation.lastSyncedAt)}</p></div>
                </Card.Content>
                <Card.Footer class="flex-wrap gap-2 border-t border-border">
                  <Button size="sm" variant="outline" disabled={Boolean(activeAction)} onclick={() => runAction(`sync:${installation.id}`, routes.gitHubInstallationSync(installation.id))}>{#if activeAction === `sync:${installation.id}`}<Spinner />{:else}<RefreshCwIcon />{/if}Sync</Button>
                  <Button size="sm" variant="outline" disabled={Boolean(activeAction)} onclick={() => runAction(`verify:${installation.id}`, routes.gitHubInstallationVerify(installation.id))}>{#if activeAction === `verify:${installation.id}`}<Spinner />{/if}Verify</Button>
                  <Button size="sm" variant="destructive" disabled={Boolean(activeAction)} onclick={() => askToArchive({ kind: 'installation', id: installation.id, name: installation.accountLogin })}>Archive</Button>
                </Card.Footer>
              </Card.Root>
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
  <ConfirmActionDialog bind:open={archiveDialogOpen} title={`Archive ${archiveTarget?.name ?? 'connection'}?`} description="This removes the connection from DeployCrate. Applications that depend on its repositories may no longer build." confirmLabel="Archive" destructive processing={activeAction.startsWith('archive:')} error={archiveError} onconfirm={archive} />
</DashboardLayout>
