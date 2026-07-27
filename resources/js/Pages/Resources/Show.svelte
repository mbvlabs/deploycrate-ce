<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Kind = { kind: string; label: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: Array<{ name: string; label: string; required: boolean }>; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTLSMode: string }
  type Options = { kinds: Kind[]; servers: Array<{ id: string; name: string; address: string; environmentId: string }>; privateNetworks: Array<{ id: string; name: string; environmentId: string }>; registryCredentials: Array<{ id: string; name: string }> }
  let { auth, resource, options }: { auth: { email: string }; resource: any; options: Options } = $props()
  const definition = $derived(options.kinds.find((kind) => kind.kind === resource.kind) ?? options.kinds[0])
  const servers = $derived(options.servers.filter((server) => server.environmentId === resource.ownerEnvironmentId))
  const networks = $derived(options.privateNetworks.filter((network) => network.environmentId === resource.ownerEnvironmentId))
  const credentialFields = $derived(definition.credentialFields.length ? definition.credentialFields : [{ name: 'secret', label: 'Secret value', required: true }])
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm'
  const textareaClass = 'min-h-20 w-full border border-input bg-background px-3 py-2 font-mono text-xs'
  let jsonError = $state('')

  let endpointNew = $state<any>(initialEndpoint())
  let endpointDrafts = $state<any>(initialEndpointDrafts())
  let credentialNew = $state<any>({ name: 'Application', role: 'application', username: '', metadataText: '{}', secretValues: {}, resourceInstallationId: '' })
  let credentialDrafts = $state<any>(initialCredentialDrafts())
  let installationNew = $state<any>(initialInstallation())
  let installationDrafts = $state<any>(initialInstallationDrafts())
  let volumeNew = $state<any>(initialVolume())
  let volumeDrafts = $state<any>(initialVolumeDrafts())
  let mountNew = $state<any>(initialMount())
  let mountDrafts = $state<any>(initialMountDrafts())
  let healthNew = $state<any>(initialHealthCheck())
  let healthDrafts = $state<any>(initialHealthCheckDrafts())

  function currentDefinition() {
    return options.kinds.find((kind) => kind.kind === resource.kind) ?? options.kinds[0]
  }

  function firstServerID() {
    return options.servers.find((server) => server.environmentId === resource.ownerEnvironmentId)?.id ?? ''
  }

  function initialEndpoint() {
    const current = currentDefinition()
    return { name: 'Primary', role: current.endpointRoles[0] ?? 'primary', address: '', port: current.defaultPort, protocol: current.defaultProtocol, tlsMode: current.defaultTLSMode, settingsText: '{}', resourceInstallationId: '', privateNetworkId: '' }
  }

  function initialEndpointDrafts() {
    return Object.fromEntries(resource.endpoints.map((item: any) => [item.id, { ...item, settingsText: JSON.stringify(item.settings ?? {}, null, 2), resourceInstallationId: item.resourceInstallationId ?? '', privateNetworkId: item.privateNetworkId ?? '' }]))
  }

  function initialCredentialDrafts() {
    return Object.fromEntries(resource.credentials.map((item: any) => [item.id, { ...item, metadataText: JSON.stringify(item.metadata ?? {}, null, 2), secretValues: {}, rotate: false, resourceInstallationId: item.resourceInstallationId ?? '' }]))
  }

  function initialInstallation() {
    return { imageReference: '', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', configurationText: '{}', serverId: firstServerID(), registryCredentialId: '' }
  }

  function initialInstallationDrafts() {
    return Object.fromEntries(resource.installations.map((item: any) => [item.id, { ...item, configurationText: JSON.stringify(item.configuration ?? {}, null, 2), registryCredentialId: item.registryCredentialId ?? '' }]))
  }

  function initialVolume() {
    return { name: '', driver: 'local', configurationText: '{}', serverId: firstServerID() }
  }

  function initialVolumeDrafts() {
    return Object.fromEntries(resource.volumes.map((item: any) => [item.id, { ...item, configurationText: JSON.stringify(item.configuration ?? {}, null, 2) }]))
  }

  function initialMount() {
    return { mountPath: '/data', readOnly: false, resourceVolumeId: resource.volumes[0]?.id ?? '', resourceInstallationId: resource.installations[0]?.id ?? '' }
  }

  function initialMountDrafts() {
    return Object.fromEntries(resource.mounts.map((item: any) => [item.id, { ...item }]))
  }

  function initialHealthCheck() {
    const current = currentDefinition()
    return { name: 'Readiness', kind: current.healthCheckKinds[0] ?? 'tcp', configurationText: '{}', intervalSeconds: 30, timeoutSeconds: 5, failureThreshold: 3, successThreshold: 1, enabled: true, resourceInstallationId: resource.installations[0]?.id ?? '', resourceEndpointId: '', resourceCredentialId: '' }
  }

  function initialHealthCheckDrafts() {
    return Object.fromEntries(resource.healthChecks.map((item: any) => [item.id, { ...item, configurationText: JSON.stringify(item.configuration ?? {}, null, 2), resourceEndpointId: item.resourceEndpointId ?? '', resourceCredentialId: item.resourceCredentialId ?? '' }]))
  }

  function json(text: string) {
    try { jsonError = ''; return JSON.parse(text || '{}') }
    catch { jsonError = 'Configuration and metadata fields must contain valid JSON.'; throw new Error(jsonError) }
  }

  function submit(action: () => void) { try { action() } catch {} }
  function postEndpoint() { submit(() => router.post(routes.resourceEndpointCreate(resource.id), { ...endpointNew, settings: json(endpointNew.settingsText) })) }
  function patchEndpoint(id: string) { const value = endpointDrafts[id]; submit(() => router.patch(routes.resourceEndpointUpdate(resource.id, id), { ...value, settings: json(value.settingsText) })) }
  function postCredential() { submit(() => router.post(routes.resourceCredentialCreate(resource.id), { ...credentialNew, metadata: json(credentialNew.metadataText) }, { onSuccess: () => { credentialNew.secretValues = {} } })) }
  function patchCredential(id: string) { const value = credentialDrafts[id]; submit(() => router.patch(routes.resourceCredentialUpdate(resource.id, id), { ...value, metadata: json(value.metadataText) }, { onSuccess: () => { value.secretValues = {}; value.rotate = false } })) }
  function postInstallation() { submit(() => router.post(routes.resourceInstallationCreate(resource.id), { ...installationNew, configuration: json(installationNew.configurationText) })) }
  function patchInstallation(id: string) { const value = installationDrafts[id]; submit(() => router.patch(routes.resourceInstallationUpdate(resource.id, id), { ...value, configuration: json(value.configurationText) })) }
  function postVolume() { submit(() => router.post(routes.resourceVolumeCreate(resource.id), { ...volumeNew, configuration: json(volumeNew.configurationText) })) }
  function patchVolume(id: string) { const value = volumeDrafts[id]; submit(() => router.patch(routes.resourceVolumeUpdate(resource.id, id), { ...value, configuration: json(value.configurationText) })) }
  function postMount() { router.post(routes.resourceMountCreate(resource.id), mountNew) }
  function patchMount(id: string) { router.patch(routes.resourceMountUpdate(resource.id, id), mountDrafts[id]) }
  function postHealth() { submit(() => router.post(routes.resourceHealthCheckCreate(resource.id), { ...healthNew, configuration: json(healthNew.configurationText) })) }
  function patchHealth(id: string) { const value = healthDrafts[id]; submit(() => router.patch(routes.resourceHealthCheckUpdate(resource.id, id), { ...value, configuration: json(value.configurationText) })) }
</script>

<svelte:head><title>{resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{resource.ownerApplication} / {resource.ownerEnvironment}</p><h1 class="mt-3 text-3xl font-semibold">{resource.name}</h1><p class="mt-2 text-sm capitalize text-muted-foreground">{resource.kind} · {resource.category} · {resource.managementMode} · {resource.sharingScope}</p></div>
      {#if !resource.isSystem}<div class="flex gap-2"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.resourceEdit(resource.id)}>Edit identity</Link>{/snippet}</Button><Button variant="destructive" onclick={() => router.delete(routes.resourceDestroy(resource.id))}>Archive Resource</Button></div>{/if}
    </header>

    {#if resource.managementMode === 'managed'}<div class="border border-primary/30 bg-primary/5 p-4 text-sm"><strong>Desired topology only.</strong> Installation and placement changes are recorded for a future reconciler. They do not start, recreate, stop, or remove containers.</div>{/if}
    {#if jsonError}<div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{jsonError}</div>{/if}

    <Card.Root>
      <Card.Header><Card.Action><span class="text-xs">{resource.endpoints.length} active</span></Card.Action><Card.Title>Connections</Card.Title><Card.Description>Endpoints and write-only encrypted credentials.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        <section class="space-y-3"><div class="flex items-center justify-between"><h3 class="font-medium">Endpoints</h3><details><summary class="cursor-pointer text-xs text-primary">Add endpoint</summary><div class="mt-3 grid min-w-[min(42rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2">
          <Input bind:value={endpointNew.name} placeholder="Name" /><Input bind:value={endpointNew.address} placeholder="Address" />
          {#if resource.kind === 'custom'}<Input bind:value={endpointNew.role} placeholder="Role" /><Input bind:value={endpointNew.protocol} placeholder="Protocol" />{:else}<select bind:value={endpointNew.role} class={selectClass}>{#each definition.endpointRoles as role}<option value={role}>{role}</option>{/each}</select><select bind:value={endpointNew.protocol} class={selectClass}>{#each definition.protocols as protocol}<option value={protocol}>{protocol}</option>{/each}</select>{/if}
          <Input type="number" bind:value={endpointNew.port} min="1" max="65535" /><select bind:value={endpointNew.tlsMode} class={selectClass}>{#each definition.tlsModes as mode}<option value={mode}>{mode}</option>{/each}</select>
          <select bind:value={endpointNew.resourceInstallationId} class={selectClass}><option value="">No installation</option>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><select bind:value={endpointNew.privateNetworkId} class={selectClass}><option value="">No private network</option>{#each networks as network}<option value={network.id}>{network.name}</option>{/each}</select>
          <textarea class={textareaClass + ' sm:col-span-2'} bind:value={endpointNew.settingsText} aria-label="Endpoint settings"></textarea><Button onclick={postEndpoint}>Create endpoint</Button>
        </div></details></div>
        {#if resource.endpoints.length === 0}<p class="text-sm text-muted-foreground">No active endpoints.</p>{/if}
        {#each resource.endpoints as endpoint}
          <details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-medium">{endpoint.name}</span><span class="ml-2 font-mono text-xs text-muted-foreground">{endpoint.protocol}://{endpoint.address}:{endpoint.port}</span></summary>
            <div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={endpointDrafts[endpoint.id].name} /><Input bind:value={endpointDrafts[endpoint.id].address} /><Input bind:value={endpointDrafts[endpoint.id].role} /><Input bind:value={endpointDrafts[endpoint.id].protocol} /><Input type="number" bind:value={endpointDrafts[endpoint.id].port} /><select bind:value={endpointDrafts[endpoint.id].tlsMode} class={selectClass}>{#each definition.tlsModes as mode}<option value={mode}>{mode}</option>{/each}</select><select bind:value={endpointDrafts[endpoint.id].resourceInstallationId} class={selectClass}><option value="">No installation</option>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><select bind:value={endpointDrafts[endpoint.id].privateNetworkId} class={selectClass}><option value="">No private network</option>{#each networks as network}<option value={network.id}>{network.name}</option>{/each}</select><textarea class={textareaClass + ' sm:col-span-2'} bind:value={endpointDrafts[endpoint.id].settingsText}></textarea><div class="flex gap-2"><Button size="sm" onclick={() => patchEndpoint(endpoint.id)}>Save</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceEndpointDestroy(resource.id, endpoint.id))}>Archive</Button></div></div>
          </details>
        {/each}</section>

        <section class="space-y-3 border-t border-border pt-5"><div class="flex items-center justify-between"><h3 class="font-medium">Credentials</h3><details><summary class="cursor-pointer text-xs text-primary">Add credential</summary><div class="mt-3 grid min-w-[min(42rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2"><Input bind:value={credentialNew.name} placeholder="Name" /><Input bind:value={credentialNew.role} placeholder="Role" /><Input bind:value={credentialNew.username} placeholder="Username" /><select bind:value={credentialNew.resourceInstallationId} class={selectClass}><option value="">All installations</option>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select>{#each credentialFields as field}<Input type="password" value={credentialNew.secretValues[field.name] ?? ''} oninput={(event) => credentialNew.secretValues[field.name] = event.currentTarget.value} placeholder={field.label} autocomplete="new-password" />{/each}<textarea class={textareaClass + ' sm:col-span-2'} bind:value={credentialNew.metadataText} aria-label="Credential metadata"></textarea><Button onclick={postCredential}>Encrypt and create</Button></div></details></div>
        {#if resource.credentials.length === 0}<p class="text-sm text-muted-foreground">No encrypted credentials.</p>{/if}
        {#each resource.credentials as credential}
          <details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-medium">{credential.name}</span><span class="ml-2 text-xs text-muted-foreground">{credential.role} · {credential.hasEncryptedPayload ? 'encrypted payload stored' : 'no payload'}</span></summary>
            <div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={credentialDrafts[credential.id].name} /><Input bind:value={credentialDrafts[credential.id].role} /><Input bind:value={credentialDrafts[credential.id].username} placeholder="Username" /><select bind:value={credentialDrafts[credential.id].resourceInstallationId} class={selectClass}><option value="">All installations</option>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><textarea class={textareaClass + ' sm:col-span-2'} bind:value={credentialDrafts[credential.id].metadataText}></textarea><label class="sm:col-span-2 flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={credentialDrafts[credential.id].rotate} /> Rotate encrypted payload with this update</label>{#if credentialDrafts[credential.id].rotate}{#each credentialFields as field}<Input type="password" value={credentialDrafts[credential.id].secretValues[field.name] ?? ''} oninput={(event) => credentialDrafts[credential.id].secretValues[field.name] = event.currentTarget.value} placeholder={`New ${field.label}`} autocomplete="new-password" />{/each}{/if}<div class="flex gap-2"><Button size="sm" onclick={() => patchCredential(credential.id)}>{credentialDrafts[credential.id].rotate ? 'Rotate' : 'Save metadata'}</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceCredentialDestroy(resource.id, credential.id))}>Archive</Button></div></div>
          </details>
        {/each}</section>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Action><span class="text-xs">{resource.installations.length} installations</span></Card.Action><Card.Title>Placement and storage</Card.Title><Card.Description>Desired Servers, containers, volumes, and mounts. Observed installation status is read only.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        <section class="space-y-3"><div class="flex items-center justify-between"><h3 class="font-medium">Installations</h3>{#if resource.managementMode === 'managed'}<details><summary class="cursor-pointer text-xs text-primary">Add installation</summary><div class="mt-3 grid min-w-[min(42rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2"><Input bind:value={installationNew.imageReference} placeholder="Image reference" /><Input bind:value={installationNew.containerName} placeholder="Container name" /><select bind:value={installationNew.serverId} class={selectClass}><option value="">Select Server</option>{#each servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}</select><select bind:value={installationNew.restartPolicy} class={selectClass}><option value="no">No restart</option><option value="always">Always</option><option value="on-failure">On failure</option><option value="unless-stopped">Unless stopped</option></select><Input bind:value={installationNew.imageDigest} placeholder="Optional image digest" /><select bind:value={installationNew.registryCredentialId} class={selectClass}><option value="">No registry credential</option>{#each options.registryCredentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select><textarea class={textareaClass + ' sm:col-span-2'} bind:value={installationNew.configurationText}></textarea><Button onclick={postInstallation}>Create installation</Button></div></details>{/if}</div>
        {#if resource.installations.length === 0}<p class="text-sm text-muted-foreground">No desired installations.</p>{/if}
        {#each resource.installations as installation}
          <details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-medium">{installation.containerName}</span><span class="ml-2 text-xs text-muted-foreground">{installation.serverName} · observed {installation.state || 'unknown'} / {installation.health || 'unknown'}</span></summary><div class="mt-2 border-l-2 border-muted pl-3 text-xs text-muted-foreground">Observed service: {installation.serviceState || 'unknown'}{installation.healthReason ? ` · ${installation.healthReason}` : ''}. This observation cannot be edited here.</div>
            <div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={installationDrafts[installation.id].imageReference} /><Input bind:value={installationDrafts[installation.id].containerName} /><select bind:value={installationDrafts[installation.id].serverId} class={selectClass}>{#each servers as server}<option value={server.id}>{server.name}</option>{/each}</select><select bind:value={installationDrafts[installation.id].restartPolicy} class={selectClass}><option value="no">No restart</option><option value="always">Always</option><option value="on-failure">On failure</option><option value="unless-stopped">Unless stopped</option></select><Input bind:value={installationDrafts[installation.id].imageDigest} placeholder="Image digest" /><select bind:value={installationDrafts[installation.id].registryCredentialId} class={selectClass}><option value="">No registry credential</option>{#each options.registryCredentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select><textarea class={textareaClass + ' sm:col-span-2'} bind:value={installationDrafts[installation.id].configurationText}></textarea><div class="flex gap-2"><Button size="sm" onclick={() => patchInstallation(installation.id)}>Save</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceInstallationDestroy(resource.id, installation.id))}>Archive</Button></div></div>
          </details>
        {/each}</section>

        <section class="space-y-3 border-t border-border pt-5"><div class="flex items-center justify-between"><h3 class="font-medium">Volumes</h3>{#if resource.managementMode === 'managed'}<details><summary class="cursor-pointer text-xs text-primary">Add volume</summary><div class="mt-3 grid min-w-[min(36rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2"><Input bind:value={volumeNew.name} placeholder="Name" /><Input bind:value={volumeNew.driver} placeholder="Driver" /><select bind:value={volumeNew.serverId} class={selectClass}>{#each servers as server}<option value={server.id}>{server.name}</option>{/each}</select><textarea class={textareaClass} bind:value={volumeNew.configurationText}></textarea><Button onclick={postVolume}>Create volume</Button></div></details>{/if}</div>
        {#if resource.volumes.length === 0}<p class="text-sm text-muted-foreground">No active volumes.</p>{/if}
        {#each resource.volumes as volume}<details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-medium">{volume.name}</span><span class="ml-2 text-xs text-muted-foreground">{volume.driver} · {volume.serverName}</span></summary><div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={volumeDrafts[volume.id].name} /><Input bind:value={volumeDrafts[volume.id].driver} /><select bind:value={volumeDrafts[volume.id].serverId} class={selectClass}>{#each servers as server}<option value={server.id}>{server.name}</option>{/each}</select><textarea class={textareaClass} bind:value={volumeDrafts[volume.id].configurationText}></textarea><div class="flex gap-2"><Button size="sm" onclick={() => patchVolume(volume.id)}>Save</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceVolumeDestroy(resource.id, volume.id))}>Archive</Button></div></div></details>{/each}</section>

        <section class="space-y-3 border-t border-border pt-5"><div class="flex items-center justify-between"><h3 class="font-medium">Mounts</h3>{#if resource.managementMode === 'managed'}<details><summary class="cursor-pointer text-xs text-primary">Add mount</summary><div class="mt-3 grid min-w-[min(36rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2"><Input bind:value={mountNew.mountPath} placeholder="/data" /><select bind:value={mountNew.resourceVolumeId} class={selectClass}>{#each resource.volumes as volume}<option value={volume.id}>{volume.name}</option>{/each}</select><select bind:value={mountNew.resourceInstallationId} class={selectClass}>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={mountNew.readOnly} /> Read only</label><Button onclick={postMount}>Create mount</Button></div></details>{/if}</div>
        {#if resource.mounts.length === 0}<p class="text-sm text-muted-foreground">No active mounts.</p>{/if}
        {#each resource.mounts as mount}<details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-mono font-medium">{mount.mountPath}</span><span class="ml-2 text-xs text-muted-foreground">{mount.volumeName} → {mount.containerName}</span></summary><div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={mountDrafts[mount.id].mountPath} /><select bind:value={mountDrafts[mount.id].resourceVolumeId} class={selectClass}>{#each resource.volumes as volume}<option value={volume.id}>{volume.name}</option>{/each}</select><select bind:value={mountDrafts[mount.id].resourceInstallationId} class={selectClass}>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={mountDrafts[mount.id].readOnly} /> Read only</label><div class="flex gap-2"><Button size="sm" onclick={() => patchMount(mount.id)}>Save</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceMountDestroy(resource.id, mount.id))}>Archive</Button></div></div></details>{/each}</section>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Action><span class="text-xs">{resource.healthChecks.length} configured</span></Card.Action><Card.Title>Health</Card.Title><Card.Description>Configuration is editable. Latest observed status is read only.</Card.Description></Card.Header>
      <Card.Content class="space-y-3"><div class="flex justify-end">{#if resource.managementMode === 'managed'}<details><summary class="cursor-pointer text-xs text-primary">Add health check</summary><div class="mt-3 grid min-w-[min(42rem,80vw)] gap-3 border border-border p-4 sm:grid-cols-2"><Input bind:value={healthNew.name} placeholder="Name" />{#if resource.kind === 'custom'}<Input bind:value={healthNew.kind} placeholder="Kind" />{:else}<select bind:value={healthNew.kind} class={selectClass}>{#each definition.healthCheckKinds as kind}<option value={kind}>{kind}</option>{/each}</select>{/if}<select bind:value={healthNew.resourceInstallationId} class={selectClass}>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><select bind:value={healthNew.resourceEndpointId} class={selectClass}><option value="">No endpoint</option>{#each resource.endpoints as endpoint}<option value={endpoint.id}>{endpoint.name}</option>{/each}</select><select bind:value={healthNew.resourceCredentialId} class={selectClass}><option value="">No credential</option>{#each resource.credentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select><Input type="number" bind:value={healthNew.intervalSeconds} min="1" /><Input type="number" bind:value={healthNew.timeoutSeconds} min="1" /><Input type="number" bind:value={healthNew.failureThreshold} min="1" /><Input type="number" bind:value={healthNew.successThreshold} min="1" /><textarea class={textareaClass + ' sm:col-span-2'} bind:value={healthNew.configurationText}></textarea><Button onclick={postHealth}>Create health check</Button></div></details>{/if}</div>
      {#if resource.healthChecks.length === 0}<p class="text-sm text-muted-foreground">No health checks configured.</p>{/if}
      {#each resource.healthChecks as check}<details class="border border-border p-3"><summary class="cursor-pointer"><span class="font-medium">{check.name}</span><span class:text-success={check.state === 'healthy' || check.state === 'passing'} class:text-destructive={check.state === 'unhealthy' || check.state === 'failed'} class="ml-2 text-xs">observed {check.state || 'unknown'}</span></summary><p class="mt-2 border-l-2 border-muted pl-3 text-xs text-muted-foreground">{check.message || 'No observation message.'} This status cannot be edited here.</p><div class="mt-4 grid gap-3 sm:grid-cols-2"><Input bind:value={healthDrafts[check.id].name} /><Input bind:value={healthDrafts[check.id].kind} /><select bind:value={healthDrafts[check.id].resourceInstallationId} class={selectClass}>{#each resource.installations as installation}<option value={installation.id}>{installation.containerName}</option>{/each}</select><select bind:value={healthDrafts[check.id].resourceEndpointId} class={selectClass}><option value="">No endpoint</option>{#each resource.endpoints as endpoint}<option value={endpoint.id}>{endpoint.name}</option>{/each}</select><select bind:value={healthDrafts[check.id].resourceCredentialId} class={selectClass}><option value="">No credential</option>{#each resource.credentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select><Input type="number" bind:value={healthDrafts[check.id].intervalSeconds} min="1" /><Input type="number" bind:value={healthDrafts[check.id].timeoutSeconds} min="1" /><Input type="number" bind:value={healthDrafts[check.id].failureThreshold} min="1" /><Input type="number" bind:value={healthDrafts[check.id].successThreshold} min="1" /><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={healthDrafts[check.id].enabled} /> Enabled</label><textarea class={textareaClass + ' sm:col-span-2'} bind:value={healthDrafts[check.id].configurationText}></textarea><div class="flex gap-2"><Button size="sm" onclick={() => patchHealth(check.id)}>Save</Button><Button size="sm" variant="destructive" onclick={() => router.delete(routes.resourceHealthCheckDestroy(resource.id, check.id))}>Archive</Button></div></div></details>{/each}</Card.Content>
    </Card.Root>

    <Card.Root><Card.Header><Card.Title>Dependencies</Card.Title><Card.Description>Archive protection is evaluated again under a row lock when an operation runs.</Card.Description></Card.Header><Card.Content><p class="text-sm"><span class="font-medium">{resource.bindingCount}</span> active Environment binding{resource.bindingCount === 1 ? '' : 's'}.</p><p class="mt-2 text-xs text-muted-foreground">Endpoints and credentials selected by bindings or health checks cannot be archived. Installations and volumes must be detached from dependent topology first.</p></Card.Content></Card.Root>
  </div>
</DashboardLayout>
