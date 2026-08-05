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
  import * as Table from '@/Components/ui/table'
  import { Textarea } from '@/Components/ui/textarea'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type EnvironmentKey = { name: string; label: string; defaultKey: string }
  type Engine = { engine: string; label: string; resourceType: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; environmentKeys: EnvironmentKey[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type Publication = { id: string; resourceEndpointId: string; externalId: string; hostname: string; healthPath: string; state: string; lastError: string; appliedAt: string; observedAt: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { engines: Engine[]; servers: Array<{ id: string; name: string; address: string }>; privateNetworks: PrivateNetwork[]; registryCredentials: Array<{ id: string; name: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }
  type BackupDestination = { id: string; name: string; provider: string; endpoint: string; region: string; bucket: string; prefix: string; verifiedAt: string | null; lastUsedAt: string | null }
  type BackupPolicy = { id: string; schedule: string; active: boolean; nextRunAt: string; backupDestinationId: string; keepLast: number; keepDaily: number; keepWeekly: number; keepMonthly: number }
  type BackupHistory = { id: string; status: string; triggerType: string; scheduledAt: string; finishedAt: string | null; verifiedAt: string | null; sizeBytes: number | null; error: string; canRestore: boolean }
  type RestoreHistory = { id: string; status: string; requestedAt: string; startedAt: string | null; finishedAt: string | null; verifiedAt: string | null; cutoverAt: string | null; rolledBackAt: string | null; error: string; backupId: string; backupScheduledAt: string; safetyBackupId: string | null }
  type RevealedCredential = { id: string; name: string; username: string; values: Record<string, string> }
  type DatabaseBackups = { databaseName: string; eligibility: { eligible: boolean; reason: string; installationId: string | null }; policy: BackupPolicy | null; history: BackupHistory[]; restores: RestoreHistory[]; activeRestore: boolean }
  type Backups = { destinations: BackupDestination[]; databases: DatabaseBackups[] }
  type DestructiveAction =
    | { kind: 'remove-container'; installationId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'disable-private-access'; title: string; description: string; confirmationLabel: string }
    | { kind: 'revoke-device'; deviceId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-endpoint'; endpointId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-credential'; credentialId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-volume'; volumeId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-mount'; mountId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-health'; healthId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-resource'; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-backup-policy'; databaseName: string; policyId: string; title: string; description: string; confirmationLabel: string }
  let { auth, resource, backups, options, publications = [], section = 'overview', selectedDatabase = '', enrollment = null, errors = {} }: { auth: { email: string }; resource: any; backups: Backups; options: Options; publications?: Publication[]; section?: string; selectedDatabase?: string; enrollment?: Enrollment | null; errors?: Record<string, string> } = $props()
  let selectedBackupDatabaseName = $state(initialSelectedBackupDatabase())

  const definition = $derived(options.engines.find((kind) => kind.engine === resource.engine) ?? { engine: resource.engine, label: resource.engine, resourceType: resource.resourceType, protocols: [], endpointRoles: [], tlsModes: [], credentialFields: [], environmentKeys: [], healthCheckKinds: [], defaultPort: 1, defaultProtocol: '', defaultTlsMode: 'disable' })
  const endpointNetworks = $derived(resource.installations.length === 1
    ? options.privateNetworks.filter((network) => network.serverIds.includes(resource.installations[0].serverId) && Boolean(network.serverAddresses[resource.installations[0].serverId]))
    : options.privateNetworks)
  const primaryEndpoint = $derived(resource.endpoints.find((item: any) => item.role === 'primary' && !item.privateNetworkId))
  const serviceMappings = $derived(resource.installations.flatMap((installation: any) => (installation.configuration?.portMappings ?? []).map((mapping: any) => ({ ...mapping, installationId: installation.id, containerName: installation.containerName }))))
  const privateAccess = $derived(resource.privateAccess)
  const privateNetwork = $derived(privateAccess ? options.privateNetworks.find((item) => item.id === privateAccess.privateNetworkId) : undefined)
  const administratorCredentials = $derived(resource.credentials.filter((item: any) => item.metadata?.purpose === 'administrator'))
  const applicationCredentials = $derived(resource.credentials.filter((item: any) => item.metadata?.purpose === 'application'))
	const databaseBacked = $derived(resource.resourceType === 'database')
	const managedPostgreSQL = $derived(resource.engine === 'postgresql')
  const containerRunning = $derived(resource.installations.some((item: any) => item.serviceState === 'running'))
  const canAddApplicationUser = $derived((!databaseBacked || resource.databases.length > 0) && (!managedPostgreSQL || containerRunning))
  const selectedBackups = $derived(backups.databases.find((item) => item.databaseName === selectedBackupDatabaseName) ?? backups.databases[0])
  const configuredBackups = $derived(backups.databases.filter((item) => Boolean(item.policy)))
  const lastSuccessfulBackup = $derived(selectedBackups?.history.find((item) => item.status === 'verified'))
  const activeRestore = $derived(Boolean(selectedBackups?.activeRestore))
  const sectionTitle = $derived(({
    overview: 'Overview',
    databases: 'Databases',
    backups: 'Backups',
    endpoints: 'Endpoints',
    credentials: 'Credentials',
    health: 'Health checks',
  } as Record<string, string>)[section] ?? 'Overview')
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
  let deviceDialogOpen = $state(false)
  let credentialDialogOpen = $state(false)
  let credentialPasswordDialogOpen = $state(false)
  let revealedCredentialDialogOpen = $state(false)
  let databaseDialogOpen = $state(false)
  let volumeDialogOpen = $state(false)
  let mountDialogOpen = $state(false)
  let healthDialogOpen = $state(false)
  let connectionKeysDialogOpen = $state(false)
  let destructiveActionDialogOpen = $state(false)
  let restoreDialogOpen = $state(false)
  let backupPolicyDialogOpen = $state(false)
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
  let editingEndpointId = $state('')
  let credential = $state({ name: 'Application user', username: '', database: '', purpose: 'application', metadata: {} as Record<string, unknown>, rotate: false, secretValues: {} as Record<string, string> })
  let editingCredentialId = $state('')
  let selectedCredential = $state<any>(null)
  let currentPassword = $state('')
  let credentialRevealError = $state('')
  let credentialRevealProcessing = $state(false)
  let revealedCredential = $state<RevealedCredential | null>(null)
  let database = $state({ name: '', encoding: 'UTF8', collation: '' })
  let privateAccessNetworkId = $state('')
  let device = $state({ name: '', deviceId: '' })
  let volume = $state(initialVolume())
  let editingVolumeId = $state('')
  let mount = $state(initialMount())
  let editingMountId = $state('')
  let health = $state(initialHealth())
  let editingHealthId = $state('')
  let backupPolicy = $state(initialBackupPolicy())
  let restoreBackup = $state<BackupHistory | null>(null)
  let restoreConfirmation = $state('')
  let connectionKeysConnection = $state<any>(null)
  let connectionKeys = $state({} as Record<string, string>)

  onMount(() => {
    const restoreInterval = window.setInterval(() => {
      if (section === 'backups' && activeRestore) router.reload({ only: ['backups'] })
    }, 5000)
    const healthInterval = window.setInterval(() => {
      if (section === 'health' && resource.healthChecks.some((item: any) => item.enabled)) router.reload({ only: ['resource'] })
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
    const available = section === 'backups' ? configuredBackups : backups.databases
    if (available.some((item) => item.databaseName === selectedBackupDatabaseName)) return
    const first = available[0]
    selectedBackupDatabaseName = first?.databaseName ?? ''
    if (!credential.database) credential.database = resource.databases[0]?.name ?? ''
    backupPolicy = initialBackupPolicy(first)
  })

  function initialEndpoint() {
    return { name: 'Primary', role: definition?.endpointRoles[0] ?? 'primary', audience: 'local_system', addressSource: 'system_loopback', address: '127.0.0.1', port: definition?.defaultPort ?? 1, protocol: definition?.defaultProtocol ?? 'tcp', tlsMode: definition?.defaultTlsMode ?? 'disable', privateNetworkId: '', settings: {}, publication: { enabled: false, hostname: '', healthPath: '' } }
  }
  function initialSelectedBackupDatabase() { return selectedDatabase }
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

  function openBackupPolicyDialog(databaseName: string) {
    selectBackupDatabase(databaseName)
    backupPolicyDialogOpen = true
  }

  function json(value: string) {
    try { jsonError = ''; return JSON.parse(value || '{}') }
    catch { jsonError = 'Configuration and metadata must contain valid JSON.'; throw new Error(jsonError) }
  }

  function submit(action: () => void) { try { action() } catch {} }
  function actionURL(url: string) { return `${url}${url.includes('?') ? '&' : '?'}returnTo=${section}` }
  function openWireGuardConfiguration() { if (enrollment?.clientConfiguration) wireGuardConfigurationDialogOpen = true }
  function createDatabase() { if (dialogAction) return; dialogAction = 'database'; router.post(actionURL(routes.resourceDatabaseCreateForResource(resource.id)), database, { onSuccess: () => { databaseDialogOpen = false; database = { name: '', encoding: 'UTF8', collation: '' } }, onError: () => (databaseDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function openEndpointDialog(item: any = null) {
    editingEndpointId = item?.id ?? ''
    const publication = publications.find((value) => value.resourceEndpointId === item?.id)
    const settings = item?.settings ?? {}
    const caddy = settings.caddy?.managed ? settings.caddy : null
    const originSettings = { ...settings }
    delete originSettings.caddy
    const originAddress = caddy?.origin_address ?? item?.address ?? ''
    const originPrivateNetworkId = caddy?.origin_private_network_id ?? item?.privateNetworkId ?? ''
    endpoint = item ? { name: item.name, role: item.role, audience: originPrivateNetworkId ? 'environment' : ['127.0.0.1', '::1', 'localhost'].includes(originAddress) ? 'local_system' : 'custom', addressSource: originPrivateNetworkId ? 'server_wireguard' : ['127.0.0.1', '::1', 'localhost'].includes(originAddress) ? 'system_loopback' : 'manual', address: originAddress, port: caddy?.origin_port ?? item.port, protocol: caddy?.origin_protocol ?? item.protocol, tlsMode: caddy?.origin_tls_mode ?? item.tlsMode, privateNetworkId: originPrivateNetworkId, settings: originSettings, publication: { enabled: Boolean(caddy || publication), hostname: item.address, healthPath: caddy?.health_path ?? publication?.healthPath ?? '' } } : initialEndpoint()
    endpointDialogOpen = true
  }
  function saveEndpoint() {
    if (dialogAction) return
    dialogAction = 'endpoint'
    const settings = { ...endpoint.settings, audience: endpoint.audience, address_source: endpoint.addressSource, ...(resource.engine === 'opentelemetry' ? { exposure: endpoint.audience === 'environment' ? 'environment' : 'system', transport: 'http/protobuf', authentication: endpoint.audience === 'environment' ? 'signed_identity' : 'none' } : {}) }
    const payload = { ...endpoint, settings, publication: endpoint.publication }
    const options = { onSuccess: () => { endpointDialogOpen = false; editingEndpointId = '' }, onError: () => (endpointDialogOpen = true), onFinish: () => (dialogAction = '') }
    if (editingEndpointId) router.patch(actionURL(routes.resourceEndpointUpdate(resource.id, editingEndpointId)), payload, options)
    else router.post(actionURL(routes.resourceEndpointCreate(resource.id)), payload, options)
  }
  function chooseEndpointNetwork(networkId: string) {
    endpoint.privateNetworkId = networkId
    const selected = options.privateNetworks.find((network) => network.id === networkId)
    const serverId = resource.installations[0]?.serverId
    endpoint.address = selected && serverId ? selected.serverAddresses[serverId] ?? '127.0.0.1' : '127.0.0.1'
  }
  function chooseEndpointAudience(audience: string) {
    endpoint.audience = audience
    endpoint.role = audience === 'environment' && definition.endpointRoles.includes('wireguard')
      ? 'wireguard'
      : audience === 'local_system' && definition.endpointRoles.includes('local')
        ? 'local'
        : definition.endpointRoles[0] ?? 'primary'
    if (audience === 'local_system') {
      endpoint.addressSource = 'system_loopback'
      endpoint.privateNetworkId = ''
      endpoint.address = '127.0.0.1'
    } else if (audience === 'environment') {
      endpoint.addressSource = 'server_wireguard'
      chooseEndpointNetwork(endpoint.privateNetworkId || endpointNetworks[0]?.id || '')
    } else {
      endpoint.addressSource = 'manual'
      endpoint.privateNetworkId = ''
    }
  }
  function chooseServiceMapping(index: number) {
    const mapping = serviceMappings[index]
    if (mapping) endpoint.port = mapping.hostPort
  }
  function openCredentialDialog(item: any = null) {
    if (!item && !canAddApplicationUser) return
    editingCredentialId = item?.id ?? ''
    credential = item
      ? { name: item.name, username: item.username, database: item.metadata?.database ?? '', purpose: item.metadata?.purpose ?? 'application', metadata: item.metadata ?? {}, rotate: false, secretValues: {} }
      : { name: 'Application user', username: '', database: resource.databases[0]?.name ?? '', purpose: 'application', metadata: {}, rotate: false, secretValues: {} }
    credentialDialogOpen = true
  }
  function saveCredential() {
    if ((!editingCredentialId && !canAddApplicationUser) || dialogAction) return
    dialogAction = 'credential'
    const payload = { name: credential.name, username: credential.username, rotate: credential.rotate, secretValues: credential.secretValues, metadata: { ...credential.metadata, purpose: credential.purpose, database: credential.database } }
    const options = { onSuccess: () => { credentialDialogOpen = false; editingCredentialId = ''; credential.secretValues = {} }, onError: () => (credentialDialogOpen = true), onFinish: () => (dialogAction = '') }
    if (editingCredentialId) router.patch(actionURL(routes.resourceCredentialUpdate(resource.id, editingCredentialId)), payload, options)
    else router.post(actionURL(routes.resourceCredentialCreate(resource.id)), payload, options)
  }
  function enablePrivateAccess() { if (dialogAction) return; dialogAction = 'private-access'; router.post(actionURL(routes.resourcePrivateAccessCreate(resource.id)), { privateNetworkId: privateAccessNetworkId }, { onSuccess: () => (privateAccessDialogOpen = false), onError: () => (privateAccessDialogOpen = true), onFinish: () => (dialogAction = '') }) }
  function disablePrivateAccess() {
    confirmDestructive({
      kind: 'disable-private-access',
      title: 'Turn off WireGuard access?',
      description: 'The private listener will be removed and every device grant for this Resource will be revoked. Archive endpoints using WireGuard before turning access off.',
      confirmationLabel: 'Turn off WireGuard access',
    })
  }
  function submitDevice() { if (pendingAction) return; pendingAction = 'device'; router.post(actionURL(routes.resourcePrivateAccessDeviceCreate(resource.id)), device, { onSuccess: () => (deviceDialogOpen = false), onFinish: () => (pendingAction = '') }) }
  function revokeDevice(deviceId: string, deviceName: string) {
    confirmDestructive({
      kind: 'revoke-device',
      deviceId,
      title: `Revoke access for ${deviceName}?`,
      description: 'The device-specific firewall rule will be removed. The enrolled device remains available for other Resources.',
      confirmationLabel: 'Revoke access',
    })
  }
  function retryDevice(deviceId: string) { router.post(actionURL(routes.resourcePrivateAccessDeviceCreate(resource.id)), { deviceId, name: '' }) }
  function openVolumeDialog(item: any = null) { editingVolumeId = item?.id ?? ''; volume = item ? { name: item.name, driver: item.driver, configurationText: JSON.stringify(item.configuration ?? {}, null, 2), serverId: item.serverId } : initialVolume(); volumeDialogOpen = true }
  function saveVolume() { if (dialogAction) return; submit(() => { const configuration = json(volume.configurationText); dialogAction = 'volume'; const options = { onSuccess: () => { volumeDialogOpen = false; editingVolumeId = '' }, onError: () => (volumeDialogOpen = true), onFinish: () => (dialogAction = '') }; if (editingVolumeId) router.patch(actionURL(routes.resourceVolumeUpdate(resource.id, editingVolumeId)), { ...volume, configuration }, options); else router.post(actionURL(routes.resourceVolumeCreate(resource.id)), { ...volume, configuration }, options) }) }
  function openMountDialog(item: any = null) { editingMountId = item?.id ?? ''; mount = item ? { mountPath: item.mountPath, readOnly: item.readOnly, resourceVolumeId: item.resourceVolumeId, resourceInstallationId: item.resourceInstallationId } : initialMount(); mountDialogOpen = true }
  function saveMount() { if (dialogAction) return; dialogAction = 'mount'; const options = { onSuccess: () => { mountDialogOpen = false; editingMountId = '' }, onError: () => (mountDialogOpen = true), onFinish: () => (dialogAction = '') }; if (editingMountId) router.patch(actionURL(routes.resourceMountUpdate(resource.id, editingMountId)), mount, options); else router.post(actionURL(routes.resourceMountCreate(resource.id)), mount, options) }
  function openHealthDialog(item: any = null) { editingHealthId = item?.id ?? ''; health = item ? { name: item.name, kind: item.kind, configurationText: JSON.stringify(item.configuration ?? {}, null, 2), intervalSeconds: item.intervalSeconds, timeoutSeconds: item.timeoutSeconds, failureThreshold: item.failureThreshold, successThreshold: item.successThreshold, enabled: item.enabled, resourceEndpointId: item.resourceEndpointId ?? '', resourceCredentialId: item.resourceCredentialId ?? '' } : initialHealth(); healthDialogOpen = true }
  function saveHealth() { if (dialogAction) return; submit(() => { const configuration = json(health.configurationText); dialogAction = 'health'; const options = { onSuccess: () => { healthDialogOpen = false; editingHealthId = '' }, onError: () => (healthDialogOpen = true), onFinish: () => (dialogAction = '') }; if (editingHealthId) router.patch(actionURL(routes.resourceHealthCheckUpdate(resource.id, editingHealthId)), { ...health, configuration }, options); else router.post(actionURL(routes.resourceHealthCheckCreate(resource.id)), { ...health, configuration }, options) }) }
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
    router.patch(actionURL(routes.resourceConnectionEnvironmentKeysUpdate(resource.id, connectionKeysConnection.id)), { environmentKeys: connectionKeys }, {
      onSuccess: () => (connectionKeysDialogOpen = false),
      onError: () => (connectionKeysDialogOpen = true),
      onFinish: () => (dialogAction = ''),
    })
  }
  function databaseRouteName(name: string) { return encodeURIComponent(name) }
  function saveBackupPolicy() {
    if (!selectedBackups) return
    const databaseName = databaseRouteName(selectedBackups.databaseName)
    const options = { onSuccess: () => (backupPolicyDialogOpen = false), onError: () => (backupPolicyDialogOpen = true) }
    if (selectedBackups.policy) router.patch(actionURL(routes.resourceBackupPolicyUpdate(resource.id, databaseName, selectedBackups.policy.id)), backupPolicy, options)
    else router.post(actionURL(routes.resourceBackupPolicyCreate(resource.id, databaseName)), backupPolicy, options)
  }
  function pauseBackupPolicy() { if (selectedBackups?.policy) router.post(actionURL(routes.resourceBackupPolicyPause(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id)), {}) }
  function resumeBackupPolicy() { if (selectedBackups?.policy) router.post(actionURL(routes.resourceBackupPolicyResume(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id)), {}) }
  function runBackupPolicy() { if (selectedBackups?.policy) router.post(actionURL(routes.resourceBackupPolicyRun(resource.id, databaseRouteName(selectedBackups.databaseName), selectedBackups.policy.id)), {}) }
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
    router.post(actionURL(routes.resourceRestoreCreate(resource.id, databaseRouteName(selectedBackups.databaseName))), {
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
    if (action === 'remove') router.delete(actionURL(route), done)
    else router.post(actionURL(route), {}, done)
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

  function archiveEndpoint() { if (!editingEndpointId) return; endpointDialogOpen = false; confirmDestructive({ kind: 'archive-endpoint', endpointId: editingEndpointId, title: `Archive ${endpoint.name}?`, description: 'The endpoint can no longer be selected by Environments or health checks.', confirmationLabel: 'Archive endpoint' }) }
  function archiveCredential() { if (!editingCredentialId) return; credentialDialogOpen = false; confirmDestructive({ kind: 'archive-credential', credentialId: editingCredentialId, title: `Archive ${credential.name}?`, description: 'The credential can no longer be selected by Environments or health checks.', confirmationLabel: 'Archive credential' }) }
  function archiveVolume() { if (!editingVolumeId) return; volumeDialogOpen = false; confirmDestructive({ kind: 'archive-volume', volumeId: editingVolumeId, title: `Archive ${volume.name}?`, description: 'Remove its mounts before archiving this volume.', confirmationLabel: 'Archive volume' }) }
  function archiveMount() { if (!editingMountId) return; mountDialogOpen = false; confirmDestructive({ kind: 'archive-mount', mountId: editingMountId, title: `Archive mount ${mount.mountPath}?`, description: 'The volume will no longer be mounted into this installation.', confirmationLabel: 'Archive mount' }) }
  function archiveHealth() { if (!editingHealthId) return; healthDialogOpen = false; confirmDestructive({ kind: 'archive-health', healthId: editingHealthId, title: `Archive ${health.name}?`, description: 'DeployCrate will stop evaluating this health check.', confirmationLabel: 'Archive health check' }) }

  function destructiveActionURL(action: DestructiveAction) {
    switch (action.kind) {
      case 'remove-container': return routes.resourceInstallationRemove(resource.id, action.installationId)
      case 'disable-private-access': return routes.resourcePrivateAccessDestroy(resource.id)
      case 'revoke-device': return routes.resourcePrivateAccessDeviceDestroy(resource.id, action.deviceId)
      case 'archive-endpoint': return routes.resourceEndpointDestroy(resource.id, action.endpointId)
      case 'archive-credential': return routes.resourceCredentialDestroy(resource.id, action.credentialId)
      case 'archive-volume': return routes.resourceVolumeDestroy(resource.id, action.volumeId)
      case 'archive-mount': return routes.resourceMountDestroy(resource.id, action.mountId)
      case 'archive-health': return routes.resourceHealthCheckDestroy(resource.id, action.healthId)
      case 'archive-resource': return routes.resourceDestroy(resource.id)
      case 'archive-backup-policy': return routes.resourceBackupPolicyDestroy(resource.id, databaseRouteName(action.databaseName), action.policyId)
    }
  }

  function executeDestructiveAction() {
    const action = destructiveAction
    if (!action) return

    if (destructiveProcessing) return
    destructiveProcessing = true
    destructiveError = ''
    const url = destructiveActionURL(action)
    router.delete(actionURL(url), {
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

  function askForCredential(item: any) {
    selectedCredential = item
    currentPassword = ''
    credentialRevealError = ''
    credentialPasswordDialogOpen = true
  }

  async function revealCredential(event: SubmitEvent) {
    event.preventDefault()
    if (!selectedCredential || !currentPassword || credentialRevealProcessing) return
    credentialRevealProcessing = true
    credentialRevealError = ''
    try {
      const response = await window.fetch(routes.resourceCredentialReveal(resource.id, selectedCredential.id), {
        method: 'POST',
        credentials: 'same-origin',
        headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: currentPassword }),
      })
      const payload = await response.json().catch(() => ({})) as Partial<RevealedCredential> & { error?: string }
      if (!response.ok || !payload.id || !payload.name || !payload.values || Object.keys(payload.values).length === 0) throw new Error(payload.error || 'Resource credential could not be loaded')
      revealedCredential = { id: payload.id, name: payload.name, username: payload.username ?? '', values: payload.values }
      currentPassword = ''
      credentialPasswordDialogOpen = false
      revealedCredentialDialogOpen = true
    } catch (error) {
      credentialRevealError = error instanceof Error ? error.message : 'Resource credential could not be loaded'
    } finally {
      credentialRevealProcessing = false
    }
  }

  function closeRevealedCredential() {
    revealedCredentialDialogOpen = false
    revealedCredential = null
    selectedCredential = null
  }

  async function copyCredential(value: string, label: string) {
    try { await navigator.clipboard.writeText(value); toast.success(`${label} copied`) }
    catch { toast.error(`${label} could not be copied`) }
  }

  function displayLabel(value: string) {
    return value.replaceAll('_', ' ').replace(/\b\w/g, (character) => character.toUpperCase())
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
<DashboardLayout email={auth.email} resourceNavigation={resource}>
  <div class="mx-auto max-w-6xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{resource.engine} · {resource.resourceType}</p>
        <h1 class="mt-3 text-3xl font-semibold">{resource.name}</h1>
        <p class="mt-2 text-sm text-muted-foreground">{sectionTitle}</p>
        {#if databaseBacked}<p class="mt-1 text-xs text-muted-foreground">{resource.databases.length} {resource.databases.length === 1 ? 'Database' : 'Databases'}</p>{/if}
      </div>
    </header>

    {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"><Alert.Title>The item could not be created</Alert.Title><Alert.Description>Review the dialog fields and try again.</Alert.Description></Alert.Root>{/if}
    {#if jsonError}<Alert.Root variant="destructive"><Alert.Title>Invalid configuration</Alert.Title><Alert.Description>{jsonError}</Alert.Description></Alert.Root>{/if}

    {#if section === 'overview'}
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
    {/if}

    {#if section === 'databases' && databaseBacked}
      <Card.Root>
        <Card.Header>
          <Card.Action><Button size="sm" variant="outline" onclick={() => (databaseDialogOpen = true)}>Add Database</Button></Card.Action>
          <Card.Title>Databases</Card.Title>
          <Card.Description>Logical Databases published through this Resource's endpoints.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if resource.databases.length === 0}<p class="text-sm text-muted-foreground">No Databases published.</p>{:else}
            <div class="overflow-hidden border border-border">
              <Table.Root>
                <Table.Header><Table.Row><Table.Head>Database</Table.Head><Table.Head>Encoding</Table.Head><Table.Head>Collation</Table.Head><Table.Head>Endpoints</Table.Head><Table.Head class="text-right">Backups</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                  {#each resource.databases as item}
                    {@const backup = backups.databases.find((detail) => detail.databaseName === item.name)}
                    <Table.Row>
                      <Table.Cell class="font-mono font-medium">{item.name}</Table.Cell>
                      <Table.Cell>{item.encoding || 'Default'}</Table.Cell>
                      <Table.Cell>{item.collation || 'Default'}</Table.Cell>
                      <Table.Cell>{resource.endpoints.length}</Table.Cell>
                      <Table.Cell class="text-right">
                        {#if backup?.policy}
                          <Button size="sm" variant="ghost">{#snippet child({ props })}<Link {...props} href={`${routes.resourceBackups(resource.id)}?database=${encodeURIComponent(item.name)}`}>Manage</Link>{/snippet}</Button>
                        {:else}
                          <Button size="sm" variant="outline" disabled={!backup?.eligibility.eligible} title={backup?.eligibility.reason || undefined} onclick={() => openBackupPolicyDialog(item.name)}>Set up</Button>
                        {/if}
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if section === 'backups' && resource.resourceType === 'database'}
      <div class="space-y-6">
        <Card.Root>
          <Card.Header>
            <Card.Title>Backups</Card.Title>
            <Card.Description>Manage encrypted logical backups by Database.</Card.Description>
          </Card.Header>
          <Card.Content>
            {#if configuredBackups.length === 0}
              <div class="flex flex-wrap items-center justify-between gap-3">
                <p class="text-sm text-muted-foreground">No backup policy is configured. Set one up from Databases.</p>
                <Button size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.resourceDatabases(resource.id)}>Open Databases</Link>{/snippet}</Button>
              </div>
            {:else if selectedBackups}
              <FormField label="Database">
                <NativeSelect.Root value={selectedBackups.databaseName} onchange={(event) => selectBackupDatabase(event.currentTarget.value)} class="w-full">
                  {#each configuredBackups as database}<NativeSelect.Option value={database.databaseName}>{database.databaseName}</NativeSelect.Option>{/each}
                </NativeSelect.Root>
              </FormField>
            {/if}
          </Card.Content>
        </Card.Root>

        {#if selectedBackups?.policy}
          <Card.Root>
            <Card.Header>
              <Card.Action><div class="flex items-center gap-2"><StatusBadge status={selectedBackups.policy.active ? 'active' : 'paused'} /><Button size="sm" variant="outline" onclick={() => openBackupPolicyDialog(selectedBackups.databaseName)}>Edit policy</Button></div></Card.Action>
              <Card.Title>Backup policy</Card.Title>
              <Card.Description>Schedule, verification status, and policy controls for {selectedBackups.databaseName}.</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Schedule</Table.Head><Table.Head>Next run</Table.Head><Table.Head>Last successful</Table.Head><Table.Head>Last verified</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header><Table.Body><Table.Row><Table.Cell class="font-mono text-xs">{selectedBackups.policy.schedule}</Table.Cell><Table.Cell>{selectedBackups.policy.active ? observedLabel(selectedBackups.policy.nextRunAt) : 'Paused'}</Table.Cell><Table.Cell>{observedLabel(lastSuccessfulBackup?.finishedAt ?? null)}</Table.Cell><Table.Cell>{observedLabel(lastSuccessfulBackup?.verifiedAt ?? null)}</Table.Cell><Table.Cell><div class="flex justify-end gap-2">{#if selectedBackups.policy.active}<Button size="sm" variant="ghost" onclick={pauseBackupPolicy}>Pause</Button>{:else}<Button size="sm" variant="ghost" onclick={resumeBackupPolicy}>Resume</Button>{/if}<Button size="sm" variant="outline" disabled={!selectedBackups.policy.active} onclick={runBackupPolicy}>Run now</Button><Button size="sm" variant="destructive" onclick={confirmBackupPolicyArchive}>Archive</Button></div></Table.Cell></Table.Row></Table.Body></Table.Root></div>
            </Card.Content>
          </Card.Root>

          <Card.Root>
            <Card.Header>
              <Card.Title>Recent backups</Card.Title>
              <Card.Description>The latest backup attempts, including retained outcomes after policy archival.</Card.Description>
            </Card.Header>
            <Card.Content>
              {#if selectedBackups.history.length === 0}<p class="text-sm text-muted-foreground">No backups have been requested for this Database.</p>{/if}
              {#if selectedBackups.history.length > 0}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Status</Table.Head><Table.Head>Trigger</Table.Head><Table.Head>Scheduled</Table.Head><Table.Head>Size</Table.Head><Table.Head class="w-24"><span class="sr-only">Actions</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each selectedBackups.history as item (item.id)}<Table.Row><Table.Cell><div><span class="font-medium capitalize">{item.status.replaceAll('_', ' ')}</span>{#if item.error}<p class="mt-1 max-w-md text-xs text-destructive">{item.error}</p>{/if}</div></Table.Cell><Table.Cell class="capitalize">{item.triggerType}</Table.Cell><Table.Cell>{observedLabel(item.scheduledAt)}</Table.Cell><Table.Cell>{bytesLabel(item.sizeBytes)}</Table.Cell><Table.Cell class="text-right">{#if item.canRestore}<Button size="sm" variant="destructive" onclick={() => openRestoreDialog(item)}>Restore</Button>{/if}</Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}
            </Card.Content>
          </Card.Root>

          <Card.Root>
            <Card.Header>
              <Card.Action>{#if activeRestore}<span class="border border-primary/50 px-2 py-1 text-xs text-primary">Restore in progress</span>{/if}</Card.Action>
              <Card.Title>Restore history</Card.Title>
              <Card.Description>A safety backup is verified before any Database is changed.</Card.Description>
            </Card.Header>
            <Card.Content>
              {#if selectedBackups.restores.length === 0}<p class="text-sm text-muted-foreground">No restores have been requested for this Database.</p>{/if}
              {#if selectedBackups.restores.length > 0}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Status</Table.Head><Table.Head>Requested</Table.Head><Table.Head>Source backup</Table.Head><Table.Head>Safety backup</Table.Head></Table.Row></Table.Header><Table.Body>{#each selectedBackups.restores as restore (restore.id)}<Table.Row><Table.Cell><div><span class="font-medium capitalize">{restore.status.replaceAll('_', ' ')}</span>{#if restore.error}<p class="mt-1 max-w-md text-xs text-destructive">{restore.error}</p>{/if}</div></Table.Cell><Table.Cell>{observedLabel(restore.requestedAt)}</Table.Cell><Table.Cell>{observedLabel(restore.backupScheduledAt)}</Table.Cell><Table.Cell class="font-mono text-xs">{restore.safetyBackupId?.slice(0, 8) ?? 'None'}</Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}
            </Card.Content>
          </Card.Root>
        {/if}
      </div>
    {/if}

    {#if section === 'overview'}
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
    {/if}

    {#if section === 'endpoints'}
      <div class="space-y-6">
        <Card.Root>
          <Card.Header><Card.Action><Button size="sm" variant="outline" onclick={() => openEndpointDialog()}>Add endpoint</Button></Card.Action><Card.Title>Endpoints</Card.Title><Card.Description>Every address published by this Resource, including endpoints routed through enabled WireGuard access.</Card.Description></Card.Header>
          <Card.Content>
            {#if resource.endpoints.length === 0}<p class="text-sm text-muted-foreground">No endpoints configured.</p>{/if}
            {#if resource.endpoints.length > 0}
              <div class="overflow-hidden border border-border">
                <Table.Root>
                  <Table.Header><Table.Row><Table.Head>Name</Table.Head><Table.Head>Address</Table.Head><Table.Head>Available to</Table.Head><Table.Head>Caddy route</Table.Head><Table.Head class="w-20"><span class="sr-only">Actions</span></Table.Head></Table.Row></Table.Header>
                  <Table.Body>
                    {#each resource.endpoints as item (item.id)}
                      {@const publication = publications.find((value) => value.resourceEndpointId === item.id)}
                      <Table.Row>
                        <Table.Cell class="font-medium">{item.name}</Table.Cell>
                        <Table.Cell class="font-mono text-xs">{item.address}:{item.port}</Table.Cell>
                        <Table.Cell class="capitalize">{String(item.settings?.audience ?? (item.privateNetworkId ? 'environment' : 'local system')).replaceAll('_', ' ')}</Table.Cell>
                        <Table.Cell>{#if publication}<div class="flex flex-wrap items-center gap-2"><span class="font-mono text-xs">{publication.hostname}</span><StatusBadge status={publication.state} /></div>{#if publication.lastError}<p class="mt-1 max-w-sm text-xs text-destructive">{publication.lastError}</p>{/if}{:else}<span class="text-xs text-muted-foreground">Not published</span>{/if}</Table.Cell>
                        <Table.Cell class="text-right">{#if item.managed}<span class="text-xs text-muted-foreground">Managed</span>{:else}<Button size="sm" variant="ghost" onclick={() => openEndpointDialog(item)}>Edit</Button>{/if}</Table.Cell>
                      </Table.Row>
                    {/each}
                  </Table.Body>
                </Table.Root>
              </div>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header>
            <Card.Action>{#if privateAccess}<Button size="sm" variant="destructive" onclick={disablePrivateAccess}>Turn off WireGuard access</Button>{:else}<Button size="sm" variant="outline" onclick={() => (privateAccessDialogOpen = true)}>Turn on WireGuard access</Button>{/if}</Card.Action>
            <Card.Title>WireGuard network access</Card.Title>
            <Card.Description>Enable private access and manage the devices allowed to reach this Resource.</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="flex flex-wrap items-start justify-between gap-3">
              {#if !privateAccess}<p class="text-sm text-muted-foreground">Turn on WireGuard access to create a managed private endpoint and grant device access.</p>{/if}
            </div>
            {#if privateAccess}
              <div class="grid gap-5 sm:grid-cols-3">
                <DataField label="Gateway" value={`${privateAccess.address}:${privateAccess.port}`} />
                <DataField label="Network" value={privateNetwork?.name ?? 'Unknown'} />
                <DataField label="State" value={accessStateLabel(resource.privateAccessState)} />
              </div>
              <div class="mt-6 space-y-3">
                <div class="flex flex-wrap items-center justify-between gap-3"><h4 class="text-sm font-medium">Granted devices</h4><div class="flex items-center gap-2">{#if enrollment?.clientConfiguration}<Button size="sm" variant="outline" onclick={openWireGuardConfiguration}>View client configuration</Button>{/if}<Button size="sm" variant="outline" onclick={() => (deviceDialogOpen = true)}>Grant access</Button></div></div>
                {#if resource.deviceGrants.length === 0}<p class="text-sm text-muted-foreground">No device grant has applied the listener yet. The Resource is on the private network, but its listener is not ready.</p>{/if}
                {#if resource.deviceGrants.length > 0}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Device</Table.Head><Table.Head>Address</Table.Head><Table.Head>State</Table.Head><Table.Head>Latest handshake</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header><Table.Body>{#each resource.deviceGrants as grant}<Table.Row><Table.Cell><div><span class="font-medium">{grant.deviceName}</span>{#if grant.applicationError}<p class="mt-1 max-w-xs text-xs text-destructive">{grant.applicationError}</p>{/if}</div></Table.Cell><Table.Cell class="font-mono text-xs">{grant.privateAddress}</Table.Cell><Table.Cell>{accessStateLabel(grant.applicationState === 'applied' ? 'ready' : grant.applicationState === 'failed' ? 'failed' : 'applying')}</Table.Cell><Table.Cell>{observedLabel(grant.latestHandshakeAt)}</Table.Cell><Table.Cell><div class="flex justify-end gap-2">{#if grant.applicationState !== 'applied'}<Button size="sm" variant="ghost" onclick={() => retryDevice(grant.deviceId)}>Retry</Button>{/if}<Button size="sm" variant="destructive" onclick={() => revokeDevice(grant.deviceId, grant.deviceName)}>Revoke</Button></div></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}
              </div>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>
    {/if}

    {#if section === 'credentials'}
    <Card.Root>
      <Card.Header><Card.Action><Button size="sm" variant="outline" disabled={!canAddApplicationUser} onclick={openCredentialDialog}>Add application credential</Button></Card.Action><Card.Title>Credentials</Card.Title><Card.Description>{databaseBacked ? 'The administrator stays internal. Each application credential is tied to one configured Database.' : 'Encrypted application credentials for this Resource.'}</Card.Description></Card.Header>
      <Card.Content class="space-y-4">
        {#if managedPostgreSQL && !containerRunning}<p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground">Start the PostgreSQL container before adding an application user. DeployCrate must connect to the running server to create its LOGIN role.</p>{/if}
        {#if resource.credentials.length === 0}<p class="text-sm text-muted-foreground">No credentials configured.</p>{:else}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Name</Table.Head><Table.Head>Username</Table.Head><Table.Head>Purpose</Table.Head><Table.Head>Database</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header><Table.Body>{#each resource.credentials as item (item.id)}<Table.Row><Table.Cell class="font-medium">{item.name}</Table.Cell><Table.Cell class="font-mono text-xs">{item.username || 'None'}</Table.Cell><Table.Cell class="capitalize">{item.metadata?.purpose ?? 'Application'}</Table.Cell><Table.Cell>{item.metadata?.database || 'All / none'}</Table.Cell><Table.Cell><div class="flex justify-end gap-2">{#if item.hasEncryptedPayload}<Button size="sm" variant="ghost" onclick={() => askForCredential(item)}>View</Button>{/if}<Button size="sm" variant="outline" onclick={() => openCredentialDialog(item)}>Edit</Button></div></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}
      </Card.Content>
    </Card.Root>
    {/if}

    {#if section === 'overview'}
    <div class="grid gap-6 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Action><div class="flex gap-2"><Button size="sm" variant="outline" onclick={() => openVolumeDialog()}>Add volume</Button>{#if resource.volumes.length > 0 && resource.installations.length > 0}<Button size="sm" variant="outline" onclick={() => openMountDialog()}>Add mount</Button>{/if}</div></Card.Action><Card.Title>Storage</Card.Title><Card.Description>Durable volumes and their installation mounts.</Card.Description></Card.Header><Card.Content class="space-y-5">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No volumes configured.</p>{:else}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Volume</Table.Head><Table.Head>Driver</Table.Head><Table.Head>Server</Table.Head><Table.Head class="w-20"><span class="sr-only">Actions</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each resource.volumes as item}<Table.Row><Table.Cell class="font-medium">{item.name}</Table.Cell><Table.Cell>{item.driver}</Table.Cell><Table.Cell>{item.serverName}</Table.Cell><Table.Cell class="text-right"><Button size="sm" variant="ghost" onclick={() => openVolumeDialog(item)}>Edit</Button></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}{#if resource.mounts.length > 0}<div><p class="mb-2 text-xs font-medium text-muted-foreground">Mounts</p><div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Path</Table.Head><Table.Head>Volume</Table.Head><Table.Head>Installation</Table.Head><Table.Head>Mode</Table.Head><Table.Head class="w-20"><span class="sr-only">Actions</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each resource.mounts as item}<Table.Row><Table.Cell class="font-mono text-xs">{item.mountPath}</Table.Cell><Table.Cell>{item.volumeName}</Table.Cell><Table.Cell>{item.containerName}</Table.Cell><Table.Cell>{item.readOnly ? 'Read only' : 'Read/write'}</Table.Cell><Table.Cell class="text-right"><Button size="sm" variant="ghost" onclick={() => openMountDialog(item)}>Edit</Button></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div></div>{/if}</Card.Content></Card.Root>
    </div>
    {/if}

    {#if section === 'health'}
    <Card.Root><Card.Header><Card.Action><Button size="sm" variant="outline" disabled={resource.endpoints.length === 0 || (databaseBacked && applicationCredentials.length === 0)} onclick={() => openHealthDialog()}>Add check</Button></Card.Action><Card.Title>Health checks</Card.Title><Card.Description>Desired checks and their latest observations.</Card.Description></Card.Header><Card.Content>{#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{:else}<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Name</Table.Head><Table.Head>Kind</Table.Head><Table.Head>State</Table.Head><Table.Head>Interval</Table.Head><Table.Head>Observed</Table.Head><Table.Head class="w-20"><span class="sr-only">Actions</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each resource.healthChecks as item}<Table.Row><Table.Cell><div><span class="font-medium">{item.name}</span>{#if item.message}<p class="mt-1 max-w-xs text-xs text-muted-foreground">{item.message}</p>{/if}</div></Table.Cell><Table.Cell class="capitalize">{item.kind}</Table.Cell><Table.Cell><StatusBadge status={item.state || 'unknown'} /></Table.Cell><Table.Cell>{item.intervalSeconds}s</Table.Cell><Table.Cell>{observedLabel(item.observedAt)}</Table.Cell><Table.Cell class="text-right"><Button size="sm" variant="ghost" onclick={() => openHealthDialog(item)}>Edit</Button></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>{/if}</Card.Content></Card.Root>
    {/if}

    {#if section === 'overview'}
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
    {/if}
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
    requiredPhrase={destructiveAction?.kind === 'archive-endpoint' ? 'DELETE' : ''}
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
        <Dialog.Header><Dialog.Title>Turn on WireGuard access</Dialog.Title><Dialog.Description>Enable a private network for this Resource. You can then create endpoints routed through it.</Dialog.Description></Dialog.Header>
        <FormField label="Private network" error={errors.privateNetworkId}><NativeSelect.Root bind:value={privateAccessNetworkId} class="w-full" required><NativeSelect.Option value="">Select a private network</NativeSelect.Option>{#each endpointNetworks as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
        <Dialog.Footer><Button type="button" variant="outline" disabled={dialogAction === 'private-access'} onclick={() => (privateAccessDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'private-access'} aria-busy={dialogAction === 'private-access'}>{#if dialogAction === 'private-access'}<Spinner />{/if}Turn on access</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={deviceDialogOpen}>
    <Dialog.Content>
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); submitDevice() }}>
        <Dialog.Header><Dialog.Title>Grant WireGuard access</Dialog.Title><Dialog.Description>Grant an existing device or enroll a new one for this Resource.</Dialog.Description></Dialog.Header>
        <FormField label="Existing device"><NativeSelect.Root bind:value={device.deviceId} class="w-full"><NativeSelect.Option value="">Enroll a new device</NativeSelect.Option>{#each resource.availableDevices as item}<NativeSelect.Option value={item.id}>{item.name} · {item.privateAddress}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
        {#if !device.deviceId}<FormField label="New device name" error={errors.name}><Input bind:value={device.name} placeholder="MBV MacBook" required /></FormField>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={pendingAction === 'device'} onclick={() => (deviceDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={Boolean(pendingAction)} aria-busy={pendingAction === 'device'}>{#if pendingAction === 'device'}<Spinner />{/if}{device.deviceId ? 'Grant access' : 'Enroll and grant'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={endpointDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveEndpoint() }}>
        <Dialog.Header><Dialog.Title>{editingEndpointId ? 'Edit endpoint' : 'Add endpoint'}</Dialog.Title><Dialog.Description>{editingEndpointId ? 'Update how this Resource endpoint is published.' : 'Define another address for this Resource.'}</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Name" error={errors.name}><Input bind:value={endpoint.name} required /></FormField>
          <FormField label="Available to"><NativeSelect.Root value={endpoint.audience} onchange={(event) => chooseEndpointAudience(event.currentTarget.value)} class="w-full"><NativeSelect.Option value="local_system">Local system</NativeSelect.Option><NativeSelect.Option value="environment">Environments through WireGuard</NativeSelect.Option>{#if resource.engine !== 'opentelemetry'}<NativeSelect.Option value="custom">Custom address</NativeSelect.Option>{/if}</NativeSelect.Root></FormField>
          {#if endpoint.audience === 'environment'}
            <FormField label="Reach via" error={errors.privateNetworkId}><NativeSelect.Root value={endpoint.privateNetworkId} onchange={(event) => chooseEndpointNetwork(event.currentTarget.value)} class="w-full" required><NativeSelect.Option value="">Select a private network</NativeSelect.Option>{#each endpointNetworks as network}<NativeSelect.Option value={network.id}>{network.name} · {network.serverAddresses[resource.installations[0]?.serverId] ?? 'No Server address'}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          {:else if endpoint.audience === 'local_system'}
            <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Reach via</p><p class="mt-1 font-mono text-sm">127.0.0.1 · this DeployCrate system</p></div>
          {:else}
            <FormField label="Address" error={errors.address}><Input bind:value={endpoint.address} required placeholder="Hostname or IP address" /></FormField>
          {/if}
          {#if serviceMappings.length > 0}<FormField label="Installation service"><NativeSelect.Root value={String(serviceMappings.findIndex((mapping: any) => mapping.hostPort === endpoint.port))} onchange={(event) => chooseServiceMapping(Number(event.currentTarget.value))} class="w-full">{#each serviceMappings as mapping, index}<NativeSelect.Option value={String(index)}>{mapping.containerName} · {mapping.containerPort}/{mapping.protocol} → {mapping.hostPort}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{:else}<FormField label="Port"><Input type="number" bind:value={endpoint.port} min="1" max="65535" /></FormField>{/if}
          {#if !endpoint.publication.enabled}<FormField label="Protocol"><NativeSelect.Root bind:value={endpoint.protocol} class="w-full">{#each definition.protocols as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{/if}
          <FormField label="Origin TLS"><NativeSelect.Root bind:value={endpoint.tlsMode} class="w-full">{#each definition.tlsModes as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
        </div>
        {#if endpoint.publication.enabled || endpoint.protocol === 'http' || endpoint.protocol === 'https'}<div class="space-y-4 border border-border bg-muted/20 p-4">
          <label class="flex items-center gap-3 text-sm"><Checkbox bind:checked={endpoint.publication.enabled} /> Publish through Caddy</label>
          {#if endpoint.publication.enabled}<div class="grid gap-4 sm:grid-cols-2"><FormField label="Public hostname" error={errors.hostname}><Input bind:value={endpoint.publication.hostname} required placeholder="database.deploycrate.com" /></FormField>{#if endpoint.protocol === 'http' || endpoint.protocol === 'https'}<FormField label="Public health path" error={errors.healthPath}><Input bind:value={endpoint.publication.healthPath} placeholder="Optional, for example /health" /></FormField>{/if}<div class="sm:col-span-2 text-xs text-muted-foreground"><span class="font-mono">https://{endpoint.publication.hostname || 'hostname'}</span> will reverse proxy to <span class="font-mono">{endpoint.address}:{endpoint.port}</span>.</div></div>{/if}
        </div>{/if}
        <Dialog.Footer>{#if editingEndpointId}<Button type="button" variant="destructive" disabled={dialogAction === 'endpoint'} onclick={archiveEndpoint}>Archive</Button>{/if}<Button type="button" variant="outline" disabled={dialogAction === 'endpoint'} onclick={() => (endpointDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'endpoint'} aria-busy={dialogAction === 'endpoint'}>{#if dialogAction === 'endpoint'}<Spinner />{/if}{editingEndpointId ? 'Save endpoint' : 'Create endpoint'}</Button></Dialog.Footer>
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
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveCredential() }}>
        <Dialog.Header><Dialog.Title>{editingCredentialId ? 'Edit credential' : 'Add application credential'}</Dialog.Title><Dialog.Description>{editingCredentialId ? 'Update credential metadata or rotate its encrypted secret values.' : 'Create an encrypted application credential for this Resource.'}</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Display name" error={errors.name}><Input bind:value={credential.name} required /></FormField><FormField label="Username" error={errors.username}><Input bind:value={credential.username} readonly={Boolean(editingCredentialId) && managedPostgreSQL} required autocomplete="username" /></FormField>{#if databaseBacked}<FormField label="Database" error={errors['metadata.database']}><NativeSelect.Root bind:value={credential.database} class="w-full" disabled={credential.purpose === 'administrator'} required={credential.purpose === 'application'}><NativeSelect.Option value="">No Database</NativeSelect.Option>{#each resource.databases as item}<NativeSelect.Option value={item.name}>{item.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{/if}{#if editingCredentialId}<label class="flex items-center gap-2 self-end text-xs"><Checkbox bind:checked={credential.rotate} /> Rotate secret values</label>{/if}{#if !editingCredentialId || credential.rotate}{#each definition.credentialFields as field}<FormField label={field.label} error={errors[`secretValues.${field.name}`]}><Input type={field.secret ? 'password' : 'text'} value={credential.secretValues[field.name] ?? ''} oninput={(event) => credential.secretValues[field.name] = event.currentTarget.value} required={field.required} autocomplete="new-password" /></FormField>{/each}{/if}</div>
        <Dialog.Footer>{#if editingCredentialId}<Button type="button" variant="destructive" disabled={dialogAction === 'credential'} onclick={archiveCredential}>Archive</Button>{/if}<Button type="button" variant="outline" disabled={dialogAction === 'credential'} onclick={() => (credentialDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={(!editingCredentialId && !canAddApplicationUser) || dialogAction === 'credential'} aria-busy={dialogAction === 'credential'}>{#if dialogAction === 'credential'}<Spinner />{/if}{editingCredentialId ? (credential.rotate ? 'Save and rotate' : 'Save credential') : 'Create application credential'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={backupPolicyDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveBackupPolicy() }}>
        <Dialog.Header><Dialog.Title>{selectedBackups?.policy ? 'Edit backup policy' : 'Set up backups'}</Dialog.Title><Dialog.Description>{selectedBackups?.databaseName ? `Configure backups for ${selectedBackups.databaseName}.` : 'Configure backups for this Database.'}</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Object Storage" error={errors.backupDestinationId}><NativeSelect.Root bind:value={backupPolicy.backupDestinationId} class="w-full" required>{#each backups.destinations as destination}<NativeSelect.Option value={destination.id}>{destination.name} · {destination.bucket}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Schedule" error={errors.schedule}><Input bind:value={backupPolicy.schedule} class="font-mono" placeholder="0 2 * * *" required /></FormField>
          <FormField label="Keep last"><Input type="number" min="0" bind:value={backupPolicy.keepLast} /></FormField>
          <FormField label="Keep daily"><Input type="number" min="0" bind:value={backupPolicy.keepDaily} /></FormField>
          <FormField label="Keep weekly"><Input type="number" min="0" bind:value={backupPolicy.keepWeekly} /></FormField>
          <FormField label="Keep monthly"><Input type="number" min="0" bind:value={backupPolicy.keepMonthly} /></FormField>
        </div>
        <Dialog.Footer><Button type="button" variant="outline" onclick={() => (backupPolicyDialogOpen = false)}>Cancel</Button><Button type="submit">{selectedBackups?.policy ? 'Save policy' : 'Create policy'}</Button></Dialog.Footer>
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
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveVolume() }}>
        <Dialog.Header><Dialog.Title>{editingVolumeId ? 'Edit volume' : 'Add volume'}</Dialog.Title><Dialog.Description>{editingVolumeId ? 'Update this durable storage definition.' : 'Create durable storage for this Resource.'}</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={volume.name} required /></FormField><FormField label="Driver"><Input bind:value={volume.driver} required /></FormField><FormField label="Server"><NativeSelect.Root bind:value={volume.serverId} class="w-full">{#each options.servers as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><div class="sm:col-span-2"><FormField label="Configuration JSON" error={jsonError}><Textarea bind:value={volume.configurationText} class="min-h-28 font-mono" /></FormField></div></div>
        <Dialog.Footer>{#if editingVolumeId}<Button type="button" variant="destructive" disabled={dialogAction === 'volume'} onclick={archiveVolume}>Archive</Button>{/if}<Button type="button" variant="outline" disabled={dialogAction === 'volume'} onclick={() => (volumeDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'volume'} aria-busy={dialogAction === 'volume'}>{#if dialogAction === 'volume'}<Spinner />{/if}{editingVolumeId ? 'Save volume' : 'Create volume'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={mountDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveMount() }}>
        <Dialog.Header><Dialog.Title>{editingMountId ? 'Edit mount' : 'Add mount'}</Dialog.Title><Dialog.Description>Attach a Resource volume to an installation.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Mount path"><Input bind:value={mount.mountPath} required /></FormField><FormField label="Volume"><NativeSelect.Root bind:value={mount.resourceVolumeId} class="w-full">{#each resource.volumes as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Installation"><NativeSelect.Root bind:value={mount.resourceInstallationId} class="w-full">{#each resource.installations as value}<NativeSelect.Option value={value.id}>{value.containerName}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><label class="flex items-center gap-2 self-end text-xs"><Checkbox bind:checked={mount.readOnly} /> Read only</label></div>
        <Dialog.Footer>{#if editingMountId}<Button type="button" variant="destructive" disabled={dialogAction === 'mount'} onclick={archiveMount}>Archive</Button>{/if}<Button type="button" variant="outline" disabled={dialogAction === 'mount'} onclick={() => (mountDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'mount'} aria-busy={dialogAction === 'mount'}>{#if dialogAction === 'mount'}<Spinner />{/if}{editingMountId ? 'Save mount' : 'Create mount'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={healthDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
      <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); saveHealth() }}>
        <Dialog.Header><Dialog.Title>{editingHealthId ? 'Edit health check' : 'Add health check'}</Dialog.Title><Dialog.Description>Define how DeployCrate should evaluate this Resource.</Dialog.Description></Dialog.Header>
        <div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={health.name} required /></FormField><FormField label="Kind"><NativeSelect.Root bind:value={health.kind} class="w-full">{#each definition.healthCheckKinds as value}<NativeSelect.Option value={value}>{value}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Endpoint"><NativeSelect.Root bind:value={health.resourceEndpointId} class="w-full"><NativeSelect.Option value="">None</NativeSelect.Option>{#each resource.endpoints as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Credential"><NativeSelect.Root bind:value={health.resourceCredentialId} class="w-full"><NativeSelect.Option value="">None</NativeSelect.Option>{#each resource.credentials as value}<NativeSelect.Option value={value.id}>{value.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Interval seconds"><Input type="number" bind:value={health.intervalSeconds} min="1" /></FormField><FormField label="Timeout seconds"><Input type="number" bind:value={health.timeoutSeconds} min="1" /></FormField><FormField label="Failure threshold"><Input type="number" bind:value={health.failureThreshold} min="1" /></FormField><FormField label="Success threshold"><Input type="number" bind:value={health.successThreshold} min="1" /></FormField><label class="flex items-center gap-2 text-xs"><Checkbox bind:checked={health.enabled} /> Enabled</label><div class="sm:col-span-2"><FormField label="Configuration JSON" error={jsonError}><Textarea bind:value={health.configurationText} class="min-h-28 font-mono" /></FormField></div></div>
        <Dialog.Footer>{#if editingHealthId}<Button type="button" variant="destructive" disabled={dialogAction === 'health'} onclick={archiveHealth}>Archive</Button>{/if}<Button type="button" variant="outline" disabled={dialogAction === 'health'} onclick={() => (healthDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={dialogAction === 'health'} aria-busy={dialogAction === 'health'}>{#if dialogAction === 'health'}<Spinner />{/if}{editingHealthId ? 'Save health check' : 'Create health check'}</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={credentialPasswordDialogOpen} onOpenChange={(open) => { if (!open && !credentialRevealProcessing) { currentPassword = ''; credentialRevealError = ''; selectedCredential = null } }}>
    <Dialog.Content showCloseButton={!credentialRevealProcessing}>
      <form class="grid gap-4" onsubmit={revealCredential}>
        <Dialog.Header><Dialog.Title>View Resource credential</Dialog.Title><Dialog.Description>Enter your current password to reveal {selectedCredential?.name ?? 'this credential'}.</Dialog.Description></Dialog.Header>
        <FormField label="Current password"><Input type="password" bind:value={currentPassword} autocomplete="current-password" autofocus required disabled={credentialRevealProcessing} /></FormField>
        {#if credentialRevealError}<p class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{credentialRevealError}</p>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={credentialRevealProcessing} onclick={() => (credentialPasswordDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={!currentPassword || credentialRevealProcessing}>{#if credentialRevealProcessing}<Spinner />{/if}Continue</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={revealedCredentialDialogOpen} onOpenChange={(open) => { if (!open) closeRevealedCredential() }}>
    <Dialog.Content class="sm:max-w-xl">
      <Dialog.Header><Dialog.Title>{revealedCredential?.name ?? 'Resource credential'}</Dialog.Title><Dialog.Description>This decrypted credential is shown only until you close this dialog.</Dialog.Description></Dialog.Header>
      {#if revealedCredential}
        <div class="grid gap-4">
          {#if revealedCredential.username}<div class="grid gap-2"><p class="text-xs font-medium">Username</p><div class="flex gap-2"><Input value={revealedCredential.username} readonly /><Button type="button" variant="outline" onclick={() => copyCredential(revealedCredential!.username, 'Username')}>Copy</Button></div></div>{/if}
          {#each Object.entries(revealedCredential.values) as [name, value] (name)}
            <div class="grid gap-2"><p class="text-xs font-medium">{displayLabel(name)}</p><div class="flex gap-2"><Input type="text" {value} readonly autocomplete="off" /><Button type="button" variant="outline" onclick={() => copyCredential(value, displayLabel(name))}>Copy</Button></div></div>
          {/each}
        </div>
      {/if}
      <Dialog.Footer><Button type="button" onclick={closeRevealedCredential}>Done</Button></Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
