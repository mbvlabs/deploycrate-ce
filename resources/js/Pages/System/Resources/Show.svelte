<script lang="ts">
  import { Link, router, useForm } from '@inertiajs/svelte'
  import * as Card from '@/Components/ui/card'
  import { Button } from '@/Components/ui/button'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import JsonCode from '@/Components/JsonCode.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Binding = { id: string; createdAt: string; updatedAt: string; alias: string; configuration: unknown; environmentId: string; environmentName: string; environmentKind: string; endpointId: string; credentialId: string }
  type Endpoint = { id: string; createdAt: string; updatedAt: string; name: string; role: string; address: string; port: number; protocol: string; tlsMode: string; settings: Record<string, unknown>; installationId: string; privateNetworkId: string }
  type Credential = { id: string; name: string; username: string; metadata: unknown; hasEncryptedPayload: boolean; installationId: string }
  type Installation = { id: string; createdAt: string; updatedAt: string; imageReference: string; imageDigest: string; containerName: string; restartPolicy: string; configuration: unknown; serverId: string; serverName: string; serverAddress: string; state: string; serviceState: string; health: string; healthReason: string; observedAt: string | null }
  type Volume = { id: string; name: string; driver: string; configuration: unknown; serverId: string; serverName: string; mounts: Array<{ id: string; mountPath: string; readOnly: boolean; installationId: string }> }
  type HealthCheck = { id: string; name: string; kind: string; configuration: unknown; intervalSeconds: number; timeoutSeconds: number; failureThreshold: number; successThreshold: number; enabled: boolean; state: string; message: string; observedAt: string | null }
  type DeviceGrant = { deviceId: string; deviceName: string; ownerEmail: string; privateAddress: string; grantId: string; grantedAt: string; applicationState: string; applicationError: string; latestHandshakeAt: string | null; observedAt: string | null }
  type Resource = { id: string; createdAt: string; updatedAt: string; name: string; category: string; kind: string; sharingScope: string; bindings: Binding[]; endpoints: Endpoint[]; credentials: Credential[]; installations: Installation[]; volumes: Volume[]; healthChecks: HealthCheck[]; deviceGrants: DeviceGrant[]; privateNetworks: Array<{ id: string; name: string }>; availableDevices: Array<{ id: string; name: string; privateAddress: string }> }
  type Enrollment = { deviceId: string; grantId: string; clientConfiguration: string }

  let { auth, resource, enrollment = null }: { auth: { email: string }; resource: Resource; enrollment?: Enrollment | null } = $props()
  const endpointForm = useForm(() => ({ name: '', role: 'primary', address: '', port: 0, protocol: resource.kind === 'postgresql' ? 'postgresql' : resource.kind === 'clickhouse' ? 'http' : resource.kind, tlsMode: 'disable', database: '', user: '', settings: {} as Record<string, string>, resourceInstallationId: '', privateNetworkId: '' }))
  const deviceForm = useForm(() => ({ name: '', deviceId: '' }))
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const timestamp = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not recorded'
  const wireguardEndpoint = $derived(resource.endpoints.find((endpoint) => endpoint.role === 'wireguard'))
  const connectionExample = $derived.by(() => {
    if (!wireguardEndpoint) return ''
    const database = String(wireguardEndpoint.settings?.database ?? '')
    const user = String(wireguardEndpoint.settings?.user ?? (resource.kind === 'postgresql' ? 'deploycrate' : ''))
    if (resource.kind === 'postgresql') return `psql "host=${wireguardEndpoint.address} port=${wireguardEndpoint.port} dbname=${database || 'deploycrate'} user=${user || 'deploycrate'}"`
    if (resource.kind === 'clickhouse') return `http://${wireguardEndpoint.address}:${wireguardEndpoint.port}/?database=${database || 'deploycrate'}`
    return `${wireguardEndpoint.protocol}://${wireguardEndpoint.address}:${wireguardEndpoint.port}`
  })
  function submitEndpoint(event: SubmitEvent) {
    event.preventDefault()
    $endpointForm.settings = Object.fromEntries(Object.entries({ database: $endpointForm.database, user: $endpointForm.user }).filter(([, value]) => value.trim() !== ''))
    $endpointForm.post(routes.systemResourceEndpointCreate(resource.id))
  }
  function submitDevice(event: SubmitEvent) { event.preventDefault(); $deviceForm.post(routes.systemResourceWireGuardDeviceCreate(resource.id)) }
  function revokeGrant(deviceId: string) { router.delete(routes.systemResourceWireGuardDeviceDestroy(resource.id, deviceId)) }
  function retryGrant(deviceId: string) { router.post(routes.systemResourceWireGuardDeviceCreate(resource.id), { deviceId, name: '' }) }
</script>

<svelte:head><title>{resource.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <Link class="text-xs text-muted-foreground hover:text-foreground" href={routes.systemResources()}>System resources</Link>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">{resource.name}</h1>
      <p class="mt-3 text-sm text-muted-foreground">{label(resource.kind)} · {label(resource.sharingScope)} sharing · {resource.bindings.length} connected {resource.bindings.length === 1 ? 'Environment' : 'Environments'}</p>
    </header>

    {#if enrollment?.clientConfiguration}
      <Card.Root class="border-primary">
        <Card.Header><Card.Title>One-time WireGuard configuration</Card.Title><Card.Description>Import this configuration now. The private key is not stored and cannot be shown again.</Card.Description></Card.Header>
        <Card.Content><pre class="overflow-x-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{enrollment.clientConfiguration}</pre></Card.Content>
      </Card.Root>
    {/if}

    <Card.Root>
      <Card.Header><Card.Title>Resource identity</Card.Title></Card.Header>
      <Card.Content><dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4"><div><dt class="text-muted-foreground">Category</dt><dd>{label(resource.category)}</dd></div><div><dt class="text-muted-foreground">Kind</dt><dd>{label(resource.kind)}</dd></div><div><dt class="text-muted-foreground">Created</dt><dd>{timestamp(resource.createdAt)}</dd></div><div><dt class="text-muted-foreground">Updated</dt><dd>{timestamp(resource.updatedAt)}</dd></div><div class="sm:col-span-2"><dt class="text-muted-foreground">Resource ID</dt><dd class="break-all font-mono text-xs">{resource.id}</dd></div></dl></Card.Content>
    </Card.Root>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Connected Environments</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.bindings.length === 0}<p class="text-sm text-muted-foreground">No Connected Environments.</p>{:else}{#each resource.bindings as binding (binding.id)}<div class="border border-border p-4"><p class="font-medium">{binding.environmentName} · {binding.alias}</p><p class="mt-1 text-xs text-muted-foreground">{label(binding.environmentKind)} · endpoint {binding.endpointId}</p><div class="mt-3"><JsonCode value={binding.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>Credentials metadata</Card.Title><Card.Description>Secret payloads are never returned.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.credentials.length === 0}<p class="text-sm text-muted-foreground">No Resource credential records.</p>{:else}{#each resource.credentials as credential (credential.id)}<div class="border border-border p-4"><p class="font-medium">{credential.name}</p><p class="text-xs text-muted-foreground">{credential.username || 'No username'} · {credential.hasEncryptedPayload ? 'encrypted payload stored' : 'no payload'}</p><div class="mt-3"><JsonCode value={credential.metadata} /></div></div>{/each}{/if}</Card.Content></Card.Root>
    </div>

    <Card.Root><Card.Header><Card.Title>Endpoints</Card.Title><Card.Description>Origin and private WireGuard addresses. Credentials are managed separately.</Card.Description></Card.Header><Card.Content><div class="grid gap-4 lg:grid-cols-2">{#each resource.endpoints as endpoint (endpoint.id)}<div class="border border-border p-4"><div class="flex justify-between gap-4"><p class="font-medium">{endpoint.name}</p><span class="text-xs text-muted-foreground">{label(endpoint.role)}</span></div><p class="mt-2 font-mono text-xs">{endpoint.address}:{endpoint.port} · {endpoint.protocol} · TLS {endpoint.tlsMode}</p><div class="mt-3"><JsonCode value={endpoint.settings} /></div></div>{/each}</div>{#if connectionExample}<div class="mt-5"><p class="mb-2 text-xs text-muted-foreground">Connection example without credentials</p><pre class="overflow-x-auto border border-border bg-muted/30 p-3 font-mono text-xs">{connectionExample}</pre></div>{/if}</Card.Content></Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Add endpoint</Card.Title><Card.Description>TCP endpoints only. Settings containing credential material are rejected.</Card.Description></Card.Header>
      <Card.Content><form class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" onsubmit={submitEndpoint}><FormField label="Name" error={$endpointForm.errors.name}><Input bind:value={$endpointForm.name} required /></FormField><FormField label="Role" error={$endpointForm.errors.role}><select class="h-9 w-full border border-input bg-background px-3 text-sm" bind:value={$endpointForm.role}><option value="primary">Primary</option><option value="wireguard">WireGuard</option></select></FormField><FormField label="Address" error={$endpointForm.errors.address}><Input bind:value={$endpointForm.address} required /></FormField><FormField label="Port" error={$endpointForm.errors.port}><Input type="number" min="1" max="65535" bind:value={$endpointForm.port} required /></FormField><FormField label="Protocol" error={$endpointForm.errors.protocol}><Input bind:value={$endpointForm.protocol} required /></FormField><FormField label="TLS mode" error={$endpointForm.errors.tlsMode}><select class="h-9 w-full border border-input bg-background px-3 text-sm" bind:value={$endpointForm.tlsMode}><option value="disable">Disable</option><option value="prefer">Prefer</option><option value="require">Require</option><option value="verify-ca">Verify CA</option><option value="verify-full">Verify full</option></select></FormField><FormField label="Database"><Input bind:value={$endpointForm.database} /></FormField><FormField label="Username"><Input bind:value={$endpointForm.user} /></FormField><FormField label="Installation"><select class="h-9 w-full border border-input bg-background px-3 text-sm" bind:value={$endpointForm.resourceInstallationId}><option value="">None</option>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select></FormField><FormField label="Private network" error={$endpointForm.errors.privateNetworkId}><select class="h-9 w-full border border-input bg-background px-3 text-sm" bind:value={$endpointForm.privateNetworkId}><option value="">None</option>{#each resource.privateNetworks as network}<option value={network.id}>{network.name}</option>{/each}</select></FormField><div class="flex items-end"><Button type="submit" disabled={$endpointForm.processing}>Add endpoint</Button></div></form></Card.Content>
    </Card.Root>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root><Card.Header><Card.Title>Installations</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.installations.length === 0}<p class="text-sm text-muted-foreground">Externally managed Resource.</p>{:else}{#each resource.installations as installation (installation.id)}<div class="border border-border p-4"><p class="font-medium">{installation.containerName}</p><p class="mt-1 break-all font-mono text-xs">{installation.imageReference}</p><p class="mt-2 text-xs text-muted-foreground">Docker · {installation.serverName} · {label(installation.state)} · health {label(installation.health)}</p>{#if installation.healthReason}<p class="mt-2 text-xs text-destructive">{installation.healthReason}</p>{/if}<div class="mt-3"><JsonCode value={installation.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>
      <Card.Root><Card.Header><Card.Title>Health checks</Card.Title></Card.Header><Card.Content class="space-y-4">{#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{:else}{#each resource.healthChecks as check (check.id)}<div class="border border-border p-4"><p class="font-medium">{check.name}</p><p class="mt-1 text-xs text-muted-foreground">{label(check.kind)} · {label(check.state)} · every {check.intervalSeconds}s</p>{#if check.message}<p class="mt-2 text-xs">{check.message}</p>{/if}</div>{/each}{/if}</Card.Content></Card.Root>
    </div>

    <Card.Root><Card.Header><Card.Title>Volumes</Card.Title><Card.Description>Durable storage and installation mount placement.</Card.Description></Card.Header><Card.Content class="space-y-4">{#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No Resource volume records.</p>{:else}{#each resource.volumes as volume (volume.id)}<div class="border border-border p-4"><p class="font-medium">{volume.name}</p><p class="mt-1 text-xs text-muted-foreground">{label(volume.driver)} · {volume.serverName}</p>{#if volume.mounts.length > 0}<ul class="mt-3 space-y-1 font-mono text-xs">{#each volume.mounts as mount (mount.id)}<li>{mount.mountPath} · {mount.readOnly ? 'read only' : 'read write'}</li>{/each}</ul>{/if}<div class="mt-3"><JsonCode value={volume.configuration} /></div></div>{/each}{/if}</Card.Content></Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>WireGuard access</Card.Title><Card.Description>A grant permits network reachability to this Resource only. It does not reveal credentials.</Card.Description></Card.Header>
      <Card.Content class="space-y-5">
        {#if resource.deviceGrants.length === 0}<p class="text-sm text-muted-foreground">No devices have access.</p>{:else}<div class="space-y-3">{#each resource.deviceGrants as grant (grant.grantId)}<div class="flex flex-col justify-between gap-3 border border-border p-4 sm:flex-row sm:items-center"><div><p class="font-medium">{grant.deviceName}</p><p class="mt-1 font-mono text-xs">{grant.privateAddress} · {label(grant.applicationState)}</p><p class="mt-1 text-xs text-muted-foreground">Latest handshake: {timestamp(grant.latestHandshakeAt)}</p>{#if grant.applicationError}<p class="mt-1 text-xs text-destructive">{grant.applicationError}</p>{/if}</div><div class="flex gap-2">{#if grant.applicationState !== 'applied'}<Button variant="outline" size="sm" onclick={() => retryGrant(grant.deviceId)}>Retry</Button>{/if}<Button variant="destructive" size="sm" onclick={() => revokeGrant(grant.deviceId)}>Remove access</Button></div></div>{/each}</div>{/if}
        <form class="grid gap-4 border-t border-border pt-5 sm:grid-cols-3" onsubmit={submitDevice}><FormField label="Existing device"><select class="h-9 w-full border border-input bg-background px-3 text-sm" bind:value={$deviceForm.deviceId}><option value="">Enroll a new device</option>{#each resource.availableDevices as device}<option value={device.id}>{device.name} · {device.privateAddress}</option>{/each}</select></FormField>{#if !$deviceForm.deviceId}<FormField label="New device name" error={$deviceForm.errors.name}><Input bind:value={$deviceForm.name} placeholder="MBV MacBook" required /></FormField>{/if}<div class="flex items-end"><Button type="submit" disabled={$deviceForm.processing}>{$deviceForm.deviceId ? 'Grant access' : 'Enroll device'}</Button></div></form>
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
