<script lang="ts">
  import { page, router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type Kind = { kind: string; label: string; category: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type Server = { id: string; name: string; address: string }
  type Options = { kinds: Kind[]; servers: Server[]; privateNetworks: Array<{ id: string; name: string; serverIds: string[] }>; registryCredentials: Array<{ id: string; name: string }> }
  let { auth, options }: { auth: { email: string }; options: Options } = $props()
  let step = $state(1)
  let includeEndpoint = $state(true)
  let includePlacement = $state(true)
  let includeVolume = $state(false)
  let includeCredential = $state(false)
  let includeHealth = $state(false)
  let processing = $state(false)
  let form = $state<any>(initialForm())
  const definition = $derived(options.kinds.find((kind) => kind.kind === form.kind) ?? options.kinds[0])
  const servers = $derived(options.servers)
  const networks = $derived(options.privateNetworks.filter((network) => !form.installation.serverId || network.serverIds.includes(form.installation.serverId)))
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm'

  function initialForm() {
    const initialKind = options.kinds[0]
    return {
      name: '', category: initialKind?.category ?? 'database', kind: initialKind?.kind ?? 'postgresql', sharingScope: 'environment', managementMode: 'managed',
      endpoint: { name: 'Primary', role: initialKind?.endpointRoles[0] ?? 'primary', address: '', port: initialKind?.defaultPort ?? 5432, protocol: initialKind?.defaultProtocol ?? 'postgresql', tlsMode: initialKind?.defaultTlsMode ?? 'prefer', settings: {}, resourceInstallationId: '', privateNetworkId: '' },
      installation: { imageReference: '', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', configuration: {}, serverId: '', registryCredentialId: '' },
      volume: { name: '', driver: 'local', configuration: {}, serverId: '' }, mount: { mountPath: '/data', readOnly: false, resourceVolumeId: '', resourceInstallationId: '' },
      credential: { name: 'Application', role: 'application', username: '', metadata: {}, secretValues: {}, resourceInstallationId: '' },
      healthCheck: { name: 'Readiness', kind: initialKind?.healthCheckKinds[0] ?? 'tcp', configuration: {}, intervalSeconds: 30, timeoutSeconds: 5, failureThreshold: 3, successThreshold: 1, enabled: true, resourceInstallationId: '', resourceEndpointId: '', resourceCredentialId: '' },
    }
  }

  function chooseKind() {
    const selected = options.kinds.find((kind) => kind.kind === form.kind)
    if (!selected) return
    form.category = selected.category
    form.endpoint.port = selected.defaultPort
    form.endpoint.protocol = selected.defaultProtocol
    form.endpoint.role = selected.endpointRoles[0] ?? 'primary'
    form.endpoint.tlsMode = selected.defaultTlsMode
    form.healthCheck.kind = selected.healthCheckKinds[0] ?? 'tcp'
    form.credential.secretValues = {}
  }

  function submit() {
    processing = true
    const managed = form.managementMode === 'managed'
    router.post(routes.resourceCreate(), {
      name: form.name, category: form.category,
      kind: form.kind, sharingScope: form.sharingScope, managementMode: form.managementMode,
      endpoint: form.managementMode === 'external' || includeEndpoint ? form.endpoint : null,
      installation: managed && includePlacement ? form.installation : null,
      volume: managed && includePlacement && includeVolume ? { ...form.volume, serverId: form.installation.serverId } : null,
      mount: managed && includePlacement && includeVolume ? form.mount : null,
      credential: includeCredential ? form.credential : null,
      healthCheck: managed && includePlacement && includeHealth ? form.healthCheck : null,
    }, { onFinish: () => { processing = false } })
  }
</script>

<svelte:head><title>New Resource</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="mx-auto max-w-4xl space-y-8">
    <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Resources · Step {step} of 7</p><h1 class="mt-3 text-3xl font-semibold">Create a Resource</h1><p class="mt-2 text-sm text-muted-foreground">Define identity and desired topology. Managed placement records intent only and does not start containers.</p></header>
    {#if Object.keys($page.props.errors ?? {}).length > 0}<div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">Review the highlighted server validation errors before continuing.</div>{/if}
    <Card.Root>
      <Card.Header><Card.Title>{['Identity', 'Management', 'Endpoint', 'Placement', 'Credential', 'Health', 'Review'][step - 1]}</Card.Title><Card.Description>{['Choose the Resource identity and type.', 'Choose who manages the infrastructure lifecycle.', 'Optionally define the first reachable endpoint.', 'Record desired Server and storage topology.', 'Optionally store a write-only encrypted credential.', 'Optionally define an initial observed health target.', 'Confirm the aggregate before creating it.'][step - 1]}</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        {#if step === 1}
          <FormField label="Name"><Input bind:value={form.name} required /></FormField>
          <FormField label="Kind"><select bind:value={form.kind} onchange={chooseKind} class={selectClass}>{#each options.kinds as kind}<option value={kind.kind}>{kind.label}</option>{/each}</select></FormField>
          <FormField label="Category"><Input bind:value={form.category} readonly={form.kind !== 'custom'} required /></FormField>
          <FormField label="Sharing scope"><select bind:value={form.sharingScope} class={selectClass}><option value="environment">Environment policy</option><option value="application">Application policy</option><option value="global">Global policy</option></select></FormField>
        {:else if step === 2}
          <label class="border border-border p-4"><input type="radio" bind:group={form.managementMode} value="managed" /> <span class="ml-2 font-medium">Managed</span><p class="mt-2 text-xs text-muted-foreground">DeployCrate stores desired installation, Server, volume, and health topology. A later reconciler will provision it.</p></label>
          <label class="border border-border p-4"><input type="radio" bind:group={form.managementMode} value="external" /> <span class="ml-2 font-medium">External</span><p class="mt-2 text-xs text-muted-foreground">DeployCrate stores connection details for infrastructure it does not provision. At least one endpoint is required.</p></label>
        {:else if step === 3}
          {#if form.managementMode === 'managed'}<label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeEndpoint} /> Add an initial endpoint</label>{/if}
          {#if form.managementMode === 'external' || includeEndpoint}
            <FormField label="Endpoint name"><Input bind:value={form.endpoint.name} /></FormField>
            <FormField label="Address"><Input bind:value={form.endpoint.address} placeholder="db.internal.example" /></FormField>
            {#if form.kind === 'custom'}
              <FormField label="Role"><Input bind:value={form.endpoint.role} /></FormField><FormField label="Protocol"><Input bind:value={form.endpoint.protocol} /></FormField>
            {:else}
              <FormField label="Role"><select bind:value={form.endpoint.role} class={selectClass}>{#each definition.endpointRoles as role}<option value={role}>{role}</option>{/each}</select></FormField>
              <FormField label="Protocol"><select bind:value={form.endpoint.protocol} class={selectClass}>{#each definition.protocols as protocol}<option value={protocol}>{protocol}</option>{/each}</select></FormField>
            {/if}
            <FormField label="Port"><Input type="number" bind:value={form.endpoint.port} min="1" max="65535" /></FormField>
            <FormField label="TLS mode"><select bind:value={form.endpoint.tlsMode} class={selectClass}>{#each definition.tlsModes as mode}<option value={mode}>{mode}</option>{/each}</select></FormField>
            <FormField label="Private network"><select bind:value={form.endpoint.privateNetworkId} class={selectClass}><option value="">None</option>{#each networks as network}<option value={network.id}>{network.name}</option>{/each}</select></FormField>
          {/if}
        {:else if step === 4}
          {#if form.managementMode === 'external'}<div class="col-span-full border border-border p-5 text-sm text-muted-foreground">External Resources cannot have DeployCrate-managed installations, volumes, mounts, or health checks.</div>{:else}
            <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includePlacement} /> Add desired placement</label>
            {#if includePlacement}
              <FormField label="Server"><select bind:value={form.installation.serverId} class={selectClass} required><option value="">Select a Server</option>{#each servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}</select></FormField>
              <FormField label="Image reference"><Input bind:value={form.installation.imageReference} placeholder="postgres:17-alpine" /></FormField>
              <FormField label="Container name"><Input bind:value={form.installation.containerName} /></FormField>
              <FormField label="Restart policy"><select bind:value={form.installation.restartPolicy} class={selectClass}><option value="no">No restart</option><option value="always">Always</option><option value="on-failure">On failure</option><option value="unless-stopped">Unless stopped</option></select></FormField>
              <FormField label="Registry credential"><select bind:value={form.installation.registryCredentialId} class={selectClass}><option value="">None</option>{#each options.registryCredentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select></FormField>
              <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeVolume} /> Add a local volume mounted at <code>/data</code></label>
              {#if includeVolume}<FormField label="Volume name"><Input bind:value={form.volume.name} /></FormField><FormField label="Mount path"><Input bind:value={form.mount.mountPath} /></FormField>{/if}
            {/if}
          {/if}
        {:else if step === 5}
          <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeCredential} /> Store an encrypted credential</label>
          {#if includeCredential}
            <FormField label="Credential name"><Input bind:value={form.credential.name} /></FormField><FormField label="Role"><Input bind:value={form.credential.role} /></FormField><FormField label="Username"><Input bind:value={form.credential.username} /></FormField>
            {#each definition.credentialFields.length ? definition.credentialFields : [{ name: 'secret', label: 'Secret value', required: true, secret: true }] as field}
              <FormField label={field.label}><Input type="password" value={form.credential.secretValues[field.name] ?? ''} oninput={(event) => form.credential.secretValues[field.name] = event.currentTarget.value} required={field.required} autocomplete="new-password" /></FormField>
            {/each}
            <p class="col-span-full text-xs text-muted-foreground">Plaintext is accepted only for this submission and is never returned by the server.</p>
          {/if}
        {:else if step === 6}
          {#if form.managementMode !== 'managed' || !includePlacement}<div class="col-span-full border border-border p-5 text-sm text-muted-foreground">Health checks require a managed installation and can be added later.</div>{:else}
            <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeHealth} /> Add an initial health check</label>
            {#if includeHealth}<FormField label="Name"><Input bind:value={form.healthCheck.name} /></FormField>{#if form.kind === 'custom'}<FormField label="Kind"><Input bind:value={form.healthCheck.kind} /></FormField>{:else}<FormField label="Kind"><select bind:value={form.healthCheck.kind} class={selectClass}>{#each definition.healthCheckKinds as kind}<option value={kind}>{kind}</option>{/each}</select></FormField>{/if}<FormField label="Interval seconds"><Input type="number" bind:value={form.healthCheck.intervalSeconds} min="1" /></FormField><FormField label="Timeout seconds"><Input type="number" bind:value={form.healthCheck.timeoutSeconds} min="1" /></FormField>{/if}
          {/if}
        {:else}
          <div class="col-span-full grid gap-4 text-sm sm:grid-cols-3"><div><p class="text-muted-foreground">Identity</p><p class="mt-1 font-medium">{form.name || 'Unnamed'} · {definition.label}</p></div><div><p class="text-muted-foreground">Mode</p><p class="mt-1 capitalize">{form.managementMode}</p></div><div><p class="text-muted-foreground">Sharing</p><p class="mt-1 capitalize">{form.sharingScope}</p></div><div><p class="text-muted-foreground">Endpoint</p><p class="mt-1">{form.managementMode === 'external' || includeEndpoint ? `${form.endpoint.address || 'No address'}:${form.endpoint.port}` : 'Later'}</p></div><div><p class="text-muted-foreground">Placement</p><p class="mt-1">{form.managementMode === 'managed' && includePlacement ? form.installation.containerName || 'Incomplete' : 'Not configured'}</p></div><div><p class="text-muted-foreground">Credential / Health</p><p class="mt-1">{includeCredential ? 'Credential' : 'No credential'} · {includeHealth ? 'Health check' : 'No check'}</p></div></div>
        {/if}
      </Card.Content>
      <Card.Footer class="justify-between border-t border-border"><Button variant="outline" disabled={step === 1} onclick={() => step--}>Back</Button>{#if step < 7}<Button onclick={() => step++}>Continue</Button>{:else}<Button onclick={submit} disabled={processing || !form.name}>Create Resource</Button>{/if}</Card.Footer>
    </Card.Root>
  </div>
</DashboardLayout>
