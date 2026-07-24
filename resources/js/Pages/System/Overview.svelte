<script lang="ts">
  import ActivityIcon from '@lucide/svelte/icons/activity'
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import CheckCircleIcon from '@lucide/svelte/icons/circle-check'
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link'
  import GitCommitHorizontalIcon from '@lucide/svelte/icons/git-commit-horizontal'
  import NetworkIcon from '@lucide/svelte/icons/network'
  import ServerIcon from '@lucide/svelte/icons/server'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert'
  import { Link } from '@inertiajs/svelte'

  import * as Card from '@/Components/ui/card'
  import { Separator } from '@/Components/ui/separator'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type SystemOverview = {
    applicationName: string
    applicationSlug: string
    environmentName: string
    environmentKind: string
    serverName: string
    serverAddress: string
    serverStatus: string
    operatingSystem: string
    distribution: string
    distributionVersion: string
    architecture: string
    networkName: string
    networkDriver: string
    networkState: string
    databaseName: string
    databaseKind: string
    databaseAddress: string
    databasePort: number
    releaseVersion: string
    artifactReference: string
    deploymentStatus: string
    deploymentStep: string
    activeSlot: string
    activeService: string
    activeState: string
    activePort: number
    domain: string
    routeExternalId: string
    routeState: string
    observedAt: string
  }

  type SystemHealth = {
    ok: boolean
    checkedAt: string
    checks: Array<{
      name: string
      ok: boolean
      detail: string
    }>
  }

  let { auth, system, health }: { auth: { email: string }; system: SystemOverview; health: SystemHealth } = $props()

  const stateLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'Unknown'
  const checkLabel = (value: string) => stateLabel(value).replace(/\b\w/g, (letter) => letter.toUpperCase())
  const versionLabel = (version: string) => version ? `v${version.replace(/^v/, '')}` : 'Development build'
  const platformLabel = $derived(
    [system.distribution, system.distributionVersion, system.architecture].filter(Boolean).join(' ') || system.operatingSystem || 'Unknown',
  )
</script>

<svelte:head>
  <title>System overview</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div class="max-w-3xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">{system.applicationName}</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          The CONTROL plane is managed as a protected system application. Its deployment topology is visible here but cannot be edited from the applications workspace.
        </p>
      </div>
      <Link
        href={routes.systemUpdate()}
        class="inline-flex h-9 shrink-0 items-center justify-center gap-2 border border-border bg-background px-4 text-sm font-medium transition-colors hover:bg-muted"
      >
        Manage updates
        <ExternalLinkIcon class="size-4" />
      </Link>
    </section>

    <Card.Root class={health.ok ? 'border-success/40' : 'border-destructive/60'}>
      <Card.Header>
        <Card.Action>
          <span class={health.ok ? 'text-success' : 'text-destructive'}>
            {health.ok ? 'All checks passed' : 'Attention required'}
          </span>
        </Card.Action>
        {#if health.ok}
          <CheckCircleIcon class="mb-2 size-5 text-success" />
        {:else}
          <TriangleAlertIcon class="mb-2 size-5 text-destructive" />
        {/if}
        <Card.Title>Live system health</Card.Title>
        <Card.Description>
          Checked by the running DeployCrate application at {new Date(health.checkedAt).toLocaleString()}.
        </Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-3 md:grid-cols-2">
        {#each health.checks as check (check.name)}
          <div class="flex items-start gap-3 border border-border/70 bg-muted/20 p-3">
            {#if check.ok}
              <CheckCircleIcon class="mt-0.5 size-4 shrink-0 text-success" />
            {:else}
              <TriangleAlertIcon class="mt-0.5 size-4 shrink-0 text-destructive" />
            {/if}
            <div class="min-w-0">
              <p class="text-sm font-medium">{checkLabel(check.name)}</p>
              <p class="mt-1 break-words text-xs leading-5 text-muted-foreground">{check.detail}</p>
            </div>
          </div>
        {/each}
      </Card.Content>
    </Card.Root>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-label="System health">
      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class="text-[10px] uppercase tracking-[0.14em] text-success">{stateLabel(system.serverStatus)}</span>
          </Card.Action>
          <ShieldCheckIcon class="mb-2 size-5 text-primary" />
          <Card.Title>Control plane</Card.Title>
          <Card.Description>{system.applicationSlug}</Card.Description>
        </Card.Header>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class="text-[10px] uppercase tracking-[0.14em] text-success">{stateLabel(system.deploymentStatus)}</span>
          </Card.Action>
          <GitCommitHorizontalIcon class="mb-2 size-5 text-primary" />
          <Card.Title>{versionLabel(system.releaseVersion)}</Card.Title>
          <Card.Description>Active release</Card.Description>
        </Card.Header>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class="text-[10px] uppercase tracking-[0.14em] text-success">{stateLabel(system.activeState)}</span>
          </Card.Action>
          <ActivityIcon class="mb-2 size-5 text-primary" />
          <Card.Title class="capitalize">{system.activeSlot} slot</Card.Title>
          <Card.Description>127.0.0.1:{system.activePort}</Card.Description>
        </Card.Header>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Action>
            <span class="text-[10px] uppercase tracking-[0.14em] text-success">{stateLabel(system.routeState)}</span>
          </Card.Action>
          <BoxesIcon class="mb-2 size-5 text-primary" />
          <Card.Title>{system.environmentName}</Card.Title>
          <Card.Description class="capitalize">{system.environmentKind} environment</Card.Description>
        </Card.Header>
      </Card.Root>
    </section>

    <section class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header>
          <ServerIcon class="mb-2 size-5 text-primary" />
          <Card.Title>Runtime</Card.Title>
          <Card.Description>The server and systemd service currently serving DeployCrate CE.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-3 text-sm">
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Server</span>
            <span class="text-right font-medium">{system.serverName}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Address</span>
            <span class="font-mono text-xs">{system.serverAddress}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Platform</span>
            <span class="text-right">{platformLabel}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Service</span>
            <span class="font-mono text-xs">{system.activeService}</span>
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <NetworkIcon class="mb-2 size-5 text-primary" />
          <Card.Title>Routing and network</Card.Title>
          <Card.Description>The host network and Caddy route attached to the system environment.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-3 text-sm">
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Network</span>
            <span class="text-right font-medium">{system.networkName || 'Not configured'}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Driver</span>
            <span class="capitalize">{system.networkDriver || 'Unknown'} · {stateLabel(system.networkState)}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Domain</span>
            <span class="font-mono text-xs">{system.domain}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Caddy route</span>
            <span class="max-w-64 truncate font-mono text-xs">{system.routeExternalId}</span>
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <DatabaseIcon class="mb-2 size-5 text-primary" />
          <Card.Title>Database</Card.Title>
          <Card.Description>The database resource bound to the system environment.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-3 text-sm">
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Resource</span>
            <span class="text-right font-medium">{system.databaseName || 'Not configured'}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Kind</span>
            <span class="capitalize">{system.databaseKind || 'Unknown'}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Endpoint</span>
            <span class="font-mono text-xs">{system.databaseAddress}:{system.databasePort}</span>
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <GitCommitHorizontalIcon class="mb-2 size-5 text-primary" />
          <Card.Title>Deployment</Card.Title>
          <Card.Description>The persisted release and deployment state used by self-update.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-3 text-sm">
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Release</span>
            <span class="font-mono text-xs">{versionLabel(system.releaseVersion)}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Status</span>
            <span class="capitalize">{stateLabel(system.deploymentStatus)}</span>
          </div>
          <Separator />
          <div class="flex items-center justify-between gap-4">
            <span class="text-muted-foreground">Current step</span>
            <span class="capitalize">{stateLabel(system.deploymentStep)}</span>
          </div>
          <Separator />
          <div class="flex items-start justify-between gap-4">
            <span class="shrink-0 text-muted-foreground">Artifact</span>
            <span class="max-w-[70%] break-all text-right font-mono text-xs">{system.artifactReference}</span>
          </div>
        </Card.Content>
      </Card.Root>
    </section>
  </div>
</DashboardLayout>
