<script lang="ts">
  import ArrowUpRightIcon from '@lucide/svelte/icons/arrow-up-right'
  import { Link, router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import * as Table from '@/Components/ui/table'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = {
    environmentId: string
    environmentName: string
    environmentKind: string
    setupComplete: boolean
    sourceType: 'buildpacks' | 'image'
    installationAccount: string
    repositoryFullName: string
    repositoryRemovedAt: unknown
    installationSuspendedAt: unknown
    reference: string
    imageRepository: string
    registryName: string
  }

  type Deployment = {
    id: string
    environmentId: string
    environmentName: string
    environmentKind: string
    status: string
    currentStep: string
    error: string
    releaseId: string
    sourceRevision: string
    createdAt: string
    active: boolean
  }

  type Application = {
    id: string
    name: string
    slug: string
    environments: Environment[]
    deployments: Deployment[]
  }

  type TelemetryRow = {
    environment: string
    available: boolean
    cpuCores: number
    memoryBytes: number
    diskReadBytesPerSecond: number
    diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number
    networkTransmitBytesPerSecond: number
    cpuAvailable: boolean
    memoryAvailable: boolean
    diskReadAvailable: boolean
    diskWriteAvailable: boolean
    networkReceiveAvailable: boolean
    networkTransmitAvailable: boolean
  }

  type TelemetrySummary = {
    cpuCores: number
    memoryBytes: number
    diskReadBytesPerSecond: number
    diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number
    networkTransmitBytesPerSecond: number
    cpuAvailable: boolean
    memoryAvailable: boolean
    diskReadAvailable: boolean
    diskWriteAvailable: boolean
    networkReceiveAvailable: boolean
    networkTransmitAvailable: boolean
  }

  let { auth, application, telemetry }: {
    auth: { email: string }
    application: Application
    telemetry: TelemetryRow[]
  } = $props()

  let deleteDialogOpen = $state(false)
  let deleteProcessing = $state(false)

  const staging = $derived(application.environments.find((environment) => environment.environmentKind === 'staging') ?? null)
  const production = $derived(application.environments.find((environment) => environment.environmentKind === 'production') ?? null)
  const otherEnvironments = $derived(application.environments.filter((environment) => (
    environment.environmentId !== staging?.environmentId && environment.environmentId !== production?.environmentId
  )))
  const currentTelemetry = $derived((telemetry ?? []).filter((row) => row.available))
  const combinedTelemetry = $derived(summarizeTelemetry(currentTelemetry))

  function hasTimestamp(value: unknown) {
    if (typeof value === 'string') return value.trim() !== ''
    if (!value || typeof value !== 'object') return false
    const timestamp = value as { Valid?: boolean; valid?: boolean }
    return Boolean(timestamp.Valid ?? timestamp.valid)
  }

  function healthy(environment: Environment) {
    return environment.sourceType === 'image'
      || (!hasTimestamp(environment.repositoryRemovedAt) && !hasTimestamp(environment.installationSuspendedAt))
  }

  function sourceLabel(environment: Environment) {
    if (environment.sourceType === 'image') {
      return [environment.registryName, environment.imageRepository].filter(Boolean).join(' / ')
    }
    return [environment.installationAccount, environment.repositoryFullName].filter(Boolean).join(' / ')
  }

  function latestDeployment(environmentId: string) {
    return application.deployments.find((deployment) => deployment.environmentId === environmentId) ?? null
  }

  function telemetryForEnvironment(environmentId: string) {
    return summarizeTelemetry(currentTelemetry.filter((row) => row.environment === environmentId))
  }

  function summarizeTelemetry(rows: TelemetryRow[]): TelemetrySummary {
    const summary: TelemetrySummary = {
      cpuCores: 0,
      memoryBytes: 0,
      diskReadBytesPerSecond: 0,
      diskWriteBytesPerSecond: 0,
      networkReceiveBytesPerSecond: 0,
      networkTransmitBytesPerSecond: 0,
      cpuAvailable: false,
      memoryAvailable: false,
      diskReadAvailable: false,
      diskWriteAvailable: false,
      networkReceiveAvailable: false,
      networkTransmitAvailable: false,
    }
    for (const row of rows) {
      if (row.cpuAvailable) { summary.cpuCores += row.cpuCores; summary.cpuAvailable = true }
      if (row.memoryAvailable) { summary.memoryBytes += row.memoryBytes; summary.memoryAvailable = true }
      if (row.diskReadAvailable) { summary.diskReadBytesPerSecond += row.diskReadBytesPerSecond; summary.diskReadAvailable = true }
      if (row.diskWriteAvailable) { summary.diskWriteBytesPerSecond += row.diskWriteBytesPerSecond; summary.diskWriteAvailable = true }
      if (row.networkReceiveAvailable) { summary.networkReceiveBytesPerSecond += row.networkReceiveBytesPerSecond; summary.networkReceiveAvailable = true }
      if (row.networkTransmitAvailable) { summary.networkTransmitBytesPerSecond += row.networkTransmitBytesPerSecond; summary.networkTransmitAvailable = true }
    }
    return summary
  }

  function formatBytes(value: number) {
    if (!Number.isFinite(value) || value < 0) return 'Unavailable'
    if (value === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
    return `${(value / (1024 ** index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
  }

  const formatRate = (value: number) => `${formatBytes(value)}/s`
  const formatCPU = (value: number) => `${value.toFixed(2)} cores`
  const formatTimestamp = (value: string) => value ? new Date(value).toLocaleString() : 'Not recorded'
  const short = (value: string) => value ? value.slice(0, 12) : 'Not recorded'
  const label = (value: string) => value ? value.replaceAll('_', ' ') : 'Not recorded'

  function openEnvironment(environmentId: string) {
    router.visit(routes.environmentShow(application.id, environmentId))
  }

  function activateEnvironmentRow(event: KeyboardEvent, environmentId: string) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    openEnvironment(environmentId)
  }

  function deleteApplication() {
    if (deleteProcessing) return
    deleteProcessing = true
    router.delete(routes.applicationDestroy(application.id), {
      onSuccess: () => (deleteDialogOpen = false),
      onFinish: () => (deleteProcessing = false),
    })
  }
</script>

<svelte:head><title>{application.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-10">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Application</p>
        <h1 class="mt-3 text-3xl font-semibold">{application.name}</h1>
        <p class="mt-2 font-mono text-xs text-muted-foreground">{application.slug}</p>
      </div>
      <div class="flex gap-2">
        <Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationEdit(application.id)}>Edit application</Link>{/snippet}</Button>
        <Button>{#snippet child({ props })}<Link {...props} href={routes.environmentNew(application.id)}>Add environment</Link>{/snippet}</Button>
      </div>
    </header>

    <section aria-labelledby="featured-environments-heading" class="space-y-4">
      <div>
        <h2 id="featured-environments-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Primary environments</h2>
        <p class="mt-1 text-sm text-muted-foreground">Open staging or production to deploy, inspect logs, and manage configuration.</p>
      </div>
      <div class="grid gap-4 lg:grid-cols-2">
        {#each [{ kind: 'staging', environment: staging }, { kind: 'production', environment: production }] as featured (featured.kind)}
          {#if featured.environment}
            {@const environment = featured.environment}
            {@const deployment = latestDeployment(environment.environmentId)}
            {@const environmentTelemetry = telemetryForEnvironment(environment.environmentId)}
            <Link href={routes.environmentShow(application.id, environment.environmentId)} class="group block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              <Card.Root class="h-full transition-colors group-hover:bg-muted/30">
                <Card.Header>
                  <Card.Action><StatusBadge status={deployment?.active ? 'serving' : deployment?.status ?? (environment.setupComplete ? 'ready' : 'pending')} /></Card.Action>
                  <Card.Title class="flex items-center gap-2 capitalize">{featured.kind}<ArrowUpRightIcon class="size-4 text-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" /></Card.Title>
                  <Card.Description>{environment.environmentName} · {sourceLabel(environment)}</Card.Description>
                </Card.Header>
                <Card.Content class="grid gap-5 sm:grid-cols-3">
                  <div><p class="text-[10px] uppercase tracking-[0.12em] text-muted-foreground">Reference</p><p class="mt-1 truncate font-mono text-xs">{environment.reference || 'Not configured'}</p></div>
                  <div><p class="text-[10px] uppercase tracking-[0.12em] text-muted-foreground">CPU</p><p class="mt-1 font-medium">{environmentTelemetry.cpuAvailable ? formatCPU(environmentTelemetry.cpuCores) : 'Unavailable'}</p></div>
                  <div><p class="text-[10px] uppercase tracking-[0.12em] text-muted-foreground">Memory</p><p class="mt-1 font-medium">{environmentTelemetry.memoryAvailable ? formatBytes(environmentTelemetry.memoryBytes) : 'Unavailable'}</p></div>
                </Card.Content>
                <Card.Footer class="flex items-center justify-between border-t border-border text-xs text-muted-foreground">
                  <span>{deployment ? `${deployment.active ? 'Serving' : label(deployment.status)} · ${short(deployment.sourceRevision || deployment.releaseId)}` : 'No deployments yet'}</span>
                  <StatusBadge status={healthy(environment) ? 'ready' : 'degraded'} label={healthy(environment) ? 'Source ready' : 'Source degraded'} />
                </Card.Footer>
              </Card.Root>
            </Link>
          {:else}
            <Link href={routes.environmentNew(application.id)} class="group block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              <Card.Root class="h-full border-dashed bg-muted/10 transition-colors group-hover:bg-muted/30">
                <Card.Header>
                  <Card.Title class="flex items-center gap-2 capitalize">{featured.kind}<ArrowUpRightIcon class="size-4 text-muted-foreground" /></Card.Title>
                  <Card.Description>No {featured.kind} environment has been configured.</Card.Description>
                </Card.Header>
                <Card.Content><p class="text-sm font-medium">Add {featured.kind} environment</p></Card.Content>
              </Card.Root>
            </Link>
          {/if}
        {/each}
      </div>
    </section>

    <section aria-labelledby="other-environments-heading" class="space-y-4">
      <div>
        <h2 id="other-environments-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Other environments</h2>
        <p class="mt-1 text-sm text-muted-foreground">Every environment outside the primary staging and production pair.</p>
      </div>
      {#if otherEnvironments.length === 0}
        <Empty.Root class="border border-dashed border-border py-10"><Empty.Header><Empty.Title>No other environments</Empty.Title><Empty.Description>Additional environments will appear here.</Empty.Description></Empty.Header></Empty.Root>
      {:else}
        <div class="border border-border">
          <Table.Root>
            <Table.Header><Table.Row><Table.Head>Environment</Table.Head><Table.Head>Kind</Table.Head><Table.Head>Source</Table.Head><Table.Head>Reference</Table.Head><Table.Head>Latest deployment</Table.Head></Table.Row></Table.Header>
            <Table.Body>
              {#each otherEnvironments as environment (environment.environmentId)}
                {@const deployment = latestDeployment(environment.environmentId)}
                <Table.Row class="cursor-pointer" tabindex={0} onclick={() => openEnvironment(environment.environmentId)} onkeydown={(event) => activateEnvironmentRow(event, environment.environmentId)}>
                  <Table.Cell><div class="font-medium">{environment.environmentName}</div><div class="text-xs text-muted-foreground">{environment.environmentId}</div></Table.Cell>
                  <Table.Cell class="capitalize">{environment.environmentKind}</Table.Cell>
                  <Table.Cell><StatusBadge status={healthy(environment) ? 'ready' : 'degraded'} label={environment.sourceType} /></Table.Cell>
                  <Table.Cell class="font-mono text-xs">{environment.reference || 'Not configured'}</Table.Cell>
                  <Table.Cell>{#if deployment}<StatusBadge status={deployment.active ? 'serving' : deployment.status} />{:else}<span class="text-muted-foreground">None</span>{/if}</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </div>
      {/if}
    </section>

    <section aria-labelledby="telemetry-heading" class="space-y-4">
      <div>
        <h2 id="telemetry-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Combined telemetry</h2>
        <p class="mt-1 text-sm text-muted-foreground">Current resource use across fresh workload telemetry from all application environments.</p>
      </div>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card.Root size="sm"><Card.Header><Card.Description>CPU</Card.Description><Card.Title>{combinedTelemetry.cpuAvailable ? formatCPU(combinedTelemetry.cpuCores) : 'Unavailable'}</Card.Title></Card.Header></Card.Root>
        <Card.Root size="sm"><Card.Header><Card.Description>Memory</Card.Description><Card.Title>{combinedTelemetry.memoryAvailable ? formatBytes(combinedTelemetry.memoryBytes) : 'Unavailable'}</Card.Title></Card.Header></Card.Root>
        <Card.Root size="sm"><Card.Header><Card.Description>Disk read / write</Card.Description><Card.Title class="text-base">{combinedTelemetry.diskReadAvailable ? formatRate(combinedTelemetry.diskReadBytesPerSecond) : 'Unavailable'} / {combinedTelemetry.diskWriteAvailable ? formatRate(combinedTelemetry.diskWriteBytesPerSecond) : 'Unavailable'}</Card.Title></Card.Header></Card.Root>
        <Card.Root size="sm"><Card.Header><Card.Description>Network receive / transmit</Card.Description><Card.Title class="text-base">{combinedTelemetry.networkReceiveAvailable ? formatRate(combinedTelemetry.networkReceiveBytesPerSecond) : 'Unavailable'} / {combinedTelemetry.networkTransmitAvailable ? formatRate(combinedTelemetry.networkTransmitBytesPerSecond) : 'Unavailable'}</Card.Title></Card.Header></Card.Root>
      </div>
      <div class="border border-border">
        <Table.Root>
          <Table.Header><Table.Row><Table.Head>Environment</Table.Head><Table.Head>CPU</Table.Head><Table.Head>Memory</Table.Head><Table.Head>Disk read / write</Table.Head><Table.Head>Network receive / transmit</Table.Head></Table.Row></Table.Header>
          <Table.Body>
            {#each application.environments as environment (environment.environmentId)}
              {@const summary = telemetryForEnvironment(environment.environmentId)}
              <Table.Row>
                <Table.Cell><div class="font-medium">{environment.environmentName}</div><div class="text-xs capitalize text-muted-foreground">{environment.environmentKind}</div></Table.Cell>
                <Table.Cell>{summary.cpuAvailable ? formatCPU(summary.cpuCores) : 'Unavailable'}</Table.Cell>
                <Table.Cell>{summary.memoryAvailable ? formatBytes(summary.memoryBytes) : 'Unavailable'}</Table.Cell>
                <Table.Cell>{summary.diskReadAvailable ? formatRate(summary.diskReadBytesPerSecond) : 'Unavailable'} / {summary.diskWriteAvailable ? formatRate(summary.diskWriteBytesPerSecond) : 'Unavailable'}</Table.Cell>
                <Table.Cell>{summary.networkReceiveAvailable ? formatRate(summary.networkReceiveBytesPerSecond) : 'Unavailable'} / {summary.networkTransmitAvailable ? formatRate(summary.networkTransmitBytesPerSecond) : 'Unavailable'}</Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    </section>

    <section aria-labelledby="deployments-heading" class="space-y-4">
      <div>
        <h2 id="deployments-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Recent deployments</h2>
        <p class="mt-1 text-sm text-muted-foreground">The latest deployment activity across all environments.</p>
      </div>
      {#if application.deployments.length === 0}
        <Empty.Root class="border border-dashed border-border py-10"><Empty.Header><Empty.Title>No deployments yet</Empty.Title><Empty.Description>Deployment activity will appear here after the first release.</Empty.Description></Empty.Header></Empty.Root>
      {:else}
        <div class="border border-border">
          <Table.Root>
            <Table.Header><Table.Row><Table.Head>Environment</Table.Head><Table.Head>Revision</Table.Head><Table.Head>Status</Table.Head><Table.Head>Step</Table.Head><Table.Head>Created</Table.Head></Table.Row></Table.Header>
            <Table.Body>
              {#each application.deployments as deployment (deployment.id)}
                <Table.Row class="cursor-pointer" tabindex={0} onclick={() => openEnvironment(deployment.environmentId)} onkeydown={(event) => activateEnvironmentRow(event, deployment.environmentId)}>
                  <Table.Cell><div class="font-medium">{deployment.environmentName}</div><div class="text-xs capitalize text-muted-foreground">{deployment.environmentKind}</div></Table.Cell>
                  <Table.Cell class="font-mono text-xs">{short(deployment.sourceRevision || deployment.releaseId)}</Table.Cell>
                  <Table.Cell><StatusBadge status={deployment.active ? 'serving' : deployment.status} /></Table.Cell>
                  <Table.Cell class="capitalize">{deployment.active ? 'serving' : label(deployment.currentStep || deployment.status)}</Table.Cell>
                  <Table.Cell>{formatTimestamp(deployment.createdAt)}</Table.Cell>
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </div>
      {/if}
    </section>

    <div><Button variant="destructive" onclick={() => (deleteDialogOpen = true)}>Delete application</Button></div>
  </div>

  <ConfirmActionDialog bind:open={deleteDialogOpen} title="Permanently delete application?" description={`Delete ${application.name}, every Environment, deployment record, secret, domain, and runtime workload. Shared Resources and Servers are kept. This cannot be undone.`} confirmLabel="Delete application" destructive processing={deleteProcessing} onconfirm={deleteApplication} />
</DashboardLayout>
