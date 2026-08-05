<script lang="ts">
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import * as Empty from '@/Components/ui/empty'
  import * as NativeSelect from '@/Components/ui/native-select'
  import * as Table from '@/Components/ui/table'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import TelemetryHistory from '@/Components/System/TelemetryHistory.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'
  import { page, router } from '@inertiajs/svelte'

  type SystemIdentity = {
    applicationName: string
  }

  type ResourceUsage = {
    used: number
    free: number
  }

  type AttributedTelemetry = {
    scope: string
    component: string
    resourceName: string
    containerName: string
    application: string
    environment: string
    release: string
    deployment: string
    target: string
    instance: string
    resource: string
    installation: string
    available: boolean
    observedAt: string
    cpuCores: number
    memoryBytes: number
    diskReadBytesPerSecond: number
    diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number
    networkTransmitBytesPerSecond: number
    oomEvents: number
    cpuThrottlingRatio: number
    tasks: number
    cpuAvailable: boolean
    memoryAvailable: boolean
    diskReadAvailable: boolean
    diskWriteAvailable: boolean
    networkReceiveAvailable: boolean
    networkTransmitAvailable: boolean
    oomAvailable: boolean
    cpuThrottlingAvailable: boolean
    tasksAvailable: boolean
    history: AttributedTelemetryPoint[]
  }

  type AttributedTelemetryPoint = {
    observedAt: string
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

  type HostHistoryPoint = {
    observedAt: string
    cpuCores: number
    cpuCoresTotal: number
    diskReadBytesPerSecond: number
    diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number
    networkTransmitBytesPerSecond: number
    cpuAvailable: boolean
    diskReadAvailable: boolean
    diskWriteAvailable: boolean
    networkReceiveAvailable: boolean
    networkTransmitAvailable: boolean
  }

  type UsageHistoryPoint = {
    observedAt: string
    used: number
    free: number
  }

  type ChartSeries = {
    label: string
    points: Array<{ observedAt: string; value: number }>
    comparison?: boolean
  }

  type SystemTelemetry = {
    available: boolean
    observedAt: string
    cpu: ResourceUsage
    cpuCoresUsed: number
    cpuCoresTotal: number
    diskReadBytesPerSecond: number
    diskWriteBytesPerSecond: number
    networkReceiveBytesPerSecond: number
    networkTransmitBytesPerSecond: number
    oomEvents: number
    tasks: number
    diskReadAvailable: boolean
    diskWriteAvailable: boolean
    networkReceiveAvailable: boolean
    networkTransmitAvailable: boolean
    oomAvailable: boolean
    tasksAvailable: boolean
    memory: ResourceUsage
    storage: ResourceUsage
    hostHistory: HostHistoryPoint[]
    memoryHistory: UsageHistoryPoint[]
    storageHistory: UsageHistoryPoint[]
    platform: AttributedTelemetry[]
    systemContainers: AttributedTelemetry[]
  }

  type ApplicationTelemetry = {
    available: boolean
    observedAt: string
    windowSeconds: number
    requestsPerSecond: number
    serverErrorRate: number
    clientErrorRate: number
    meanRequestDurationMs: number
    runtimeMemoryBytes: number
    heapAllocatedBytes: number
    heapAllocations: number
    heapGoalBytes: number
    goroutines: number
    history: ApplicationTelemetryPoint[]
    database: DatabaseTelemetry
    recentTraces: TraceSummary[]
  }

  type ApplicationTelemetryPoint = {
    observedAt: string
    requestsPerSecond: number
    clientErrorsPerSecond: number
    serverErrorsPerSecond: number
    p50DurationMs: number
    p95DurationMs: number
    p99DurationMs: number
  }

  type DatabaseTelemetry = {
    available: boolean
    observedAt: string
    operationsPerSecond: number
    errorsPerSecond: number
    p95DurationMs: number
    history: DatabaseTelemetryPoint[]
  }

  type DatabaseTelemetryPoint = {
    observedAt: string
    operationsPerSecond: number
    errorsPerSecond: number
    p50DurationMs: number
    p95DurationMs: number
    p99DurationMs: number
  }

  type TraceSummary = {
    traceId: string
    rootSpanName: string
    startedAt: string
    durationNs: number
    spanCount: number
    errorCount: number
  }

  type TraceSpan = {
    traceId: string
    spanId: string
    parentSpanId: string
    name: string
    kind: string
    serviceName: string
    scope: string
    statusCode: string
    statusMessage: string
    resourceAttributes: Record<string, string>
    spanAttributes: Record<string, string>
    startedAt: string
    durationNs: number
  }

  type SystemLog = {
    id: string
    message: string
    severity: string
    severityNumber: number
    attributes: Record<string, string>
    traceId: string
    spanId: string
    scope: string
    source: string
    line: string
    instance: string
    slot: string
    occurredAt: string
  }

  type SystemLogSnapshot = {
    logs: SystemLog[]
    nextCursor: string
    hasMore: boolean
  }

  type TelemetryView = 'overview' | 'services' | 'logs' | 'traces'

  let { auth, system, telemetry, applicationTelemetry, telemetryRange }: {
    auth: { email: string }
    system: SystemIdentity
    telemetry: SystemTelemetry
    applicationTelemetry: ApplicationTelemetry
    telemetryRange: '1h' | '6h' | '24h' | '7d'
  } = $props()

  let systemLogs = $state<SystemLog[]>([])
  let systemLogCursor = $state('')
  let systemLogsLoaded = $state(false)
  let systemLogsPaused = $state(false)
  let systemLogConnectionError = $state('')
  let followingSystemLogs = $state(true)
  let systemLogViewport = $state<HTMLDivElement>()
  let traceDialogOpen = $state(false)
  let selectedTraceID = $state('')
  let traceSpans = $state<TraceSpan[]>([])
  let traceLoading = $state(false)
  let traceError = $state('')
  let selectedAttributionID = $state('')
  let attributionComparison = $state<'host' | 'service'>('host')
  let live = $state(false)

  const activeView = $derived.by<TelemetryView>(() => {
    const view = new URLSearchParams($page.url.split('?')[1] ?? '').get('view')
    return view === 'services' || view === 'logs' || view === 'traces' ? view : 'overview'
  })
  const telemetryHref = (view: TelemetryView, range: string) => {
    const query = new URLSearchParams({ view, range })
    return `${routes.systemTelemetry()}?${query.toString()}`
  }

  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value < 0) return 'Unavailable'
    if (value === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
    return `${(value / (1024 ** index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
  }
  const formatRate = (value: number) => `${formatBytes(value)}/s`
  const formatCores = (value: number) => `${value.toFixed(2)} cores`
  const formatCount = (value: number) => value.toFixed(0)
  const formatPerSecond = (value: number) => `${value.toFixed(value < 1 ? 2 : 1)}/s`
  const formatPercent = (value: number) => `${(value * 100).toFixed(2)}%`
  const formatDuration = (milliseconds: number) => milliseconds < 1 ? `${(milliseconds * 1000).toFixed(0)} µs` : `${milliseconds.toFixed(1)} ms`
  const formatSpanDuration = (nanoseconds: number) => formatDuration(nanoseconds / 1_000_000)
  const short = (value: string) => value ? value.slice(0, 8) : 'Unknown'
  const stamp = (value: string) => value ? new Date(value).toLocaleString() : 'Unknown'
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const current = (row: AttributedTelemetry, metricAvailable: boolean, value: string) => row.available && metricAvailable ? value : 'Unavailable'
  const systemContainerName = (row: AttributedTelemetry) => {
    if (row.resourceName) return row.resourceName
    if (row.installation) return `Managed resource ${short(row.resource)}`
    return label(row.component)
  }
  const systemContainerIdentity = (row: AttributedTelemetry) => {
    if (row.containerName) return row.containerName
    if (row.installation) return `Installation ${short(row.installation)}`
    return 'System container'
  }

  const platform = $derived(telemetry.platform ?? [])
  const systemContainers = $derived(telemetry.systemContainers ?? [])
  const hostHistory = $derived(telemetry.hostHistory ?? [])
  const memoryHistory = $derived(telemetry.memoryHistory ?? [])
  const storageHistory = $derived(telemetry.storageHistory ?? [])
  const applicationHistory = $derived(applicationTelemetry.history ?? [])
  const databaseHistory = $derived(applicationTelemetry.database?.history ?? [])
  const recentTraces = $derived(applicationTelemetry.recentTraces ?? [])
  const rangeSeconds = $derived(({ '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800 })[telemetryRange] ?? 86400)
  const rangeLabel = $derived(({ '1h': 'last hour', '6h': 'last 6 hours', '24h': 'last 24 hours', '7d': 'last 7 days' })[telemetryRange] ?? 'last 24 hours')
  const attributionRows = $derived([...platform, ...systemContainers])
  const rowName = (row: AttributedTelemetry) => {
    if (row.scope === 'native') return label(row.component)
    if (row.resourceName) return row.resourceName
    if (row.installation) return `Managed resource ${short(row.resource)}`
    return label(row.component)
  }
  const attributionID = (row: AttributedTelemetry) => JSON.stringify([
    row.scope,
    row.component,
    row.application,
    row.environment,
    row.release,
    row.deployment,
    row.target,
    row.instance,
    row.resource,
    row.installation,
  ])
  const attributionOptions = $derived(attributionRows.map((row) => ({
    id: attributionID(row),
    label: rowName(row),
    scope: row.scope === 'native' ? 'Native service' : 'System container',
  })))
  const selectedAttributionRow = $derived(attributionRows.find((row) => attributionID(row) === selectedAttributionID))
  const selectedAttributionLabel = $derived(selectedAttributionRow ? rowName(selectedAttributionRow) : 'No service selected')
  const attributedSeries = (metric: 'cpu' | 'memory'): ChartSeries[] => attributionRows.flatMap((row) => {
    if (attributionID(row) !== selectedAttributionID) return []
    const points = (row.history ?? []).flatMap((point) => {
      if (metric === 'cpu' && point.cpuAvailable) return [{ observedAt: point.observedAt, value: point.cpuCores }]
      if (metric === 'memory' && point.memoryAvailable) return [{ observedAt: point.observedAt, value: point.memoryBytes }]
      return []
    })
    return points.length ? [{ label: rowName(row), points }] : []
  })
  const hostCPUSeries = $derived<ChartSeries[]>([{ label: 'Host CPU', points: hostHistory.filter((point) => point.cpuAvailable).map((point) => ({ observedAt: point.observedAt, value: point.cpuCores })) }])
  const hostMemorySeries = $derived<ChartSeries[]>([{ label: 'Host memory', points: memoryHistory.map((point) => ({ observedAt: point.observedAt, value: point.used })) }])
  const hostDiskSeries = $derived<ChartSeries[]>([
    { label: 'Read', points: hostHistory.filter((point) => point.diskReadAvailable).map((point) => ({ observedAt: point.observedAt, value: point.diskReadBytesPerSecond })) },
    { label: 'Write', points: hostHistory.filter((point) => point.diskWriteAvailable).map((point) => ({ observedAt: point.observedAt, value: point.diskWriteBytesPerSecond })) },
  ])
  const hostNetworkSeries = $derived<ChartSeries[]>([
    { label: 'Receive', points: hostHistory.filter((point) => point.networkReceiveAvailable).map((point) => ({ observedAt: point.observedAt, value: point.networkReceiveBytesPerSecond })) },
    { label: 'Transmit', points: hostHistory.filter((point) => point.networkTransmitAvailable).map((point) => ({ observedAt: point.observedAt, value: point.networkTransmitBytesPerSecond })) },
  ])
  const applicationTrafficSeries = $derived<ChartSeries[]>([
    { label: 'Requests', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.requestsPerSecond })) },
    { label: 'Server errors', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.serverErrorsPerSecond })) },
    { label: 'Client errors', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.clientErrorsPerSecond })) },
  ])
  const applicationLatencySeries = $derived<ChartSeries[]>([
    { label: 'p50', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.p50DurationMs })) },
    { label: 'p95', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.p95DurationMs })) },
    { label: 'p99', points: applicationHistory.map((point) => ({ observedAt: point.observedAt, value: point.p99DurationMs })) },
  ])
  const databaseActivitySeries = $derived<ChartSeries[]>([
    { label: 'Operations', points: databaseHistory.map((point) => ({ observedAt: point.observedAt, value: point.operationsPerSecond })) },
    { label: 'Errors', points: databaseHistory.map((point) => ({ observedAt: point.observedAt, value: point.errorsPerSecond })) },
  ])
  const databaseLatencySeries = $derived<ChartSeries[]>([
    { label: 'p50', points: databaseHistory.map((point) => ({ observedAt: point.observedAt, value: point.p50DurationMs })) },
    { label: 'p95', points: databaseHistory.map((point) => ({ observedAt: point.observedAt, value: point.p95DurationMs })) },
    { label: 'p99', points: databaseHistory.map((point) => ({ observedAt: point.observedAt, value: point.p99DurationMs })) },
  ])
  const applicationService = $derived(platform.find((row) => row.application) ?? platform.find((row) => row.component === 'deploycrate-ce'))
  const applicationCPUSeries = $derived<ChartSeries[]>(applicationService ? [{
    label: 'CPU', points: applicationService.history.filter((point) => point.cpuAvailable).map((point) => ({ observedAt: point.observedAt, value: point.cpuCores })),
  }] : [])
  const applicationMemorySeries = $derived<ChartSeries[]>(applicationService ? [{
    label: 'Memory', points: applicationService.history.filter((point) => point.memoryAvailable).map((point) => ({ observedAt: point.observedAt, value: point.memoryBytes })),
  }] : [])
  const hostCapacitySeries = $derived<ChartSeries[]>([
    { label: 'CPU', points: hostHistory.filter((point) => point.cpuAvailable && point.cpuCoresTotal > 0).map((point) => ({ observedAt: point.observedAt, value: point.cpuCores / point.cpuCoresTotal * 100 })) },
    { label: 'Memory', points: memoryHistory.filter((point) => point.used + point.free > 0).map((point) => ({ observedAt: point.observedAt, value: point.used / (point.used + point.free) * 100 })) },
    { label: 'Disk', points: storageHistory.filter((point) => point.used + point.free > 0).map((point) => ({ observedAt: point.observedAt, value: point.used / (point.used + point.free) * 100 })) },
  ])
  const healthIssues = $derived.by(() => {
    const issues: string[] = []
    if (!telemetry.available) issues.push('Host telemetry is stale or unavailable.')
    if (!applicationTelemetry.available) issues.push('Application telemetry is stale or unavailable.')
    if (applicationTelemetry.serverErrorRate >= 0.01) issues.push(`Server errors are ${formatPercent(applicationTelemetry.serverErrorRate)} of recent requests.`)
    if (applicationTelemetry.database?.errorsPerSecond > 0) issues.push(`PostgreSQL is reporting ${formatPerSecond(applicationTelemetry.database.errorsPerSecond)} failed operations.`)
    const memoryTotal = telemetry.memory.used + telemetry.memory.free
    if (memoryTotal > 0 && telemetry.memory.free / memoryTotal < 0.1) issues.push('Host memory has less than 10% available.')
    const storageTotal = telemetry.storage.used + telemetry.storage.free
    if (storageTotal > 0 && telemetry.storage.free / storageTotal < 0.1) issues.push('Root disk has less than 10% available.')
    if (telemetry.oomAvailable && telemetry.oomEvents > 0) issues.push('The host reported an out-of-memory event.')
    return issues
  })
  const healthLabel = $derived(healthIssues.length === 0 ? 'Healthy' : healthIssues.length === 1 ? 'Needs attention' : 'Degraded')
  const attributedCPUSeries = $derived<ChartSeries[]>(selectedAttributionRow ? [
    ...attributedSeries('cpu'),
    ...(attributionComparison === 'host' ? hostCPUSeries.map((series) => ({ ...series, label: 'Host total', comparison: true })) : []),
  ] : [])
  const attributedMemorySeries = $derived<ChartSeries[]>(selectedAttributionRow ? [
    ...attributedSeries('memory'),
    ...(attributionComparison === 'host' ? hostMemorySeries.map((series) => ({ ...series, label: 'Host total', comparison: true })) : []),
  ] : [])
  const focusedCurrent = (available: boolean | undefined, value: string) => selectedAttributionRow
    ? current(selectedAttributionRow, available === true, value)
    : 'Unavailable'

  $effect(() => {
    if (attributionOptions.some((option) => option.id === selectedAttributionID)) return
    selectedAttributionID = attributionOptions[0]?.id ?? ''
  })

  const systemLogLevel = (log: SystemLog) => {
    if (log.severity) return log.severity.toUpperCase()
    if (log.severityNumber >= 17) return 'ERROR'
    if (log.severityNumber >= 13) return 'WARN'
    if (log.severityNumber >= 9) return 'INFO'
    return 'DEBUG'
  }
  const systemLogSource = (log: SystemLog) => {
    if (log.source) return log.line ? `${log.source}:${log.line}` : log.source
    return log.scope || 'application'
  }
  const systemLogContext = (log: SystemLog) => Object.entries(log.attributes ?? {})
    .filter(([key, value]) => value && !key.startsWith('code.')
      && (key !== 'trace_id' || !log.traceId) && (key !== 'span_id' || !log.spanId))
    .sort(([left], [right]) => left.localeCompare(right))

  async function loadSystemLogs(signal?: AbortSignal) {
    const endpoint = new URL(routes.systemTelemetryLogs(), window.location.origin)
    if (systemLogCursor) endpoint.searchParams.set('after', systemLogCursor)
    const response = await window.fetch(endpoint, {
      cache: 'no-store',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal,
    })
    if (!response.ok) throw new Error(`System logs returned ${response.status}`)
    const snapshot = (await response.json()) as SystemLogSnapshot
    if (snapshot.logs.length > 0) systemLogs = [...systemLogs, ...snapshot.logs].slice(-2000)
    systemLogCursor = snapshot.nextCursor
    systemLogsLoaded = true
    systemLogConnectionError = ''
    return snapshot
  }

  async function loadTrace(traceID: string) {
    selectedTraceID = traceID
    traceDialogOpen = true
    traceSpans = []
    traceError = ''
    traceLoading = true
    try {
      const response = await window.fetch(routes.systemTelemetryTrace(traceID), {
        cache: 'no-store',
        credentials: 'same-origin',
        headers: { Accept: 'application/json' },
      })
      if (!response.ok) throw new Error(`Trace returned ${response.status}`)
      traceSpans = ((await response.json()) as { spans: TraceSpan[] }).spans
    } catch {
      traceError = 'This trace could not be loaded.'
    } finally {
      traceLoading = false
    }
  }

  function closeTrace() {
    selectedTraceID = ''
    traceSpans = []
    traceError = ''
  }

  function updateSystemLogFollow() {
    if (!systemLogViewport) return
    followingSystemLogs = systemLogViewport.scrollHeight
      - systemLogViewport.scrollTop
      - systemLogViewport.clientHeight < 48
  }

  $effect(() => {
    systemLogs.length
    if (!followingSystemLogs) return
    const frame = window.requestAnimationFrame(() => {
      systemLogViewport?.scrollTo({ top: systemLogViewport.scrollHeight })
    })
    return () => window.cancelAnimationFrame(frame)
  })

  $effect(() => {
    if (activeView !== 'logs' || systemLogsPaused) return
    const abortController = new AbortController()
    let timer: number | undefined
    let retryDelay = 2000

    async function poll() {
      try {
        const snapshot = await loadSystemLogs(abortController.signal)
        if (abortController.signal.aborted) return
        retryDelay = 2000
        timer = window.setTimeout(poll, snapshot.hasMore ? 0 : retryDelay)
      } catch {
        if (abortController.signal.aborted) return
        systemLogConnectionError = 'Reconnecting to the DeployCrate CE log stream...'
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
    if (!live) return
    let refreshing = false
    const refresh = () => {
      if (refreshing || document.visibilityState !== 'visible') return
      refreshing = true
      router.reload({
        only: ['telemetry', 'applicationTelemetry'],
        preserveScroll: true,
        preserveState: true,
        onFinish: () => (refreshing = false),
      })
    }
    refresh()
    const timer = window.setInterval(refresh, 3000)
    return () => window.clearInterval(timer)
  })
</script>

<svelte:head><title>System telemetry</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-col gap-5 xl:flex-row xl:items-end xl:justify-between">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">System health</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Application, PostgreSQL, and host telemetry for {system.applicationName}.
        </p>
      </div>
      <div class="flex flex-wrap gap-1 xl:justify-end" aria-label="Telemetry time range">
        {#each [{ value: '1h', label: '1h' }, { value: '6h', label: '6h' }, { value: '24h', label: '24h' }, { value: '7d', label: '7d' }] as option}
          <Button
            size="sm"
            variant={telemetryRange === option.value ? 'default' : 'outline'}
            aria-pressed={telemetryRange === option.value}
            href={telemetryHref(activeView, option.value)}
          >{option.label}</Button>
        {/each}
        <Button size="sm" variant={live ? 'default' : 'outline'} aria-pressed={live} onclick={() => (live = !live)}>
          <span class={`size-1.5 rounded-full ${live ? 'bg-primary-foreground animate-pulse' : 'bg-muted-foreground'}`}></span>
          Live
        </Button>
      </div>
    </header>

    <nav class="flex flex-wrap gap-2 border-b border-border pb-3" aria-label="Telemetry views">
      <Button size="sm" variant={activeView === 'overview' ? 'default' : 'ghost'} href={telemetryHref('overview', telemetryRange)}>Overview</Button>
      <Button size="sm" variant={activeView === 'services' ? 'default' : 'ghost'} href={telemetryHref('services', telemetryRange)}>Services</Button>
      <Button size="sm" variant={activeView === 'logs' ? 'default' : 'ghost'} href={telemetryHref('logs', telemetryRange)}>Logs</Button>
      <Button size="sm" variant={activeView === 'traces' ? 'default' : 'ghost'} href={telemetryHref('traces', telemetryRange)}>Traces</Button>
    </nav>

    {#if activeView === 'overview'}
      <section aria-labelledby="health-summary-heading">
        <Card.Root class={healthIssues.length === 0 ? 'border-success/40' : 'border-warning/50'}>
          <Card.Header>
            <Card.Title id="health-summary-heading" class="flex items-center gap-2">
              <span class={`size-2 rounded-full ${healthIssues.length === 0 ? 'bg-success' : 'bg-warning'}`}></span>
              {healthLabel}
            </Card.Title>
            <Card.Description>Health signals from {rangeLabel}. Synthetic health-check traffic is excluded.</Card.Description>
          </Card.Header>
          <Card.Content>
            {#if healthIssues.length}
              <ul class="grid gap-2 text-sm">
                {#each healthIssues as issue}<li>{issue}</li>{/each}
              </ul>
            {:else}
              <p class="text-sm text-muted-foreground">Application, PostgreSQL, and host signals are current with no detected pressure or errors.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </section>

      <section aria-labelledby="application-health-heading" class="space-y-4">
        <div>
          <h2 id="application-health-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Application</h2>
          <p class="mt-1 text-sm text-muted-foreground">Traffic, errors, and tail latency for DeployCrate CE</p>
        </div>
        <div class="grid gap-4 xl:grid-cols-2">
          <TelemetryHistory label="Traffic and errors" description={`${applicationTelemetry.requestsPerSecond.toFixed(2)} requests/s now`} series={applicationTrafficSeries} formatValue={formatPerSecond} windowSeconds={rangeSeconds} />
          <TelemetryHistory label="Response latency" description={`Recent mean ${formatDuration(applicationTelemetry.meanRequestDurationMs)}`} series={applicationLatencySeries} formatValue={formatDuration} windowSeconds={rangeSeconds} />
        </div>
      </section>

      <section aria-labelledby="database-health-heading" class="space-y-4">
        <div>
          <h2 id="database-health-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">PostgreSQL</h2>
          <p class="mt-1 text-sm text-muted-foreground">Database operations emitted by the system application</p>
        </div>
        <div class="grid gap-4 xl:grid-cols-2">
          <TelemetryHistory label="Database activity" description={applicationTelemetry.database?.available ? `${formatPerSecond(applicationTelemetry.database.operationsPerSecond)} now` : 'No recent database operations'} series={databaseActivitySeries} formatValue={formatPerSecond} windowSeconds={rangeSeconds} />
          <TelemetryHistory label="Database latency" description={applicationTelemetry.database?.available ? `p95 ${formatDuration(applicationTelemetry.database.p95DurationMs)}` : 'No recent database latency samples'} series={databaseLatencySeries} formatValue={formatDuration} windowSeconds={rangeSeconds} />
        </div>
      </section>

      <section aria-labelledby="capacity-health-heading" class="space-y-4">
        <div>
          <h2 id="capacity-health-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Resources</h2>
          <p class="mt-1 text-sm text-muted-foreground">Application consumption and remaining host capacity</p>
        </div>
        <div class="grid gap-4 xl:grid-cols-2">
          <TelemetryHistory label="Application CPU" description={applicationService?.cpuAvailable ? formatCores(applicationService.cpuCores) : 'Current usage unavailable'} series={applicationCPUSeries} formatValue={formatCores} windowSeconds={rangeSeconds} />
          <TelemetryHistory label="Application memory" description={applicationService?.memoryAvailable ? formatBytes(applicationService.memoryBytes) : 'Current usage unavailable'} series={applicationMemorySeries} formatValue={formatBytes} windowSeconds={rangeSeconds} />
          <div class="xl:col-span-2"><TelemetryHistory label="Host capacity used" description="CPU, memory, and root disk utilization" series={hostCapacitySeries} formatValue={(value) => `${value.toFixed(0)}%`} windowSeconds={rangeSeconds} maximum={100} /></div>
        </div>
      </section>
    {/if}

    {#if activeView === 'services'}
    <section aria-labelledby="attribution-heading" class="space-y-4">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
        <div>
          <h2 id="attribution-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">System service attribution</h2>
          <p class="mt-1 text-sm text-muted-foreground">Focus the graphs on one service and optionally compare it with total host usage</p>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="grid gap-1.5 text-xs font-medium">
            <span class="text-muted-foreground">Service</span>
            <NativeSelect.Root class="w-full min-w-64 [&_select]:h-9 [&_select]:text-sm" bind:value={selectedAttributionID} disabled={attributionOptions.length === 0} aria-label="Select system service">
              {#each attributionOptions as option}
                <NativeSelect.Option value={option.id}>{option.label} · {option.scope}</NativeSelect.Option>
              {/each}
            </NativeSelect.Root>
          </label>
          <label class="grid gap-1.5 text-xs font-medium">
            <span class="text-muted-foreground">Comparison</span>
            <NativeSelect.Root class="w-full min-w-44 [&_select]:h-9 [&_select]:text-sm" bind:value={attributionComparison} aria-label="Choose graph comparison">
              <NativeSelect.Option value="host">Show host total</NativeSelect.Option>
              <NativeSelect.Option value="service">Service only</NativeSelect.Option>
            </NativeSelect.Root>
          </label>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card.Root><Card.Header><Card.Title class="text-sm">CPU usage</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{focusedCurrent(selectedAttributionRow?.cpuAvailable, formatCores(selectedAttributionRow?.cpuCores ?? 0))}</p><p class="mt-1 text-xs text-muted-foreground">Current usage for {selectedAttributionLabel}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Memory usage</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{focusedCurrent(selectedAttributionRow?.memoryAvailable, formatBytes(selectedAttributionRow?.memoryBytes ?? 0))}</p><p class="mt-1 text-xs text-muted-foreground">Current working set for {selectedAttributionLabel}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Disk throughput</Card.Title></Card.Header><Card.Content><p class="text-sm"><span class="text-muted-foreground">Read</span> {focusedCurrent(selectedAttributionRow?.diskReadAvailable, formatRate(selectedAttributionRow?.diskReadBytesPerSecond ?? 0))}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Write</span> {focusedCurrent(selectedAttributionRow?.diskWriteAvailable, formatRate(selectedAttributionRow?.diskWriteBytesPerSecond ?? 0))}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Network throughput</Card.Title></Card.Header><Card.Content><p class="text-sm"><span class="text-muted-foreground">Receive</span> {focusedCurrent(selectedAttributionRow?.networkReceiveAvailable, formatRate(selectedAttributionRow?.networkReceiveBytesPerSecond ?? 0))}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Transmit</span> {focusedCurrent(selectedAttributionRow?.networkTransmitAvailable, formatRate(selectedAttributionRow?.networkTransmitBytesPerSecond ?? 0))}</p></Card.Content></Card.Root>
      </div>

      <div class="grid gap-4 xl:grid-cols-2">
        <TelemetryHistory label="CPU usage" description={`${selectedAttributionLabel}${attributionComparison === 'host' ? ' compared with total host CPU' : ''}`} series={attributedCPUSeries} formatValue={formatCores} windowSeconds={rangeSeconds} />
        <TelemetryHistory label="Memory usage" description={`${selectedAttributionLabel}${attributionComparison === 'host' ? ' compared with total host memory' : ''}`} series={attributedMemorySeries} formatValue={formatBytes} windowSeconds={rangeSeconds} />
      </div>

      <Card.Root>
        <Card.Header><Card.Title>Platform services</Card.Title><Card.Description>Native services managed as part of the DeployCrate control plane. Network attribution is not collected for native services.</Card.Description></Card.Header>
        <Card.Content class="p-0">
          {#if platform.length}
            <div class="overflow-x-auto">
              <Table.Root class="min-w-[900px] text-xs">
                <Table.Header class="border-y border-border bg-muted/30"><Table.Row><Table.Head>Service</Table.Head><Table.Head>CPU</Table.Head><Table.Head>Memory</Table.Head><Table.Head>Disk read / write</Table.Head><Table.Head>Network</Table.Head><Table.Head>Tasks</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each platform as row (attributionID(row))}
                    <Table.Row>
                      <Table.Cell><div class="flex items-center gap-2"><p class="font-medium text-sm">{label(row.component)}</p>{#if !row.available}<StatusBadge status="stale" label="Stale" />{/if}</div>{#if row.installation}<p class="mt-1 font-mono text-[11px] text-muted-foreground">Installation {short(row.installation)}</p>{/if}</Table.Cell>
                      <Table.Cell>{current(row, row.cpuAvailable, formatCores(row.cpuCores))}</Table.Cell><Table.Cell>{current(row, row.memoryAvailable, formatBytes(row.memoryBytes))}</Table.Cell>
                      <Table.Cell>{current(row, row.diskReadAvailable, formatRate(row.diskReadBytesPerSecond))} / {current(row, row.diskWriteAvailable, formatRate(row.diskWriteBytesPerSecond))}</Table.Cell>
                      <Table.Cell class="text-muted-foreground">Not collected</Table.Cell><Table.Cell>{current(row, row.tasksAvailable, formatCount(row.tasks))}</Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </div>
          {:else}<Empty.Root class="py-10"><Empty.Header><Empty.Title>No platform telemetry</Empty.Title><Empty.Description>Native service samples will appear after the collector reports them.</Empty.Description></Empty.Header></Empty.Root>{/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>System containers</Card.Title><Card.Description>Containerized services managed as part of the DeployCrate system.</Card.Description></Card.Header>
        <Card.Content class="p-0">
          {#if systemContainers.length}
            <div class="overflow-x-auto">
              <Table.Root class="min-w-[1180px] text-xs">
                <Table.Header class="border-y border-border bg-muted/30"><Table.Row><Table.Head>Service</Table.Head><Table.Head>CPU</Table.Head><Table.Head>Memory</Table.Head><Table.Head>Disk read / write</Table.Head><Table.Head>Network receive / transmit</Table.Head><Table.Head>Tasks</Table.Head><Table.Head>Throttling / OOM</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each systemContainers as row (attributionID(row))}
                    <Table.Row>
                      <Table.Cell><div class="flex items-center gap-2"><p class="font-medium text-sm">{systemContainerName(row)}</p>{#if !row.available}<StatusBadge status="stale" label="Stale" />{/if}</div><p class="mt-1 font-mono text-[11px] text-muted-foreground">{systemContainerIdentity(row)}</p></Table.Cell>
                      <Table.Cell>{current(row, row.cpuAvailable, formatCores(row.cpuCores))}</Table.Cell><Table.Cell>{current(row, row.memoryAvailable, formatBytes(row.memoryBytes))}</Table.Cell>
                      <Table.Cell>{current(row, row.diskReadAvailable, formatRate(row.diskReadBytesPerSecond))} / {current(row, row.diskWriteAvailable, formatRate(row.diskWriteBytesPerSecond))}</Table.Cell>
                      <Table.Cell>{current(row, row.networkReceiveAvailable, formatRate(row.networkReceiveBytesPerSecond))} / {current(row, row.networkTransmitAvailable, formatRate(row.networkTransmitBytesPerSecond))}</Table.Cell>
                      <Table.Cell>{current(row, row.tasksAvailable, formatCount(row.tasks))}</Table.Cell><Table.Cell>{current(row, row.cpuThrottlingAvailable, `${(row.cpuThrottlingRatio * 100).toFixed(1)}%`)} / {current(row, row.oomAvailable, formatCount(row.oomEvents))}</Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </div>
          {:else}<Empty.Root class="py-10"><Empty.Header><Empty.Title>No container telemetry</Empty.Title><Empty.Description>System container samples will appear after the collector reports them.</Empty.Description></Empty.Header></Empty.Root>{/if}
        </Card.Content>
      </Card.Root>
    </section>
    {/if}

    {#if activeView === 'logs'}
    <section aria-labelledby="deploycrate-logs-heading">
      <Card.Root>
        <Card.Header>
          <Card.Action><Button size="sm" variant="outline" onclick={() => (systemLogsPaused = !systemLogsPaused)}>{systemLogsPaused ? 'Resume' : 'Pause'}</Button></Card.Action>
          <Card.Title id="deploycrate-logs-heading">DeployCrate CE logs</Card.Title>
          <Card.Description>Live structured application logs from OpenTelemetry. ClickHouse retains logs for seven days.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if systemLogConnectionError}<p class="mb-3 text-xs text-warning">{systemLogConnectionError}</p>{/if}
          <div
            bind:this={systemLogViewport}
            onscroll={updateSystemLogFollow}
            class="max-h-[36rem] min-h-48 overflow-auto border border-border bg-black/35 p-3 font-mono text-[11px] leading-relaxed"
          >
            {#each systemLogs as log (log.id)}
              <div
                class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 py-1"
                class:text-warning={log.severityNumber >= 13 && log.severityNumber < 17}
                class:text-destructive={log.severityNumber >= 17}
              >
                <span class="select-none whitespace-nowrap text-muted-foreground">{stamp(log.occurredAt)}</span>
                <div class="min-w-0">
                  <p class="select-none text-[10px] text-muted-foreground">
                    {systemLogLevel(log)} · {log.slot || 'slot unknown'} · {systemLogSource(log)}{#if log.traceId} · <Button variant="link" size="xs" class="h-auto p-0 font-mono text-[10px]" onclick={() => loadTrace(log.traceId)}>trace {short(log.traceId)}</Button>{/if}{#if log.spanId} · span {short(log.spanId)}{/if}{#if log.instance} · instance {short(log.instance)}{/if}
                  </p>
                  <pre class="whitespace-pre-wrap break-words font-mono">{log.message}</pre>
                  {#if systemLogContext(log).length > 0}
                    <dl class="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-muted-foreground">
                      {#each systemLogContext(log) as [key, value] (key)}
                        <div class="flex min-w-0 gap-1">
                          <dt class="shrink-0">{key}=</dt>
                          <dd class="break-all text-foreground/80">{value}</dd>
                        </div>
                      {/each}
                    </dl>
                  {/if}
                </div>
              </div>
            {:else}
              <p class="text-muted-foreground">{systemLogsLoaded ? 'No DeployCrate CE logs have been collected yet.' : 'Loading DeployCrate CE logs...'}</p>
            {/each}
          </div>
        </Card.Content>
      </Card.Root>
    </section>
    {/if}

    {#if activeView === 'traces'}
      <section aria-labelledby="recent-traces-heading">
        <Card.Root>
          <Card.Header>
            <Card.Title id="recent-traces-heading">Recent traces</Card.Title>
            <Card.Description>Up to 100 application traces from {rangeLabel}. Select one to inspect every retained span.</Card.Description>
          </Card.Header>
          <Card.Content class="p-0">
            {#if recentTraces.length}
              <div class="overflow-x-auto">
                <Table.Root class="min-w-[780px] text-xs">
                  <Table.Header class="border-y border-border bg-muted/30"><Table.Row><Table.Head>Started</Table.Head><Table.Head>Root span</Table.Head><Table.Head>Duration</Table.Head><Table.Head>Spans</Table.Head><Table.Head>Errors</Table.Head><Table.Head>Trace</Table.Head></Table.Row></Table.Header>
                  <Table.Body>
                    {#each recentTraces as trace (trace.traceId)}
                      <Table.Row>
                        <Table.Cell class="whitespace-nowrap">{stamp(trace.startedAt)}</Table.Cell>
                        <Table.Cell class="font-medium">{trace.rootSpanName || 'Unknown root span'}</Table.Cell>
                        <Table.Cell>{formatSpanDuration(trace.durationNs)}</Table.Cell>
                        <Table.Cell>{trace.spanCount}</Table.Cell>
                        <Table.Cell>{#if trace.errorCount > 0}<span class="text-destructive">{trace.errorCount}</span>{:else}0{/if}</Table.Cell>
                        <Table.Cell><Button variant="link" size="xs" class="h-auto p-0 font-mono" onclick={() => loadTrace(trace.traceId)}>{short(trace.traceId)}</Button></Table.Cell>
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              </div>
            {:else}
              <Empty.Root class="py-12"><Empty.Header><Empty.Title>No traces in this range</Empty.Title><Empty.Description>Traces will appear after application requests are sampled.</Empty.Description></Empty.Header></Empty.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </section>
    {/if}

    <Dialog.Root bind:open={traceDialogOpen} onOpenChange={(open) => { if (!open) closeTrace() }}>
      <Dialog.Content class="sm:max-w-6xl">
        <Dialog.Header>
          <Dialog.Title>Trace {selectedTraceID}</Dialog.Title>
          <Dialog.Description>Correlated OpenTelemetry spans across every service that contributed to this trace.</Dialog.Description>
        </Dialog.Header>
        {#if traceLoading}<p class="py-6 text-sm text-muted-foreground">Loading trace...</p>
        {:else if traceError}<p class="py-6 text-sm text-destructive">{traceError}</p>
        {:else if traceSpans.length}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[920px] text-xs">
              <Table.Header class="border-b border-border bg-muted/30"><Table.Row><Table.Head>Started</Table.Head><Table.Head>Service</Table.Head><Table.Head>Span</Table.Head><Table.Head>Span ID / parent</Table.Head><Table.Head>Duration</Table.Head><Table.Head>Status</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each traceSpans as span (span.spanId)}
                  <Table.Row>
                    <Table.Cell class="whitespace-nowrap">{stamp(span.startedAt)}</Table.Cell><Table.Cell><p class="font-medium">{span.serviceName}</p><p class="mt-1 text-muted-foreground">{span.kind || span.scope}</p></Table.Cell>
                    <Table.Cell class="font-medium">{span.name}</Table.Cell><Table.Cell class="font-mono"><p>{span.spanId}</p><p class="mt-1 text-muted-foreground">{span.parentSpanId || 'root'}</p></Table.Cell>
                    <Table.Cell>{formatSpanDuration(span.durationNs)}</Table.Cell><Table.Cell><StatusBadge status={span.statusCode || 'unset'} />{#if span.statusMessage}<p class="mt-1 text-muted-foreground">{span.statusMessage}</p>{/if}</Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {:else}<p class="py-6 text-sm text-muted-foreground">No spans were retained for this trace.</p>{/if}
      </Dialog.Content>
    </Dialog.Root>
  </div>
</DashboardLayout>
