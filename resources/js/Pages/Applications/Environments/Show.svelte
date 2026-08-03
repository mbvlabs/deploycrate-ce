<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { untrack } from 'svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import DataField from '@/Components/DataField.svelte'
  import EnvironmentDeleteDialog from '@/Components/EnvironmentDeleteDialog.svelte'
  import TelemetryDonut from '@/Components/Applications/Environments/TelemetryDonut.svelte'
  import TelemetryHistory from '@/Components/Applications/Environments/TelemetryHistory.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Secret = { id: string; key: string; digestPrefix: string; sourceType: string; createdAt: string; status: 'deployed' | 'deploying' | 'pending' | 'failed' | 'pending_removal'; desired: boolean }
  type Variable = { key: string; value: string; source: string; sourceId: string }
  type Resource = { id: string; alias: string; name: string; engine: string }
  type Build = { id: string; sourceRevision: string; status: string; currentStep: string; error: string; createdAt: string; startedAt?: string; finishedAt?: string; registryEndpoint: string; jobId: number | null; jobState: string }
  type BuildLog = { id: string; sequence: number; stream: 'system' | 'pack'; message: string; occurredAt: string }
  type BuildLogSnapshot = { build: Build; logs: BuildLog[]; nextSequence: number; hasMore: boolean }
  type Release = { id: string; sourceRevision: string; artifactReference: string; createdAt: string }
  type Deployment = { id: string; status: string; currentStep: string; error: string; releaseId: string; createdAt: string; active: boolean }
  type DeploymentEvent = { id: string; sequence: number; eventType: string; status: string; step: string; message: string; error: string; occurredAt: string }
  type DeploymentEventSnapshot = { deployment: Deployment; events: DeploymentEvent[]; nextSequence: number; hasMore: boolean }
  type EnvironmentLog = { id: string; message: string; stream: string; container: string; deployment: string; instance: string; release: string; occurredAt: string }
  type EnvironmentLogSnapshot = { logs: EnvironmentLog[]; nextCursor: string; hasMore: boolean }
  type Instance = { id: string; state: string; slot: string; ports: { host?: string; http?: number }; releaseId: string; observedAt: string }
  type TelemetryPoint = { observedAt: string; cpuCores: number; memoryBytes: number; diskReadBytesPerSecond: number; diskWriteBytesPerSecond: number; networkReceiveBytesPerSecond: number; networkTransmitBytesPerSecond: number; cpuAvailable: boolean; memoryAvailable: boolean; diskReadAvailable: boolean; diskWriteAvailable: boolean; networkReceiveAvailable: boolean; networkTransmitAvailable: boolean }
  type TelemetryRow = {
    application: string; environment: string; release: string; deployment: string; target: string; instance: string
    available: boolean; observedAt: string; cpuCores: number; memoryBytes: number
    diskReadBytesPerSecond: number; diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number; networkTransmitBytesPerSecond: number
    oomEvents: number; cpuThrottlingRatio: number; tasks: number; history: TelemetryPoint[]
    cpuAvailable: boolean; memoryAvailable: boolean; diskReadAvailable: boolean; diskWriteAvailable: boolean
    networkReceiveAvailable: boolean; networkTransmitAvailable: boolean; oomAvailable: boolean; cpuThrottlingAvailable: boolean; tasksAvailable: boolean
  }
  type Overview = {
    applicationId: string
    applicationName: string
    sourceType: 'buildpacks' | 'image'
    environment: { id: string; name: string; kind: string }
    repository: string
    reference: string
    contextPath: string
    registryName: string
    registryEndpoint: string
    runtimeServerId: string
    runtimeServer: string
    domain: string
    deployability: { deployable: boolean; missing: string[] }
    secrets: Secret[]
    variables: Variable[]
    resources: Resource[]
    builds: Build[]
    releases: Release[]
    deployments: Deployment[]
    instances: Instance[]
    apiTokenPrefix: string
  }
  let { auth, environment, telemetry }: { auth: { email: string }; environment: Overview; telemetry: TelemetryRow[] } = $props()
  let key = $state('')
  let value = $state('')
  let imageReference = $state(untrack(() => environment.reference))
  let apiToken = $state('')
  let apiTokenError = $state('')
  let apiTokenDialogOpen = $state(false)
  let apiTokenProcessing = $state(false)
  let liveBuilds = $state<Build[] | null>(null)
  let liveDeployments = $state<Deployment[] | null>(null)
  let buildLogs = $state<Record<string, BuildLog[]>>({})
  let buildLogCursors = $state<Record<string, number>>({})
  let expandedBuildId = $state('')
  let buildLogConnectionError = $state('')
  let deploymentEvents = $state<Record<string, DeploymentEvent[]>>({})
  let deploymentEventCursors = $state<Record<string, number>>({})
  let expandedDeploymentId = $state('')
  let deploymentEventConnectionError = $state('')
  let environmentLogs = $state<EnvironmentLog[]>([])
  let environmentLogCursor = $state('')
  let environmentLogsLoaded = $state(false)
  let environmentLogConnectionError = $state('')
  let environmentLogsPaused = $state(false)
  let followingEnvironmentLogs = $state(true)
  let environmentLogViewport: HTMLDivElement
  let activeReleaseDeployment = $state('')
  let activeBuildAction = $state('')
  let buildActionDialogOpen = $state(false)
  let pendingBuildAction = $state<{ action: 'start' | 'stop' | 'retry'; build: Build } | null>(null)
  let rotateDialogOpen = $state(false)
  let rotatingSecret = $state<Secret | null>(null)
  let rotatedSecretValue = $state('')
  let secretActionProcessing = $state(false)
  let secretActionError = $state('')
  const loadingBuilds = new Set<string>()
  const loadingDeployments = new Set<string>()
  const builds = $derived(liveBuilds ?? environment.builds)
  const deployments = $derived(liveDeployments ?? environment.deployments)
  const activeDeployment = $derived(deployments.find((deployment) => deployment.active) ?? null)
  const activeInstance = $derived(environment.instances.find((instance) => instance.state === 'serving') ?? null)
  const activeTelemetry = $derived(
    activeDeployment && activeInstance
      ? telemetry.find((row) => row.deployment === activeDeployment.id && row.instance === activeInstance.id) ?? null
      : null,
  )
  const activeBuildId = $derived(builds.find((build) => build.status === 'running')?.id ?? builds.find((build) => build.status === 'pending')?.id ?? '')
  const expandedDeploymentStatus = $derived(deployments.find((deployment) => deployment.id === expandedDeploymentId)?.status ?? '')
  const short = (value: string) => value ? value.slice(0, 12) : 'Unavailable'
  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value < 0) return 'Unavailable'
    if (value === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
    return `${(value / (1024 ** index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
  }
  const formatRate = (value: number) => `${formatBytes(value)}/s`
  const stamp = (value: string) => value ? new Date(value).toLocaleString() : 'Pending'
  const stepLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'waiting for worker'
  const deploymentStep = (deployment: Deployment) => deployment.active ? 'serving' : deployment.status === 'succeeded' ? 'superseded' : deployment.currentStep || 'queued'
  const secretStatusLabel = (secret: Secret) => secret.status === 'pending_removal' ? 'Pending removal' : secret.status[0].toUpperCase() + secret.status.slice(1)
  function createSecret() { router.post(routes.environmentSecretsCreate(environment.applicationId, environment.environment.id), { key, value }, { onSuccess: () => { key = ''; value = '' } }) }
  function askToRotate(secret: Secret) {
    rotatingSecret = secret
    rotatedSecretValue = ''
    secretActionError = ''
    rotateDialogOpen = true
  }
  function rotateSecret(event: SubmitEvent) {
    event.preventDefault()
    if (!rotatingSecret || rotatedSecretValue === '' || secretActionProcessing) return
    secretActionProcessing = true
    secretActionError = ''
    router.post(routes.environmentSecretRotate(environment.applicationId, environment.environment.id, rotatingSecret.id), { value: rotatedSecretValue }, {
      preserveScroll: true,
      onSuccess: () => {
        rotatedSecretValue = ''
        rotateDialogOpen = false
      },
      onError: (errors) => (secretActionError = Object.values(errors).map(String).join('\n') || 'The secret could not be rotated.'),
      onFinish: () => (secretActionProcessing = false),
    })
  }
  function buildAndDeploy() {
    router.post(routes.environmentDeploymentsCreate(environment.applicationId, environment.environment.id), environment.sourceType === 'image' ? { reference: imageReference } : {}, {
      onSuccess: () => {
        liveBuilds = null
        buildLogs = {}
        buildLogCursors = {}
        expandedBuildId = ''
      },
    })
  }
  async function rotateAPIToken() {
    apiTokenProcessing = true
    apiTokenError = ''
    try {
      const response = await window.fetch(routes.environmentAPITokenRotate(environment.applicationId, environment.environment.id), {
        method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json' },
      })
      const payload = await response.json() as { token?: string; error?: string }
      if (!response.ok || !payload.token) throw new Error(payload.error || 'API token could not be created')
      apiToken = payload.token
      apiTokenDialogOpen = true
      router.reload({ only: ['environment'], preserveScroll: true })
    } catch (error) {
      apiTokenError = error instanceof Error ? error.message : 'API token could not be created'
    } finally {
      apiTokenProcessing = false
    }
  }
  function redeployRelease(releaseId: string) {
    activeReleaseDeployment = releaseId
    router.post(routes.environmentReleaseDeploymentsCreate(environment.applicationId, environment.environment.id, releaseId), {}, {
      preserveScroll: true,
      onSuccess: () => {
        liveDeployments = null
        deploymentEvents = {}
        deploymentEventCursors = {}
        expandedDeploymentId = ''
      },
      onFinish: () => (activeReleaseDeployment = ''),
    })
  }
  function askForBuildAction(action: 'start' | 'stop' | 'retry', build: Build) {
    pendingBuildAction = { action, build }
    buildActionDialogOpen = true
  }
  function confirmBuildAction() {
    if (!pendingBuildAction) return
    const { action, build } = pendingBuildAction
    activeBuildAction = `${action}:${build.id}`
    const url = action === 'start'
      ? routes.environmentBuildStart(environment.applicationId, environment.environment.id, build.id)
      : action === 'stop'
        ? routes.environmentBuildStop(environment.applicationId, environment.environment.id, build.id)
        : routes.environmentBuildRetry(environment.applicationId, environment.environment.id, build.id)
    router.post(url, {}, {
      preserveScroll: true,
      onSuccess: () => (buildActionDialogOpen = false),
      onFinish: () => (activeBuildAction = ''),
    })
  }
  async function loadBuildLogs(buildId: string, signal?: AbortSignal) {
    if (loadingBuilds.has(buildId)) return null
    loadingBuilds.add(buildId)
    try {
      const after = buildLogCursors[buildId] ?? 0
      const response = await window.fetch(`${routes.environmentBuildLogs(environment.environment.id, buildId)}?after=${after}`, {
        cache: 'no-store',
        credentials: 'same-origin',
        headers: { Accept: 'application/json' },
        signal,
      })
      if (!response.ok) throw new Error(`Build logs returned ${response.status}`)
      const snapshot = (await response.json()) as BuildLogSnapshot
      liveBuilds = builds.map((build) => build.id === snapshot.build.id ? snapshot.build : build)
      if (snapshot.logs.length > 0) {
        buildLogs = { ...buildLogs, [buildId]: [...(buildLogs[buildId] ?? []), ...snapshot.logs] }
      } else if (!(buildId in buildLogs)) {
        buildLogs = { ...buildLogs, [buildId]: [] }
      }
      buildLogCursors = { ...buildLogCursors, [buildId]: snapshot.nextSequence }
      buildLogConnectionError = ''
      return snapshot
    } finally {
      loadingBuilds.delete(buildId)
    }
  }
  async function toggleBuildLogs(buildId: string) {
    if (expandedBuildId === buildId) {
      expandedBuildId = ''
      return
    }
    expandedBuildId = buildId
    if (!(buildId in buildLogs)) {
      try {
        let snapshot = await loadBuildLogs(buildId)
        while (snapshot?.hasMore) snapshot = await loadBuildLogs(buildId)
      } catch { buildLogConnectionError = 'Build logs are temporarily unavailable.' }
    }
  }

  async function loadDeploymentEvents(deploymentId: string, signal?: AbortSignal) {
    if (loadingDeployments.has(deploymentId)) return null
    loadingDeployments.add(deploymentId)
    try {
      const after = deploymentEventCursors[deploymentId] ?? 0
      const response = await window.fetch(`${routes.environmentDeploymentEvents(environment.environment.id, deploymentId)}?after=${after}`, {
        cache: 'no-store',
        credentials: 'same-origin',
        headers: { Accept: 'application/json' },
        signal,
      })
      if (!response.ok) throw new Error(`Deployment events returned ${response.status}`)
      const snapshot = (await response.json()) as DeploymentEventSnapshot
      liveDeployments = deployments.map((deployment) => {
        if (deployment.id === snapshot.deployment.id) return snapshot.deployment
        if (snapshot.deployment.active) return { ...deployment, active: false }
        return deployment
      })
      if (snapshot.events.length > 0) {
        deploymentEvents = { ...deploymentEvents, [deploymentId]: [...(deploymentEvents[deploymentId] ?? []), ...snapshot.events] }
      } else if (!(deploymentId in deploymentEvents)) {
        deploymentEvents = { ...deploymentEvents, [deploymentId]: [] }
      }
      deploymentEventCursors = { ...deploymentEventCursors, [deploymentId]: snapshot.nextSequence }
      deploymentEventConnectionError = ''
      return snapshot
    } finally {
      loadingDeployments.delete(deploymentId)
    }
  }

  async function toggleDeploymentEvents(deploymentId: string) {
    if (expandedDeploymentId === deploymentId) {
      expandedDeploymentId = ''
      return
    }
    expandedDeploymentId = deploymentId
    const deployment = deployments.find((item) => item.id === deploymentId)
    if (deployment?.status === 'queued' || deployment?.status === 'running') return
    if (!(deploymentId in deploymentEvents)) {
      try {
        let snapshot = await loadDeploymentEvents(deploymentId)
        while (snapshot?.hasMore) snapshot = await loadDeploymentEvents(deploymentId)
      } catch { deploymentEventConnectionError = 'Deployment events are temporarily unavailable.' }
    }
  }

  async function loadEnvironmentLogs(signal?: AbortSignal) {
    const endpoint = new URL(
      routes.environmentLogs(environment.applicationId, environment.environment.id),
      window.location.origin,
    )
    if (environmentLogCursor) endpoint.searchParams.set('after', environmentLogCursor)
    const response = await window.fetch(endpoint, {
      cache: 'no-store',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal,
    })
    if (!response.ok) throw new Error(`Environment logs returned ${response.status}`)
    const snapshot = (await response.json()) as EnvironmentLogSnapshot
    if (snapshot.logs.length > 0) {
      environmentLogs = [...environmentLogs, ...snapshot.logs].slice(-2000)
    }
    environmentLogCursor = snapshot.nextCursor
    environmentLogsLoaded = true
    environmentLogConnectionError = ''
    return snapshot
  }

  function updateEnvironmentLogFollow() {
    followingEnvironmentLogs = environmentLogViewport.scrollHeight
      - environmentLogViewport.scrollTop
      - environmentLogViewport.clientHeight < 48
  }

  $effect(() => {
    environmentLogs.length
    if (!followingEnvironmentLogs) return
    const frame = window.requestAnimationFrame(() => {
      environmentLogViewport?.scrollTo({ top: environmentLogViewport.scrollHeight })
    })
    return () => window.cancelAnimationFrame(frame)
  })

  $effect(() => {
    if (environmentLogsPaused) return
    const abortController = new AbortController()
    let timer: number | undefined
    let retryDelay = 2000

    async function poll() {
      try {
        const snapshot = await loadEnvironmentLogs(abortController.signal)
        if (abortController.signal.aborted) return
        retryDelay = 2000
        timer = window.setTimeout(poll, snapshot.hasMore ? 0 : retryDelay)
      } catch {
        if (abortController.signal.aborted) return
        environmentLogConnectionError = 'Reconnecting to the workload log stream...'
        retryDelay = Math.min(retryDelay * 2, 10000)
        timer = window.setTimeout(poll, retryDelay)
      }
    }

    timer = window.setTimeout(poll, 0)
    return () => {
      abortController.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  })

  $effect(() => {
    const buildId = activeBuildId
    if (!buildId) return
    if (!expandedBuildId) expandedBuildId = buildId

    const abortController = new AbortController()
    let timer: number | undefined
    let retryDelay = 1000

    async function poll() {
      try {
        const snapshot = await loadBuildLogs(buildId, abortController.signal)
        if (!snapshot || abortController.signal.aborted) return
        retryDelay = 1000
        if (snapshot.hasMore) {
          timer = window.setTimeout(poll, 0)
          return
        }
        if (snapshot.build.status !== 'pending' && snapshot.build.status !== 'running') {
          router.reload({ only: ['environment', 'telemetry'], preserveScroll: true })
          return
        }
      } catch {
        if (abortController.signal.aborted) return
        buildLogConnectionError = 'Reconnecting to the Build log...'
        retryDelay = Math.min(retryDelay * 2, 5000)
      }
      timer = window.setTimeout(poll, retryDelay)
    }

    timer = window.setTimeout(poll, 0)
    return () => {
      abortController.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  })

  $effect(() => {
    const deploymentId = expandedDeploymentId
    const deploymentStatus = expandedDeploymentStatus
    if (!deploymentId || (deploymentStatus !== 'queued' && deploymentStatus !== 'running')) return

    const abortController = new AbortController()
    let timer: number | undefined
    let retryDelay = 1000

    async function poll() {
      try {
        const snapshot = await loadDeploymentEvents(deploymentId, abortController.signal)
        if (!snapshot || abortController.signal.aborted) return
        retryDelay = 1000
        if (snapshot.hasMore) {
          timer = window.setTimeout(poll, 0)
          return
        }
        if (snapshot.deployment.status !== 'queued' && snapshot.deployment.status !== 'running') {
          router.reload({ only: ['environment', 'telemetry'], preserveScroll: true })
          return
        }
      } catch {
        if (abortController.signal.aborted) return
        deploymentEventConnectionError = 'Reconnecting to the Deployment timeline...'
        retryDelay = Math.min(retryDelay * 2, 5000)
      }
      timer = window.setTimeout(poll, retryDelay)
    }

    timer = window.setTimeout(poll, 0)
    return () => {
      abortController.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  })
</script>

<svelte:head><title>{environment.environment.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex items-end justify-between gap-4">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environment.applicationName} · {environment.environment.kind}</p><h1 class="mt-3 text-3xl font-semibold">{environment.environment.name}</h1></div>
      <div class="flex flex-wrap gap-2"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentEdit(environment.applicationId, environment.environment.id)}>Edit environment</Link>{/snippet}</Button><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(environment.applicationId, environment.environment.id)}>Edit source</Link>{/snippet}</Button>{#if environment.sourceType === 'buildpacks'}<Button onclick={buildAndDeploy}>Build & deploy</Button>{/if}<EnvironmentDeleteDialog applicationId={environment.applicationId} environmentId={environment.environment.id} environmentName={environment.environment.name} /></div>
    </header>

    <Card.Root><Card.Header><Card.Action><span class:text-success={environment.deployability.deployable} class:text-destructive={!environment.deployability.deployable}>{environment.deployability.deployable ? 'Ready' : 'Blocked'}</span></Card.Action><Card.Title>Desired state</Card.Title></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"><DataField label="Repository" value={environment.repository} /><DataField label="Reference" value={environment.reference} /><DataField label="Build context" value={environment.contextPath} /><DataField label="Domain" value={environment.domain} /><DataField label="Runtime Server" value={environment.runtimeServer} /><DataField label="Registry" value={environment.registryName} /><DataField label="Registry endpoint" value={environment.registryEndpoint} />{#if !environment.deployability.deployable}<DataField label="Missing" value={environment.deployability.missing.join(', ')} />{/if}</Card.Content></Card.Root>

    {#if environment.sourceType === 'image'}
      <Card.Root><Card.Header><Card.Title>Deploy image version</Card.Title><Card.Description>Use the configured reference or override it with another tag or sha256 digest.</Card.Description></Card.Header><Card.Content class="flex flex-col gap-3 sm:flex-row"><Input bind:value={imageReference} placeholder="latest" /><Button onclick={buildAndDeploy}>Resolve & deploy</Button></Card.Content></Card.Root>
    {/if}

    {#if environment.sourceType === 'image'}
      <Card.Root><Card.Header><Card.Action><Button size="sm" variant="outline" disabled={apiTokenProcessing} onclick={rotateAPIToken}>{apiTokenProcessing ? 'Creating...' : environment.apiTokenPrefix ? 'Rotate token' : 'Create token'}</Button></Card.Action><Card.Title>Deployment API</Card.Title><Card.Description>Environment-scoped bearer token for image deployments. A replacement token invalidates the previous token immediately.</Card.Description></Card.Header><Card.Content class="space-y-2 text-sm"><p class="font-mono">POST /api/environments/{environment.environment.id}/deployments</p><p class="text-xs text-muted-foreground">JSON: {`{"reference":"1.2.3"}`} · Token: {environment.apiTokenPrefix ? `${environment.apiTokenPrefix}...` : 'Not configured'}</p>{#if apiTokenError}<p class="text-xs text-destructive">{apiTokenError}</p>{/if}</Card.Content></Card.Root>
    {/if}

    <section aria-labelledby="workload-telemetry-heading" class="space-y-4">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="workload-telemetry-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Workload telemetry</h2>
          <p class="mt-1 text-xs text-muted-foreground">Current rates and 24-hour usage for the active container</p>
        </div>
        {#if activeTelemetry}
          <div class="text-left text-xs sm:text-right">
            <p class="font-medium">Instance {short(activeTelemetry.instance)}</p>
            <p class="mt-1 font-mono text-[11px] text-muted-foreground">Deployment {short(activeTelemetry.deployment)} · Release {short(activeTelemetry.release)}</p>
            <p class={`mt-1 ${activeTelemetry.available ? 'text-success' : 'text-warning'}`}>{activeTelemetry.available ? `Observed ${stamp(activeTelemetry.observedAt)}` : 'Stale'}</p>
          </div>
        {/if}
      </div>

      {#if activeTelemetry}
        <div class="grid gap-3 lg:grid-cols-2">
          <TelemetryDonut
            label="Disk throughput"
            primaryLabel="Read"
            secondaryLabel="Write"
            primary={activeTelemetry.diskReadBytesPerSecond}
            secondary={activeTelemetry.diskWriteBytesPerSecond}
            centerValue={activeTelemetry.diskReadBytesPerSecond + activeTelemetry.diskWriteBytesPerSecond}
            centerLabel="total"
            formatValue={formatRate}
            available={activeTelemetry.available && activeTelemetry.diskReadAvailable && activeTelemetry.diskWriteAvailable}
          />
          <TelemetryDonut
            label="Network throughput"
            primaryLabel="Receive"
            secondaryLabel="Transmit"
            primary={activeTelemetry.networkReceiveBytesPerSecond}
            secondary={activeTelemetry.networkTransmitBytesPerSecond}
            centerValue={activeTelemetry.networkReceiveBytesPerSecond + activeTelemetry.networkTransmitBytesPerSecond}
            centerLabel="total"
            formatValue={formatRate}
            available={activeTelemetry.available && activeTelemetry.networkReceiveAvailable && activeTelemetry.networkTransmitAvailable}
          />
        </div>
        <TelemetryHistory points={activeTelemetry.history} />
      {:else}
        <div class="border border-border bg-card/35 px-5 py-16 text-center">
          <p class="text-sm text-muted-foreground">No telemetry is available for the active container yet.</p>
        </div>
      {/if}
    </section>

    <Card.Root>
      <Card.Header>
        <Card.Action><Button size="sm" variant="outline" onclick={() => (environmentLogsPaused = !environmentLogsPaused)}>{environmentLogsPaused ? 'Resume' : 'Pause'}</Button></Card.Action>
        <Card.Title>Workload logs</Card.Title>
        <Card.Description>Live stdout and stderr from this Environment's containers. ClickHouse retains logs for seven days.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if environmentLogConnectionError}<p class="mb-3 text-xs text-warning">{environmentLogConnectionError}</p>{/if}
        <div
          bind:this={environmentLogViewport}
          onscroll={updateEnvironmentLogFollow}
          class="max-h-[32rem] min-h-48 overflow-auto border border-border bg-black/35 p-3 font-mono text-[11px] leading-relaxed"
        >
          {#each environmentLogs as log (log.id)}
            <div class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 py-1" class:text-destructive={log.stream === 'stderr'}>
              <span class="select-none whitespace-nowrap text-muted-foreground">{stamp(log.occurredAt)}</span>
              <div class="min-w-0">
                <p class="select-none text-[10px] text-muted-foreground">{log.stream} · {log.container || 'container'} · deployment {short(log.deployment)} · instance {short(log.instance)}</p>
                <pre class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
              </div>
            </div>
          {:else}
            <p class="text-muted-foreground">{environmentLogsLoaded ? 'No workload logs have been collected yet.' : 'Loading workload logs...'}</p>
          {/each}
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root><Card.Header><Card.Title>Resources</Card.Title><Card.Description>Explicit connections available to this Environment.</Card.Description></Card.Header><Card.Content class="space-y-2">{#each environment.resources as resource}<div class="grid gap-1 border border-border p-3 sm:grid-cols-3"><span class="font-mono text-sm">{resource.alias}</span><span>{resource.name}</span><span class="text-muted-foreground">{resource.engine}</span></div>{:else}<p class="text-sm text-muted-foreground">No Resources selected.</p>{/each}</Card.Content></Card.Root>

    <Card.Root><Card.Header><Card.Title>Environment variables</Card.Title><Card.Description>Secret status compares the desired value fingerprint with the revision running in the serving container.</Card.Description></Card.Header><Card.Content class="space-y-3">{#each environment.variables as variable}<div class="grid gap-1 border border-border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]"><p class="font-mono text-sm">{variable.key}</p><p class="break-all font-mono text-sm">{variable.value}</p><p class="text-xs text-muted-foreground">{variable.source}</p></div>{/each}{#each environment.secrets as secret}<div class="flex items-center justify-between gap-4 border border-border p-3"><div><div class="flex flex-wrap items-center gap-2"><p class="font-mono text-sm">{secret.key}</p><span class:text-success={secret.status === 'deployed'} class:text-warning={secret.status === 'deploying' || secret.status === 'pending' || secret.status === 'pending_removal'} class:text-destructive={secret.status === 'failed'} class="border border-border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider">{secretStatusLabel(secret)}</span></div><p class="font-mono text-sm">••••••••</p><p class="text-xs text-muted-foreground">{secret.digestPrefix} · {secret.sourceType}</p></div>{#if secret.desired && secret.sourceType === 'user'}<div class="flex gap-2"><Button size="sm" variant="outline" onclick={() => askToRotate(secret)}>Rotate</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.environmentSecretDestroy(environment.applicationId, environment.environment.id, secret.id))}>Archive</Button></div>{/if}</div>{/each}{#if environment.variables.length === 0 && environment.secrets.length === 0}<p class="text-sm text-muted-foreground">No Environment variables configured.</p>{/if}<div class="grid gap-3 border-t border-border pt-4 sm:grid-cols-[1fr_2fr_auto]"><Input bind:value={key} placeholder="SECRET_KEY" autocomplete="off" /><Input type="password" bind:value={value} placeholder="Write-only value" autocomplete="new-password" /><Button onclick={createSecret}>Add secret</Button></div></Card.Content></Card.Root>

    <div class="grid gap-8 xl:grid-cols-2">
      <Card.Root>
        <Card.Header><Card.Title>Builds</Card.Title><Card.Description>Builds run in the background. Active output is loaded automatically.</Card.Description></Card.Header>
        <Card.Content class="space-y-2">
          {#if buildLogConnectionError}<p class="text-xs text-warning">{buildLogConnectionError}</p>{/if}
          {#each builds as build}
            <div class="border border-border text-sm">
              <div class="flex items-start gap-2 p-3">
                <button type="button" class="min-w-0 flex-1 text-left" onclick={() => toggleBuildLogs(build.id)}>
                  <div class="flex justify-between gap-3"><span class="font-mono">{short(build.sourceRevision)}</span><span>{build.status}</span></div>
                  <p class="mt-1 text-xs text-muted-foreground">{stamp(build.createdAt)} · {stepLabel(build.currentStep)}</p>
                  <p class="mt-1 break-all font-mono text-[11px] text-muted-foreground">{build.registryEndpoint || 'Registry unavailable'}{build.jobId ? ` · Job #${build.jobId} · ${build.jobState}` : ''}</p>
                </button>
                <div class="flex shrink-0 flex-wrap justify-end gap-1">
                  {#if build.status === 'pending' || (build.status === 'running' && ['scheduled', 'retryable', 'pending'].includes(build.jobState))}<Button size="xs" disabled={Boolean(activeBuildAction)} onclick={() => askForBuildAction('start', build)}>{activeBuildAction === `start:${build.id}` ? 'Starting...' : 'Run now'}</Button>{/if}
                  {#if build.status === 'pending' || build.status === 'running'}<Button size="xs" variant="outline" disabled={Boolean(activeBuildAction)} onclick={() => askForBuildAction('stop', build)}>{activeBuildAction === `stop:${build.id}` ? 'Stopping...' : 'Stop'}</Button>{/if}
                  {#if build.status === 'failed' || build.status === 'cancelled'}<Button size="xs" variant="outline" disabled={Boolean(activeBuildAction)} onclick={() => askForBuildAction('retry', build)}>{activeBuildAction === `retry:${build.id}` ? 'Retrying...' : 'Retry'}</Button>{/if}
                </div>
              </div>
              {#if expandedBuildId === build.id}
                <div class="border-t border-border bg-black/30 p-3">
                  <div class="max-h-96 space-y-2 overflow-auto font-mono text-[11px] leading-relaxed">
                    {#each buildLogs[build.id] ?? [] as log (log.id)}
                      <div class:text-primary={log.stream === 'system'}>
                        <span class="select-none text-muted-foreground">{stamp(log.occurredAt)} · {log.stream}</span>
                        <pre class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                      </div>
                    {:else}
                      <p class="text-muted-foreground">Waiting for Build output...</p>
                    {/each}
                  </div>
                  {#if build.error}<pre class="mt-3 whitespace-pre-wrap break-words border-t border-destructive/30 pt-3 text-xs text-destructive">{build.error}</pre>{/if}
                </div>
              {/if}
            </div>
          {:else}<p class="text-sm text-muted-foreground">No Builds yet.</p>{/each}
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header><Card.Title>Releases</Card.Title><Card.Description>Redeploy an existing image with the current Environment secrets and configuration.</Card.Description></Card.Header>
        <Card.Content class="space-y-2">
          {#each environment.releases as release}
            <div class="border border-border p-3 text-sm">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0"><p class="font-mono">{short(release.sourceRevision)}</p><p class="mt-1 text-xs text-muted-foreground">{stamp(release.createdAt)}</p></div>
                <Button size="sm" variant="outline" disabled={Boolean(activeReleaseDeployment)} onclick={() => redeployRelease(release.id)}>{activeReleaseDeployment === release.id ? 'Queueing...' : 'Redeploy'}</Button>
              </div>
              <p class="mt-2 break-all font-mono text-xs text-muted-foreground">{release.artifactReference}</p>
            </div>
          {:else}<p class="text-sm text-muted-foreground">No Releases yet.</p>{/each}
        </Card.Content>
      </Card.Root>
    </div>

    <Card.Root>
      <Card.Header><Card.Title>Deployments</Card.Title><Card.Description>Durable blue-green rollout attempts and recovery actions.</Card.Description></Card.Header>
      <Card.Content class="space-y-2">
        {#if deploymentEventConnectionError}<p class="text-xs text-warning">{deploymentEventConnectionError}</p>{/if}
        {#each deployments as deployment}
          <div class="border border-border text-sm">
            <div class="flex items-start justify-between gap-4 p-3">
              <button type="button" class="min-w-0 flex-1 text-left" onclick={() => toggleDeploymentEvents(deployment.id)} aria-expanded={expandedDeploymentId === deployment.id}>
                <p><span class="font-mono">{short(deployment.id)}</span> · {deployment.status} · {deploymentStep(deployment)}</p>
                <p class="mt-1 text-xs text-muted-foreground">{stamp(deployment.createdAt)} · Release {short(deployment.releaseId)} · {expandedDeploymentId === deployment.id ? 'Hide timeline' : 'Show timeline'}</p>
                {#if deployment.error}<p class="mt-2 text-xs text-destructive">{deployment.error}</p>{/if}
              </button>
              {#if deployment.status === 'failed'}<Button size="sm" variant="outline" onclick={() => router.post(routes.environmentDeploymentRetry(environment.applicationId, environment.environment.id, deployment.id))}>Retry</Button>{/if}
            </div>
            {#if expandedDeploymentId === deployment.id}
              <div class="border-t border-border bg-muted/20 p-3">
                <div class="max-h-80 space-y-3 overflow-auto font-mono text-[11px] leading-relaxed">
                  {#each deploymentEvents[deployment.id] ?? [] as event (event.id)}
                    <div class:text-destructive={event.status === 'failed'} class:text-warning={event.status === 'warning'} class:text-success={event.status === 'succeeded'}>
                      <p class="select-none text-muted-foreground">{stamp(event.occurredAt)} · {event.step || event.eventType} · {event.status}</p>
                      <pre class="whitespace-pre-wrap break-words font-mono">{event.message}</pre>
                      {#if event.error && event.error !== event.message}<pre class="whitespace-pre-wrap break-words font-mono text-destructive">{event.error}</pre>{/if}
                    </div>
                  {:else}
                    <p class="text-muted-foreground">Waiting for Deployment events...</p>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {:else}
          <p class="text-sm text-muted-foreground">No Deployments yet.</p>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root><Card.Header><Card.Title>Active instance</Card.Title></Card.Header><Card.Content>{#if activeInstance}<div class="grid gap-1 border border-border p-3 text-sm sm:grid-cols-4"><span class="font-mono">{short(activeInstance.id)}</span><span>{activeInstance.state}</span><span>{activeInstance.slot}</span><span class="text-muted-foreground">{activeInstance.ports?.http ? `${activeInstance.ports.host || '127.0.0.1'}:${activeInstance.ports.http}` : 'No observed port'}</span></div>{:else}<p class="text-sm text-muted-foreground">No active Instance yet.</p>{/if}</Card.Content></Card.Root>
  </div>

  <ConfirmActionDialog
    bind:open={buildActionDialogOpen}
    title={pendingBuildAction?.action === 'stop' ? 'Stop Build?' : pendingBuildAction?.action === 'retry' ? 'Retry Build?' : 'Start Build?'}
    description={pendingBuildAction?.action === 'stop'
      ? 'Running Pack and Docker work will receive a cancellation signal.'
      : pendingBuildAction?.action === 'retry'
        ? `Retry this Build using its original source revision and ${pendingBuildAction.build.registryEndpoint} registry snapshot.`
        : 'Start this pending Build now.'}
    confirmLabel={pendingBuildAction?.action === 'stop' ? 'Stop Build' : pendingBuildAction?.action === 'retry' ? 'Retry Build' : 'Start Build'}
    destructive={pendingBuildAction?.action === 'stop'}
    processing={Boolean(activeBuildAction)}
    onconfirm={confirmBuildAction}
  />

  <Dialog.Root bind:open={rotateDialogOpen}>
    <Dialog.Content showCloseButton={!secretActionProcessing}>
      <form class="grid gap-4" onsubmit={rotateSecret}>
        <Dialog.Header><Dialog.Title>Rotate {rotatingSecret?.key ?? 'secret'}</Dialog.Title><Dialog.Description>Enter the replacement value. It remains visible while typing and takes effect on the next deployment.</Dialog.Description></Dialog.Header>
        <Input type="text" bind:value={rotatedSecretValue} autocomplete="off" autofocus required disabled={secretActionProcessing} />
        {#if secretActionError}<p class="whitespace-pre-wrap border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{secretActionError}</p>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={secretActionProcessing} onclick={() => (rotateDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={!rotatedSecretValue || secretActionProcessing}>{secretActionProcessing ? 'Rotating...' : 'Rotate secret'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={apiTokenDialogOpen}>
    <Dialog.Content><Dialog.Header><Dialog.Title>Deployment API token</Dialog.Title><Dialog.Description>Copy this token now. DeployCrate stores only its digest and cannot show it again.</Dialog.Description></Dialog.Header><pre class="whitespace-pre-wrap break-all border border-border bg-muted/30 p-3 font-mono text-xs">{apiToken}</pre><Dialog.Footer><Button variant="outline" onclick={() => navigator.clipboard.writeText(apiToken)}>Copy token</Button><Button onclick={() => (apiTokenDialogOpen = false)}>Done</Button></Dialog.Footer></Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
