<script lang="ts">
  import { router, useForm } from '@inertiajs/svelte'
  import { toast } from 'svelte-sonner'
  import * as Card from '@/Components/ui/card'
  import { Button } from '@/Components/ui/button'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import * as Dialog from '@/Components/ui/dialog'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import JsonCode from '@/Components/JsonCode.svelte'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import * as Table from '@/Components/ui/table'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Binding = { id: string; createdAt: string; updatedAt: string; alias: string; configuration: unknown; environmentId: string; environmentName: string; environmentKind: string; endpointId: string; credentialId: string }
  type Endpoint = { id: string; createdAt: string; updatedAt: string; name: string; role: string; address: string; port: number; protocol: string; tlsMode: string; settings: Record<string, unknown>; privateNetworkId: string }
  type Credential = { id: string; name: string; username: string; metadata: unknown; hasEncryptedPayload: boolean }
  type RevealedCredential = { id: string; name: string; username: string; values: Record<string, string> }
  type Installation = { id: string; createdAt: string; updatedAt: string; imageReference: string; imageDigest: string; containerName: string; restartPolicy: string; configuration: unknown; serverId: string; serverName: string; serverAddress: string; state: string; serviceState: string; health: string; healthReason: string; observedAt: string | null }
  type Volume = { id: string; name: string; driver: string; configuration: unknown; serverId: string; serverName: string; mounts: Array<{ id: string; mountPath: string; readOnly: boolean; installationId: string }> }
  type HealthCheck = { id: string; name: string; kind: string; configuration: unknown; intervalSeconds: number; timeoutSeconds: number; failureThreshold: number; successThreshold: number; enabled: boolean; state: string; message: string; observedAt: string | null }
  type DeviceGrant = { deviceId: string; deviceName: string; ownerEmail: string; privateAddress: string; grantId: string; grantedAt: string; applicationState: string; applicationError: string; latestHandshakeAt: string | null; observedAt: string | null }
  type Resource = { id: string; createdAt: string; updatedAt: string; name: string; resourceType: string; engine: string; bindings: Binding[]; endpoints: Endpoint[]; credentials: Credential[]; installations: Installation[]; volumes: Volume[]; healthChecks: HealthCheck[]; deviceGrants: DeviceGrant[]; privateNetworks: Array<{ id: string; name: string }>; availableDevices: Array<{ id: string; name: string; privateAddress: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }
  type BackupPolicy = { id: string; schedule: string; active: boolean; nextRunAt: string | null; backupDestinationId: string }
  type BackupHistory = { id: string; status: string; triggerType: string; scheduledAt: string; finishedAt: string | null; verifiedAt: string | null; sizeBytes: number | null; error: string }
  type DatabaseBackups = { databaseName: string; eligibility: { eligible: boolean; reason: string }; policy: BackupPolicy | null; history: BackupHistory[] }
  type Backups = { databases: DatabaseBackups[] }

  let { auth, resource, section = 'overview', backups, enrollment = null }: { auth: { email: string }; resource: Resource; section?: string; backups: Backups; enrollment?: Enrollment | null } = $props()
	const resourceNavigation = $derived({ id: resource.id, name: resource.name, engine: resource.engine, resourceType: resource.resourceType, systemManaged: true })
	const sectionTitle = $derived(({ overview: 'Overview', backups: 'Backups', endpoints: 'Endpoints', credentials: 'Credentials', health: 'Health checks', access: 'Access' } as Record<string, string>)[section] ?? 'Overview')
  const endpointForm = useForm(() => ({ name: '', role: resource.engine === 'opentelemetry' ? 'wireguard' : 'primary', address: '', port: resource.engine === 'opentelemetry' ? 4318 : 0, protocol: resource.engine === 'postgresql' ? 'postgresql' : resource.engine === 'clickhouse' || resource.engine === 'opentelemetry' ? 'http' : resource.engine, tlsMode: 'disable', database: '', user: '', settings: {} as Record<string, string>, privateNetworkId: '' }))
  const deviceForm = useForm(() => ({ name: '', deviceId: '' }))
  let revokeDialogOpen = $state(false)
  let revokeProcessing = $state(false)
  let revokeError = $state('')
  let pendingGrant = $state<DeviceGrant | null>(null)
  let credentialPasswordDialogOpen = $state(false)
  let revealedCredentialDialogOpen = $state(false)
  let selectedCredential = $state<Credential | null>(null)
  let currentPassword = $state('')
  let credentialProcessing = $state(false)
  let credentialError = $state('')
  let revealedCredential = $state<RevealedCredential | null>(null)
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const timestamp = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not recorded'
	const bytes = (value: number | null) => value === null ? 'Not available' : new Intl.NumberFormat(undefined, { style: 'unit', unit: 'byte', notation: 'compact', unitDisplay: 'narrow' }).format(value)
  const wireguardEndpoint = $derived(resource.endpoints.find((endpoint) => endpoint.role === 'wireguard'))
  const connectionExample = $derived.by(() => {
    if (!wireguardEndpoint) return ''
    const database = String(wireguardEndpoint.settings?.database ?? '')
    const user = String(wireguardEndpoint.settings?.user ?? (resource.engine === 'postgresql' ? 'deploycrate' : ''))
    if (resource.engine === 'postgresql') return `psql "host=${wireguardEndpoint.address} port=${wireguardEndpoint.port} dbname=${database || 'deploycrate'} user=${user || 'deploycrate'}"`
    if (resource.engine === 'clickhouse') return `http://${wireguardEndpoint.address}:${wireguardEndpoint.port}/?database=${database || 'deploycrate'}`
    return `${wireguardEndpoint.protocol}://${wireguardEndpoint.address}:${wireguardEndpoint.port}`
  })
  function submitEndpoint(event: SubmitEvent) {
    event.preventDefault()
    $endpointForm.settings = resource.engine === 'opentelemetry'
      ? { exposure: $endpointForm.role === 'local' ? 'system' : 'environment', transport: 'http/protobuf', authentication: $endpointForm.role === 'local' ? 'none' : 'signed_identity' }
      : Object.fromEntries(Object.entries({ database: $endpointForm.database, user: $endpointForm.user }).filter(([, value]) => value.trim() !== ''))
    $endpointForm.post(routes.systemResourceEndpointCreate(resource.id))
  }
  function submitDevice(event: SubmitEvent) { event.preventDefault(); $deviceForm.post(routes.systemResourceWireGuardDeviceCreate(resource.id)) }
  function askToRevokeGrant(grant: DeviceGrant) { pendingGrant = grant; revokeError = ''; revokeDialogOpen = true }
  function revokeGrant() {
    if (!pendingGrant || revokeProcessing) return
    revokeProcessing = true
    router.delete(routes.systemResourceWireGuardDeviceDestroy(resource.id, pendingGrant.deviceId), {
      preserveScroll: true,
      onSuccess: () => { revokeDialogOpen = false; pendingGrant = null },
      onError: () => { revokeError = 'Access could not be removed. Please try again.' },
      onFinish: () => { revokeProcessing = false },
    })
  }
  function retryGrant(deviceId: string) { router.post(routes.systemResourceWireGuardDeviceCreate(resource.id), { deviceId, name: '' }) }
  function askForCredential(credential: Credential) {
    selectedCredential = credential
    currentPassword = ''
    credentialError = ''
    revealedCredential = null
    credentialPasswordDialogOpen = true
  }
  async function revealCredential(event: SubmitEvent) {
    event.preventDefault()
    if (!selectedCredential || !currentPassword || credentialProcessing) return
    credentialProcessing = true
    credentialError = ''
    try {
      const response = await window.fetch(routes.systemResourceCredentialReveal(resource.id, selectedCredential.id), {
        method: 'POST',
        credentials: 'same-origin',
        headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: currentPassword }),
      })
      const payload = await response.json().catch(() => ({})) as Partial<RevealedCredential> & { error?: string }
      if (!response.ok || !payload.id || !payload.name || !payload.values || Object.keys(payload.values).length === 0) {
        throw new Error(payload.error || 'System Resource credential could not be loaded')
      }
      revealedCredential = {
        id: payload.id,
        name: payload.name,
        username: payload.username ?? '',
        values: payload.values,
      }
      currentPassword = ''
      credentialPasswordDialogOpen = false
      revealedCredentialDialogOpen = true
    } catch (error) {
      credentialError = error instanceof Error ? error.message : 'System Resource credential could not be loaded'
    } finally {
      credentialProcessing = false
    }
  }
  function closeRevealedCredential() {
    revealedCredentialDialogOpen = false
    revealedCredential = null
    selectedCredential = null
  }
  async function copyCredential(value: string, name: string) {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${name} copied`)
    } catch {
      toast.error(`${name} could not be copied`)
    }
  }
</script>

<svelte:head><title>{resource.name}</title></svelte:head>

<DashboardLayout email={auth.email} {resourceNavigation}>
  <div class="space-y-8">
    <header>
		<p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{resource.engine} · {resource.resourceType}</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">{resource.name}</h1>
      <p class="mt-2 text-sm text-muted-foreground">{sectionTitle}</p>
    </header>

    {#if section === 'access' && enrollment?.clientConfiguration}
      <Card.Root class="border-primary">
        <Card.Header><Card.Title>One-time WireGuard configuration</Card.Title><Card.Description>Import this configuration now. The private key is not stored and cannot be shown again.</Card.Description></Card.Header>
        <Card.Content><pre class="overflow-x-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{enrollment.clientConfiguration}</pre></Card.Content>
      </Card.Root>
    {/if}

    {#if section === 'overview'}
	<Card.Root>
      <Card.Header><Card.Title>Resource identity</Card.Title></Card.Header>
      <Card.Content><dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"><div><dt class="text-muted-foreground">Type</dt><dd>{label(resource.resourceType)}</dd></div><div><dt class="text-muted-foreground">Engine</dt><dd>{label(resource.engine)}</dd></div><div><dt class="text-muted-foreground">Created</dt><dd>{timestamp(resource.createdAt)}</dd></div><div><dt class="text-muted-foreground">Updated</dt><dd>{timestamp(resource.updatedAt)}</dd></div><div class="sm:col-span-2"><dt class="text-muted-foreground">Resource ID</dt><dd class="break-all font-mono text-xs">{resource.id}</dd></div></dl></Card.Content>
    </Card.Root>
	{/if}

    {#if section === 'overview'}
	<div class="grid gap-4 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Attached Environments</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.bindings.length === 0}<p class="text-sm text-muted-foreground">No attached Environments.</p>{:else}{#each resource.bindings as binding (binding.id)}<div class="border border-border p-4"><p class="font-medium">{binding.environmentName} · {binding.alias}</p><p class="mt-1 text-xs text-muted-foreground">{label(binding.environmentKind)} · endpoint {binding.endpointId}</p><div class="mt-3"><JsonCode value={binding.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
    </div>
	{/if}

	{#if section === 'credentials'}
	<Card.Root><Card.Header><Card.Title>Credentials</Card.Title><Card.Description>Confirm your administrator password before revealing an encrypted credential.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.credentials.length === 0}<p class="text-sm text-muted-foreground">No Resource credential records.</p>{:else}{#each resource.credentials as credential (credential.id)}<div class="border border-border p-4"><div class="flex flex-wrap items-start justify-between gap-3"><div><p class="font-medium">{credential.name}</p><p class="text-xs text-muted-foreground">{credential.username || 'No username'} · {credential.hasEncryptedPayload ? 'encrypted payload stored' : 'no payload'}</p></div>{#if credential.hasEncryptedPayload}<Button type="button" size="sm" variant="outline" onclick={() => askForCredential(credential)}>View credentials</Button>{/if}</div><div class="mt-3"><JsonCode value={credential.metadata} /></div></div>{/each}{/if}</Card.Content></Card.Root>
	{/if}

	{#if section === 'endpoints'}
    <Card.Root><Card.Header><Card.Title>Endpoints</Card.Title><Card.Description>Origin and private WireGuard addresses. Credentials are managed separately.</Card.Description></Card.Header><Card.Content><div class="grid gap-4 lg:grid-cols-2">{#each resource.endpoints as endpoint (endpoint.id)}<div class="border border-border p-4"><div class="flex justify-between gap-4"><p class="font-medium">{endpoint.name}</p><span class="text-xs text-muted-foreground">{label(endpoint.role)}</span></div><p class="mt-2 font-mono text-xs">{endpoint.address}:{endpoint.port} · {endpoint.protocol} · TLS {endpoint.tlsMode}</p><div class="mt-3"><JsonCode value={endpoint.settings} /></div></div>{/each}</div>{#if connectionExample}<div class="mt-5"><p class="mb-2 text-xs text-muted-foreground">Connection example without credentials</p><pre class="overflow-x-auto border border-border bg-muted/30 p-3 font-mono text-xs">{connectionExample}</pre></div>{/if}</Card.Content></Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Add endpoint</Card.Title><Card.Description>{resource.engine === 'opentelemetry' ? 'Environment endpoints use OTLP over HTTP/protobuf with signed Environment identity authentication.' : 'Settings containing credential material are rejected.'}</Card.Description></Card.Header>
      <Card.Content><form class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" onsubmit={submitEndpoint}><FormField label="Name" error={$endpointForm.errors.name}><Input bind:value={$endpointForm.name} required /></FormField><FormField label="Role" error={$endpointForm.errors.role}><NativeSelect.Root class="w-full" bind:value={$endpointForm.role}>{#if resource.engine === 'opentelemetry'}<NativeSelect.Option value="local">Local system</NativeSelect.Option><NativeSelect.Option value="wireguard">Environment</NativeSelect.Option>{:else}<NativeSelect.Option value="primary">Primary</NativeSelect.Option><NativeSelect.Option value="wireguard">WireGuard</NativeSelect.Option>{/if}</NativeSelect.Root></FormField><FormField label="Address" error={$endpointForm.errors.address}><Input bind:value={$endpointForm.address} required /></FormField><FormField label="Port" error={$endpointForm.errors.port}><Input type="number" min="1" max="65535" bind:value={$endpointForm.port} required /></FormField><FormField label="Protocol" error={$endpointForm.errors.protocol}><Input bind:value={$endpointForm.protocol} required /></FormField><FormField label="TLS mode" error={$endpointForm.errors.tlsMode}><NativeSelect.Root class="w-full" bind:value={$endpointForm.tlsMode}><NativeSelect.Option value="disable">Disable</NativeSelect.Option><NativeSelect.Option value="prefer">Prefer</NativeSelect.Option><NativeSelect.Option value="require">Require</NativeSelect.Option><NativeSelect.Option value="verify-ca">Verify CA</NativeSelect.Option><NativeSelect.Option value="verify-full">Verify full</NativeSelect.Option></NativeSelect.Root></FormField>{#if resource.engine !== 'opentelemetry'}<FormField label="Database"><Input bind:value={$endpointForm.database} /></FormField><FormField label="Username"><Input bind:value={$endpointForm.user} /></FormField>{/if}<FormField label="Private network" error={$endpointForm.errors.privateNetworkId}><NativeSelect.Root class="w-full" bind:value={$endpointForm.privateNetworkId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each resource.privateNetworks as network}<NativeSelect.Option value={network.id}>{network.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><div class="flex items-end"><Button type="submit" disabled={$endpointForm.processing} aria-busy={$endpointForm.processing}>{#if $endpointForm.processing}<Spinner />{/if}Add endpoint</Button></div></form></Card.Content>
    </Card.Root>
	{/if}

    {#if section === 'overview'}
	<div class="grid gap-4 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Installations</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.installations.length === 0}<p class="text-sm text-muted-foreground">Externally managed Resource.</p>{:else}{#each resource.installations as installation (installation.id)}<div class="border border-border p-4"><div class="flex flex-wrap items-center justify-between gap-2"><p class="font-medium">{installation.containerName}</p><StatusBadge status={installation.serviceState || installation.state} /></div><p class="mt-1 break-all font-mono text-xs">{installation.imageReference}</p><p class="mt-2 text-xs text-muted-foreground">Docker · {installation.serverName} · health {label(installation.health)}</p>{#if installation.healthReason}<p class="mt-2 text-xs text-destructive">{installation.healthReason}</p>{/if}<div class="mt-3"><JsonCode value={installation.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
    </div>

    <Card.Root><Card.Header><Card.Title>Volumes</Card.Title><Card.Description>Durable storage and installation mount placement.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No Resource volume records.</p>{:else}{#each resource.volumes as volume (volume.id)}<div class="border border-border p-4"><p class="font-medium">{volume.name}</p><p class="mt-1 text-xs text-muted-foreground">{label(volume.driver)} · {volume.serverName}</p>{#if volume.mounts.length > 0}<ul class="mt-3 space-y-1 font-mono text-xs">{#each volume.mounts as mount (mount.id)}<li>{mount.mountPath} · {mount.readOnly ? 'read only' : 'read write'}</li>{/each}</ul>{/if}<div class="mt-3"><JsonCode value={volume.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
	{/if}

	{#if section === 'health'}
	<Card.Root><Card.Header><Card.Title>Health checks</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{:else}{#each resource.healthChecks as check (check.id)}<div class="border border-border p-4"><div class="flex flex-wrap items-center justify-between gap-2"><p class="font-medium">{check.name}</p><StatusBadge status={check.state || 'unknown'} /></div><p class="mt-1 text-xs text-muted-foreground">{label(check.kind)} · every {check.intervalSeconds}s</p>{#if check.message}<p class="mt-2 text-xs">{check.message}</p>{/if}</div>{/each}{/if}</Card.Content></Card.Root>
	{/if}

	{#if section === 'access'}
    <Card.Root>
      <Card.Header><Card.Title>WireGuard access</Card.Title><Card.Description>A grant permits network reachability to this Resource only. It does not reveal credentials.</Card.Description></Card.Header>
      <Card.Content class="space-y-5">
        {#if resource.deviceGrants.length === 0}<p class="text-sm text-muted-foreground">No devices have access.</p>{:else}<div class="space-y-3">{#each resource.deviceGrants as grant (grant.grantId)}<div class="flex flex-col justify-between gap-3 border border-border p-4 sm:flex-row sm:items-center"><div><div class="flex flex-wrap items-center gap-2"><p class="font-medium">{grant.deviceName}</p><StatusBadge status={grant.applicationState} /></div><p class="mt-1 font-mono text-xs">{grant.privateAddress}</p><p class="mt-1 text-xs text-muted-foreground">Latest handshake: {timestamp(grant.latestHandshakeAt)}</p>{#if grant.applicationError}<p class="mt-1 text-xs text-destructive">{grant.applicationError}</p>{/if}</div><div class="flex flex-wrap gap-2">{#if grant.applicationState !== 'applied'}<Button variant="outline" size="sm" onclick={() => retryGrant(grant.deviceId)}>Retry</Button>{/if}<Button variant="destructive" size="sm" onclick={() => askToRevokeGrant(grant)}>Remove access</Button></div></div>{/each}</div>{/if}
        <form class="grid gap-4 border-t border-border pt-5 sm:grid-cols-3" onsubmit={submitDevice}><FormField label="Existing device"><NativeSelect.Root class="w-full" bind:value={$deviceForm.deviceId}><NativeSelect.Option value="">Enroll a new device</NativeSelect.Option>{#each resource.availableDevices as device}<NativeSelect.Option value={device.id}>{device.name} · {device.privateAddress}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{#if !$deviceForm.deviceId}<FormField label="New device name" error={$deviceForm.errors.name}><Input bind:value={$deviceForm.name} placeholder="MBV MacBook" required /></FormField>{/if}<div class="flex items-end"><Button type="submit" disabled={$deviceForm.processing} aria-busy={$deviceForm.processing}>{#if $deviceForm.processing}<Spinner />{/if}{$deviceForm.deviceId ? 'Grant access' : 'Enroll device'}</Button></div></form>
      </Card.Content>
    </Card.Root>
	{/if}

	{#if section === 'backups'}
		{#if backups.databases.length === 0}
			<Card.Root><Card.Header><Card.Title>Backups</Card.Title><Card.Description>No logical Database backup targets are configured for this Resource.</Card.Description></Card.Header></Card.Root>
		{:else}
			{#each backups.databases as database (database.databaseName)}
				<Card.Root>
					<Card.Header>
						<Card.Action>{#if database.policy}<StatusBadge status={database.policy.active ? 'active' : 'paused'} />{/if}</Card.Action>
						<Card.Title>{database.databaseName}</Card.Title>
						<Card.Description>{database.policy ? 'Backup policy and recent outcomes.' : database.eligibility.reason || 'No backup policy is configured.'}</Card.Description>
					</Card.Header>
					{#if database.policy || database.history.length > 0}
						<Card.Content class="space-y-5">
							{#if database.policy}
								<dl class="grid gap-4 text-sm sm:grid-cols-3"><div><dt class="text-muted-foreground">Schedule</dt><dd class="mt-1 font-mono text-xs">{database.policy.schedule}</dd></div><div><dt class="text-muted-foreground">State</dt><dd class="mt-1 capitalize">{database.policy.active ? 'Active' : 'Paused'}</dd></div><div><dt class="text-muted-foreground">Next run</dt><dd class="mt-1">{database.policy.active ? timestamp(database.policy.nextRunAt) : 'Paused'}</dd></div></dl>
							{/if}
							{#if database.history.length > 0}
								<div class="overflow-hidden border border-border"><Table.Root><Table.Header><Table.Row><Table.Head>Status</Table.Head><Table.Head>Trigger</Table.Head><Table.Head>Scheduled</Table.Head><Table.Head>Size</Table.Head></Table.Row></Table.Header><Table.Body>{#each database.history as backup (backup.id)}<Table.Row><Table.Cell><div><span class="font-medium capitalize">{backup.status.replaceAll('_', ' ')}</span>{#if backup.error}<p class="mt-1 max-w-md text-xs text-destructive">{backup.error}</p>{/if}</div></Table.Cell><Table.Cell class="capitalize">{backup.triggerType}</Table.Cell><Table.Cell>{timestamp(backup.scheduledAt)}</Table.Cell><Table.Cell>{bytes(backup.sizeBytes)}</Table.Cell></Table.Row>{/each}</Table.Body></Table.Root></div>
							{/if}
						</Card.Content>
					{/if}
				</Card.Root>
			{/each}
		{/if}
	{/if}
  </div>

  <ConfirmActionDialog bind:open={revokeDialogOpen} title={`Remove access for ${pendingGrant?.deviceName ?? 'this device'}?`} description="The device-specific firewall rule will be removed. The device remains enrolled for other Resources." confirmLabel="Remove access" processing={revokeProcessing} error={revokeError} destructive onconfirm={revokeGrant} />

  <Dialog.Root bind:open={credentialPasswordDialogOpen} onOpenChange={(open) => { if (!open && !credentialProcessing) { currentPassword = ''; credentialError = ''; selectedCredential = null } }}>
    <Dialog.Content showCloseButton={!credentialProcessing}>
      <form class="grid gap-4" onsubmit={revealCredential}>
        <Dialog.Header><Dialog.Title>View Resource credential</Dialog.Title><Dialog.Description>Enter your current administrator password to reveal {selectedCredential?.name ?? 'this credential'}.</Dialog.Description></Dialog.Header>
        <FormField label="Current password"><Input type="password" bind:value={currentPassword} autocomplete="current-password" autofocus required disabled={credentialProcessing} /></FormField>
        {#if credentialError}<p class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive" role="alert">{credentialError}</p>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={credentialProcessing} onclick={() => (credentialPasswordDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={!currentPassword || credentialProcessing}>{#if credentialProcessing}<Spinner />{/if}Continue</Button></Dialog.Footer>
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
            <div class="grid gap-2"><p class="text-xs font-medium">{label(name)}</p><div class="flex gap-2"><Input type="text" {value} readonly autocomplete="off" /><Button type="button" variant="outline" onclick={() => copyCredential(value, label(name))}>Copy</Button></div></div>
          {/each}
        </div>
      {/if}
      <Dialog.Footer><Button type="button" onclick={closeRevealedCredential}>Done</Button></Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
