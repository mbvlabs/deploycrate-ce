<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { toast } from 'svelte-sonner'
  import { onMount } from 'svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Checkbox } from '@/Components/ui/checkbox'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import * as Dialog from '@/Components/ui/dialog'
  import DataField from '@/Components/DataField.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Textarea } from '@/Components/ui/textarea'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type EnvironmentKey = { name: string; label: string; defaultKey: string }
  type Engine = { engine: string; label: string; resourceType: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; environmentKeys: EnvironmentKey[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { engines: Engine[]; servers: Array<{ id: string; name: string; address: string }>; privateNetworks: PrivateNetwork[]; registryCredentials: Array<{ id: string; name: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }
  type BackupDestination = { id: string; name: string; provider: string; endpoint: string; region: string; bucket: string; prefix: string; verifiedAt: string | null; lastUsedAt: string | null }
  type BackupPolicy = { id: string; schedule: string; active: boolean; nextRunAt: string; backupDestinationId: string; keepLast: number; keepDaily: number; keepWeekly: number; keepMonthly: number }
  type BackupHistory = { id: string; status: string; triggerType: string; scheduledAt: string; finishedAt: string | null; verifiedAt: string | null; sizeBytes: number | null; error: string; canRestore: boolean }
  type RestoreHistory = { id: string; status: string; requestedAt: string; startedAt: string | null; finishedAt: string | null; verifiedAt: string | null; cutoverAt: string | null; rolledBackAt: string | null; error: string; backupId: string; backupScheduledAt: string; safetyBackupId: string | null }
  type DatabaseBackups = { databaseName: string; eligibility: { eligible: boolean; reason: string; installationId: string | null }; policy: BackupPolicy | null; history: BackupHistory[]; restores: RestoreHistory[]; activeRestore: boolean }
  type Backups = { destinations: BackupDestination[]; databases: DatabaseBackups[] }
  type DestructiveAction =
    | { kind: 'remove-container'; installationId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'disable-private-access'; title: string; description: string; confirmationLabel: string }
    | { kind: 'revoke-device'; deviceId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-resource'; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-backup-policy'; databaseName: string; policyId: string; title: string; description: string; confirmationLabel: string }
  let { auth, resource, backups, options, enrollment = null, errors = {} }: { auth: { email: string }; resource: any; backups: Backups; options: Options; enrollment?: Enrollment | null; errors?: Record<string, string> } = $props()
  let selectedBackupDatabaseName = $state('')

  const definition = $derived(options.engines.find((kind) => kind.engine === resource.engine) ?? options.engines[0])
  const endpointNetworks = $derived(resource.installations.length === 1
    ? options.privateNetworks.filter((network) => network.serverIds.includes(resource.installations[0].serverId) && Boolean(network.serverAddresses[resource.installations[0].serverId]))
    : options.privateNetworks)
  const primaryEndpoint = $derived(resource.endpoints.find((item: any) => item.role === 'primary' && !item.privateNetworkId))
  const privateEndpoint = $derived(resource.endpoints.find((item: any) => Boolean(item.privateNetworkId)))
  const privateNetwork = $derived(privateEndpoint ? options.privateNetworks.find((item) => item.id === privateEndpoint.privateNetworkId) : undefined)
  const administratorCredentials = $derived(resource.credentials.filter((item: any) => item.metadata?.purpose === 'administrator'))
  const applicationCredentials = $derived(resource.credentials.filter((item: any) => item.metadata?.purpose === 'application'))
	const databaseBacked = $derived(resource.resourceType === 'database')
	const managedPostgreSQL = $derived(resource.engine === 'postgresql')
  const containerRunning = $derived(resource.installations.some((item: any) => item.serviceState === 'running'))
  const canAddApplicationUser = $derived((!databaseBacked || resource.databases.length > 0) && (!managedPostgreSQL || containerRunning))
  const selectedBackups = $derived(backups.databases.find((item) => item.databaseName === selectedBackupDatabaseName) ?? backups.databases[0])
  const lastSuccessfulBackup = $derived(selectedBackups?.history.find((item) => item.status === 'verified'))
  const activeRestore = $derived(Boolean(selectedBackups?.activeRestore))
  const overallStatus = $derived.by(() => {
		if (databaseBacked) {
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'unhealthy')) return { label: 'Unhealthy', tone: 'bad', detail: 'The published Database access check reached its failure threshold.' }
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'degraded')) return { label: 'Degraded', tone: 'warn', detail: 'The published Database access check is failing below its threshold.' }
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'healthy')) return { label: 'Available', tone: 'good', detail: 'The published endpoint and application credential can access the Database.' }
			return { label: 'Access unknown', tone: 'neutral', detail: 'No fresh Database access observation is available.' }
		}
    if (resource.installations.length === 0) return { label: 'Not deployed', tone: 'neutral', detail: 'No runtime installation is configured.' }
    if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'unhealthy')) return { label: 'Unhealthy', tone: 'bad', detail: 'A Resource health check reached its failure threshold.' }
    if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'degraded')) return { label: 'Degraded', tone: 'warn', detail: 'A Resource health check is failing below its failure threshold.' }
    if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'unknown')) return { label: 'Health unknown', tone: 'neutral', detail: 'A Resource health check has no fresh observation.' }
    if (resource.installations.some((item: any) => item.health === 'unhealthy')) return { label: 'Degraded', tone: 'bad', detail: 'At least one installation reports an unhealthy container.' }
    const running = resource.installations.filter((item: any) => item.serviceState === 'running').length
    if (running === resource.installations.length) return { label: 'Running', tone: 'good', detail: 'All installations are running.' }
    if (running > 0) return { label: 'Partially running', tone: 'warn', detail: `${running} of ${resource.installations.length} installations are running.` }
    if (resource.installations.some((item: any) => item.serviceState === 'exited' || item.serviceState === 'stopped')) return { label: 'Stopped', tone: 'warn', detail: 'The configured runtime is not running.' }
    if (resource.installations.every((item: any) => item.state === 'missing')) return { label: 'Not deployed', tone: 'neutral', detail: 'No container has been created for this Resource.' }
    return { label: 'Unknown', tone: 'neutral', detail: 'DeployCrate could not confirm the current runtime state.' }
  })

  let endpointDialogOpen = $state(false)
  let privateAccessDialogOpen = $state(false)
  let credentialDialogOpen = $state(false)
  let databaseDialogOpen = $state(false)
  let volumeDialogOpen = $state(false)
  let mountDialogOpen = $state(false)
  let healthDialogOpen = $state(false)
  let connectionKeysDialogOpen = $state(false)
  let destructiveActionDialogOpen = $state(false)
  let restoreDialogOpen = $state(false)
  let wireGuardConfigurationDialogOpen = $state(false)
  let containerLogsDialogOpen = $state(false)
  let containerLogsInstallation = $state<any>(null)
  let containerLogs = $state('')
  let containerLogsError = $state('')
  let containerLogsLoading = $state(false)
  let jsonError = $state('')
  let pendingAction = $state('')
  let dialogAction = $state('')
  let destructiveProcessing = $state(false)
  let destructiveError = $state('')
  let destructiveAction = $state<DestructiveAction | null>(null)
  let shownEnrollmentGrantId = $state('')
  let endpoint = $state(initialEndpoint())
  let credential = $state({ name: 'Application user', username: '', database: '', secretValues: {} as Record<string, string> })
  let database = $state({ name: '', encoding: 'UTF8', collation: '' })
  let privateAccessNetworkId = $state('')
  let device = $state({ name: '', deviceId: '' })
  let volume = $state(initialVolume())
  let mount = $state(initialMount())
  let health = $state(initialHealth())
  let backupPolicy = $state(initialBackupPolicy())
  let restoreBackup = $state<BackupHistory | null>(null)
  let restoreConfirmation = $state('')
  let connectionKeysConnection = $state<any>(null)
  let connectionKeys = $state({} as Record<string, string>)

  onMount(() => {
    const restoreInterval = window.setInterval(() => {
      if (activeRestore) router.reload({ only: ['backups'] })
    }, 5000)
    const healthInterval = window.setInterval(() => {
      if (resource.healthChecks.some((item: any) => item.enabled)) router.reload({ only: ['resource'] })
    }, 15000)
    return () => {
      window.clearInterval(restoreInterval)
      window.clearInterval(healthInterval)
    }
  })

  $effect(() => {
    if (!enrollment?.clientConfiguration || !enrollment.grantId || enrollment.grantId === shownEnrollmentGrantId) return
    shownEnrollmentGrantId = enrollment.grantId
    wireGuardConfigurationDialogOpen = true
  })

  $effect(() => {
    if (backups.databases.some((item) => item.databaseName === selectedBackupDatabaseName)) return
    const first = backups.databases[0]
    selectedBackupDatabaseName = first?.databaseName ?? ''
    if (!credential.database) credential.database = resource.databases[0]?.name ?? ''
    backupPolicy = initialBackupPolicy(first)
  })

  function initialEndpoint() {
    return { name: 'Primary', role: definition?.endpointRoles[0] ?? 'primary', address: '127.0.0.1', port: definition?.defaultPort ?? 1, protocol: definition?.defaultProtocol ?? 'tcp', tlsMode: definition?.defaultTlsMode ?? 'disable', privateNetworkId: '' }
  }
  function initialVolume() { return { name: '', driver: 'local', configurationText: '{}', serverId: options.servers[0]?.id ?? '' } }
  function initialMount() { return { mountPath: '/data', readOnly: false, resourceVolumeId: resource.volumes[0]?.id ?? '', resourceInstallationId: resource.installations[0]?.id ?? '' } }
	function initialHealth() { return { name: 'Readiness', kind: resource.engine === 'postgresql' || resource.engine === 'clickhouse' ? resource.engine : definition?.healthCheckKinds?.[0] ?? 'tcp', configurationText: '{}', intervalSeconds: 30, timeoutSeconds: 5, failureThreshold: 3, successThreshold: 1, enabled: true, resourceEndpointId: primaryEndpoint?.id ?? '', resourceCredentialId: applicationCredentials[0]?.id ?? administratorCredentials[0]?.id ?? '' } }
  function initialBackupPolicy(detail: DatabaseBackups | undefined = selectedBackups) {
    return {
      schedule: detail?.policy?.schedule ?? '0 2 * * *',
      backupDestinationId: detail?.policy?.backupDestinationId ?? backups.destinations[0]?.id ?? '',
      keepLast: detail?.policy?.keepLast ?? 7,
      keepDaily: detail?.policy?.keepDaily ?? 7,
      keepWeekly: detail?.policy?.keepWeekly ?? 4,
      keepMonthly: detail?.policy?.keepMonthly ?? 6,
    }
  }

  function selectBackupDatabase(databaseName: string) {
    selectedBackupDatabaseName = databaseName
    backupPolicy = initialBackupPolicy(backups.databases.find((item) => item.databaseName === databaseName))
  }

  function json(value: string) {
    try { jsonError = ''; return JSON.parse(value || '{}') }
    catch { jsonError = 'Configuration and metadata must contain valid JSON.'; throw new Error(jsonError) }
  }

  function submit(action: () => void) { try { action() } catch {} }
  function openWireGuardConfiguration() { if (enrollment?.clientConfiguration) wireGuardConfigurationDialogOpen = true }
  function createDatabase() { if (dialogAction) return; dialogAction = 'database'; router.post(routes.resourceDatabaseCreateForResource(resource.id), database, { onSuccess: () => { databaseDialogOpen = false; database = { name: '', encoding: 'UTF8', collation: '' } }, onError: () => (databaseDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function createEndpoint() { if (dialogAction) return; dialogAction = 'endpoint'; router.post(routes.resourceEndpointCreate(resource.id), { ...endpoint, settings: {} }, { onSuccess: () => (endpointDialogOpen = false), onError: () => (endpointDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function chooseEndpointNetwork(networkId: string) {
    endpoint.privateNetworkId = networkId
    const selected = options.privateNetworks.find((network) => network.id === networkId)
    const serverId = resource.installations[0]?.serverId
    endpoint.address = selected && serverId ? selected.serverAddresses[serverId] ?? '127.0.0.1' : '127.0.0.1'
  }
  function openCredentialDialog() { if (canAddApplicationUser) credentialDialogOpen = true }
  function createCredential() { if (!canAddApplicationUser || dialogAction) return; dialogAction = 'credential'; router.post(routes.resourceCredentialCreate(resource.id), { name: credential.name, username: credential.username, secretValues: credential.secretValues, metadata: { purpose: 'application', database: credential.database } }, { onSuccess: () => { credentialDialogOpen = false; credential.secretValues = {} }, onError: () => (credentialDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function enablePrivateAccess() { if (dialogAction) return; dialogAction = 'private-access'; router.post(routes.resourcePrivateAccessCreate(resource.id), { privateNetworkId: privateAccessNetworkId }, { onSuccess: () => (privateAccessDialogOpen = false), onError: () => (privateAccessDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function disablePrivateAccess() {
    confirmDestructive({
      kind: 'disable-private-access',
      title: 'Remove from private network?',
      description: 'The private listener will be removed and every device grant for this Resource will be revoked.',
      confirmationLabel: 'Remove from private network',
    })
  }
  function submitDevice() { if (pendingAction) return; pendingAction = 'device'; router.post(routes.resourcePrivateAccessDeviceCreate(resource.id), device, { onFinish: () => (pendingAction = '') }) }
  function revokeDevice(deviceId: string, deviceName: string) {
    confirmDestructive({
      kind: 'revoke-device',
      deviceId,
      title: `Revoke access for ${deviceName}?`,
      description: 'The device-specific firewall rule will be removed. The enrolled device remains available for other Resources.',
      confirmationLabel: 'Revoke access',
    })
  }
  function retryDevice(deviceId: string) { router.post(routes.resourcePrivateAccessDeviceCreate(resource.id), { deviceId, name: '' }) }
  function createVolume() { if (dialogAction) return; submit(() => { const configuration = json(volume.configurationText); dialogAction = 'volume'; router.post(routes.resourceVolumeCreate(resource.id), { ...volume, configuration }, { onSuccess: () => (volumeDialogOpen = false), onError: () => (volumeDialogOpen = true), onFinish: () => (dialogAction = '') }) }) }
  function createMount() { if (dialogAction) return; dialogAction = 'mount'; router.post(routes.resourceMountCreate(resource.id), mount, { onSuccess: () => (mountDialogOpen = false), onError: () => (mountDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function createHealth() { if (dialogAction) return; submit(() => { const configuration = json(health.configurationText); dialogAction = 'health'; router.post(routes.resourceHealthCheckCreate(resource.id), { ...health, configuration }, { onSuccess: () => (healthDialogOpen = false), onError: () => (healthDialogOpen = true), onFinish: () => (dialogAction = '') }) }) }
  function openConnectionKeys(connection: any) {
    connectionKeysConnection = connection
    connectionKeys = { ...connection.environmentKeys }
    connectionKeysDialogOpen = true
  }
  function connectionKeyDefinitions(connection: any) {
    if (connection?.configuration?.credential_projection === 'connection_url') {
      return definition.environmentKeys.filter((key) => key.name === 'url')
    }
    return definition.environmentKeys.filter((key) => key.name !== 'url')
  }
  function resourceDefaultEnvironmentKey(key: EnvironmentKey) {
    return resource.configuration?.environment_keys?.[key.name] ?? key.defaultKey
  }
  function saveConnectionKeys() {
    if (!connectionKeysConnection || dialogAction) return
    dialogAction = 'connection-keys'
    router.patch(routes.resourceConnectionEnvironmentKeysUpdate(resource.id, connectionKeysConnection.id), { environmentKeys: connectionKeys }, {
      onSuccess: () => (connectionKeysDialogOpen = false),
      onError: () => (connectionKeysDialogOpen = true),
      onFinish: () => (dialogAction = ''),
    })
  }
  function databaseRouteName(name: string) { return encodeURIComponent(name) }
  function saveBackupPolicy() {
    if (!selectedBackups) return
    const databaseName = databaseRouteName(selectedBackups.databaseName)
    if (selectedBackups.policy) router.patch(routes.resourceBackupPolicyUpdate(resource.id, databaseName, selectedBackups.policy.id), backupPolicy)
    else router.post(routes.resourceBackupPolicyCreate(resource.id, databaseName), backupPolicy)
  }
  function pauseBackupPolicy() { if (selectedBackups?.policy) router.post(routes.resourceBackupPolicyPause(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id), {}) }
  function resumeBackupPolicy() { if (selectedBackups?.policy) router.post(routes.resourceBackupPolicyResume(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id), {}) }
  function runBackupPolicy() { if (selectedBackups?.policy) router.post(routes.resourceBackupPolicyRun(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id), {}) }
  function confirmBackupPolicyArchive() {
    if (!selectedBackups?.policy) return
    confirmDestructive({ kind: 'archive-backup-policy', databaseName: selectedBackups.databaseName, policyId: selectedBackups.policy.id, title: 'Archive backup policy?', description: 'Future schedules stop immediately. Existing backup history and artifacts are retained.', confirmationLabel: 'Archive policy' })
  }

  function openRestoreDialog(backup: BackupHistory) {
    restoreBackup = backup
    restoreConfirmation = ''
    restoreDialogOpen = true
  }

  function submitRestore() {
    if (!restoreBackup || restoreConfirmation !== resource.name || dialogAction) return
    if (!selectedBackups) return
    dialogAction = 'restore'
    router.post(routes.resourceRestoreCreate(resource.id, databaseRouteName(selectedBackups.databaseName)), {
      backupId: restoreBackup.id,
      confirmation: restoreConfirmation,
    }, {
      onSuccess: () => { restoreDialogOpen = false; restoreBackup = null; restoreConfirmation = '' },
      onError: () => (restoreDialogOpen = true),
      onFinish: () => (dialogAction = ''),
    })
  }

  function lifecycle(installationId: string, action: 'start' | 'stop' | 'restart' | 'remove') {
    if (pendingAction) return
    pendingAction = `${installationId}:${action}`
    const route = action === 'start' ? routes.resourceInstallationStart(resource.id, installationId)
      : action === 'stop' ? routes.resourceInstallationStop(resource.id, installationId)
      : action === 'restart' ? routes.resourceInstallationRestart(resource.id, installationId)
      : routes.resourceInstallationRemove(resource.id, installationId)
    const done = { onFinish: () => { pendingAction = '' } }
    if (action === 'remove') router.delete(route, done)
    else router.post(route, {}, done)
  }

  function confirmContainerRemoval(installationId: string, containerName: string) {
    confirmDestructive({
      kind: 'remove-container',
      installationId,
      title: `Remove ${containerName}?`,
      description: 'The Docker container will be removed. Its installation configuration and volumes will remain.',
      confirmationLabel: 'Remove container',
    })
  }

  function confirmResourceArchive() {
    confirmDestructive({
      kind: 'archive-resource',
      title: `Archive ${resource.name}?`,
      description: 'The Resource, its Docker container, and its associated Docker volume will be permanently removed. Existing dependencies must be removed first, and volume data cannot be recovered.',
      confirmationLabel: 'Archive Resource',
    })
  }

  function confirmDestructive(action: DestructiveAction) {
    destructiveAction = action
    destructiveError = ''
    destructiveActionDialogOpen = true
  }

  function executeDestructiveAction() {
    const action = destructiveAction
    if (!action) return

    if (destructiveProcessing) return
    destructiveProcessing = true
    destructiveError = ''
    const url = action.kind === 'remove-container' ? routes.resourceInstallationRemove(resource.id, action.installationId)
      : action.kind === 'disable-private-access' ? routes.resourcePrivateAccessDestroy(resource.id)
      : action.kind === 'revoke-device' ? routes.resourcePrivateAccessDeviceDestroy(resource.id, action.deviceId)
      : action.kind === 'archive-resource' ? routes.resourceDestroy(resource.id)
      : routes.resourceBackupPolicyDestroy(resource.id, databaseRouteName(action.databaseName), action.policyId)
    router.delete(url, {
      onSuccess: () => { destructiveActionDialogOpen = false; destructiveAction = null },
      onError: (errors) => (destructiveError = Object.values(errors).map(String).join('\n') || 'The action could not be completed.'),
      onFinish: () => (destructiveProcessing = false),
    })
  }

  async function copyWireGuardConfiguration() {
    const configuration = enrollment?.clientConfiguration
    if (!configuration) return
    try { await navigator.clipboard.writeText(configuration); toast.success('WireGuard configuration copied') }
    catch { toast.error('WireGuard configuration could not be copied') }
  }

  async function openContainerLogs(installation: any) {
    containerLogsInstallation = installation
    containerLogs = ''
    containerLogsError = ''
    containerLogsLoading = true
    containerLogsDialogOpen = true
    try {
      const response = await window.fetch(`${routes.resourceInstallationLogs(resource.id, installation.id)}?tail=200`, {
        credentials: 'same-origin',
        headers: { Accept: 'application/json' },
      })
      const payload = await response.json().catch(() => ({})) as { logs?: string; error?: string }
      if (!response.ok) throw new Error(payload.error || 'Container logs could not be loaded')
      containerLogs = payload.logs || 'The container has not written any logs.'
    } catch (error) {
      containerLogsError = error instanceof Error ? error.message : 'Container logs could not be loaded'
    } finally {
      containerLogsLoading = false
    }
  }

  function observedLabel(value: string | null) {
    if (!value) return 'Never observed'
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
  }

  function accessStateLabel(value: string) {
    return ({ configured: 'Configured', applying: 'Applying', ready: 'Ready', failed: 'Failed' } as Record<string, string>)[value] ?? 'Not configured'
  }

  function bytesLabel(value: number | null) {
    if (value === null) return 'Not available'
    return new Intl.NumberFormat(undefined, { style: 'unit', unit: 'byte', notation: 'compact', unitDisplay: 'narrow' }).format(value)
  }
</script>

<svelte:head><title>{resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="mx-auto max-w-6xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{resource.engine} · {resource.resourceType}</p>
        <h1 class="mt-3 text-3xl font-semibold">{resource.name}</h1>
        <p class="mt-2 text-sm text-muted-foreground">Docker Resource</p>
        {#if databaseBacked}<p class="mt-1 text-xs text-muted-foreground">{resource.databases.length} {resource.databases.length === 1 ? 'Database' : 'Databases'}</p>{/if}
      </div>
      <div class="flex gap-2">
        <Button variant="outline" onclick={() => router.reload({ only: ['resource'] })}>Refresh status</Button>
        <Button>{#snippet child({ props })}<Link {...props} href={routes.resourceEdit(resource.id)}>Edit Resource</Link>{/snippet}</Button>
      </div>
    </header>

    {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"><Alert.Title>The item could not be created</Alert.Title><Alert.Description>Review the dialog fields and try again.</Alert.Description></Alert.Root>{/if}
    {#if jsonError}<Alert.Root variant="destructive"><Alert.Title>Invalid configuration</Alert.Title><Alert.Description>{jsonError}</Alert.Description></Alert.Root>{/if}

    <Card.Root class="overflow-hidden">
      <Card.Content class="grid gap-0 p-0 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div class="p-6">
          <div class="flex flex-wrap items-center gap-3">
            <StatusBadge status={overallStatus.label} />
            {#if resource.installations.length > 0}<StatusBadge status="Docker" />{/if}
          </div>
		  <h2 class="mt-5 text-xl font-semibold">{databaseBacked ? 'Access status' : 'Container status'}</h2>
          <p class="mt-2 max-w-2xl text-sm text-muted-foreground">{overallStatus.detail}</p>
        </div>
        <div class="grid grid-cols-2 gap-5 border-t border-border bg-muted/20 p-6 text-sm lg:grid-cols-1 lg:border-l lg:border-t-0">
          <DataField label="Installations" value={String(resource.installations.length)} />
          <DataField label="Attached Environments" value={String(resource.connectionCount)} />
        </div>
      </Card.Content>
    </Card.Root>

    {#if databaseBacked}
      <Card.Root>
        <Card.Header>
          <Card.Action><Button size="sm" variant="outline" onclick={() => (databaseDialogOpen = true)}>Add Database</Button></Card.Action>
          <Card.Title>Databases</Card.Title>
          <Card.Description>Logical Databases published through this Resource's endpoints.</Card.Description>
        </Card.Header>
        <Card.Content class="space-y-3">
          {#if resource.databases.length === 0}<p class="text-sm text-muted-foreground">No Databases published.</p>{/if}
          {#each resource.databases as item}
            <article class="border border-border p-3">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div><p class="font-mono text-sm">{item.name}</p><p class="mt-1 text-xs text-muted-foreground">{item.encoding || 'Default encoding'}{item.collation ? ` · ${item.collation}` : ''}</p></div>
                <p class="text-xs text-muted-foreground">Available through {resource.endpoints.length} {resource.endpoints.length === 1 ? 'endpoint' : 'endpoints'}</p>
              </div>
              {#if resource.endpoints.length === 0}
                <p class="mt-3 border-t border-border pt-3 text-xs text-destructive">No published endpoints are configured.</p>
              {:else}
                <div class="mt-3 grid gap-2 border-t border-border pt-3 lg:grid-cols-2">
                  {#each resource.endpoints as endpoint (endpoint.id)}
                    <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 bg-muted/20 px-3 py-2">
                      <div class="min-w-0">
                        <div class="flex flex-wrap items-center gap-2">
                          <p class="text-xs font-medium">{endpoint.name}</p>
                          <span class="text-[10px] uppercase tracking-wider text-muted-foreground">{endpoint.role}</span>
                        </div>
                        <p class="mt-1 break-all font-mono text-xs">{endpoint.address}:{endpoint.port}</p>
                      </div>
                      <p class="text-xs text-muted-foreground">{endpoint.protocol} · TLS {endpoint.tlsMode}</p>
                    </div>
                  {/each}
                </div>
              {/if}
            </article>
          {/each}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if resource.engine === 'postgresql'}
    <Card.Root>
      <Card.Header>
        <Card.Action>{#if selectedBackups?.policy}<StatusBadge status={selectedBackups.policy.active ? 'active' : 'paused'} />{/if}</Card.Action>
        <Card.Title>Backups</Card.Title>
        <Card.Description>Encrypted logical backups of one PostgreSQL Database using a verified Object Storage connection.</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-6">
        {#if backups.databases.length === 0}
          <p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground">No active Database is available for backup.</p>
        {:else if selectedBackups}
          <FormField label="Database">
            <NativeSelect.Root value={selectedBackups.databaseName} onchange={(event) => selectBackupDatabase(event.currentTarget.value)} class="w-full">
              {#each backups.databases as database}<NativeSelect.Option value={database.databaseName}>{database.databaseName}</NativeSelect.Option>{/each}
            </NativeSelect.Root>
          </FormField>
        {/if}
        {#if selectedBackups && !selectedBackups.eligibility.eligible}
          <p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground">{selectedBackups.eligibility.reason}</p>
        {:else if selectedBackups}
          <form class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" onsubmit={(event) => { event.preventDefault(); saveBackupPolicy() }}>
            <FormField label="Object Storage" error={errors.backupDestinationId}>
              <NativeSelect.Root bind:value={backupPolicy.backupDestinationId} class="w-full" required>
                {#each backups.destinations as destination}<NativeSelect.Option value={destination.id}>{destination.name} · {destination.bucket}</NativeSelect.Option>{/each}
              </NativeSelect.Root>
            </FormField>
            <FormField label="Schedule" error={errors.schedule}><Input bind:value={backupPolicy.schedule} class="font-mono" placeholder="0 2 * * *" required /></FormField>
            <div class="hidden lg:block"></div>
            <FormField label="Keep last"><Input type="number" min="0" bind:value={backupPolicy.keepLast} /></FormField>
            <FormField label="Keep daily"><Input type="number" min="0" bind:value={backupPolicy.keepDaily} /></FormField>
            <FormField label="Keep weekly"><Input type="number" min="0" bind:value={backupPolicy.keepWeekly} /></FormField>
            <FormField label="Keep monthly"><Input type="number" min="0" bind:value={backupPolicy.keepMonthly} /></FormField>
            <div class="flex flex-wrap items-end gap-2 sm:col-span-2">
              <Button type="submit">{selectedBackups.policy ? 'Save policy' : 'Create policy'}</Button>
              {#if selectedBackups.policy}
                {#if selectedBackups.policy.active}<Button type="button" variant="outline" onclick={pauseBackupPolicy}>Pause</Button>{:else}<Button type="button" variant="outline" onclick={resumeBackupPolicy}>Resume</Button>{/if}
                <Button type="button" variant="outline" disabled={!selectedBackups.policy.active} onclick={runBackupPolicy}>Back up now</Button>
                <Button type="button" variant="destructive" onclick={confirmBackupPolicyArchive}>Archive</Button>
              {/if}
            </div>
          </form>
          {#if selectedBackups.policy}
            <div class="grid gap-4 border-t border-border pt-5 sm:grid-cols-3">
              <DataField label="Next run" value={selectedBackups.policy.active ? observedLabel(selectedBackups.policy.nextRunAt) : 'Paused'} />
              <DataField label="Last successful" value={observedLabel(lastSuccessfulBackup?.finishedAt ?? null)} />
              <DataField label="Last verified" value={observedLabel(lastSuccessfulBackup?.verifiedAt ?? null)} />
            </div>
          {/if}
        {/if}

        <section class="space-y-3 border-t border-border pt-5">
          <div><h3 class="text-sm font-medium">Recent history</h3><p class="mt-1 text-xs text-muted-foreground">The latest backup attempts for the selected Database, including retained outcomes after policy archival.</p></div>
          {#if !selectedBackups || selectedBackups.history.length === 0}<p class="text-sm text-muted-foreground">No backups have been requested for this Database.</p>{/if}
          {#each selectedBackups?.history ?? [] as item (item.id)}
            <div class="grid gap-3 border border-border p-3 text-sm sm:grid-cols-[1fr_auto]">
              <div><div class="flex flex-wrap items-center gap-2"><span class="font-medium capitalize">{item.status.replaceAll('_', ' ')}</span><span class="text-xs uppercase tracking-wider text-muted-foreground">{item.triggerType}</span></div><p class="mt-1 text-xs text-muted-foreground">{observedLabel(item.scheduledAt)} · {bytesLabel(item.sizeBytes)}</p>{#if item.error}<p class="mt-2 text-xs text-destructive">{item.error}</p>{/if}</div>
              <div class="flex items-center gap-3"><span class="font-mono text-xs text-muted-foreground">{item.id.slice(0, 8)}</span>{#if item.canRestore}<Button size="sm" variant="destructive" onclick={() => openRestoreDialog(item)}>Restore</Button>{/if}</div>
            </div>
          {/each}
        </section>

        <section class="space-y-3 border-t border-border pt-5">
          <div class="flex flex-wrap items-start justify-between gap-3"><div><h3 class="text-sm font-medium">Restore history</h3><p class="mt-1 text-xs text-muted-foreground">A safety backup is verified before any database is changed.</p></div>{#if activeRestore}<span class="border border-primary/50 px-2 py-1 text-xs text-primary">Restore in progress</span>{/if}</div>
          {#if !selectedBackups || selectedBackups.restores.length === 0}<p class="text-sm text-muted-foreground">No restores have been requested for this Database.</p>{/if}
          {#each selectedBackups?.restores ?? [] as restore (restore.id)}
            <div class="grid gap-3 border border-border p-3 text-sm sm:grid-cols-[1fr_auto]">
              <div><div class="flex flex-wrap items-center gap-2"><span class="font-medium capitalize">{restore.status.replaceAll('_', ' ')}</span><span class="text-xs text-muted-foreground">Source {observedLabel(restore.backupScheduledAt)}</span></div><p class="mt-1 text-xs text-muted-foreground">Requested {observedLabel(restore.requestedAt)}{restore.safetyBackupId ? ` · Safety backup ${restore.safetyBackupId.slice(0, 8)}` : ''}</p>{#if restore.error}<p class="mt-2 text-xs text-destructive">{restore.error}</p>{/if}</div>
              <span class="font-mono text-xs text-muted-foreground">{restore.id.slice(0, 8)}</span>
            </div>
          {/each}
        </section>
      </Card.Content>
    </Card.Root>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Title>Docker installations</Card.Title>
        <Card.Description>Observed state and controls for each Resource installation.</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-4">
        {#if resource.installations.length === 0}<p class="text-sm text-muted-foreground">No installation is configured.</p>{/if}
        {#each resource.installations as item}
          <article class="border border-border p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div>
                <div class="flex items-center gap-2"><h3 class="font-medium">{item.containerName}</h3><span class="border border-border px-1.5 py-0.5 text-[10px] uppercase tracking-wider">Docker</span></div>
                <p class="mt-1 font-mono text-xs text-muted-foreground">{item.imageReference}</p>
              </div>
              <div class="flex flex-wrap gap-2">
                {#if item.state !== 'missing'}<Button size="sm" variant="outline" disabled={containerLogsLoading && containerLogsInstallation?.id === item.id} onclick={() => openContainerLogs(item)}>{#if containerLogsLoading && containerLogsInstallation?.id === item.id}<Spinner />{/if}View logs</Button>{/if}
                {#if item.canControl}
                  {#if item.serviceState === 'running'}
                    <Button size="sm" variant="outline" disabled={Boolean(pendingAction)} onclick={() => lifecycle(item.id, 'stop')}>Stop</Button>
                    <Button size="sm" variant="outline" disabled={Boolean(pendingAction)} onclick={() => lifecycle(item.id, 'restart')}>Restart</Button>
                  {:else}
                    <Button size="sm" disabled={Boolean(pendingAction)} onclick={() => lifecycle(item.id, 'start')}>Start</Button>
                  {/if}
                  {#if item.state !== 'missing'}<Button size="sm" variant="destructive" disabled={Boolean(pendingAction)} onclick={() => confirmContainerRemoval(item.id, item.containerName)}>Remove container</Button>{/if}
                {/if}
              </div>
            </div>
            {#if item.state !== 'missing' && item.serviceState !== 'running'}
              <Alert.Root variant="destructive" class="mt-4">
                <Alert.Title>Container is {item.serviceState || 'not running'}</Alert.Title>
                <Alert.Description>Docker reports exit code {item.containerDetails?.exitCode ?? 'unknown'} and {item.containerDetails?.restartCount ?? 0} restarts. Open the container logs to see the process error.</Alert.Description>
              </Alert.Root>
            {/if}
            <div class="mt-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-5">
              <DataField label="Service" value={item.serviceState || 'Unknown'} />
              <DataField label="Health" value={item.health || 'Unknown'} />
              <DataField label="Server" value={item.serverName} />
              <DataField label="Container ID" value={item.containerDetails?.id?.slice(0, 12) || 'Not created'} />
              <DataField label="Observed" value={observedLabel(item.observedAt)} />
            </div>
            {#if item.healthReason}<p class="mt-4 border-l-2 border-border pl-3 text-xs text-muted-foreground">{item.healthReason}</p>{/if}
            {#if !item.canControl}<p class="mt-4 text-xs text-muted-foreground">Container controls are unavailable because the selected Server cannot currently be reached.</p>{/if}
          </article>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Action><Button size="sm" variant="outline" onclick={() => (endpointDialogOpen = true)}>Add endpoint</Button></Card.Action><Card.Title>{databaseBacked ? 'Primary endpoint' : 'Primary service'}</Card.Title><Card.Description>{databaseBacked ? 'The default published Database access endpoint and its optional private path.' : 'The Docker origin and its optional private network path.'}</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#if primaryEndpoint}
          <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            <DataField label={databaseBacked ? 'Address' : 'Runtime origin'} value={`${primaryEndpoint.address}:${primaryEndpoint.port}`} />
            <DataField label="Installation" value={resource.installations[0]?.containerName ?? 'Not installed'} />
            <DataField label="Protocol" value={primaryEndpoint.protocol} />
            <DataField label="TLS" value={primaryEndpoint.tlsMode} />
          </div>
        {:else}<p class="text-sm text-destructive">No primary origin is configured.</p>{/if}

        <section class="border-t border-border pt-5">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div><h3 class="font-medium">Private network</h3><p class="mt-1 text-xs text-muted-foreground">A WireGuard listener derived from the primary origin.</p></div>
              {#if privateEndpoint}<Button size="sm" variant="destructive" onclick={disablePrivateAccess}>Remove from private network</Button>{:else}<Button size="sm" variant="outline" onclick={() => (privateAccessDialogOpen = true)}>Add to private network</Button>{/if}
            </div>
            {#if privateEndpoint}
              <div class="mt-4 grid gap-5 sm:grid-cols-3">
                <DataField label="Address" value={`${privateEndpoint.address}:${privateEndpoint.port}`} />
                <DataField label="Network" value={privateNetwork?.name ?? 'Unknown'} />
                <DataField label="State" value={accessStateLabel(resource.privateAccessState)} />
              </div>
              <div class="mt-6 space-y-3">
                <div class="flex flex-wrap items-center justify-between gap-3"><h4 class="text-sm font-medium">Granted devices</h4><div class="flex items-center gap-3">{#if enrollment?.clientConfiguration}<Button size="sm" variant="outline" onclick={openWireGuardConfiguration}>View client configuration</Button>{/if}<span class="text-xs text-muted-foreground">{resource.deviceGrants.length} granted</span></div></div>
                {#if resource.deviceGrants.length === 0}<p class="text-sm text-muted-foreground">No device grant has applied the listener yet. The Resource is on the private network, but its listener is not ready.</p>{/if}
                {#each resource.deviceGrants as grant}
                  <div class="flex flex-col justify-between gap-3 border border-border p-3 sm:flex-row sm:items-center"><div><p class="font-medium">{grant.deviceName}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{grant.privateAddress} · {accessStateLabel(grant.applicationState === 'applied' ? 'ready' : grant.applicationState === 'failed' ? 'failed' : 'applying')}</p><p class="mt-1 text-xs text-muted-foreground">Latest handshake: {observedLabel(grant.latestHandshakeAt)}</p>{#if grant.applicationError}<p class="mt-1 text-xs text-destructive">{grant.applicationError}</p>{/if}</div><div class="flex gap-2">{#if grant.applicationState !== 'applied'}<Button size="sm" variant="outline" onclick={() => retryDevice(grant.deviceId)}>Retry</Button>{/if}<Button size="sm" variant="destructive" onclick={() => revokeDevice(grant.deviceId, grant.deviceName)}>Revoke</Button></div></div>
                {/each}
              </div>
              <form class="mt-5 grid gap-4 border-t border-border pt-5 sm:grid-cols-3" onsubmit={(event) => { event.preventDefault(); submitDevice() }}><FormField label="Existing device"><NativeSelect.Root bind:value={device.deviceId} class="w-full"><NativeSelect.Option value="">Enroll a new device</NativeSelect.Option>{#each resource.availableDevices as item}<NativeSelect.Option value={item.id}>{item.name} · {item.privateAddress}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{#if !device.deviceId}<FormField label="New device name" error={errors.name}><Input bind:value={device.name} placeholder="MBV MacBook" required /></FormField>{/if}<div class="flex items-end"><Button type="submit" disabled={Boolean(pendingAction)} aria-busy={pendingAction === 'device'}>{#if pendingAction === 'device'}<Spinner />{/if}{device.deviceId ? 'Grant existing device' : 'Enroll new device'}</Button></div></form>
            {/if}
        </section>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Action><Button size="sm" variant="outline" disabled={!canAddApplicationUser} onclick={openCredentialDialog}>Add application credential</Button></Card.Action><Card.Title>Credentials</Card.Title><Card.Description>{databaseBacked ? 'The administrator stays internal. Each application credential is tied to one configured Database.' : 'Encrypted application credentials for this Resource.'}</Card.Description></Card.Header>
      <Card.Content class={databaseBacked ? 'grid gap-6' : 'grid gap-6 lg:grid-cols-2'}>
        {#if managedPostgreSQL && !containerRunning}<p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground lg:col-span-2">Start the PostgreSQL container before adding an application user. DeployCrate must connect to the running server to create its LOGIN role.</p>{/if}
        {#if databaseBacked}<section><h3 class="text-sm font-medium">Resource administrator</h3><div class="mt-3 space-y-3">{#if administratorCredentials.length === 0}<p class="text-sm text-muted-foreground">No Resource administrator.</p>{/if}{#each administratorCredentials as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.username}</p><span class="text-xs text-muted-foreground">{item.hasEncryptedPayload ? 'Encrypted' : 'Missing secret'}</span></div><p class="mt-2 text-xs text-muted-foreground">Superuser, not selectable by Environments</p></div>{/each}</div></section>{/if}
        <section><h3 class="text-sm font-medium">Application credentials</h3><div class="mt-3 space-y-3">{#if applicationCredentials.length === 0}<p class="text-sm text-muted-foreground">No application credentials.</p>{/if}{#each applicationCredentials as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.username}</p><span class="text-xs text-muted-foreground">{item.hasEncryptedPayload ? 'Encrypted' : 'Missing secret'}</span></div><p class="mt-2 text-xs text-muted-foreground">{item.name}{item.metadata?.database ? ` · Database ${item.metadata.database}` : ''}</p></div>{/each}</div></section>
      </Card.Content>
    </Card.Root>

    <div class="grid gap-6 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Action>{#if resource.volumes.length === 0 || (resource.volumes.length === 1 && resource.installations.length === 1 && resource.mounts.length === 0)}<div class="flex gap-2">{#if resource.volumes.length === 0}<Button size="sm" variant="outline" onclick={() => (volumeDialogOpen = true)}>Add volume</Button>{/if}{#if resource.volumes.length === 1 && resource.installations.length === 1 && resource.mounts.length === 0}<Button size="sm" variant="outline" onclick={() => (mountDialogOpen = true)}>Add mount</Button>{/if}</div>{/if}</Card.Action><Card.Title>Storage</Card.Title><Card.Description>The primary durable volume and its installation mount.</Card.Description></Card.Header><Card.Content class="space-y-3">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No primary volume configured.</p>{/if}{#each resource.volumes as item}<div class="border border-border p-3"><p class="font-medium">{item.name}</p><p class="mt-2 text-xs text-muted-foreground">{item.driver} on {item.serverName}</p>{#each resource.mounts.filter((mount: any) => mount.resourceVolumeId === item.id) as mount}<p class="mt-2 font-mono text-xs">{mount.mountPath} → {mount.containerName}{mount.readOnly ? ' (read only)' : ''}</p>{/each}</div>{/each}</Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Action><Button size="sm" variant="outline" disabled={databaseBacked ? applicationCredentials.length === 0 || resource.endpoints.length === 0 : resource.installations.length === 0} onclick={() => (healthDialogOpen = true)}>Add check</Button></Card.Action><Card.Title>Health checks</Card.Title><Card.Description>Desired checks and their latest observations.</Card.Description></Card.Header><Card.Content class="space-y-3">{#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{/if}{#each resource.healthChecks as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.name}</p><StatusBadge status={item.state || 'unknown'} /></div><p class="mt-2 text-xs text-muted-foreground">{item.kind} · every {item.intervalSeconds}s · {item.enabled ? 'Enabled' : 'Disabled'}</p><p class="mt-1 text-xs text-muted-foreground">Observed {observedLabel(item.observedAt)}{item.latencyMs !== null ? ` · ${item.latencyMs} ms` : ''} · successes {item.consecutiveSuccesses} · failures {item.consecutiveFailures}</p>{#if item.message}<p class="mt-2 text-xs text-muted-foreground">{item.message}</p>{/if}</div>{/each}</Card.Content></Card.Root>
    </div>

    <Card.Root>
      <Card.Header><Card.Action><span class="text-xs text-muted-foreground">{resource.connectionCount} attached</span></Card.Action><Card.Title>Attached Environments</Card.Title><Card.Description>Attach Resources from Environment pages. Manage injected key names here on the Resource.</Card.Description></Card.Header>
      <Card.Content class="space-y-3">
        {#if resource.connections.length === 0}<p class="text-sm text-muted-foreground">This Resource is not attached to an Environment.</p>{/if}
        {#each resource.connections as item}
          <div class="grid gap-3 border border-border p-3 sm:grid-cols-[1fr_auto]">
            <div><p class="font-medium">{item.applicationName} / {item.environmentName}</p><p class="mt-1 text-xs text-muted-foreground">Database {item.database || 'Not specified'} · Alias {item.alias} · Endpoint {item.endpointName}{item.credentialName ? ` · Credential ${item.credentialName}` : ''}</p><p class="mt-2 font-mono text-xs text-muted-foreground">{Object.values(item.configuration.environment_keys ?? {}).join(' · ')}</p></div>
            <div class="flex items-start gap-2"><span class="pt-2 text-xs text-muted-foreground">{Object.keys(item.environmentKeyOverrides ?? {}).length === 0 ? 'Using defaults' : `${Object.keys(item.environmentKeyOverrides ?? {}).length} overridden`}</span><Button size="sm" variant="outline" onclick={() => openConnectionKeys(item)}>Edit injected keys</Button></div>
          </div>
        {/each}
      </Card.Content>
    </Card.Root>

    <div class="border-t border-border pt-6"><Button variant="destructive" onclick={confirmResourceArchive}>Archive Resource</Button></div>
  </div>

  <Dialog.Root bind:open={containerLogsDialogOpen}>
    <Dialog.Content class="sm:max-w-4xl">
      <Dialog.Header>
        <Dialog.Title>Container logs</Dialog.Title>
        <Dialog.Description>{containerLogsInstallation?.containerName ?? 'Resource container'} · latest 200 lines, limited to 64 KiB.</Dialog.Description>
      </Dialog.Header>
      {#if containerLogsLoading}
        <div class="flex min-h-40 items-center justify-center"><Spinner /></div>
      {:else if containerLogsError}
        <Alert.Root variant="destructive"><Alert.Title>Logs unavailable</Alert.Title><Alert.Description>{containerLogsError}</Alert.Description></Alert.Root>
      {:else}
        <pre class="max-h-[60vh] overflow-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{containerLogs}</pre>
      {/if}
      <Dialog.Footer><Button type="button" variant="outline" onclick={() => (containerLogsDialogOpen = false)}>Close</Button></Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={wireGuardConfigurationDialogOpen}>
    <Dialog.Content class="sm:max-w-3xl">
      <Dialog.Header>
        <Dialog.Title>One-time WireGuard configuration</Dialog.Title>
        <Dialog.Description>Import this configuration on the newly enrolled device now. The private key is not stored and cannot be shown again after leaving this page.</Dialog.Description>
      </Dialog.Header>
      <pre class="max-h-[60vh] overflow-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{enrollment?.clientConfiguration ?? ''}</pre>
      <Dialog.Footer>
        <Button type="button" variant="outline" onclick={copyWireGuardConfiguration}>Copy configuration</Button>
        <Button type="button" onclick={() => (wireGuardConfigurationDialogOpen = false)}>I have saved it</Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={destructiveActionDialogOpen}
    title={destructiveAction?.title ?? 'Confirm destructive action'}
    description={destructiveAction?.description ?? 'This action cannot be undone.'}
    confirmLabel={destructiveAction?.confirmationLabel ?? 'Confirm'}
    processing={destructiveProcessing}
    error={destructiveError}
    destructive
    onconfirm={executeDestructiveAction}
  />

  <Dialog.Root bind:open={restoreDialogOpen} onOpenChange={(open) => { if (!open) { restoreBackup = null; restoreConfirmation = '' } }}>
    <Dialog.Content>
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); submitRestore() }}>
        <Dialog.Header>
          <Dialog.Title>Restore database from backup?</Dialog.Title>
          <Dialog.Description>DeployCrate first creates and verifies a fresh safety backup. Existing database sessions are terminated during the final cutover and clients must reconnect.</Dialog.Description>
        </Dialog.Header>
        {#if restoreBackup}<div class="border border-border bg-muted/20 p-3 text-sm"><p>Backup from {observedLabel(restoreBackup.scheduledAt)}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{restoreBackup.id}</p></div>{/if}
        <FormField label={`Enter ${resource.name} to confirm`} error={errors.confirmation}><Input bind:value={restoreConfirmation} autocomplete="off" /></FormField>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'restore'} onclick={() => (restoreDialogOpen = false)}>Cancel</Button><Button type="submit" variant="destructive" disabled={restoreConfirmation !== resource.name || dialogAction === 'restore'} aria-busy={dialogAction === 'restore'}>{#if dialogAction === 'restore'}<Spinner />{/if}Create safety backup and restore</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={privateAccessDialogOpen}>
    <Dialog.Content>
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); enablePrivateAccess() }}>
        <Dialog.Header><Dialog.Title>Add to private network</Dialog.Title><Dialog.Description>Select the WireGuard network attached to the installation Server. Address, port, protocol, and installation are derived.</Dialog.Description></Dialog.Header>
        <FormField label="Private network" error={errors.privateNetworkId}><NativeSelect.Root bind:value={privateAccessNetworkId} class="w-full" required><NativeSelect.Option value="">Select a private network</NativeSelect.Option>{#each endpointNetworks as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'private-access'} onclick={() => (privateAccessDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'private-access'} aria-busy={dialogAction === 'private-access'}>{#if dialogAction === 'private-access'}<Spinner />{/if}Add to private network</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={endpointDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createEndpoint() }}>
        <Dialog.Header><Dialog.Title>Add endpoint</Dialog.Title><Dialog.Description>Define another address for this Resource.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Name" error={errors.name}><Input bind:value={endpoint.name} required /></FormField>
          <FormField label="Address" error={errors.address}><Input bind:value={endpoint.address} required /></FormField>
          <FormField label="Role"><NativeSelect.Root bind:value={endpoint.role} class="w-full">{#each definition.endpointRoles as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Protocol"><NativeSelect.Root bind:value={endpoint.protocol} class="w-full">{#each definition.protocols as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Port"><Input type="number" bind:value={endpoint.port} min="1" max="65535" /></FormField>
          <FormField label="TLS"><NativeSelect.Root bind:value={endpoint.tlsMode} class="w-full">{#each definition.tlsModes as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Private network"><NativeSelect.Root value={endpoint.privateNetworkId} onchange={(event) => chooseEndpointNetwork(event.currentTarget.value)} class="w-full"><NativeSelect.Option value="">No private network</NativeSelect.Option>{#each endpointNetworks as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Docker installation</p><p class="mt-1 text-sm">{resource.installations[0]?.containerName ?? 'Not installed'}</p></div>
        </div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'endpoint'} onclick={() => (endpointDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'endpoint'} aria-busy={dialogAction === 'endpoint'}>{#if dialogAction === 'endpoint'}<Spinner />{/if}Create endpoint</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={connectionKeysDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveConnectionKeys() }}>
        <Dialog.Header><Dialog.Title>Injected keys for {connectionKeysConnection?.applicationName} / {connectionKeysConnection?.environmentName}</Dialog.Title><Dialog.Description>These Resource-managed names apply only to this connection. Entering the Resource default restores inheritance for that key.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2">
          {#each connectionKeyDefinitions(connectionKeysConnection) as key}
            <FormField label={key.label} error={errors[`configuration.environment_keys.${key.name}`]}>
              <Input value={connectionKeys[key.name] ?? resourceDefaultEnvironmentKey(key)} oninput={(event) => connectionKeys[key.name] = event.currentTarget.value} placeholder={resourceDefaultEnvironmentKey(key)} required autocomplete="off" />
              <p class="mt-1 text-[11px] text-muted-foreground">Default: <span class="font-mono">{resourceDefaultEnvironmentKey(key)}</span></p>
            </FormField>
          {/each}
        </div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'connection-keys'} onclick={() => (connectionKeysDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'connection-keys'} aria-busy={dialogAction === 'connection-keys'}>{#if dialogAction === 'connection-keys'}<Spinner />{/if}Save connection keys</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={credentialDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createCredential() }}>
        <Dialog.Header><Dialog.Title>Add application credential</Dialog.Title><Dialog.Description>The credential is scoped to one Database. For PostgreSQL, DeployCrate creates the LOGIN role and grants access in code.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Display name" error={errors.name}><Input bind:value={credential.name} required /></FormField><FormField label="Username" error={errors.username}><Input bind:value={credential.username} required autocomplete="username" /></FormField>{#if databaseBacked}<FormField label="Database" error={errors['metadata.database']}><NativeSelect.Root bind:value={credential.database} class="w-full" required><NativeSelect.Option value="">Select a Database</NativeSelect.Option>{#each resource.databases as item}<NativeSelect.Option value={item.name}>{item.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{/if}{#each definition.credentialFields as field}<FormField label={field.label} error={errors[`secretValues.${field.name}`]}><Input type={field.secret ? 'password' : 'text'} value={credential.secretValues[field.name] ?? ''} oninput={(event) => credential.secretValues[field.name] = event.currentTarget.value} required={field.required} autocomplete="new-password" /></FormField>{/each}</div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'credential'} onclick={() => (credentialDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={!canAddApplicationUser || dialogAction === 'credential'} aria-busy={dialogAction === 'credential'}>{#if dialogAction === 'credential'}<Spinner />{/if}Create application credential</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={databaseDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createDatabase() }}>
        <Dialog.Header><Dialog.Title>Add Database</Dialog.Title><Dialog.Description>Create a logical Database in the running Resource and record it in Resource configuration.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="sm:col-span-2"><FormField label="Database name" error={errors.name}><Input bind:value={database.name} required /></FormField></div>
          <FormField label="Encoding" error={errors.encoding}><Input bind:value={database.encoding} required /></FormField>
          <FormField label="Collation" error={errors.collation}><Input bind:value={database.collation} placeholder="Default" /></FormField>
        </div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'database'} onclick={() => (databaseDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'database'} aria-busy={dialogAction === 'database'}>{#if dialogAction === 'database'}<Spinner />{/if}Create Database</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={volumeDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createVolume() }}>
        <Dialog.Header><Dialog.Title>Add volume</Dialog.Title><Dialog.Description>Create durable storage for this Resource.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={volume.name} required /></FormField><FormField label="Driver"><Input bind:value={volume.driver} required /></FormField><FormField label="Server"><NativeSelect.Root bind:value={volume.serverId} class="w-full">{#each options.servers as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><div class="sm:col-span-2"><FormField label="Configuration JSON" error={jsonError}><Textarea bind:value={volume.configurationText} class="min-h-28 font-mono" /></FormField></div></div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'volume'} onclick={() => (volumeDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'volume'} aria-busy={dialogAction === 'volume'}>{#if dialogAction === 'volume'}<Spinner />{/if}Create volume</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={mountDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createMount() }}>
        <Dialog.Header><Dialog.Title>Add mount</Dialog.Title><Dialog.Description>Attach a Resource volume to an installation.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Mount path"><Input bind:value={mount.mountPath} required /></FormField><FormField label="Volume"><NativeSelect.Root bind:value={mount.resourceVolumeId} class="w-full">{#each resource.volumes as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Installation"><NativeSelect.Root bind:value={mount.resourceInstallationId} class="w-full">{#each resource.installations as value}<NativeSelect.Option value={value.id}>{value.containerName}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><label class="flex items-center gap-2 self-end text-xs"><Checkbox bind:checked={mount.readOnly} /> Read only</label></div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'mount'} onclick={() => (mountDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'mount'} aria-busy={dialogAction === 'mount'}>{#if dialogAction === 'mount'}<Spinner />{/if}Create mount</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={healthDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createHealth() }}>
        <Dialog.Header><Dialog.Title>Add health check</Dialog.Title><Dialog.Description>Define how DeployCrate should evaluate this Resource.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={health.name} required /></FormField><FormField label="Kind"><NativeSelect.Root bind:value={health.kind} class="w-full">{#each definition.healthCheckKinds as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Endpoint"><NativeSelect.Root bind:value={health.resourceEndpointId} class="w-full"><NativeSelect.Option value="">None</NativeSelect.Option>{#each resource.endpoints as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Credential"><NativeSelect.Root bind:value={health.resourceCredentialId} class="w-full"><NativeSelect.Option value="">None</NativeSelect.Option>{#each resource.credentials as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Interval seconds"><Input type="number" bind:value={health.intervalSeconds} min="1" /></FormField><FormField label="Timeout seconds"><Input type="number" bind:value={health.timeoutSeconds} min="1" /></FormField><FormField label="Failure threshold"><Input type="number" bind:value={health.failureThreshold} min="1" /></FormField><FormField label="Success threshold"><Input type="number" bind:value={health.successThreshold} min="1" /></FormField><label class="flex items-center gap-2 text-xs"><Checkbox bind:checked={health.enabled} /> Enabled</label><div class="sm:col-span-2"><FormField label="Configuration JSON" error={jsonError}><Textarea bind:value={health.configurationText} class="min-h-28 font-mono" /></FormField></div></div>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'health'} onclick={() => (healthDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'health'} aria-busy={dialogAction === 'health'}>{#if dialogAction === 'health'}<Spinner />{/if}Create health check</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
