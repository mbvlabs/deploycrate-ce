<script lang="ts">
  import GithubIcon from '@lucide/svelte/icons/git-fork'
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert'
  import { router, useForm } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Input } from '@/Components/ui/input'
  import { Label } from '@/Components/ui/label'
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

  function startSetup(event: SubmitEvent) {
    event.preventDefault()
    $setup.post(routes.gitHubAppSetup())
  }

  function dateLabel(value: NullableTime) {
    const raw = typeof value === 'string' ? value : value?.Time ?? value?.time
    if (!raw || (typeof value !== 'string' && (value?.Valid === false || value?.valid === false))) return 'Never'
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(raw))
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
            <div class="grid gap-2">
              <Label for="owner-type">App owner</Label>
              <select id="owner-type" bind:value={$setup.ownerType} class="h-9 border border-input bg-background px-3 text-sm">
                <option value="personal">Personal account</option>
                <option value="organization">Organization</option>
              </select>
            </div>
            {#if $setup.ownerType === 'organization'}
              <div class="grid gap-2">
                <Label for="owner-login">Organization login</Label>
                <Input id="owner-login" bind:value={$setup.ownerLogin} placeholder="acme" required />
              </div>
            {/if}
            <Button type="submit" disabled={$setup.processing}>Continue to GitHub</Button>
          </form>
        </Card.Content>
      </Card.Root>
    {:else}
      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class:text-destructive={connection.degraded} class:text-success={!connection.degraded} class="inline-flex items-center gap-1.5 text-xs">
              {#if connection.degraded}<TriangleAlertIcon class="size-4" />{:else}<ShieldCheckIcon class="size-4" />{/if}
              {connection.degraded ? 'Degraded' : 'Connected'}
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
        <Card.Footer class="justify-between gap-3 border-t border-border">
          <a href={connection.app.htmlUrl} target="_blank" rel="noreferrer" class="text-xs text-primary hover:underline">Open GitHub App settings</a>
          <div class="flex gap-2">
            <Button variant="outline" onclick={() => router.post(routes.gitHubInstall())}>Install account</Button>
            <Button variant="destructive" onclick={() => router.delete(routes.gitHubAppDestroy())}>Archive connection</Button>
          </div>
        </Card.Footer>
      </Card.Root>

      <section class="space-y-4">
        <div><h2 class="text-xl font-semibold">Installed accounts</h2><p class="mt-1 text-sm text-muted-foreground">Repository grants are reconciled by stable GitHub IDs.</p></div>
        {#if connection.installations.length === 0}
          <Card.Root><Card.Content class="py-8 text-sm text-muted-foreground">No GitHub accounts are installed yet.</Card.Content></Card.Root>
        {:else}
          <div class="grid gap-4 lg:grid-cols-2">
            {#each connection.installations as installation (installation.id)}
              <Card.Root>
                <Card.Header>
                  <Card.Action><span class="text-xs" class:text-destructive={Boolean(typeof installation.suspendedAt !== 'string' && installation.suspendedAt?.Valid)}>{typeof installation.suspendedAt !== 'string' && installation.suspendedAt?.Valid ? 'Suspended' : 'Active'}</span></Card.Action>
                  <Card.Title>{installation.accountLogin}</Card.Title>
                  <Card.Description>{installation.accountType} · {installation.repositorySelection} repositories</Card.Description>
                </Card.Header>
                <Card.Content class="grid grid-cols-2 gap-4 text-sm">
                  <div><p class="text-xs text-muted-foreground">Repositories</p><p class="mt-1 text-lg font-semibold">{installation.repositoryCount}</p></div>
                  <div><p class="text-xs text-muted-foreground">Last synchronized</p><p class="mt-1 text-xs">{dateLabel(installation.lastSyncedAt)}</p></div>
                </Card.Content>
                <Card.Footer class="gap-2 border-t border-border">
                  <Button size="sm" variant="outline" onclick={() => router.post(routes.gitHubInstallationSync(installation.id))}><RefreshCwIcon /> Sync</Button>
                  <Button size="sm" variant="outline" onclick={() => router.post(routes.gitHubInstallationVerify(installation.id))}>Verify</Button>
                  <Button size="sm" variant="destructive" onclick={() => router.delete(routes.gitHubInstallationDestroy(installation.id))}>Archive</Button>
                </Card.Footer>
              </Card.Root>
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
</DashboardLayout>
