<script lang="ts">
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import * as Table from '@/Components/ui/table'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import TelemetryHistory from '@/Components/System/TelemetryHistory.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type SystemIdentity = {
    applicationName: string
    serverName: string
    serverAddress: string
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

  let { auth, system, telemetry, applicationTelemetry, collectorEndpoint }: {
    auth: { email: string }
    system: SystemIdentity
    telemetry: SystemTelemetry
    applicationTelemetry: ApplicationTelemetry
    collectorEndpoint: string
  } = $props()

  let systemLogs = $state<SystemLog[]>([])
  let systemLogCursor = $state('')
  let systemLogsLoaded = $state(false)
  let systemLogsPaused = $state(false)
  let systemLogConnectionError = $state('')
  let followingSystemLogs = $state(true)
  let systemLogViewport: HTMLDivElement
  let selectedTraceID = $state('')
  let traceSpans = $state<TraceSpan[]>([])
  let traceLoading = $state(false)
  let traceError = $state('')

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
  const currentRows = $derived([...platform, ...systemContainers].filter((row) => row.available))
  const currentCPURows = $derived(currentRows.filter((row) => row.cpuAvailable))
  const currentMemoryRows = $derived(currentRows.filter((row) => row.memoryAvailable))
  const attributedCPUCores = $derived(currentCPURows.reduce((total, row) => total + row.cpuCores, 0))
  const attributedMemoryBytes = $derived(currentMemoryRows.reduce((total, row) => total + row.memoryBytes, 0))
  const unattributedCPUCores = $derived(Math.max(0, telemetry.cpuCoresUsed - attributedCPUCores))
  const unattributedMemoryBytes = $derived(Math.max(0, telemetry.memory.used - attributedMemoryBytes))
  const hostHistory = $derived(telemetry.hostHistory ?? [])
  const memoryHistory = $derived(telemetry.memoryHistory ?? [])
  const attributionRows = $derived([...platform, ...systemContainers])
  const rowName = (row: AttributedTelemetry) => {
    if (row.scope === 'native') return label(row.component)
    if (row.resourceName) return row.resourceName
    if (row.installation) return `Managed resource ${short(row.resource)}`
    return label(row.component)
  }
  const attributedSeries = (metric: 'cpu' | 'memory'): ChartSeries[] => attributionRows.flatMap((row) => {
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
  const attributedCPUSeries = $derived(attributedSeries('cpu'))
  const attributedMemorySeries = $derived(attributedSeries('memory'))

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
    .filter(([key, value]) => value && key !== 'code.file.path' && key !== 'code.line.number'
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

  function updateSystemLogFollow() {
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
    if (systemLogsPaused) return
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
</script>

<svelte:head><title>System telemetry</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">Telemetry</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Extended host throughput and resource attribution for {system.applicationName}.
        </p>
      </div>
      <div class="text-left text-xs text-muted-foreground sm:text-right">
        <p class="font-medium text-foreground">{system.serverName}</p>
        <p class="mt-1 font-mono">{system.serverAddress}</p>
        {#if telemetry.available}<p class="mt-1">Observed {stamp(telemetry.observedAt)}</p>{/if}
        <p class="mt-2">OTLP/HTTP over WireGuard</p>
        <p class="mt-1 font-mono text-foreground">{collectorEndpoint}</p>
      </div>
    </header>

    <section aria-labelledby="application-telemetry-heading">
      <div class="mb-4">
        <h2 id="application-telemetry-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Application signals</h2>
        <p class="mt-1 text-xs text-muted-foreground">OpenTelemetry HTTP and Go runtime metrics for the DeployCrate CE application</p>
      </div>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card.Root><Card.Header><Card.Title class="text-sm">Request rate</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{applicationTelemetry.available ? `${applicationTelemetry.requestsPerSecond.toFixed(2)} req/s` : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Rolling OpenTelemetry collection window</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">HTTP errors</Card.Title></Card.Header><Card.Content><p class="text-sm"><span class="text-muted-foreground">Server</span> {applicationTelemetry.available ? formatPercent(applicationTelemetry.serverErrorRate) : 'Unavailable'}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Client</span> {applicationTelemetry.available ? formatPercent(applicationTelemetry.clientErrorRate) : 'Unavailable'}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Request duration</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{applicationTelemetry.available ? formatDuration(applicationTelemetry.meanRequestDurationMs) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Mean server request duration</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Go runtime</Card.Title></Card.Header><Card.Content><p class="text-sm"><span class="text-muted-foreground">Heap allocated</span> {applicationTelemetry.available ? formatBytes(applicationTelemetry.heapAllocatedBytes) : 'Unavailable'}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Goroutines</span> {applicationTelemetry.available ? formatCount(applicationTelemetry.goroutines) : 'Unavailable'}</p></Card.Content></Card.Root>
      </div>
    </section>

    <section aria-labelledby="host-throughput-heading">
      <div class="mb-4">
        <h2 id="host-throughput-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Host throughput</h2>
        <p class="mt-1 text-xs text-muted-foreground">Current rates and counters reported by the system server</p>
      </div>
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card.Root>
          <Card.Header><Card.Title class="text-sm">CPU cores</Card.Title></Card.Header>
          <Card.Content><p class="text-2xl font-semibold">{telemetry.available ? formatCores(telemetry.cpuCoresUsed) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">{telemetry.available ? `${telemetry.cpuCoresTotal.toFixed(0)} total cores` : 'No fresh host sample'}</p></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header><Card.Title class="text-sm">Disk throughput</Card.Title></Card.Header>
          <Card.Content><p class="text-sm"><span class="text-muted-foreground">Read</span> {telemetry.diskReadAvailable ? formatRate(telemetry.diskReadBytesPerSecond) : 'Unavailable'}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Write</span> {telemetry.diskWriteAvailable ? formatRate(telemetry.diskWriteBytesPerSecond) : 'Unavailable'}</p></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header><Card.Title class="text-sm">Network throughput</Card.Title></Card.Header>
          <Card.Content><p class="text-sm"><span class="text-muted-foreground">Receive</span> {telemetry.networkReceiveAvailable ? formatRate(telemetry.networkReceiveBytesPerSecond) : 'Unavailable'}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">Transmit</span> {telemetry.networkTransmitAvailable ? formatRate(telemetry.networkTransmitBytesPerSecond) : 'Unavailable'}</p></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header><Card.Title class="text-sm">Host activity</Card.Title></Card.Header>
          <Card.Content><p class="text-sm"><span class="text-muted-foreground">Tasks</span> {telemetry.tasksAvailable ? formatCount(telemetry.tasks) : 'Unavailable'}</p><p class="mt-2 text-sm"><span class="text-muted-foreground">OOM events</span> {telemetry.oomAvailable ? formatCount(telemetry.oomEvents) : 'Unavailable'}</p></Card.Content>
        </Card.Root>
      </div>
    </section>

    <section aria-labelledby="host-history-heading" class="space-y-4">
      <div>
        <h2 id="host-history-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Host history</h2>
        <p class="mt-1 text-xs text-muted-foreground">Twelve two-hour buckets from the last 24 hours</p>
      </div>
      <div class="grid gap-4 xl:grid-cols-2">
        <TelemetryHistory label="CPU usage" description="Host CPU cores in use" series={hostCPUSeries} formatValue={formatCores} />
        <TelemetryHistory label="Memory usage" description="Host memory working set" series={hostMemorySeries} formatValue={formatBytes} />
        <TelemetryHistory label="Disk throughput" description="Host read and write rates" series={hostDiskSeries} formatValue={formatRate} />
        <TelemetryHistory label="Network throughput" description="Host receive and transmit rates" series={hostNetworkSeries} formatValue={formatRate} />
      </div>
    </section>

    <section aria-labelledby="attribution-heading" class="space-y-4">
      <div>
        <h2 id="attribution-heading" class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">System service attribution</h2>
        <p class="mt-1 text-xs text-muted-foreground">Host totals compared with current system service samples</p>
      </div>

      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Card.Root><Card.Header><Card.Title class="text-sm">Attributed CPU</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{currentCPURows.length ? formatCores(attributedCPUCores) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Across {currentCPURows.length} current services</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Estimated other host CPU</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{telemetry.available && currentCPURows.length ? formatCores(unattributedCPUCores) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Directional difference, not exact reconciliation</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Attributed memory</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{currentMemoryRows.length ? formatBytes(attributedMemoryBytes) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Across {currentMemoryRows.length} current working sets</p></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title class="text-sm">Estimated other host memory</Card.Title></Card.Header><Card.Content><p class="text-2xl font-semibold">{telemetry.available && currentMemoryRows.length ? formatBytes(unattributedMemoryBytes) : 'Unavailable'}</p><p class="mt-1 text-xs text-muted-foreground">Directional difference, not exact reconciliation</p></Card.Content></Card.Root>
      </div>

      <div class="grid gap-4 xl:grid-cols-2">
        <TelemetryHistory label="Attributed CPU" description="CPU usage by system service" series={attributedCPUSeries} formatValue={formatCores} />
        <TelemetryHistory label="Attributed memory" description="Working set by system service" series={attributedMemorySeries} formatValue={formatBytes} />
      </div>

      <Card.Root>
        <Card.Header><Card.Title>Platform services</Card.Title><Card.Description>Native services managed as part of the DeployCrate control plane. Network attribution is not collected for native services.</Card.Description></Card.Header>
        <Card.Content class="p-0">
          {#if platform.length}
            <div class="overflow-x-auto">
              <Table.Root class="min-w-[900px] text-xs">
                <Table.Header class="border-y border-border bg-muted/30"><Table.Row><Table.Head>Service</Table.Head><Table.Head>CPU</Table.Head><Table.Head>Memory</Table.Head><Table.Head>Disk read / write</Table.Head><Table.Head>Network</Table.Head><Table.Head>Tasks</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each platform as row (`${row.component}:${row.installation}`)}
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
                  {#each systemContainers as row (`${row.component}:${row.resource}:${row.installation}`)}
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

    {#if selectedTraceID}
      <section aria-labelledby="trace-heading">
        <Card.Root>
          <Card.Header>
            <Card.Action><Button size="sm" variant="ghost" onclick={() => { selectedTraceID = ''; traceSpans = []; traceError = '' }}>Close</Button></Card.Action>
            <Card.Title id="trace-heading">Trace {selectedTraceID}</Card.Title>
            <Card.Description>Correlated OpenTelemetry spans across every service that contributed to this trace.</Card.Description>
          </Card.Header>
          <Card.Content class="p-0">
            {#if traceLoading}<p class="p-6 text-sm text-muted-foreground">Loading trace...</p>
            {:else if traceError}<p class="p-6 text-sm text-destructive">{traceError}</p>
            {:else if traceSpans.length}
              <div class="overflow-x-auto">
                <Table.Root class="min-w-[920px] text-xs">
                  <Table.Header class="border-y border-border bg-muted/30"><Table.Row><Table.Head>Started</Table.Head><Table.Head>Service</Table.Head><Table.Head>Span</Table.Head><Table.Head>Span ID / parent</Table.Head><Table.Head>Duration</Table.Head><Table.Head>Status</Table.Head></Table.Row></Table.Header>
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
            {:else}<p class="p-6 text-sm text-muted-foreground">No spans were retained for this trace.</p>{/if}
          </Card.Content>
        </Card.Root>
      </section>
    {/if}
  </div>
</DashboardLayout>
