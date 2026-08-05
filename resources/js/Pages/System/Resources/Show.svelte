<script lang="ts">
  import { router, useForm } from '@inertiajs/svelte'
  import { toast } from 'svelte-sonner'
  import * as Card from '@/Components/ui/card'
  import { Button } from '@/Components/ui/button'
  import { Checkbox } from '@/Components/ui/checkbox'
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
  type Resource = { id: string; createdAt: string; updatedAt: string; name: string; resourceType: string; engine: string; serverId: string; bindings: Binding[]; endpoints: Endpoint[]; credentials: Credential[]; installations: Installation[]; volumes: Volume[]; healthChecks: HealthCheck[]; deviceGrants: DeviceGrant[]; privateNetworks: Array<{ id: string; name: string }>; availableDevices: Array<{ id: string; name: string; privateAddress: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }
  type Engine = { engine: string; label: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { engines: Engine[]; privateNetworks: PrivateNetwork[] }
  type Publication = { id: string; resourceEndpointId: string; externalId: string; hostname: string; healthPath: string; state: string; lastError: string; appliedAt: string; observedAt: string }
  type BackupPolicy = { id: string; schedule: string; active: boolean; nextRunAt: string | null; backupDestinationId: string }
  type BackupHistory = { id: string; status: string; triggerType: string; scheduledAt: string; finishedAt: string | null; verifiedAt: string | null; sizeBytes: number | null; error: string }
  type DatabaseBackups = { databaseName: string; eligibility: { eligible: boolean; reason: string }; policy: BackupPolicy | null; history: BackupHistory[] }
  type Backups = { databases: DatabaseBackups[] }

  let { auth, resource, section = 'overview', backups, options, publications = [], enrollment = null }: { auth: { email: string }; resource: Resource; section?: string; backups: Backups; options: Options; publications?: Publication[]; enrollment?: Enrollment | null } = $props()
	const resourceNavigation = $derived({ id: resource.id, name: resource.name, engine: resource.engine, resourceType: resource.resourceType, systemManaged: true })
	const sectionTitle = $derived(({ overview: 'Overview', backups: 'Backups', endpoints: 'Endpoints', credentials: 'Credentials', health: 'Health checks', access: 'Access' } as Record<string, string>)[section] ?? 'Overview')
  const definition = $derived(options.engines.find((engine) => engine.engine === resource.engine) ?? { engine: resource.engine, label: resource.engine, protocols: [], endpointRoles: [], tlsModes: [], defaultPort: 1, defaultProtocol: '', defaultTlsMode: 'disable' })
  const endpointServerID = $derived(resource.installations[0]?.serverId || resource.serverId)
  const endpointNetworks = $derived(options.privateNetworks.filter((network) => !endpointServerID || network.serverIds.includes(endpointServerID)))
  const serviceMappings = $derived(resource.installations.flatMap((installation) => ((installation.configuration as any)?.portMappings ?? []).map((mapping: any) => ({ ...mapping, containerName: installation.containerName }))))
  const endpointForm = useForm(() => ({ name: '', role: resource.engine === 'opentelemetry' ? 'wireguard' : definition?.endpointRoles[0] ?? 'primary', audience: 'environment', addressSource: 'server_wireguard', address: '', port: definition?.defaultPort ?? 1, protocol: definition?.defaultProtocol ?? 'tcp', tlsMode: definition?.defaultTlsMode ?? 'disable', database: '', user: '', settings: {} as Record<string, string>, privateNetworkId: '', publication: { enabled: false, hostname: '', healthPath: '' } }))
  const endpointErrors = $derived(Object.values($endpointForm.errors))
  const deviceForm = useForm(() => ({ name: '', deviceId: '' }))
  let revokeDialogOpen = $state(false)
  let revokeProcessing = $state(false)
  let revokeError = $state('')
  let pendingGrant = $state<DeviceGrant | null>(null)
  let bindingDialogOpen = $state(false)
  let selectedBinding = $state<Binding | null>(null)
  let endpointDialogOpen = $state(false)
  let endpointCreateDialogOpen = $state(false)
  let selectedEndpoint = $state<Endpoint | null>(null)
  let endpointRemovalDialogOpen = $state(false)
  let endpointRemovalProcessing = $state(false)
  let endpointRemovalError = $state('')
  let pendingEndpointRemoval = $state<Endpoint | null>(null)
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
  const wireguardEndpoint = $derived(resource.endpoints.find((endpoint) => endpoint.settings?.audience === 'environment' || endpoint.settings?.exposure === 'environment' || endpoint.role === 'wireguard'))
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
      ? { audience: $endpointForm.audience, address_source: $endpointForm.addressSource, exposure: $endpointForm.audience === 'local_system' ? 'system' : 'environment', transport: 'http/protobuf', authentication: $endpointForm.audience === 'local_system' ? 'none' : 'signed_identity' }
      : { audience: $endpointForm.audience, address_source: $endpointForm.addressSource, ...Object.fromEntries(Object.entries({ database: $endpointForm.database, user: $endpointForm.user }).filter(([, value]) => value.trim() !== '')) }
    $endpointForm.post(routes.systemResourceEndpointCreate(resource.id), {
      preserveScroll: true,
      onSuccess: () => {
        endpointCreateDialogOpen = false
        $endpointForm.reset()
      },
      onError: (errors) => toast.error(Object.values(errors)[0] || 'Resource endpoint could not be added'),
    })
  }
  function openEndpointCreateDialog() {
    $endpointForm.reset()
    $endpointForm.clearErrors()
    chooseEndpointAudience('environment')
    endpointCreateDialogOpen = true
  }
  function chooseEndpointAudience(audience: string) {
    $endpointForm.audience = audience
    if (audience === 'local_system') {
      $endpointForm.addressSource = 'system_loopback'
      $endpointForm.privateNetworkId = ''
      $endpointForm.address = '127.0.0.1'
      if (resource.engine === 'opentelemetry') $endpointForm.role = 'local'
      return
    }
    if (audience === 'environment') {
      $endpointForm.addressSource = 'server_wireguard'
      if (resource.engine === 'opentelemetry') $endpointForm.role = 'wireguard'
      chooseEndpointNetwork($endpointForm.privateNetworkId || endpointNetworks[0]?.id || '')
      return
    }
    $endpointForm.addressSource = 'manual'
    $endpointForm.privateNetworkId = ''
  }
  function chooseEndpointNetwork(networkID: string) {
    $endpointForm.privateNetworkId = networkID
    const network = options.privateNetworks.find((item) => item.id === networkID)
    const existingEndpoint = resource.endpoints.find((endpoint) => endpoint.privateNetworkId === networkID)
    $endpointForm.address = network?.serverAddresses[endpointServerID] ?? existingEndpoint?.address ?? ''
  }
  function chooseServiceMapping(index: number) {
    const mapping = serviceMappings[index]
    if (mapping) $endpointForm.port = mapping.hostPort
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
  function viewBinding(binding: Binding) {
    selectedBinding = binding
    bindingDialogOpen = true
  }
  function viewEndpoint(endpoint: Endpoint) {
    selectedEndpoint = endpoint
    endpointDialogOpen = true
  }
  function askToRemoveEndpoint(endpoint: Endpoint) {
    pendingEndpointRemoval = endpoint
    endpointRemovalError = ''
    endpointRemovalDialogOpen = true
  }
  function removeEndpoint() {
    if (!pendingEndpointRemoval || endpointRemovalProcessing) return
    endpointRemovalProcessing = true
    endpointRemovalError = ''
    router.delete(routes.systemResourceEndpointDestroy(resource.id, pendingEndpointRemoval.id), {
      preserveScroll: true,
      onSuccess: () => {
        endpointRemovalDialogOpen = false
        endpointDialogOpen = false
        pendingEndpointRemoval = null
        selectedEndpoint = null
      },
      onError: (errors) => {
        endpointRemovalError = Object.values(errors).map(String).join('\n') || 'Resource endpoint could not be removed'
      },
      onFinish: () => { endpointRemovalProcessing = false },
    })
  }
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
	<Card.Root>
      <Card.Header><Card.Title>Attached environments</Card.Title><Card.Description>Environments configured to consume this Resource.</Card.Description></Card.Header>
      <Card.Content>
        {#if resource.bindings.length === 0}
          <p class="text-sm text-muted-foreground">No attached environments.</p>
        {:else}
          <div class="overflow-hidden border border-border">
            <Table.Root>
              <Table.Header><Table.Row><Table.Head>Environment</Table.Head><Table.Head>Kind</Table.Head><Table.Head>Alias</Table.Head><Table.Head>Attached</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each resource.bindings as binding (binding.id)}
                  <Table.Row>
                    <Table.Cell class="font-medium">{binding.environmentName}</Table.Cell>
                    <Table.Cell>{label(binding.environmentKind)}</Table.Cell>
                    <Table.Cell class="font-mono text-xs">{binding.alias}</Table.Cell>
                    <Table.Cell>{timestamp(binding.createdAt)}</Table.Cell>
                    <Table.Cell class="text-right"><Button type="button" size="sm" variant="outline" onclick={() => viewBinding(binding)}>View</Button></Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
	{/if}

	{#if section === 'credentials'}
	<Card.Root><Card.Header><Card.Title>Credentials</Card.Title><Card.Description>Confirm your administrator password before revealing an encrypted credential.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.credentials.length === 0}<p class="text-sm text-muted-foreground">No Resource credential records.</p>{:else}{#each resource.credentials as credential (credential.id)}<div class="border border-border p-4"><div class="flex flex-wrap items-start justify-between gap-3"><div><p class="font-medium">{credential.name}</p><p class="text-xs text-muted-foreground">{credential.username || 'No username'} · {credential.hasEncryptedPayload ? 'encrypted payload stored' : 'no payload'}</p></div>{#if credential.hasEncryptedPayload}<Button type="button" size="sm" variant="outline" onclick={() => askForCredential(credential)}>View credentials</Button>{/if}</div><div class="mt-3"><JsonCode value={credential.metadata} /></div></div>{/each}{/if}</Card.Content></Card.Root>
	{/if}

	{#if section === 'endpoints'}
    <Card.Root>
      <Card.Header><Card.Action><Button type="button" onclick={openEndpointCreateDialog}>Add endpoint</Button></Card.Action><Card.Title>Endpoints</Card.Title><Card.Description>Connectable Resource addresses and their optional Caddy routes.</Card.Description></Card.Header>
      <Card.Content class="space-y-5">
        {#if resource.endpoints.length === 0}
          <p class="text-sm text-muted-foreground">No Resource endpoints.</p>
        {:else}
          <div class="overflow-hidden border border-border">
            <Table.Root>
              <Table.Header><Table.Row><Table.Head>Name</Table.Head><Table.Head>Address</Table.Head><Table.Head>Available to</Table.Head><Table.Head>Caddy route</Table.Head><Table.Head class="text-right">Actions</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each resource.endpoints as endpoint (endpoint.id)}
                  {@const publication = publications.find((value) => value.resourceEndpointId === endpoint.id)}
                  <Table.Row>
                    <Table.Cell class="font-medium">{endpoint.name}</Table.Cell>
                    <Table.Cell class="font-mono text-xs">{endpoint.protocol}://{endpoint.address}:{endpoint.port}</Table.Cell>
                    <Table.Cell>{label(String(endpoint.settings?.audience ?? endpoint.settings?.exposure ?? endpoint.role))}</Table.Cell>
                    <Table.Cell>{#if publication}<div class="flex flex-wrap items-center gap-2"><span class="font-mono text-xs">{publication.hostname}</span><StatusBadge status={publication.state} /></div>{:else}<span class="text-xs text-muted-foreground">Not published</span>{/if}</Table.Cell>
                    <Table.Cell><div class="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" onclick={() => viewEndpoint(endpoint)}>View</Button><Button type="button" size="sm" variant="destructive" onclick={() => askToRemoveEndpoint(endpoint)}>Remove</Button></div></Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
        {#if connectionExample}<div><p class="mb-2 text-xs text-muted-foreground">Connection example without credentials</p><pre class="overflow-x-auto border border-border bg-muted/30 p-3 font-mono text-xs">{connectionExample}</pre></div>{/if}
      </Card.Content>
    </Card.Root>

	{/if}

    {#if section === 'overview'}
	<div class="grid gap-4 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>{resource.installations.length === 0 ? 'Runtime' : 'Installations'}</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.installations.length === 0}<p class="text-sm text-muted-foreground">System managed by DeployCrate. This Resource runs without a container installation.</p>{:else}{#each resource.installations as installation (installation.id)}<div class="border border-border p-4"><div class="flex flex-wrap items-center justify-between gap-2"><p class="font-medium">{installation.containerName}</p><StatusBadge status={installation.serviceState || installation.state} /></div><p class="mt-1 break-all font-mono text-xs">{installation.imageReference}</p><p class="mt-2 text-xs text-muted-foreground">Docker · {installation.serverName} · health {label(installation.health)}</p>{#if installation.healthReason}<p class="mt-2 text-xs text-destructive">{installation.healthReason}</p>{/if}<div class="mt-3"><JsonCode value={installation.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
    </div>

    {#if resource.installations.length > 0}
      <Card.Root><Card.Header><Card.Title>Volumes</Card.Title><Card.Description>Durable storage and installation mount placement.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No Resource volume records.</p>{:else}{#each resource.volumes as volume (volume.id)}<div class="border border-border p-4"><p class="font-medium">{volume.name}</p><p class="mt-1 text-xs text-muted-foreground">{label(volume.driver)} · {volume.serverName}</p>{#if volume.mounts.length > 0}<ul class="mt-3 space-y-1 font-mono text-xs">{#each volume.mounts as mount (mount.id)}<li>{mount.mountPath} · {mount.readOnly ? 'read only' : 'read write'}</li>{/each}</ul>{/if}<div class="mt-3"><JsonCode value={volume.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
    {/if}
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

  <Dialog.Root bind:open={bindingDialogOpen} onOpenChange={(open) => { if (!open) selectedBinding = null }}>
    <Dialog.Content class="sm:max-w-2xl">
      <Dialog.Header><Dialog.Title>{selectedBinding?.environmentName ?? 'Attached environment'}</Dialog.Title><Dialog.Description>Resource attachment details and environment projection configuration.</Dialog.Description></Dialog.Header>
      {#if selectedBinding}
        <div class="space-y-5">
          <dl class="grid gap-4 text-sm sm:grid-cols-2">
            <div><dt class="text-muted-foreground">Environment</dt><dd class="mt-1 font-medium">{selectedBinding.environmentName}</dd></div>
            <div><dt class="text-muted-foreground">Kind</dt><dd class="mt-1">{label(selectedBinding.environmentKind)}</dd></div>
            <div><dt class="text-muted-foreground">Alias</dt><dd class="mt-1 font-mono text-xs">{selectedBinding.alias}</dd></div>
            <div><dt class="text-muted-foreground">Attached</dt><dd class="mt-1">{timestamp(selectedBinding.createdAt)}</dd></div>
            <div><dt class="text-muted-foreground">Environment ID</dt><dd class="mt-1 break-all font-mono text-xs">{selectedBinding.environmentId}</dd></div>
            <div><dt class="text-muted-foreground">Endpoint ID</dt><dd class="mt-1 break-all font-mono text-xs">{selectedBinding.endpointId}</dd></div>
            {#if selectedBinding.credentialId}<div><dt class="text-muted-foreground">Credential ID</dt><dd class="mt-1 break-all font-mono text-xs">{selectedBinding.credentialId}</dd></div>{/if}
            <div><dt class="text-muted-foreground">Updated</dt><dd class="mt-1">{timestamp(selectedBinding.updatedAt)}</dd></div>
          </dl>
          <div><p class="mb-2 text-xs font-medium text-muted-foreground">Environment configuration</p><JsonCode value={selectedBinding.configuration} /></div>
        </div>
      {/if}
      <Dialog.Footer><Button type="button" onclick={() => (bindingDialogOpen = false)}>Done</Button></Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={endpointCreateDialogOpen}>
    <Dialog.Content class="sm:max-w-3xl">
      <form class="space-y-5" onsubmit={submitEndpoint}>
        <Dialog.Header><Dialog.Title>Add endpoint</Dialog.Title><Dialog.Description>Choose who can connect, then select a known network address or provide one manually.</Dialog.Description></Dialog.Header>
        {#if endpointErrors.length > 0}
          <div class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive" role="alert"><p class="font-medium">Endpoint could not be added</p><ul class="mt-2 list-disc space-y-1 pl-5">{#each endpointErrors as error}<li>{error}</li>{/each}</ul></div>
        {/if}
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField label="Name" error={$endpointForm.errors.name}><Input bind:value={$endpointForm.name} required /></FormField>
          <FormField label="Available to"><NativeSelect.Root class="w-full" value={$endpointForm.audience} onchange={(event) => chooseEndpointAudience(event.currentTarget.value)}><NativeSelect.Option value="local_system">Local system</NativeSelect.Option><NativeSelect.Option value="environment">Environments through WireGuard</NativeSelect.Option>{#if resource.engine !== 'opentelemetry'}<NativeSelect.Option value="custom">Custom address</NativeSelect.Option>{/if}</NativeSelect.Root></FormField>
          {#if $endpointForm.audience === 'environment'}
            <FormField label="Reach via" error={$endpointForm.errors.privateNetworkId || $endpointForm.errors.address}><NativeSelect.Root class="w-full" value={$endpointForm.privateNetworkId} onchange={(event) => chooseEndpointNetwork(event.currentTarget.value)} required><NativeSelect.Option value="">Select a private network</NativeSelect.Option>{#each endpointNetworks as network}<NativeSelect.Option value={network.id}>{network.name} · {network.serverAddresses[endpointServerID] ?? resource.endpoints.find((endpoint) => endpoint.privateNetworkId === network.id)?.address ?? 'No server address'}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          {:else if $endpointForm.audience === 'custom'}
            <FormField label="Address" error={$endpointForm.errors.address}><Input bind:value={$endpointForm.address} required placeholder="Hostname or IP address" /></FormField>
          {:else}
            <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Reach via</p><p class="mt-1 font-mono text-sm">127.0.0.1 · this system</p></div>
          {/if}
          {#if serviceMappings.length > 0}
            <FormField label="Installation service"><NativeSelect.Root class="w-full" value={String(serviceMappings.findIndex((mapping: any) => mapping.hostPort === $endpointForm.port))} onchange={(event) => chooseServiceMapping(Number(event.currentTarget.value))}>{#each serviceMappings as mapping, index}<NativeSelect.Option value={String(index)}>{mapping.containerName} · {mapping.containerPort}/{mapping.protocol} → {mapping.hostPort}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          {:else}
            <FormField label="Port" error={$endpointForm.errors.port}><Input type="number" min="1" max="65535" bind:value={$endpointForm.port} required /></FormField>
          {/if}
          {#if !$endpointForm.publication.enabled}<FormField label="Protocol" error={$endpointForm.errors.protocol}><NativeSelect.Root class="w-full" bind:value={$endpointForm.protocol}>{#each definition.protocols as protocol}<NativeSelect.Option value={protocol}>{protocol}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>{/if}
          <FormField label="Origin TLS" error={$endpointForm.errors.tlsMode}><NativeSelect.Root class="w-full" bind:value={$endpointForm.tlsMode}>{#each definition.tlsModes as mode}<NativeSelect.Option value={mode}>{label(mode)}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          {#if resource.engine !== 'opentelemetry'}<FormField label="Database"><Input bind:value={$endpointForm.database} /></FormField><FormField label="Username"><Input bind:value={$endpointForm.user} /></FormField>{/if}
        </div>
        {#if $endpointForm.protocol === 'http' || $endpointForm.protocol === 'https'}<div class="space-y-4 border border-border bg-muted/20 p-4">
          <label class="flex items-center gap-3 text-sm"><Checkbox bind:checked={$endpointForm.publication.enabled} /> Publish through Caddy</label>
          {#if $endpointForm.publication.enabled}
            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="Public hostname" error={$endpointForm.errors.hostname}><Input bind:value={$endpointForm.publication.hostname} required placeholder="database.deploycrate.com" /></FormField>
              {#if $endpointForm.protocol === 'http' || $endpointForm.protocol === 'https'}<FormField label="Public health path" error={$endpointForm.errors.healthPath || $endpointForm.errors['settings.caddy.health_path']}><Input bind:value={$endpointForm.publication.healthPath} placeholder="Optional" /></FormField>{/if}
              <p class="sm:col-span-2 text-xs text-muted-foreground"><span class="font-mono">https://{$endpointForm.publication.hostname || 'hostname'}</span> will reverse proxy to <span class="font-mono">{$endpointForm.address}:{$endpointForm.port}</span>.</p>
            </div>
          {/if}
        </div>{/if}
        <Dialog.Footer><Button type="button" variant="outline" disabled={$endpointForm.processing} onclick={() => (endpointCreateDialogOpen = false)}>Cancel</Button><Button type="submit" disabled={$endpointForm.processing} aria-busy={$endpointForm.processing}>{#if $endpointForm.processing}<Spinner />{/if}Add endpoint</Button></Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={endpointDialogOpen} onOpenChange={(open) => { if (!open) selectedEndpoint = null }}>
    <Dialog.Content class="sm:max-w-2xl">
      <Dialog.Header><Dialog.Title>{selectedEndpoint?.name ?? 'Resource endpoint'}</Dialog.Title><Dialog.Description>Connection, network, and Caddy publication details.</Dialog.Description></Dialog.Header>
      {#if selectedEndpoint}
        {@const publication = publications.find((value) => value.resourceEndpointId === selectedEndpoint?.id)}
        <div class="space-y-5">
          <dl class="grid gap-4 text-sm sm:grid-cols-2">
            <div><dt class="text-muted-foreground">Address</dt><dd class="mt-1 break-all font-mono text-xs">{selectedEndpoint.protocol}://{selectedEndpoint.address}:{selectedEndpoint.port}</dd></div>
            <div><dt class="text-muted-foreground">Available to</dt><dd class="mt-1">{label(String(selectedEndpoint.settings?.audience ?? selectedEndpoint.settings?.exposure ?? selectedEndpoint.role))}</dd></div>
            <div><dt class="text-muted-foreground">Origin TLS</dt><dd class="mt-1">{label(selectedEndpoint.tlsMode)}</dd></div>
            <div><dt class="text-muted-foreground">Private network ID</dt><dd class="mt-1 break-all font-mono text-xs">{selectedEndpoint.privateNetworkId || 'None'}</dd></div>
            <div><dt class="text-muted-foreground">Created</dt><dd class="mt-1">{timestamp(selectedEndpoint.createdAt)}</dd></div>
            <div><dt class="text-muted-foreground">Endpoint ID</dt><dd class="mt-1 break-all font-mono text-xs">{selectedEndpoint.id}</dd></div>
            <div><dt class="text-muted-foreground">Updated</dt><dd class="mt-1">{timestamp(selectedEndpoint.updatedAt)}</dd></div>
          </dl>
          {#if publication}
            <div class="border border-border bg-muted/20 p-4">
              <div class="flex flex-wrap items-center justify-between gap-3"><div><p class="text-xs text-muted-foreground">Caddy route</p><p class="mt-1 font-mono text-sm">{publication.hostname}</p></div><StatusBadge status={publication.state} /></div>
              {#if publication.healthPath}<p class="mt-3 text-xs text-muted-foreground">Health path: <span class="font-mono text-foreground">{publication.healthPath}</span></p>{/if}
              {#if publication.lastError}<p class="mt-3 text-xs text-destructive">{publication.lastError}</p>{/if}
            </div>
          {/if}
          <div><p class="mb-2 text-xs font-medium text-muted-foreground">Endpoint settings</p><JsonCode value={selectedEndpoint.settings} /></div>
        </div>
      {/if}
      <Dialog.Footer><Button type="button" variant="destructive" onclick={() => selectedEndpoint && askToRemoveEndpoint(selectedEndpoint)}>Remove endpoint</Button><Button type="button" onclick={() => (endpointDialogOpen = false)}>Done</Button></Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog bind:open={endpointRemovalDialogOpen} title={`Remove ${pendingEndpointRemoval?.name ?? 'this endpoint'}?`} description="The endpoint and its managed Caddy route will be removed. Endpoints used by an attached Environment or health check cannot be removed." confirmLabel="Remove endpoint" requiredPhrase="DELETE" processing={endpointRemovalProcessing} error={endpointRemovalError} destructive onconfirm={removeEndpoint} />

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
