<script lang="ts">
  import { router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DataField from '@/Components/DataField.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as RadioGroup from '@/Components/ui/radio-group'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type Kind = { kind: string; label: string; category: string; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type Server = { id: string; name: string; address: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { kinds: Kind[]; servers: Server[]; privateNetworks: PrivateNetwork[]; registryCredentials: Array<{ id: string; name: string }> }
  type ResourcePreset = { id: string; label: string; badge: string; category: string; description: string; kind?: string; href?: string; mountPath?: string; imagePlaceholder?: string }
  let { auth, options, errors = {} }: { auth: { email: string }; options: Options; errors?: Record<string, string> } = $props()
  let includeVolume = $state(true)
  let includeHealth = $state(false)
  let runtime = $state('docker')
  let processing = $state(false)
  const resourceKinds = $derived(options.kinds.filter((kind) => kind.kind !== 'postgresql'))
  let form = $state<any>(initialForm())
  const presets: ResourcePreset[] = [
    { id: 'postgresql', label: 'PostgreSQL', badge: 'PG', category: 'Database', description: 'Create a managed relational database with dedicated cluster and node controls.', href: routes.resourceDatabaseNew() },
    { id: 'mysql', label: 'MySQL', badge: 'MY', category: 'Database', description: 'Run a MySQL relational database from a Docker image.', kind: 'mysql', mountPath: '/var/lib/mysql', imagePlaceholder: 'mysql:8.4' },
    { id: 'redis', label: 'Redis', badge: 'RD', category: 'Cache', description: 'Run an in-memory data store and cache in Docker.', kind: 'redis', mountPath: '/data', imagePlaceholder: 'redis:8' },
    { id: 'clickhouse', label: 'ClickHouse', badge: 'CH', category: 'Analytics', description: 'Run a column-oriented analytical database in Docker.', kind: 'clickhouse', mountPath: '/var/lib/clickhouse', imagePlaceholder: 'clickhouse/clickhouse-server' },
    { id: 'docker', label: 'Docker Image', badge: 'DK', category: 'Container', description: 'Deploy any compatible container image as a managed Resource.', kind: 'http', mountPath: '/data', imagePlaceholder: 'registry.example.com/image:tag' },
  ]
  let selectedPreset = $state<string | null>(null)
  const selectedPresetDefinition = $derived(presets.find((preset) => preset.id === selectedPreset))
  const definition = $derived(resourceKinds.find((kind) => kind.kind === form.kind) ?? resourceKinds[0])
  const errorEntries = $derived(Object.entries(errors))
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm aria-invalid:border-destructive aria-invalid:ring-1 aria-invalid:ring-destructive/20'

  function initialForm() {
    const initialKind = resourceKinds[0]
    return {
      name: '', slug: '', category: initialKind?.category ?? 'cache', kind: initialKind?.kind ?? 'redis', sharingScope: 'environment', managementMode: 'managed',
      endpoint: { name: 'Primary', role: initialKind?.endpointRoles[0] ?? 'primary', address: '127.0.0.1', port: initialKind?.defaultPort ?? 5432, protocol: initialKind?.defaultProtocol ?? 'postgresql', tlsMode: initialKind?.defaultTlsMode ?? 'prefer', settings: {}, resourceInstallationId: '', privateNetworkId: '' },
      installation: { imageReference: '', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', configuration: {}, portMappings: [{ hostPort: initialKind?.defaultPort ?? 5432, containerPort: initialKind?.defaultPort ?? 5432, protocol: 'tcp' }], serverId: '', registryCredentialId: '' },
      volume: { name: '', driver: 'local', configuration: {}, serverId: '' }, mount: { mountPath: '/data', readOnly: false, resourceVolumeId: '', resourceInstallationId: '' },
      credential: { name: 'Resource administrator', username: '', metadata: {}, secretValues: {} },
      healthCheck: { name: 'Readiness', kind: defaultHealthKind(initialKind), configuration: {}, intervalSeconds: 30, timeoutSeconds: 5, failureThreshold: 3, successThreshold: 1, enabled: true, resourceInstallationId: '', resourceEndpointId: '', resourceCredentialId: '' },
    }
  }

  function applyKindDefaults(selected: Kind) {
    form.endpoint.port = selected.defaultPort
    form.endpoint.protocol = selected.defaultProtocol
    form.endpoint.role = selected.endpointRoles[0] ?? 'primary'
    form.endpoint.tlsMode = selected.defaultTlsMode
    form.installation.portMappings = [{ hostPort: selected.defaultPort, containerPort: selected.defaultPort, protocol: 'tcp' }]
    form.healthCheck.kind = defaultHealthKind(selected)
    form.credential.secretValues = {}
  }

  function choosePreset(preset: ResourcePreset) {
    if (!preset.kind) return
    const selected = resourceKinds.find((kind) => kind.kind === preset.kind)
    if (!selected) return
    form.category = selected.category
    form.kind = selected.kind
    form.installation.imageReference = ''
    form.installation.containerName = ''
    form.volume.name = ''
    form.mount.mountPath = preset.mountPath ?? '/data'
    applyKindDefaults(selected)
    selectedPreset = preset.id
  }

  function defaultHealthKind(kind: Kind | undefined) {
    if (kind?.kind === 'postgresql' || kind?.kind === 'clickhouse') return kind.kind
    return kind?.healthCheckKinds[0] ?? 'tcp'
  }

  function chooseServer(serverId: string) {
    form.installation.serverId = serverId
  }

  function errorLabel(field: string) {
    return field
      .split('.')
      .filter((segment) => !/^\d+$/.test(segment))
      .map((segment) => segment.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, (letter) => letter.toUpperCase()))
      .join(' / ')
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    processing = true
    const managed = form.managementMode === 'managed'
    router.post(routes.resourceCreate(), {
      name: form.name, slug: form.slug, kind: form.kind, sharingScope: form.sharingScope, managementMode: form.managementMode,
      endpoint: managed ? null : form.endpoint,
      installation: managed ? form.installation : null,
      volume: managed && includeVolume ? { ...form.volume, serverId: form.installation.serverId } : null,
      mount: managed && includeVolume ? form.mount : null,
      credential: managed && form.kind === 'postgresql' ? form.credential : null,
      healthCheck: managed && includeHealth ? form.healthCheck : null,
    }, { onFinish: () => { processing = false } })
  }
</script>

<svelte:head><title>New Resource</title></svelte:head>
<DashboardLayout email={auth.email}>
  {#if selectedPreset === null}
    <div class="mx-auto max-w-5xl space-y-8">
      <header>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">New Resource</p>
        <h1 class="mt-3 text-3xl font-semibold">What would you like to deploy?</h1>
        <p class="mt-2 max-w-2xl text-sm text-muted-foreground">Choose a datastore or start from a Docker image. You can configure placement and access in the next step.</p>
      </header>

      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {#each presets as preset (preset.id)}
          {#if preset.href}
            <a href={preset.href} class="group flex min-h-52 flex-col border border-border bg-card p-5 text-left transition-colors hover:border-primary/60 hover:bg-muted/30 focus-visible:border-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary">
              <span class="flex items-start justify-between gap-4">
                <span class="grid size-11 place-items-center border border-primary/25 bg-primary/10 font-mono text-sm font-semibold text-primary">{preset.badge}</span>
                <span class="text-xs text-muted-foreground transition-transform group-hover:translate-x-0.5">Open →</span>
              </span>
              <span class="mt-7 text-base font-semibold">{preset.label}</span>
              <span class="mt-2 flex-1 text-sm leading-6 text-muted-foreground">{preset.description}</span>
              <span class="mt-5 text-[10px] font-medium uppercase tracking-[0.2em] text-muted-foreground">{preset.category}</span>
            </a>
          {:else}
            <button type="button" onclick={() => choosePreset(preset)} class="group flex min-h-52 flex-col border border-border bg-card p-5 text-left transition-colors hover:border-primary/60 hover:bg-muted/30 focus-visible:border-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary">
              <span class="flex w-full items-start justify-between gap-4">
                <span class="grid size-11 place-items-center border border-primary/25 bg-primary/10 font-mono text-sm font-semibold text-primary">{preset.badge}</span>
                <span class="text-xs text-muted-foreground transition-transform group-hover:translate-x-0.5">Configure →</span>
              </span>
              <span class="mt-7 text-base font-semibold">{preset.label}</span>
              <span class="mt-2 flex-1 text-sm leading-6 text-muted-foreground">{preset.description}</span>
              <span class="mt-5 text-[10px] font-medium uppercase tracking-[0.2em] text-muted-foreground">{preset.category}</span>
            </button>
          {/if}
        {/each}
      </div>

      <div class="flex items-center justify-between border-t border-border pt-5">
        <p class="text-xs text-muted-foreground">More Resource templates can be added without changing the creation workflow.</p>
        <Button variant="outline" href={routes.resources()}>Cancel</Button>
      </div>
    </div>
  {:else}
  <form class="mx-auto max-w-5xl space-y-6" onsubmit={submit}>
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{selectedPresetDefinition?.label ?? 'Docker'} Resource</p><h1 class="mt-3 text-3xl font-semibold">Configure {selectedPresetDefinition?.label ?? 'Resource'}</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Define its identity, runtime, placement, and initial access details.</p></div>
      <Button type="button" variant="outline" onclick={() => selectedPreset = null}>Change Resource type</Button>
    </header>

    {#if errorEntries.length > 0}
      <div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive" role="alert">
        <p class="font-medium">The Resource could not be created:</p>
        <ul class="mt-2 list-disc space-y-1 pl-5">
          {#each errorEntries as [field, message]}<li><span class="font-medium">{errorLabel(field)}:</span> {message}</li>{/each}
        </ul>
      </div>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Title>Resource</Card.Title>
		<Card.Description>Choose the stable identity and sharing policy for this {definition.label} Resource.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="Name" error={errors.name}><Input bind:value={form.name} aria-invalid={Boolean(errors.name)} required /></FormField>
		<FormField label="Slug" error={errors.slug}><Input bind:value={form.slug} aria-invalid={Boolean(errors.slug)} placeholder="shared-cache" required /></FormField>
        <FormField label="Sharing scope" error={errors.sharingScope}>
          <select bind:value={form.sharingScope} class={selectClass} aria-invalid={Boolean(errors.sharingScope)}>
            <option value="environment">Environment policy</option>
            <option value="application">Application policy</option>
            <option value="global">Global policy</option>
          </select>
        </FormField>
        <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Resource type</p><p class="mt-1 text-sm font-medium">{selectedPresetDefinition?.label ?? definition.label}</p></div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Management and runtime</Card.Title>
        <Card.Description>Choose who manages the infrastructure lifecycle and how managed Resources run.</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-5">
        <div class="space-y-3">
          <p class="text-sm font-medium">Management</p>
          <RadioGroup.Root bind:value={form.managementMode} class="grid gap-3 sm:grid-cols-2">
            <label class:border-primary={form.managementMode === 'managed'} class="flex min-h-24 cursor-pointer items-start gap-3 border border-border p-4">
              <RadioGroup.Item value="managed" aria-label="Managed" />
              <span>
                <span class="block text-sm font-medium">Managed</span>
                <span class="mt-2 block text-xs text-muted-foreground">DeployCrate stores the desired installation, Server, storage, and health topology.</span>
              </span>
            </label>
            <label class:border-primary={form.managementMode === 'external'} class="flex min-h-24 cursor-pointer items-start gap-3 border border-border p-4">
              <RadioGroup.Item value="external" aria-label="External" />
              <span>
                <span class="block text-sm font-medium">External</span>
                <span class="mt-2 block text-xs text-muted-foreground">DeployCrate stores connection details for infrastructure it does not provision.</span>
              </span>
            </label>
          </RadioGroup.Root>
          {#if errors.managementMode}<p class="text-xs font-medium text-destructive">{errors.managementMode}</p>{/if}
        </div>
        {#if form.managementMode === 'managed'}
          <div class="space-y-3 border-t border-border pt-5">
            <p class="text-sm font-medium">Runtime</p>
            <RadioGroup.Root bind:value={runtime} class="grid gap-3 sm:grid-cols-2">
              <label class:border-primary={runtime === 'docker'} class="flex min-h-24 cursor-pointer items-start gap-3 border border-border p-4">
                <RadioGroup.Item value="docker" aria-label="Docker" />
                <span>
                  <span class="block text-sm font-medium">Docker</span>
                  <span class="mt-2 block text-xs text-muted-foreground">Run this Resource from a container image on the selected Server.</span>
                </span>
              </label>
              <label class="flex min-h-24 cursor-not-allowed items-start gap-3 border border-border bg-muted/30 p-4 text-muted-foreground">
                <RadioGroup.Item value="native" aria-label="Native" disabled />
                <span>
                  <span class="block text-sm font-medium">Native <span class="ml-2 text-[10px] uppercase tracking-wider">Coming soon</span></span>
                  <span class="mt-2 block text-xs">Install and manage the Resource directly on the Server.</span>
                </span>
              </label>
            </RadioGroup.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>{form.managementMode === 'managed' ? 'Primary service' : 'Primary endpoint'}</Card.Title>
        <Card.Description>{form.managementMode === 'managed' ? 'The runtime origin is derived from the Docker installation.' : 'Provide the primary origin for infrastructure managed outside DeployCrate.'}</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        {#if form.managementMode === 'external'}
          <FormField label="Endpoint name" error={errors['endpoint.name']}><Input bind:value={form.endpoint.name} aria-invalid={Boolean(errors['endpoint.name'])} /></FormField>
          <FormField label="Address" error={errors['endpoint.address']}><Input bind:value={form.endpoint.address} aria-invalid={Boolean(errors['endpoint.address'])} placeholder="db.internal.example" /></FormField>
          <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Role</p><p class="mt-1 text-sm">Primary</p></div>
          <FormField label="Protocol" error={errors['endpoint.protocol']}><select bind:value={form.endpoint.protocol} class={selectClass} aria-invalid={Boolean(errors['endpoint.protocol'])}>{#each definition.protocols as protocol}<option value={protocol}>{protocol}</option>{/each}</select></FormField>
          <FormField label="Port" error={errors['endpoint.port']}><Input type="number" bind:value={form.endpoint.port} aria-invalid={Boolean(errors['endpoint.port'])} min="1" max="65535" /></FormField>
          <FormField label="TLS mode" error={errors['endpoint.tlsMode']}><select bind:value={form.endpoint.tlsMode} class={selectClass} aria-invalid={Boolean(errors['endpoint.tlsMode'])}>{#each definition.tlsModes as mode}<option value={mode}>{mode}</option>{/each}</select></FormField>
        {:else}
          <DataField label="Runtime origin" value={`127.0.0.1:${form.installation.portMappings[0].hostPort}`} />
          <DataField label="Protocol" value={definition?.label ?? form.endpoint.protocol} />
          <p class="col-span-full text-xs text-muted-foreground">Private access can be enabled from the Resource page after creation.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Docker placement</Card.Title>
        <Card.Description>Choose the Server, image, loopback publication, and optional persistent storage.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        {#if form.managementMode === 'external'}
          <p class="col-span-full text-sm text-muted-foreground">External Resources do not have DeployCrate-managed installations or storage.</p>
        {:else}
          <FormField label="Server" error={errors['installation.serverId']}><select bind:value={form.installation.serverId} onchange={(event) => chooseServer(event.currentTarget.value)} class={selectClass} aria-invalid={Boolean(errors['installation.serverId'])} required><option value="">Select a Server</option>{#each options.servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}</select></FormField>
            <FormField label="Image reference" error={errors['installation.imageReference']}><Input bind:value={form.installation.imageReference} aria-invalid={Boolean(errors['installation.imageReference'])} placeholder={selectedPresetDefinition?.imagePlaceholder ?? 'registry.example.com/image:tag'} /></FormField>
            <FormField label="Container name" error={errors['installation.containerName']}><Input bind:value={form.installation.containerName} aria-invalid={Boolean(errors['installation.containerName'])} /></FormField>
            <FormField label="Restart policy" error={errors['installation.restartPolicy']}><select bind:value={form.installation.restartPolicy} class={selectClass} aria-invalid={Boolean(errors['installation.restartPolicy'])}><option value="no">No restart</option><option value="always">Always</option><option value="on-failure">On failure</option><option value="unless-stopped">Unless stopped</option></select></FormField>
            <FormField label="Registry credential" error={errors['installation.registryCredentialId']}><select bind:value={form.installation.registryCredentialId} class={selectClass} aria-invalid={Boolean(errors['installation.registryCredentialId'])}><option value="">None</option>{#each options.registryCredentials as credential}<option value={credential.id}>{credential.name}</option>{/each}</select></FormField>
            <FormField label="Published host port" error={errors['installation.portMappings.0.hostPort']}><Input type="number" bind:value={form.installation.portMappings[0].hostPort} aria-invalid={Boolean(errors['installation.portMappings.0.hostPort'])} min="1" max="65535" /></FormField>
            <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Container port</p><p class="mt-1 font-mono text-sm">{definition.label} default: {definition.defaultPort}/tcp</p></div>
            <div class="col-span-full border border-border bg-muted/20 px-3 py-3"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Docker port mapping</p><p class="mt-1 font-mono text-sm">127.0.0.1:{form.installation.portMappings[0].hostPort} → {definition.label} container:{definition.defaultPort}/tcp</p><p class="mt-2 text-xs text-muted-foreground">Only the host port changes. Use a different host port for each installation on the same Server, for example 1234:5432 and 2345:5432.</p></div>
            <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeVolume} /> Add a local volume</label>
            {#if includeVolume}
              <FormField label="Volume name" error={errors['volume.name']}><Input bind:value={form.volume.name} aria-invalid={Boolean(errors['volume.name'])} /></FormField>
              <FormField label="Mount path" error={errors['mount.mountPath']}><Input bind:value={form.mount.mountPath} aria-invalid={Boolean(errors['mount.mountPath'])} /></FormField>
            {/if}
        {/if}
      </Card.Content>
    </Card.Root>

    <div class="space-y-6">
      <Card.Root>
        <Card.Header>
          <Card.Title>Resource administrator</Card.Title>
          <Card.Description>{form.kind === 'postgresql' && form.managementMode === 'managed' ? 'Required installation-specific PostgreSQL credential used for initialization and role management.' : 'Application credentials can be added after the Resource is running.'}</Card.Description>
        </Card.Header>
        <Card.Content class="grid gap-5 sm:grid-cols-2">
          {#if form.kind === 'postgresql' && form.managementMode === 'managed'}
            <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Credential type</p><p class="mt-1 text-sm">Resource administrator</p></div>
            <FormField label="Administrator username" error={errors['credential.username']}><Input bind:value={form.credential.username} aria-invalid={Boolean(errors['credential.username'])} required autocomplete="username" /></FormField>
            {#each definition.credentialFields as field}
              <FormField label={`Administrator ${field.label.toLowerCase()}`} error={errors[`credential.secretValues.${field.name}`]}><Input type="password" value={form.credential.secretValues[field.name] ?? ''} oninput={(event) => form.credential.secretValues[field.name] = event.currentTarget.value} aria-invalid={Boolean(errors[`credential.secretValues.${field.name}`] || errors['credential.secretValues'])} required autocomplete="new-password" /></FormField>
            {/each}
            {#if errors['credential.secretValues']}<p class="col-span-full text-xs font-medium text-destructive">{errors['credential.secretValues']}</p>{/if}
            <p class="col-span-full text-xs text-muted-foreground">This credential is unique to this PostgreSQL cluster and is never offered to Environment connections.</p>
          {:else}
            <p class="col-span-full text-sm text-muted-foreground">No installation-specific credential is required for this Resource kind.</p>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Health check</Card.Title>
          <Card.Description>{form.kind === 'postgresql' || form.kind === 'clickhouse' ? 'This managed Resource receives a database-aware health check automatically.' : 'Optionally define an initial observed health target.'}</Card.Description>
        </Card.Header>
        <Card.Content class="grid gap-5 sm:grid-cols-2">
          {#if form.managementMode !== 'managed'}
            <p class="col-span-full text-sm text-muted-foreground">Health checks require a managed installation and can be added later.</p>
          {:else}
            <label class="col-span-full flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeHealth} /> {form.kind === 'postgresql' || form.kind === 'clickhouse' ? 'Customize the initial health check' : 'Add an initial health check'}</label>
            {#if includeHealth}
              <FormField label="Name" error={errors['healthCheck.name']}><Input bind:value={form.healthCheck.name} aria-invalid={Boolean(errors['healthCheck.name'])} /></FormField>
              <FormField label="Kind" error={errors['healthCheck.kind']}><select bind:value={form.healthCheck.kind} class={selectClass} aria-invalid={Boolean(errors['healthCheck.kind'])}>{#each definition.healthCheckKinds as kind}<option value={kind}>{kind}</option>{/each}</select></FormField>
              <FormField label="Interval seconds" error={errors['healthCheck.intervalSeconds']}><Input type="number" bind:value={form.healthCheck.intervalSeconds} aria-invalid={Boolean(errors['healthCheck.intervalSeconds'])} min="1" /></FormField>
              <FormField label="Timeout seconds" error={errors['healthCheck.timeoutSeconds']}><Input type="number" bind:value={form.healthCheck.timeoutSeconds} aria-invalid={Boolean(errors['healthCheck.timeoutSeconds'])} min="1" /></FormField>
            {/if}
          {/if}
        </Card.Content>
      </Card.Root>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-4 border-t border-border py-5">
      <p class="text-xs text-muted-foreground">Managed placement records intent only and does not start containers.</p>
      <div class="flex gap-2">
        <Button variant="outline" href={routes.resources()}>Cancel</Button>
        <Button type="submit" disabled={processing || !form.name}>Create Resource</Button>
      </div>
    </div>
  </form>
  {/if}
</DashboardLayout>
