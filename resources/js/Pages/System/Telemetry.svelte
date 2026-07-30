<script lang="ts">
  import * as Card from '@/Components/ui/card'
  import TelemetryHistory from '@/Components/System/TelemetryHistory.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

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

  let { auth, system, telemetry }: {
    auth: { email: string }
    system: SystemIdentity
    telemetry: SystemTelemetry
  } = $props()

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
  const short = (value: string) => value ? value.slice(0, 8) : 'Unknown'
  const stamp = (value: string) => value ? new Date(value).toLocaleString() : 'Unknown'
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const current = (row: AttributedTelemetry, metricAvailable: boolean, value: string) => row.available && metricAvailable ? value : 'Unavailable'
  const systemContainerName = (row: AttributedTelemetry) => {
    if (row.installation) return `Managed resource ${short(row.resource)}`
    return label(row.component)
  }
  const systemContainerIdentity = (row: AttributedTelemetry) => {
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
      </div>
    </header>

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
              <table class="w-full min-w-[900px] text-left text-xs">
                <thead class="border-y border-border bg-muted/30 text-muted-foreground"><tr><th class="px-5 py-3">Service</th><th class="px-5 py-3">State</th><th class="px-5 py-3">CPU</th><th class="px-5 py-3">Memory</th><th class="px-5 py-3">Disk read / write</th><th class="px-5 py-3">Network</th><th class="px-5 py-3">Tasks</th></tr></thead>
                <tbody>
                  {#each platform as row (`${row.component}:${row.installation}`)}
                    <tr class="border-b border-border last:border-0">
                      <td class="px-5 py-4"><p class="font-medium text-sm">{label(row.component)}</p>{#if row.installation}<p class="mt-1 font-mono text-[11px] text-muted-foreground">Installation {short(row.installation)}</p>{/if}</td>
                      <td class="px-5 py-4"><span class={row.available ? 'text-success' : 'text-warning'}>{row.available ? `Observed ${stamp(row.observedAt)}` : 'Stale'}</span></td>
                      <td class="px-5 py-4">{current(row, row.cpuAvailable, formatCores(row.cpuCores))}</td>
                      <td class="px-5 py-4">{current(row, row.memoryAvailable, formatBytes(row.memoryBytes))}</td>
                      <td class="px-5 py-4">{current(row, row.diskReadAvailable, formatRate(row.diskReadBytesPerSecond))} / {current(row, row.diskWriteAvailable, formatRate(row.diskWriteBytesPerSecond))}</td>
                      <td class="px-5 py-4 text-muted-foreground">Not collected</td>
                      <td class="px-5 py-4">{current(row, row.tasksAvailable, formatCount(row.tasks))}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}<p class="p-6 text-sm text-muted-foreground">No platform telemetry is available yet.</p>{/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>System containers</Card.Title><Card.Description>Containerized services managed as part of the DeployCrate system.</Card.Description></Card.Header>
        <Card.Content class="p-0">
          {#if systemContainers.length}
            <div class="overflow-x-auto">
              <table class="w-full min-w-[1180px] text-left text-xs">
                <thead class="border-y border-border bg-muted/30 text-muted-foreground"><tr><th class="px-5 py-3">Service</th><th class="px-5 py-3">State</th><th class="px-5 py-3">CPU</th><th class="px-5 py-3">Memory</th><th class="px-5 py-3">Disk read / write</th><th class="px-5 py-3">Network receive / transmit</th><th class="px-5 py-3">Tasks</th><th class="px-5 py-3">Throttling / OOM</th></tr></thead>
                <tbody>
                  {#each systemContainers as row (`${row.component}:${row.resource}:${row.installation}`)}
                    <tr class="border-b border-border last:border-0">
                      <td class="px-5 py-4"><p class="font-medium text-sm">{systemContainerName(row)}</p><p class="mt-1 font-mono text-[11px] text-muted-foreground">{systemContainerIdentity(row)}</p></td>
                      <td class="px-5 py-4"><span class={row.available ? 'text-success' : 'text-warning'}>{row.available ? `Observed ${stamp(row.observedAt)}` : 'Stale'}</span></td>
                      <td class="px-5 py-4">{current(row, row.cpuAvailable, formatCores(row.cpuCores))}</td>
                      <td class="px-5 py-4">{current(row, row.memoryAvailable, formatBytes(row.memoryBytes))}</td>
                      <td class="px-5 py-4">{current(row, row.diskReadAvailable, formatRate(row.diskReadBytesPerSecond))} / {current(row, row.diskWriteAvailable, formatRate(row.diskWriteBytesPerSecond))}</td>
                      <td class="px-5 py-4">{current(row, row.networkReceiveAvailable, formatRate(row.networkReceiveBytesPerSecond))} / {current(row, row.networkTransmitAvailable, formatRate(row.networkTransmitBytesPerSecond))}</td>
                      <td class="px-5 py-4">{current(row, row.tasksAvailable, formatCount(row.tasks))}</td>
                      <td class="px-5 py-4">{current(row, row.cpuThrottlingAvailable, `${(row.cpuThrottlingRatio * 100).toFixed(1)}%`)} / {current(row, row.oomAvailable, formatCount(row.oomEvents))}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}<p class="p-6 text-sm text-muted-foreground">No system container telemetry is available yet.</p>{/if}
        </Card.Content>
      </Card.Root>
    </section>
  </div>
</DashboardLayout>
