<script lang="ts">
  import { router } from '@inertiajs/svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Checkbox } from '@/Components/ui/checkbox'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { slugify } from '@/lib/slug'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type Engine = { engine: string; label: string; resourceType: 'database' | 'cache' | 'service'; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; healthCheckKinds: string[]; defaultPort: number; defaultProtocol: string; defaultTlsMode: string }
  type Server = { id: string; name: string; address: string }
  type PrivateNetwork = { id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }
  type Options = { engines: Engine[]; resourceTypes: string[]; servers: Server[]; privateNetworks: PrivateNetwork[]; registryCredentials: Array<{ id: string; name: string }> }
  type Preset = { engine: string; badge: string; description: string; image: string; mountPath: string }

  let { auth, options, errors = {} }: { auth: { email: string }; options: Options; errors?: Record<string, string> } = $props()
  let selectedEngine = $state('')
  let includeVolume = $state(true)
  let processing = $state(false)
  let slugCustomized = $state(false)
  let form = $state(initialForm())

  const presets: Record<string, Preset> = {
    postgresql: { engine: 'postgresql', badge: 'PG', description: 'A PostgreSQL server managed as a Docker Resource.', image: 'postgres:17-alpine', mountPath: '/var/lib/postgresql/data' },
    redis: { engine: 'redis', badge: 'RD', description: 'An in-memory cache managed as a Docker Resource.', image: 'redis:8-alpine', mountPath: '/data' },
    clickhouse: { engine: 'clickhouse', badge: 'CH', description: 'A column-oriented database managed as a Docker Resource.', image: 'clickhouse/clickhouse-server:latest', mountPath: '/var/lib/clickhouse' },
    http: { engine: 'http', badge: 'HT', description: 'An HTTP service from any compatible Docker image.', image: '', mountPath: '/data' },
    tcp: { engine: 'tcp', badge: 'TC', description: 'A generic TCP service from any compatible Docker image.', image: '', mountPath: '/data' },
  }
  const availableEngines = $derived(options.engines.filter((engine) => engine.engine !== 'registry'))
  const definition = $derived(availableEngines.find((engine) => engine.engine === selectedEngine))
  const preset = $derived(selectedEngine ? presets[selectedEngine] ?? { engine: selectedEngine, badge: selectedEngine.slice(0, 2).toUpperCase(), description: `A ${definition?.label ?? selectedEngine} Docker Resource.`, image: '', mountPath: '/data' } : undefined)
  const availableNetworks = $derived(options.privateNetworks.filter((network) => network.serverIds.includes(form.installation.serverId) && Boolean(network.serverAddresses[form.installation.serverId])))

  function initialForm() {
    return {
      name: '', slug: '', sharingScope: 'environment', privateNetworkId: '',
      installation: { imageReference: '', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', configuration: {}, hostPort: 1, serverId: '', registryCredentialId: '' },
      volume: { name: '', driver: 'local', configuration: {}, serverId: '' },
      mountPath: '/data',
      administrator: { name: 'Resource administrator', username: '', password: '' },
    }
  }

  function chooseEngine(engine: Engine) {
    selectedEngine = engine.engine
    const selectedPreset = presets[engine.engine]
    form.installation.imageReference = selectedPreset?.image ?? ''
    form.installation.containerName = ''
    form.installation.hostPort = engine.defaultPort
    form.volume.name = ''
    form.mountPath = selectedPreset?.mountPath ?? '/data'
    form.administrator.username = engine.engine === 'postgresql' ? 'resource_admin' : ''
  }

  function chooseServer(serverId: string) {
    form.installation.serverId = serverId
    if (form.privateNetworkId && !options.privateNetworks.some((network) => network.id === form.privateNetworkId && network.serverIds.includes(serverId) && Boolean(network.serverAddresses[serverId]))) {
      form.privateNetworkId = ''
    }
  }

  function updateName(name: string) {
    form.name = name
    if (!slugCustomized) form.slug = slugify(name)
  }

  function updateSlug(slug: string) {
    form.slug = slug
    slugCustomized = true
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    if (!definition) return
    processing = true
    const portMappings = [{ hostPort: form.installation.hostPort, containerPort: definition.defaultPort, protocol: 'tcp' }]
    router.post(routes.resourceCreate(), {
      name: form.name,
      slug: form.slug,
      resourceType: definition.resourceType,
      configuration: { engine: definition.engine },
      sharingScope: form.sharingScope,
      privateNetworkId: form.privateNetworkId,
      installation: { ...form.installation, portMappings },
      volume: includeVolume ? { ...form.volume, serverId: form.installation.serverId } : null,
      mount: includeVolume ? { mountPath: form.mountPath, readOnly: false, resourceVolumeId: '', resourceInstallationId: '' } : null,
      credential: definition.resourceType === 'database' ? {
        name: form.administrator.name,
        username: form.administrator.username,
        metadata: { purpose: 'administrator' },
        secretValues: { password: form.administrator.password },
      } : null,
    }, { onFinish: () => { processing = false } })
  }
</script>

<svelte:head><title>New Resource</title></svelte:head>
<DashboardLayout email={auth.email}>
  {#if !definition}
    <div class="mx-auto max-w-5xl space-y-8">
      <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">New Resource</p><h1 class="mt-3 text-3xl font-semibold">What would you like to deploy?</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Every Resource is a Docker workload. Its type and engine describe how DeployCrate configures access around it.</p></header>
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {#each availableEngines as engine}
          {@const card = presets[engine.engine] ?? { badge: engine.engine.slice(0, 2).toUpperCase(), description: `A ${engine.label} Docker Resource.` }}
          <Button type="button" variant="outline" onclick={() => chooseEngine(engine)} class="group h-auto min-h-48 w-full flex-col items-stretch justify-start bg-card p-5 text-left whitespace-normal hover:border-primary/60 hover:bg-muted/30">
            <span class="grid size-11 place-items-center border border-primary/25 bg-primary/10 font-mono text-sm font-semibold text-primary">{card.badge}</span>
            <span class="mt-6 text-base font-semibold">{engine.label}</span>
            <span class="mt-2 flex-1 text-sm leading-6 text-muted-foreground">{card.description}</span>
            <span class="mt-4 text-[10px] font-medium uppercase tracking-[0.2em] text-muted-foreground">{engine.resourceType}</span>
          </Button>
        {/each}
      </div>
      <div class="flex justify-end border-t border-border pt-5"><Button variant="outline" href={routes.resources()}>Cancel</Button></div>
    </div>
  {:else}
    <form class="mx-auto max-w-5xl space-y-6" onsubmit={submit}>
      <header class="flex flex-wrap items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{definition.resourceType} · {definition.engine}</p><h1 class="mt-3 text-3xl font-semibold">Configure {definition.label}</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Create the Resource, its Docker installation, primary endpoint, and initial access metadata.</p></div><Button type="button" variant="outline" onclick={() => selectedEngine = ''}>Change type</Button></header>

      {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"><Alert.Title>The Resource could not be created</Alert.Title><Alert.Description><ul class="mt-2 list-disc space-y-1 pl-5">{#each Object.entries(errors) as [field, message]}<li>{field}: {message}</li>{/each}</ul></Alert.Description></Alert.Root>{/if}

      <Card.Root><Card.Header><Card.Title>Resource identity</Card.Title><Card.Description>The slug follows the name until you customize it.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Name" error={errors.name}><Input value={form.name} oninput={(event) => updateName(event.currentTarget.value)} required /></FormField><FormField label="Slug" error={errors.slug}><Input value={form.slug} oninput={(event) => updateSlug(event.currentTarget.value)} placeholder="shared-postgresql" required /></FormField><FormField label="Sharing scope" error={errors.sharingScope}><NativeSelect.Root class="w-full" bind:value={form.sharingScope}><NativeSelect.Option value="environment">Environment policy</NativeSelect.Option><NativeSelect.Option value="application">Application policy</NativeSelect.Option><NativeSelect.Option value="global">Global policy</NativeSelect.Option></NativeSelect.Root></FormField></Card.Content></Card.Root>

      <Card.Root><Card.Header><Card.Title>Docker installation</Card.Title><Card.Description>The container is the Resource runtime. Start, stop, restart, and endpoint controls remain available after creation.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Server" error={errors['installation.serverId']}><NativeSelect.Root class="w-full" bind:value={form.installation.serverId} onchange={(event) => chooseServer(event.currentTarget.value)} required><NativeSelect.Option value="">Select a Server</NativeSelect.Option>{#each options.servers as server}<NativeSelect.Option value={server.id}>{server.name} · {server.address}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Private network" error={errors.privateNetworkId}><NativeSelect.Root class="w-full" bind:value={form.privateNetworkId} disabled={!form.installation.serverId}><NativeSelect.Option value="">Do not attach</NativeSelect.Option>{#each availableNetworks as network}<NativeSelect.Option value={network.id}>{network.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Image reference" error={errors['installation.imageReference']}><Input bind:value={form.installation.imageReference} placeholder="registry.example.com/image:tag" required /></FormField><FormField label="Container name" error={errors['installation.containerName']}><Input bind:value={form.installation.containerName} placeholder={form.slug || 'resource-container'} required /></FormField><FormField label="Restart policy" error={errors['installation.restartPolicy']}><NativeSelect.Root class="w-full" bind:value={form.installation.restartPolicy}><NativeSelect.Option value="no">No restart</NativeSelect.Option><NativeSelect.Option value="always">Always</NativeSelect.Option><NativeSelect.Option value="on-failure">On failure</NativeSelect.Option><NativeSelect.Option value="unless-stopped">Unless stopped</NativeSelect.Option></NativeSelect.Root></FormField><FormField label="Registry credential"><NativeSelect.Root class="w-full" bind:value={form.installation.registryCredentialId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each options.registryCredentials as credential}<NativeSelect.Option value={credential.id}>{credential.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Published host port" error={errors['installation.portMappings.0.hostPort']}><Input type="number" bind:value={form.installation.hostPort} min="1" max="65535" required /></FormField><div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Container port</p><p class="mt-1 font-mono text-sm">{definition.defaultPort}/tcp</p></div></Card.Content></Card.Root>

      {#if definition.resourceType === 'database'}
        <Card.Root><Card.Header><Card.Title>Database administrator</Card.Title><Card.Description>This credential becomes the Resource superuser. It is stored encrypted and is never offered to an Environment.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Display name" error={errors['credential.name']}><Input bind:value={form.administrator.name} required /></FormField><FormField label="Administrator username" error={errors['credential.username']}><Input bind:value={form.administrator.username} autocomplete="username" required /></FormField><FormField label="Administrator password" error={errors['credential.secretValues.password']}><Input type="password" bind:value={form.administrator.password} autocomplete="new-password" required /></FormField></Card.Content></Card.Root>
      {/if}

      <Card.Root><Card.Header><Card.Title>Persistent storage</Card.Title><Card.Description>Optionally attach one Docker volume and choose where it is mounted inside the container.</Card.Description></Card.Header><Card.Content class="space-y-5"><label class="flex cursor-pointer items-center gap-3 text-sm"><Checkbox bind:checked={includeVolume} /> Create persistent volume</label>{#if includeVolume}<div class="grid gap-5 sm:grid-cols-2"><FormField label="Volume name" error={errors['volume.name']}><Input bind:value={form.volume.name} placeholder={`${form.slug || 'resource'}-data`} required /></FormField><FormField label="Driver" error={errors['volume.driver']}><Input bind:value={form.volume.driver} required /></FormField><div class="sm:col-span-2"><FormField label="Container mount path" error={errors['mount.mountPath']}><Input bind:value={form.mountPath} placeholder={preset?.mountPath ?? '/data'} required /></FormField></div></div>{/if}</Card.Content></Card.Root>

      <div class="flex flex-wrap justify-end gap-3 border-t border-border pt-5"><Button variant="outline" href={routes.resources()}>Cancel</Button><Button type="submit" disabled={processing} aria-busy={processing}>{#if processing}<Spinner />{/if}Create Resource</Button></div>
    </form>
  {/if}
</DashboardLayout>
