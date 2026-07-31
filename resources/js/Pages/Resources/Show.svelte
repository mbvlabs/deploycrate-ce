<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { onMount } from 'svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Dialog from '@/Components/ui/dialog'
  import DataField from '@/Components/DataField.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type Kind = { kind: string; label: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { kinds: Kind[]; servers: Array<{ id: string; name: string; address: string }>; privateNetworks: PrivateNetwork[]; registryCredentials: Array<{ id: string; name: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }
  type BackupDestination = { id: string; name: string; provider: string; endpoint: string; region: string; bucket: string; prefix: string; verifiedAt: string | null; lastUsedAt: string | null }
  type BackupPolicy = { id: string; schedule: string; active: boolean; nextRunAt: string; backupDestinationId: string; keepLast: number; keepDaily: number; keepWeekly: number; keepMonthly: number }
  type BackupHistory = { id: string; status: string; triggerType: string; scheduledAt: string; finishedAt: string | null; verifiedAt: string | null; sizeBytes: number | null; error: string; canRestore: boolean }
  type RestoreHistory = { id: string; status: string; requestedAt: string; startedAt: string | null; finishedAt: string | null; verifiedAt: string | null; cutoverAt: string | null; rolledBackAt: string | null; error: string; backupId: string; backupScheduledAt: string; safetyBackupId: string | null }
  type Backups = { eligibility: { eligible: boolean; reason: string; installationId: string | null }; policy: BackupPolicy | null; destinations: BackupDestination[]; history: BackupHistory[]; restores: RestoreHistory[]; activeRestore: boolean }
  type DestructiveAction =
    | { kind: 'remove-container'; installationId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'disable-private-access'; title: string; description: string; confirmationLabel: string }
    | { kind: 'revoke-device'; deviceId: string; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-resource'; title: string; description: string; confirmationLabel: string }
    | { kind: 'archive-backup-policy'; policyId: string; title: string; description: string; confirmationLabel: string }
  let { auth, resource, backups, options, enrollment = null, errors = {} }: { auth: { email: string }; resource: any; backups: Backups; options: Options; enrollment?: Enrollment | null; errors?: Record<string, string> } = $props()

  const definition = $derived(options.kinds.find((kind) => kind.kind === resource.kind) ?? options.kinds[0])
  const endpointNetworks = $derived(resource.managementMode === 'managed' && resource.installations.length === 1
    ? options.privateNetworks.filter((network) => network.serverIds.includes(resource.installations[0].serverId) && Boolean(network.serverAddresses[resource.installations[0].serverId]))
    : options.privateNetworks)
  const primaryEndpoint = $derived(resource.endpoints.find((item: any) => item.role === 'primary' && !item.privateNetworkId))
  const privateEndpoint = $derived(resource.endpoints.find((item: any) => Boolean(item.privateNetworkId)))
  const privateNetwork = $derived(privateEndpoint ? options.privateNetworks.find((item) => item.id === privateEndpoint.privateNetworkId) : undefined)
  const administratorCredentials = $derived(resource.credentials.filter((item: any) => Boolean(item.resourceInstallationId)))
  const applicationCredentials = $derived(resource.credentials.filter((item: any) => !item.resourceInstallationId))
	const databaseBacked = $derived(Boolean(resource.databaseBacking))
	const managedPostgreSQL = $derived(resource.managementMode === 'managed' && resource.kind === 'postgresql' && !databaseBacked)
  const containerRunning = $derived(resource.installations.some((item: any) => item.serviceState === 'running'))
  const canAddApplicationUser = $derived(!managedPostgreSQL || containerRunning)
  const lastSuccessfulBackup = $derived(backups.history.find((item) => item.status === 'verified'))
  const activeRestore = $derived(backups.activeRestore)
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm aria-invalid:border-destructive'
  const textareaClass = 'min-h-24 w-full border border-input bg-background px-3 py-2 font-mono text-xs'
  const overallStatus = $derived.by(() => {
		if (databaseBacked) {
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'unhealthy')) return { label: 'Unhealthy', tone: 'bad', detail: 'The published Database access check reached its failure threshold.' }
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'degraded')) return { label: 'Degraded', tone: 'warn', detail: 'The published Database access check is failing below its threshold.' }
			if (resource.healthChecks.some((item: any) => item.enabled && item.state === 'healthy')) return { label: 'Available', tone: 'good', detail: 'The published endpoint and application credential can access the Database.' }
			return { label: 'Access unknown', tone: 'neutral', detail: 'No fresh Database access observation is available.' }
		}
    if (resource.managementMode === 'external') return { label: 'External', tone: 'neutral', detail: 'Lifecycle is managed outside DeployCrate.' }
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
  let volumeDialogOpen = $state(false)
  let mountDialogOpen = $state(false)
  let healthDialogOpen = $state(false)
  let destructiveActionDialogOpen = $state(false)
  let restoreDialogOpen = $state(false)
  let wireGuardConfigurationDialogOpen = $state(false)
  let jsonError = $state('')
  let pendingAction = $state('')
  let destructiveAction = $state<DestructiveAction | null>(null)
  let shownEnrollmentGrantId = $state('')
  let endpoint = $state(initialEndpoint())
  let credential = $state({ name: 'Application user', username: '', secretValues: {} as Record<string, string> })
  let privateAccessNetworkId = $state('')
  let device = $state({ name: '', deviceId: '' })
  let volume = $state(initialVolume())
  let mount = $state(initialMount())
  let health = $state(initialHealth())
  let backupPolicy = $state(initialBackupPolicy())
  let restoreBackup = $state<BackupHistory | null>(null)
  let restoreConfirmation = $state('')

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

  function initialEndpoint() {
    return { name: 'Primary', role: definition?.endpointRoles[0] ?? 'primary', address: '127.0.0.1', port: definition?.defaultPort ?? 1, protocol: definition?.defaultProtocol ?? 'tcp', tlsMode: definition?.defaultTlsMode ?? 'disable', privateNetworkId: '' }
  }
  function initialVolume() { return { name: '', driver: 'local', configurationText: '{}', serverId: options.servers[0]?.id ?? '' } }
  function initialMount() { return { mountPath: '/data', readOnly: false, resourceVolumeId: resource.volumes[0]?.id ?? '', resourceInstallationId: resource.installations[0]?.id ?? '' } }
	function initialHealth() { return { name: 'Readiness', kind: resource.kind === 'postgresql' || resource.kind === 'clickhouse' ? resource.kind : definition?.healthCheckKinds?.[0] ?? 'tcp', configurationText: '{}', intervalSeconds: 30, timeoutSeconds: 5, failureThreshold: 3, successThreshold: 1, enabled: true, resourceInstallationId: resource.installations[0]?.id ?? '', resourceEndpointId: primaryEndpoint?.id ?? '', resourceCredentialId: (databaseBacked ? applicationCredentials[0] : administratorCredentials[0])?.id ?? '' } }
  function initialBackupPolicy() {
    return {
      schedule: backups.policy?.schedule ?? '0 2 * * *',
      backupDestinationId: backups.policy?.backupDestinationId ?? backups.destinations[0]?.id ?? '',
      keepLast: backups.policy?.keepLast ?? 7,
      keepDaily: backups.policy?.keepDaily ?? 7,
      keepWeekly: backups.policy?.keepWeekly ?? 4,
      keepMonthly: backups.policy?.keepMonthly ?? 6,
    }
  }

  function json(value: string) {
    try { jsonError = ''; return JSON.parse(value || '{}') }
    catch { jsonError = 'Configuration and metadata must contain valid JSON.'; throw new Error(jsonError) }
  }

  function submit(action: () => void) { try { action() } catch {} }
  function openWireGuardConfiguration() { if (enrollment?.clientConfiguration) wireGuardConfigurationDialogOpen = true }
  function createEndpoint() { router.post(routes.resourceEndpointCreate(resource.id), { ...endpoint, settings: {}, resourceInstallationId: resource.managementMode === 'managed' ? resource.installations[0]?.id ?? '' : '' }, { onSuccess: () => (endpointDialogOpen = false), onError: () => (endpointDialogOpen = true) }) }
  function chooseEndpointNetwork(networkId: string) {
    endpoint.privateNetworkId = networkId
    if (resource.managementMode !== 'managed') {
      if (!networkId) endpoint.address = '127.0.0.1'
      return
    }
    const selected = options.privateNetworks.find((network) => network.id === networkId)
    const serverId = resource.installations[0]?.serverId
    endpoint.address = selected && serverId ? selected.serverAddresses[serverId] ?? '127.0.0.1' : '127.0.0.1'
  }
  function openCredentialDialog() { if (canAddApplicationUser) credentialDialogOpen = true }
  function createCredential() { if (canAddApplicationUser) router.post(routes.resourceCredentialCreate(resource.id), { ...credential, metadata: {} }, { onSuccess: () => { credentialDialogOpen = false; credential.secretValues = {} }, onError: () => (credentialDialogOpen = true) }) }
  function enablePrivateAccess() { router.post(routes.resourcePrivateAccessCreate(resource.id), { privateNetworkId: privateAccessNetworkId }, { onSuccess: () => (privateAccessDialogOpen = false), onError: () => (privateAccessDialogOpen = true) }) }
  function disablePrivateAccess() {
    confirmDestructive({
      kind: 'disable-private-access',
      title: 'Remove from private network?',
      description: 'The private listener will be removed and every device grant for this Resource will be revoked.',
      confirmationLabel: 'Remove from private network',
    })
  }
  function submitDevice() { router.post(routes.resourcePrivateAccessDeviceCreate(resource.id), device) }
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
  function createVolume() { submit(() => router.post(routes.resourceVolumeCreate(resource.id), { ...volume, configuration: json(volume.configurationText) }, { onSuccess: () => (volumeDialogOpen = false), onError: () => (volumeDialogOpen = true) })) }
  function createMount() { router.post(routes.resourceMountCreate(resource.id), mount, { onSuccess: () => (mountDialogOpen = false), onError: () => (mountDialogOpen = true) }) }
  function createHealth() { submit(() => router.post(routes.resourceHealthCheckCreate(resource.id), { ...health, configuration: json(health.configurationText) }, { onSuccess: () => (healthDialogOpen = false), onError: () => (healthDialogOpen = true) })) }
  function saveBackupPolicy() {
    if (backups.policy) router.patch(routes.resourceBackupPolicyUpdate(resource.id, backups.policy.id), backupPolicy)
    else router.post(routes.resourceBackupPolicyCreate(resource.id), backupPolicy)
  }
  function pauseBackupPolicy() { if (backups.policy) router.post(routes.resourceBackupPolicyPause(resource.id, backups.policy.id), {}) }
  function resumeBackupPolicy() { if (backups.policy) router.post(routes.resourceBackupPolicyResume(resource.id, backups.policy.id), {}) }
  function runBackupPolicy() { if (backups.policy) router.post(routes.resourceBackupPolicyRun(resource.id, backups.policy.id), {}) }
  function confirmBackupPolicyArchive() {
    if (!backups.policy) return
    confirmDestructive({ kind: 'archive-backup-policy', policyId: backups.policy.id, title: 'Archive backup policy?', description: 'Future schedules stop immediately. Existing backup history and artifacts are retained.', confirmationLabel: 'Archive policy' })
  }

  function openRestoreDialog(backup: BackupHistory) {
    restoreBackup = backup
    restoreConfirmation = ''
    restoreDialogOpen = true
  }

  function submitRestore() {
    if (!restoreBackup || restoreConfirmation !== resource.name) return
    router.post(routes.resourceRestoreCreate(resource.id), {
      backupId: restoreBackup.id,
      confirmation: restoreConfirmation,
    }, {
      onSuccess: () => { restoreDialogOpen = false; restoreBackup = null; restoreConfirmation = '' },
      onError: () => (restoreDialogOpen = true),
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
      description: 'The Resource will no longer be available for new operations. Existing dependencies must be removed first.',
      confirmationLabel: 'Archive Resource',
    })
  }

  function confirmDestructive(action: DestructiveAction) {
    destructiveAction = action
    destructiveActionDialogOpen = true
  }

  function executeDestructiveAction() {
    const action = destructiveAction
    if (!action) return

    destructiveActionDialogOpen = false

    if (action.kind === 'remove-container') lifecycle(action.installationId, 'remove')
    if (action.kind === 'disable-private-access') router.delete(routes.resourcePrivateAccessDestroy(resource.id))
    if (action.kind === 'revoke-device') router.delete(routes.resourcePrivateAccessDeviceDestroy(resource.id, action.deviceId))
    if (action.kind === 'archive-resource') router.delete(routes.resourceDestroy(resource.id))
    if (action.kind === 'archive-backup-policy') router.delete(routes.resourceBackupPolicyDestroy(resource.id, action.policyId))
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
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{resource.kind} · {resource.category}</p>
        <h1 class="mt-3 text-3xl font-semibold">{resource.name}</h1>
        <p class="mt-2 text-sm capitalize text-muted-foreground">{resource.managementMode} · {resource.sharingScope} sharing</p>
		{#if resource.databaseBacking}<p class="mt-1 text-xs text-muted-foreground">{resource.databaseBacking.mode === 'dedicated' ? 'Dedicated' : 'Cluster / Server'} · Database <span class="font-mono">{resource.databaseBacking.databaseName}</span> · {resource.databaseBacking.clusterName}</p>{/if}
      </div>
      <div class="flex gap-2">
        <Button variant="outline" onclick={() => router.reload({ only: ['resource'] })}>Refresh status</Button>
        <Button>{#snippet child({ props })}<Link {...props} href={routes.resourceEdit(resource.id)}>Edit Resource</Link>{/snippet}</Button>
      </div>
    </header>

    {#if Object.keys(errors).length > 0}<div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">The item could not be created. Review the dialog fields and try again.</div>{/if}
    {#if jsonError}<div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{jsonError}</div>{/if}

    <Card.Root class="overflow-hidden">
      <Card.Content class="grid gap-0 p-0 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div class="p-6">
          <div class="flex flex-wrap items-center gap-3">
            <span class:!border-success={overallStatus.tone === 'good'} class:!text-success={overallStatus.tone === 'good'} class:!border-destructive={overallStatus.tone === 'bad'} class:!text-destructive={overallStatus.tone === 'bad'} class="border border-border px-2 py-1 text-xs font-medium">{overallStatus.label}</span>
            {#if resource.installations.length > 0}<span class="border border-border bg-muted/30 px-2 py-1 text-xs">Docker</span>{/if}
          </div>
		  <h2 class="mt-5 text-xl font-semibold">{databaseBacked ? 'Access status' : 'Container status'}</h2>
          <p class="mt-2 max-w-2xl text-sm text-muted-foreground">{overallStatus.detail}</p>
        </div>
        <div class="grid grid-cols-2 gap-5 border-t border-border bg-muted/20 p-6 text-sm lg:grid-cols-1 lg:border-l lg:border-t-0">
          <DataField label="Installations" value={String(resource.installations.length)} />
          <DataField label="Connected Environments" value={String(resource.connectionCount)} />
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Action>{#if backups.policy}<span class="border border-border px-2 py-1 text-xs" class:text-success={backups.policy.active}>{backups.policy.active ? 'Active' : 'Paused'}</span>{/if}</Card.Action>
        <Card.Title>Backups</Card.Title>
        <Card.Description>Encrypted PostgreSQL logical backups using a verified Object Storage connection.</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-6">
        {#if !backups.eligibility.eligible}
          <p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground">{backups.eligibility.reason}</p>
        {:else}
          <form class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" onsubmit={(event) => { event.preventDefault(); saveBackupPolicy() }}>
            <FormField label="Object Storage" error={errors.backupDestinationId}>
              <select bind:value={backupPolicy.backupDestinationId} class={selectClass} required>
                {#each backups.destinations as destination}<option value={destination.id}>{destination.name} · {destination.bucket}</option>{/each}
              </select>
            </FormField>
            <FormField label="Schedule" error={errors.schedule}><Input bind:value={backupPolicy.schedule} class="font-mono" placeholder="0 2 * * *" required /></FormField>
            <div class="hidden lg:block"></div>
            <FormField label="Keep last"><Input type="number" min="0" bind:value={backupPolicy.keepLast} /></FormField>
            <FormField label="Keep daily"><Input type="number" min="0" bind:value={backupPolicy.keepDaily} /></FormField>
            <FormField label="Keep weekly"><Input type="number" min="0" bind:value={backupPolicy.keepWeekly} /></FormField>
            <FormField label="Keep monthly"><Input type="number" min="0" bind:value={backupPolicy.keepMonthly} /></FormField>
            <div class="flex flex-wrap items-end gap-2 sm:col-span-2">
              <Button type="submit">{backups.policy ? 'Save policy' : 'Create policy'}</Button>
              {#if backups.policy}
                {#if backups.policy.active}<Button type="button" variant="outline" onclick={pauseBackupPolicy}>Pause</Button>{:else}<Button type="button" variant="outline" onclick={resumeBackupPolicy}>Resume</Button>{/if}
                <Button type="button" variant="outline" disabled={!backups.policy.active} onclick={runBackupPolicy}>Back up now</Button>
                <Button type="button" variant="destructive" onclick={confirmBackupPolicyArchive}>Archive</Button>
              {/if}
            </div>
          </form>
          {#if backups.policy}
            <div class="grid gap-4 border-t border-border pt-5 sm:grid-cols-3">
              <DataField label="Next run" value={backups.policy.active ? observedLabel(backups.policy.nextRunAt) : 'Paused'} />
              <DataField label="Last successful" value={observedLabel(lastSuccessfulBackup?.finishedAt ?? null)} />
              <DataField label="Last verified" value={observedLabel(lastSuccessfulBackup?.verifiedAt ?? null)} />
            </div>
          {/if}
        {/if}

        <section class="space-y-3 border-t border-border pt-5">
          <div><h3 class="text-sm font-medium">Recent history</h3><p class="mt-1 text-xs text-muted-foreground">The latest Resource backup attempts, including retained outcomes after policy archival.</p></div>
          {#if backups.history.length === 0}<p class="text-sm text-muted-foreground">No Resource backups have been requested.</p>{/if}
          {#each backups.history as item (item.id)}
            <div class="grid gap-3 border border-border p-3 text-sm sm:grid-cols-[1fr_auto]">
              <div><div class="flex flex-wrap items-center gap-2"><span class="font-medium capitalize">{item.status.replaceAll('_', ' ')}</span><span class="text-xs uppercase tracking-wider text-muted-foreground">{item.triggerType}</span></div><p class="mt-1 text-xs text-muted-foreground">{observedLabel(item.scheduledAt)} · {bytesLabel(item.sizeBytes)}</p>{#if item.error}<p class="mt-2 text-xs text-destructive">{item.error}</p>{/if}</div>
              <div class="flex items-center gap-3"><span class="font-mono text-xs text-muted-foreground">{item.id.slice(0, 8)}</span>{#if item.canRestore}<Button size="sm" variant="destructive" onclick={() => openRestoreDialog(item)}>Restore</Button>{/if}</div>
            </div>
          {/each}
        </section>

        <section class="space-y-3 border-t border-border pt-5">
          <div class="flex flex-wrap items-start justify-between gap-3"><div><h3 class="text-sm font-medium">Restore history</h3><p class="mt-1 text-xs text-muted-foreground">A safety backup is verified before any database is changed.</p></div>{#if activeRestore}<span class="border border-primary/50 px-2 py-1 text-xs text-primary">Restore in progress</span>{/if}</div>
          {#if backups.restores.length === 0}<p class="text-sm text-muted-foreground">No database restores have been requested.</p>{/if}
          {#each backups.restores as restore (restore.id)}
            <div class="grid gap-3 border border-border p-3 text-sm sm:grid-cols-[1fr_auto]">
              <div><div class="flex flex-wrap items-center gap-2"><span class="font-medium capitalize">{restore.status.replaceAll('_', ' ')}</span><span class="text-xs text-muted-foreground">Source {observedLabel(restore.backupScheduledAt)}</span></div><p class="mt-1 text-xs text-muted-foreground">Requested {observedLabel(restore.requestedAt)}{restore.safetyBackupId ? ` · Safety backup ${restore.safetyBackupId.slice(0, 8)}` : ''}</p>{#if restore.error}<p class="mt-2 text-xs text-destructive">{restore.error}</p>{/if}</div>
              <span class="font-mono text-xs text-muted-foreground">{restore.id.slice(0, 8)}</span>
            </div>
          {/each}
        </section>
      </Card.Content>
    </Card.Root>

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
            <div class="mt-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-5">
              <DataField label="Service" value={item.serviceState || 'Unknown'} />
              <DataField label="Health" value={item.health || 'Unknown'} />
              <DataField label="Server" value={item.serverName} />
              <DataField label="Container ID" value={item.containerDetails?.id?.slice(0, 12) || 'Not created'} />
              <DataField label="Observed" value={observedLabel(item.observedAt)} />
            </div>
            {#if item.healthReason}<p class="mt-4 border-l-2 border-border pl-3 text-xs text-muted-foreground">{item.healthReason}</p>{/if}
            {#if !item.canControl}<p class="mt-4 text-xs text-muted-foreground">Container controls are unavailable because this Server does not have a connected Resource container executor.</p>{/if}
          </article>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Primary service</Card.Title><Card.Description>The Docker origin and its optional private network path.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#if primaryEndpoint}
          <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            <DataField label="Runtime origin" value={`${primaryEndpoint.address}:${primaryEndpoint.port}`} />
            <DataField label="Installation" value={resource.installations.find((item: any) => item.id === primaryEndpoint.resourceInstallationId)?.containerName ?? 'External'} />
            <DataField label="Protocol" value={primaryEndpoint.protocol} />
            <DataField label="TLS" value={primaryEndpoint.tlsMode} />
          </div>
        {:else}<p class="text-sm text-destructive">No primary origin is configured.</p>{/if}

        {#if resource.managementMode === 'managed'}
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
              <form class="mt-5 grid gap-4 border-t border-border pt-5 sm:grid-cols-3" onsubmit={(event) => { event.preventDefault(); submitDevice() }}><FormField label="Existing device"><select bind:value={device.deviceId} class={selectClass}><option value="">Enroll a new device</option>{#each resource.availableDevices as item}<option value={item.id}>{item.name} · {item.privateAddress}</option>{/each}</select></FormField>{#if !device.deviceId}<FormField label="New device name" error={errors.name}><Input bind:value={device.name} placeholder="MBV MacBook" required /></FormField>{/if}<div class="flex items-end"><Button type="submit">{device.deviceId ? 'Grant existing device' : 'Enroll new device'}</Button></div></form>
            {/if}
          </section>
        {:else}
          <div class="border-t border-border pt-5"><Button size="sm" variant="outline" onclick={() => (endpointDialogOpen = true)}>Add endpoint</Button></div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Action><Button size="sm" variant="outline" disabled={!canAddApplicationUser} onclick={openCredentialDialog}>Add application user</Button></Card.Action><Card.Title>Credentials</Card.Title><Card.Description>{resource.kind === 'postgresql' ? `Administrator access is installation-specific. Application users receive migration access only to ${resource.databaseName}.` : 'Encrypted application credentials for this Resource.'}</Card.Description></Card.Header>
      <Card.Content class="grid gap-6 lg:grid-cols-2">
        {#if managedPostgreSQL && !containerRunning}<p class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground lg:col-span-2">Start the PostgreSQL container before adding an application user. DeployCrate must connect to the running server to create its LOGIN role.</p>{/if}
        <section><h3 class="text-sm font-medium">Resource administrator</h3><div class="mt-3 space-y-3">{#if administratorCredentials.length === 0}<p class="text-sm text-muted-foreground">No installation-specific administrator.</p>{/if}{#each administratorCredentials as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.username}</p><span class="text-xs text-muted-foreground">{item.hasEncryptedPayload ? 'Encrypted' : 'Missing secret'}</span></div><p class="mt-2 text-xs text-muted-foreground">Resource administrator</p></div>{/each}</div></section>
        <section><h3 class="text-sm font-medium">Application users</h3><div class="mt-3 space-y-3">{#if applicationCredentials.length === 0}<p class="text-sm text-muted-foreground">No application users.</p>{/if}{#each applicationCredentials as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.username}</p><span class="text-xs text-muted-foreground">{item.hasEncryptedPayload ? 'Encrypted' : 'Missing secret'}</span></div><p class="mt-2 text-xs text-muted-foreground">{item.name}</p></div>{/each}</div></section>
      </Card.Content>
    </Card.Root>

    <div class="grid gap-6 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Action>{#if resource.managementMode === 'managed' && (resource.volumes.length === 0 || (resource.volumes.length === 1 && resource.installations.length === 1 && resource.mounts.length === 0))}<div class="flex gap-2">{#if resource.volumes.length === 0}<Button size="sm" variant="outline" onclick={() => (volumeDialogOpen = true)}>Add volume</Button>{/if}{#if resource.volumes.length === 1 && resource.installations.length === 1 && resource.mounts.length === 0}<Button size="sm" variant="outline" onclick={() => (mountDialogOpen = true)}>Add mount</Button>{/if}</div>{/if}</Card.Action><Card.Title>Storage</Card.Title><Card.Description>The primary durable volume and its installation mount.</Card.Description></Card.Header><Card.Content class="space-y-3">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No primary volume configured.</p>{/if}{#each resource.volumes as item}<div class="border border-border p-3"><p class="font-medium">{item.name}</p><p class="mt-2 text-xs text-muted-foreground">{item.driver} on {item.serverName}</p>{#each resource.mounts.filter((mount: any) => mount.resourceVolumeId === item.id) as mount}<p class="mt-2 font-mono text-xs">{mount.mountPath} → {mount.containerName}{mount.readOnly ? ' (read only)' : ''}</p>{/each}</div>{/each}</Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Action><Button size="sm" variant="outline" disabled={resource.installations.length === 0} onclick={() => (healthDialogOpen = true)}>Add check</Button></Card.Action><Card.Title>Health checks</Card.Title><Card.Description>Desired checks and their latest observations.</Card.Description></Card.Header><Card.Content class="space-y-3">{#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{/if}{#each resource.healthChecks as item}<div class="border border-border p-3"><div class="flex justify-between gap-3"><p class="font-medium">{item.name}</p><span class:text-success={item.state === 'healthy'} class:text-warning={item.state === 'degraded'} class:text-destructive={item.state === 'unhealthy'} class="text-xs capitalize">{item.state || 'Unknown'}</span></div><p class="mt-2 text-xs text-muted-foreground">{item.kind} · every {item.intervalSeconds}s · {item.enabled ? 'Enabled' : 'Disabled'}</p><p class="mt-1 text-xs text-muted-foreground">Observed {observedLabel(item.observedAt)}{item.latencyMs !== null ? ` · ${item.latencyMs} ms` : ''} · successes {item.consecutiveSuccesses} · failures {item.consecutiveFailures}</p>{#if item.message}<p class="mt-2 text-xs text-muted-foreground">{item.message}</p>{/if}</div>{/each}</Card.Content></Card.Root>
    </div>

    <Card.Root>
      <Card.Header><Card.Action><span class="text-xs text-muted-foreground">{resource.connectionCount} connected</span></Card.Action><Card.Title>Connected Environments</Card.Title><Card.Description>Connections are managed from Environment pages.</Card.Description></Card.Header>
      <Card.Content class="space-y-3">
        {#if resource.connections.length === 0}<p class="text-sm text-muted-foreground">This Resource is not connected to an Environment.</p>{/if}
        {#each resource.connections as item}<div class="grid gap-3 border border-border p-3 sm:grid-cols-[1fr_auto]"><div><p class="font-medium">{item.applicationName} / {item.environmentName}</p><p class="mt-1 text-xs text-muted-foreground">Database {item.database || 'Not specified'} · Alias {item.alias} · Endpoint {item.endpointName}{item.credentialName ? ` · Credential ${item.credentialName}` : ''}</p></div>{#if item.environmentArchived || item.applicationArchived}<span class="text-xs text-destructive">Archived owner</span>{/if}</div>{/each}
      </Card.Content>
    </Card.Root>

    <div class="border-t border-border pt-6"><Button variant="destructive" onclick={confirmResourceArchive}>Archive Resource</Button></div>
  </div>

  <Dialog.Root bind:open={wireGuardConfigurationDialogOpen}>
    <Dialog.Content class="sm:max-w-3xl">
    <div class="space-y-5">
      <div><h2 class="text-lg font-semibold">One-time WireGuard configuration</h2><p class="mt-2 text-sm text-muted-foreground">Import this configuration on the newly enrolled device now. The private key is not stored and cannot be shown again after leaving this page.</p></div>
      <pre class="max-h-[60vh] overflow-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{enrollment?.clientConfiguration ?? ''}</pre>
      <div class="flex justify-end"><Button type="button" onclick={() => (wireGuardConfigurationDialogOpen = false)}>I have saved this configuration</Button></div>
    </div>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={destructiveActionDialogOpen} onOpenChange={(open) => { if (!open) destructiveAction = null }}>
    <Dialog.Content>
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); executeDestructiveAction() }}>
      <div>
        <h2 class="text-lg font-semibold">{destructiveAction?.title ?? 'Confirm destructive action'}</h2>
        <p class="mt-2 text-sm text-muted-foreground">{destructiveAction?.description}</p>
      </div>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" onclick={() => (destructiveActionDialogOpen = false)}>Cancel</Button>
        <Button type="submit" variant="destructive">{destructiveAction?.confirmationLabel ?? 'Confirm'}</Button>
      </div>
    </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={restoreDialogOpen} onOpenChange={(open) => { if (!open) { restoreBackup = null; restoreConfirmation = '' } }}>
    <Dialog.Content>
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); submitRestore() }}>
      <div><h2 class="text-lg font-semibold">Restore database from backup?</h2><p class="mt-2 text-sm text-muted-foreground">DeployCrate will first create and verify a fresh safety backup. Existing database sessions will be terminated during the final cutover and clients must reconnect.</p></div>
      {#if restoreBackup}<div class="border border-border bg-muted/20 p-3 text-sm"><p>Backup from {observedLabel(restoreBackup.scheduledAt)}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{restoreBackup.id}</p></div>{/if}
      <FormField label={`Enter ${resource.name} to confirm`} error={errors.confirmation}><Input bind:value={restoreConfirmation} autocomplete="off" /></FormField>
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (restoreDialogOpen = false)}>Cancel</Button><Button type="submit" variant="destructive" disabled={restoreConfirmation !== resource.name}>Create safety backup and restore</Button></div>
    </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={privateAccessDialogOpen}>
    <Dialog.Content>
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); enablePrivateAccess() }}>
      <div><h2 class="text-lg font-semibold">Add to private network</h2><p class="mt-1 text-sm text-muted-foreground">Select the WireGuard network attached to the installation Server. Address, port, protocol, and installation are derived.</p></div>
      <FormField label="Private network" error={errors.privateNetworkId}><select bind:value={privateAccessNetworkId} class={selectClass} required><option value="">Select a private network</option>{#each endpointNetworks as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField>
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (privateAccessDialogOpen = false)}>Cancel</Button><Button type="submit">Add to private network</Button></div>
    </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={endpointDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createEndpoint() }}>
      <div><h2 class="text-lg font-semibold">Add endpoint</h2><p class="mt-1 text-sm text-muted-foreground">Define another address for this Resource.</p></div>
      <div class="grid gap-4 sm:grid-cols-2"><FormField label="Name" error={errors.name}><Input bind:value={endpoint.name} required /></FormField><FormField label="Address" error={errors.address}><Input bind:value={endpoint.address} required /></FormField><FormField label="Role"><select bind:value={endpoint.role} class={selectClass}>{#each definition.endpointRoles as value}<option value={value}>{value}</option>{/each}</select></FormField><FormField label="Protocol"><select bind:value={endpoint.protocol} class={selectClass}>{#each definition.protocols as value}<option value={value}>{value}</option>{/each}</select></FormField><FormField label="Port"><Input type="number" bind:value={endpoint.port} min="1" max="65535" /></FormField><FormField label="TLS"><select bind:value={endpoint.tlsMode} class={selectClass}>{#each definition.tlsModes as value}<option value={value}>{value}</option>{/each}</select></FormField><FormField label="Private network"><select bind:value={endpoint.privateNetworkId} onchange={(event) => chooseEndpointNetwork(event.currentTarget.value)} class={selectClass}><option value="">No private network</option>{#each endpointNetworks as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField>{#if resource.managementMode === 'managed'}<div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Installation</p><p class="mt-1 text-sm">{resource.installations[0]?.containerName}</p></div>{/if}</div>
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (endpointDialogOpen = false)}>Cancel</Button><Button type="submit">Create endpoint</Button></div>
    </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={credentialDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createCredential() }}>
      <div><h2 class="text-lg font-semibold">Add application user</h2><p class="mt-1 text-sm text-muted-foreground">DeployCrate will create or update this PostgreSQL LOGIN role without restarting the container.</p></div>
      <div class="grid gap-4 sm:grid-cols-2"><FormField label="Display name" error={errors.name}><Input bind:value={credential.name} required /></FormField><FormField label="PostgreSQL username" error={errors.username}><Input bind:value={credential.username} required autocomplete="username" /></FormField>{#each definition.credentialFields as field}<FormField label={field.label} error={errors[`secretValues.${field.name}`]}><Input type={field.secret ? 'password' : 'text'} value={credential.secretValues[field.name] ?? ''} oninput={(event) => credential.secretValues[field.name] = event.currentTarget.value} required={field.required} autocomplete="new-password" /></FormField>{/each}</div>
      <div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (credentialDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={!canAddApplicationUser}>Create application user</Button></div>
    </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={volumeDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createVolume() }}><div><h2 class="text-lg font-semibold">Add volume</h2><p class="mt-1 text-sm text-muted-foreground">Create durable storage for this Resource.</p></div><div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={volume.name} required /></FormField><FormField label="Driver"><Input bind:value={volume.driver} required /></FormField><FormField label="Server"><select bind:value={volume.serverId} class={selectClass}>{#each options.servers as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField><label class="grid gap-1 text-xs sm:col-span-2">Configuration JSON<textarea class={textareaClass} bind:value={volume.configurationText}></textarea></label></div><div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (volumeDialogOpen = false)}>Cancel</Button><Button type="submit">Create volume</Button></div></form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={mountDialogOpen}>
    <Dialog.Content class="sm:max-w-xl">
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createMount() }}><div><h2 class="text-lg font-semibold">Add mount</h2><p class="mt-1 text-sm text-muted-foreground">Attach a Resource volume to an installation.</p></div><div class="grid gap-4 sm:grid-cols-2"><FormField label="Mount path"><Input bind:value={mount.mountPath} required /></FormField><FormField label="Volume"><select bind:value={mount.resourceVolumeId} class={selectClass}>{#each resource.volumes as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField><FormField label="Installation"><select bind:value={mount.resourceInstallationId} class={selectClass}>{#each resource.installations as value}<option value={value.id}>{value.containerName}</option>{/each}</select></FormField><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={mount.readOnly} /> Read only</label></div><div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (mountDialogOpen = false)}>Cancel</Button><Button type="submit">Create mount</Button></div></form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={healthDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl">
    <form class="space-y-5" onsubmit={(event) => { event.preventDefault(); createHealth() }}><div><h2 class="text-lg font-semibold">Add health check</h2><p class="mt-1 text-sm text-muted-foreground">Define how DeployCrate should evaluate this Resource.</p></div><div class="grid gap-4 sm:grid-cols-2"><FormField label="Name"><Input bind:value={health.name} required /></FormField><FormField label="Kind"><select bind:value={health.kind} class={selectClass}>{#each definition.healthCheckKinds as value}<option value={value}>{value}</option>{/each}</select></FormField>{#if resource.installations.length > 0}<FormField label="Installation"><select bind:value={health.resourceInstallationId} class={selectClass}>{#each resource.installations as value}<option value={value.id}>{value.containerName}</option>{/each}</select></FormField>{/if}<FormField label="Endpoint"><select bind:value={health.resourceEndpointId} class={selectClass}><option value="">None</option>{#each resource.endpoints as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField><FormField label="Credential"><select bind:value={health.resourceCredentialId} class={selectClass}><option value="">None</option>{#each resource.credentials as value}<option value={value.id}>{value.name}</option>{/each}</select></FormField><FormField label="Interval seconds"><Input type="number" bind:value={health.intervalSeconds} min="1" /></FormField><FormField label="Timeout seconds"><Input type="number" bind:value={health.timeoutSeconds} min="1" /></FormField><FormField label="Failure threshold"><Input type="number" bind:value={health.failureThreshold} min="1" /></FormField><FormField label="Success threshold"><Input type="number" bind:value={health.successThreshold} min="1" /></FormField><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={health.enabled} /> Enabled</label><label class="grid gap-1 text-xs sm:col-span-2">Configuration JSON<textarea class={textareaClass} bind:value={health.configurationText}></textarea></label></div><div class="flex justify-end gap-2"><Button type="button" variant="outline" onclick={() => (healthDialogOpen = false)}>Cancel</Button><Button type="submit">Create health check</Button></div></form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
