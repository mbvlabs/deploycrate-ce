<script lang="ts">
  import { Link, router, useForm } from '@inertiajs/svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import { Checkbox } from '@/Components/ui/checkbox'
  import * as Empty from '@/Components/ui/empty'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import { Textarea } from '@/Components/ui/textarea'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { slugify } from '@/lib/slug'
  import { routes } from '@/routes'

  type CredentialField = { name: string; label: string; required: boolean; secret: boolean }
  type Engine = { engine: string; label: string; resourceType: 'database' | 'cache' | 'service'; protocols: string[]; endpointRoles: string[]; tlsModes: string[]; credentialFields: CredentialField[]; healthCheckKinds: string[]; defaultPort: number }
  type EnvironmentOption = { id: string; name: string; kind: string; applicationId: string; applicationName: string }
  type ApplicationOption = { id: string; name: string }
  type Options = {
    engines: Engine[]
    environments: EnvironmentOption[]
    servers: Array<{ id: string; name: string; address: string }>
    privateNetworks: Array<{ id: string; name: string; serverIds: string[]; serverAddresses: Record<string, string> }>
    registryCredentials: Array<{ id: string; name: string }>
  }

  let { auth, resource, options, errors = {} }: { auth: { email: string }; resource: any; options: Options; errors?: Record<string, string> } = $props()

  const identity = useForm(() => ({ name: resource.name, slug: resource.slug, engine: resource.engine, engineVersion: resource.configuration?.engine_version ?? '', sharingScope: resource.sharingScope }))
  const definition = $derived(options.engines.find((engine) => engine.engine === $identity.engine) ?? options.engines[0])
  let selectedEnvironmentId = $state('')
  let selectedApplicationId = $state('')
  let slugCustomized = $state(initialSlugCustomized())
  let granting = $state(false)
  let processingKey = $state('')
  let jsonErrors = $state<Record<string, string>>({})
  let revokeTarget = $state<{ scope: 'Environment' | 'Application'; id: string; name: string } | null>(null)
  let revokeDialogOpen = $state(false)
  let revokeProcessing = $state(false)
  let revokeError = $state('')

  function initializeFromResource<T>(initialize: (value: any) => T): T {
    return initialize(resource)
  }

  let installations = $state(initializeFromResource((initial) => initial.installations.map((item: any) => ({
    ...item,
    imageDigest: item.imageDigest ?? '',
    registryCredentialId: item.registryCredentialId ?? '',
    configurationText: formattedJSON(item.configuration),
    hostPort: item.configuration?.portMappings?.[0]?.hostPort ?? 1,
    containerPort: item.configuration?.portMappings?.[0]?.containerPort ?? 1,
    protocol: item.configuration?.portMappings?.[0]?.protocol ?? 'tcp',
  }))))
  let endpoints = $state(initializeFromResource((initial) => initial.endpoints.map((item: any) => ({ ...item, settingsText: formattedJSON(item.settings), privateNetworkId: item.privateNetworkId ?? '' }))))
  let credentials = $state(initializeFromResource((initial) => initial.credentials.map((item: any) => ({ ...item, metadataText: formattedJSON(item.metadata), rotate: false, secretValues: {} as Record<string, string> }))))
  let volumes = $state(initializeFromResource((initial) => initial.volumes.map((item: any) => ({ ...item, configurationText: formattedJSON(item.configuration) }))))
  let mounts = $state(initializeFromResource((initial) => initial.mounts.map((item: any) => ({ ...item }))))
  let healthChecks = $state(initializeFromResource((initial) => initial.healthChecks.map((item: any) => ({
    ...item,
    configurationText: formattedJSON(item.configuration),
    resourceEndpointId: item.resourceEndpointId ?? '',
    resourceCredentialId: item.resourceCredentialId ?? '',
  }))))

  function formattedJSON(value: unknown) { return JSON.stringify(value ?? {}, null, 2) }
  function initialSlugCustomized() { return resource.slug !== slugify(resource.name) }
  function editURL(url: string) { return `${url}?returnTo=edit` }
  function isProcessing(key: string) { return processingKey === key }

  function updateName(name: string) {
    $identity.name = name
    if (!slugCustomized) $identity.slug = slugify(name)
  }

  function updateSlug(slug: string) {
    $identity.slug = slug
    slugCustomized = true
  }

  function parseJSON(key: string, value: string): unknown | undefined {
    try {
      const parsed = JSON.parse(value || '{}')
      jsonErrors[key] = ''
      return parsed
    } catch {
      jsonErrors[key] = 'Must contain valid JSON.'
      return undefined
    }
  }

  function patch(key: string, url: string, payload: Record<string, unknown>) {
    if (processingKey) return
    processingKey = key
    router.patch(editURL(url), payload, { preserveScroll: true, onFinish: () => (processingKey = '') })
  }

  function saveIdentity(event: SubmitEvent) {
    event.preventDefault()
    const configuration = { ...resource.configuration, engine: $identity.engine }
    if ($identity.engineVersion) configuration.engine_version = $identity.engineVersion
    else delete configuration.engine_version
    $identity.transform((values) => ({ name: values.name, slug: values.slug, resourceType: definition.resourceType, configuration, sharingScope: values.sharingScope })).patch(routes.resourceUpdate(resource.id))
  }

  function saveInstallation(item: any) {
    const configuration = parseJSON(`installation:${item.id}`, item.configurationText)
    if (configuration === undefined) return
    patch(`installation:${item.id}`, routes.resourceInstallationUpdate(resource.id, item.id), {
      imageReference: item.imageReference,
      imageDigest: item.imageDigest ?? '',
      containerName: item.containerName,
      restartPolicy: item.restartPolicy,
      configuration,
      portMappings: [{ hostPort: Number(item.hostPort), containerPort: Number(item.containerPort), protocol: item.protocol }],
      serverId: item.serverId,
      registryCredentialId: item.registryCredentialId ?? '',
    })
  }

  function saveEndpoint(item: any) {
    const settings = parseJSON(`endpoint:${item.id}`, item.settingsText)
    if (settings === undefined) return
    patch(`endpoint:${item.id}`, routes.resourceEndpointUpdate(resource.id, item.id), {
      name: item.name, role: item.role, address: item.address, port: Number(item.port), protocol: item.protocol,
      tlsMode: item.tlsMode, settings, privateNetworkId: item.privateNetworkId ?? '',
    })
  }

  function saveCredential(item: any, rotate: boolean) {
    const metadata = parseJSON(`credential:${item.id}`, item.metadataText)
    if (metadata === undefined) return
    patch(`credential:${item.id}`, routes.resourceCredentialUpdate(resource.id, item.id), {
      name: item.name, username: item.username ?? '', metadata, rotate, secretValues: rotate ? item.secretValues : {},
    })
  }

  function saveVolume(item: any) {
    const configuration = parseJSON(`volume:${item.id}`, item.configurationText)
    if (configuration === undefined) return
    patch(`volume:${item.id}`, routes.resourceVolumeUpdate(resource.id, item.id), {
      name: item.name, driver: item.driver, configuration, serverId: item.serverId,
    })
  }

  function saveMount(item: any) {
    patch(`mount:${item.id}`, routes.resourceMountUpdate(resource.id, item.id), {
      mountPath: item.mountPath, readOnly: item.readOnly,
      resourceVolumeId: item.resourceVolumeId, resourceInstallationId: item.resourceInstallationId,
    })
  }

  function saveHealthCheck(item: any) {
    const configuration = parseJSON(`health:${item.id}`, item.configurationText)
    if (configuration === undefined) return
    patch(`health:${item.id}`, routes.resourceHealthCheckUpdate(resource.id, item.id), {
      name: item.name, kind: item.kind, configuration,
      intervalSeconds: Number(item.intervalSeconds), timeoutSeconds: Number(item.timeoutSeconds),
      failureThreshold: Number(item.failureThreshold), successThreshold: Number(item.successThreshold),
      enabled: item.enabled, resourceEndpointId: item.resourceEndpointId ?? '', resourceCredentialId: item.resourceCredentialId ?? '',
    })
  }

  function availableEnvironments() { const granted = new Set(resource.environmentGrants.map((grant: any) => grant.environmentId)); return options.environments.filter((environment) => !granted.has(environment.id)) }
  function applications() { const values = new Map<string, ApplicationOption>(); for (const environment of options.environments) values.set(environment.applicationId, { id: environment.applicationId, name: environment.applicationName }); return [...values.values()].sort((left, right) => left.name.localeCompare(right.name)) }
  function availableApplications() { const granted = new Set(resource.applicationGrants.map((grant: any) => grant.applicationId)); return applications().filter((application) => !granted.has(application.id)) }
  function grantEnvironment() { if (!selectedEnvironmentId) return; granting = true; router.post(editURL(routes.resourceEnvironmentGrantCreate(resource.id, selectedEnvironmentId)), {}, { preserveScroll: true, onSuccess: () => selectedEnvironmentId = '', onFinish: () => (granting = false) }) }
  function grantApplication() { if (!selectedApplicationId) return; granting = true; router.post(editURL(routes.resourceApplicationGrantCreate(resource.id, selectedApplicationId)), {}, { preserveScroll: true, onSuccess: () => selectedApplicationId = '', onFinish: () => (granting = false) }) }
  function askToRevoke(scope: 'Environment' | 'Application', id: string, name: string) { revokeTarget = { scope, id, name }; revokeError = ''; revokeDialogOpen = true }
  function confirmRevoke() {
    if (!revokeTarget) return
    revokeProcessing = true
    revokeError = ''
    const url = revokeTarget.scope === 'Environment'
      ? routes.resourceEnvironmentGrantDestroy(resource.id, revokeTarget.id)
      : routes.resourceApplicationGrantDestroy(resource.id, revokeTarget.id)
    router.delete(editURL(url), {
      preserveScroll: true,
      onSuccess: () => (revokeDialogOpen = false),
      onError: () => (revokeError = 'Access could not be revoked. Please try again.'),
      onFinish: () => (revokeProcessing = false),
    })
  }
</script>

<svelte:head><title>Edit {resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="mx-auto max-w-6xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Resource configuration</p><h1 class="mt-3 text-3xl font-semibold">Edit {resource.name}</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Manage identity, Docker runtime, networking, credentials, storage, health, and selection policy.</p></div><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.resourceShow(resource.id)}>Done editing</Link>{/snippet}</Button></header>

    {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"><Alert.Title>The changes could not be saved</Alert.Title><Alert.Description><ul class="mt-2 list-disc pl-5">{#each Object.entries(errors) as [field, message]}<li>{field}: {message}</li>{/each}</ul></Alert.Description></Alert.Root>{/if}

    <form onsubmit={saveIdentity}>
      <Card.Root><Card.Header><Card.Title>Identity and engine</Card.Title><Card.Description>The Resource type follows the selected engine.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Name" error={errors.name}><Input value={$identity.name} oninput={(event) => updateName(event.currentTarget.value)} required /></FormField><FormField label="Slug" error={errors.slug}><Input value={$identity.slug} oninput={(event) => updateSlug(event.currentTarget.value)} required /></FormField><FormField label="Engine" error={errors['configuration.engine']}><NativeSelect.Root class="w-full" bind:value={$identity.engine} required>{#each options.engines as engine}<NativeSelect.Option value={engine.engine}>{engine.label} · {engine.resourceType}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Engine version" error={errors['configuration.engine_version']}><Input bind:value={$identity.engineVersion} placeholder="Optional" /></FormField><FormField label="Sharing scope" error={errors.sharingScope}><NativeSelect.Root class="w-full" bind:value={$identity.sharingScope}><NativeSelect.Option value="environment">Environment policy</NativeSelect.Option><NativeSelect.Option value="application">Application policy</NativeSelect.Option><NativeSelect.Option value="global">Global policy</NativeSelect.Option></NativeSelect.Root></FormField></Card.Content><Card.Footer class="border-t border-border"><Button type="submit" disabled={$identity.processing} aria-busy={$identity.processing}>{#if $identity.processing}<Spinner />{/if}Save identity</Button></Card.Footer></Card.Root>
    </form>

    <Card.Root>
      <Card.Header><Card.Title>Docker installation</Card.Title><Card.Description>Changes to the image, ports, or storage take effect after removing and recreating the container from the Resource page.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#if installations.length === 0}<Empty.Root><Empty.Header><Empty.Title>No installation</Empty.Title><Empty.Description>This Resource has no Docker installation to edit.</Empty.Description></Empty.Header></Empty.Root>{/if}
        {#each installations as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveInstallation(item) }}>
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <FormField label="Server" error={errors.serverId}><NativeSelect.Root class="w-full" bind:value={item.serverId}>{#each options.servers as server}<NativeSelect.Option value={server.id}>{server.name} · {server.address}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <FormField label="Image reference" error={errors.imageReference}><Input bind:value={item.imageReference} required /></FormField>
              <FormField label="Image digest" error={errors.imageDigest}><Input bind:value={item.imageDigest} placeholder="Optional" /></FormField>
              <FormField label="Container name" error={errors.containerName}><Input bind:value={item.containerName} required /></FormField>
              <FormField label="Restart policy"><NativeSelect.Root class="w-full" bind:value={item.restartPolicy}><NativeSelect.Option value="no">No restart</NativeSelect.Option><NativeSelect.Option value="always">Always</NativeSelect.Option><NativeSelect.Option value="on-failure">On failure</NativeSelect.Option><NativeSelect.Option value="unless-stopped">Unless stopped</NativeSelect.Option></NativeSelect.Root></FormField>
              <FormField label="Registry credential"><NativeSelect.Root class="w-full" bind:value={item.registryCredentialId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each options.registryCredentials as credential}<NativeSelect.Option value={credential.id}>{credential.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <FormField label="Published host port" error={errors['portMappings.0.hostPort']}><Input type="number" min="1" max="65535" bind:value={item.hostPort} required /></FormField>
              <FormField label="Container port" error={errors['portMappings.0.containerPort']}><Input type="number" min="1" max="65535" bind:value={item.containerPort} required /></FormField>
              <FormField label="Protocol"><NativeSelect.Root class="w-full" bind:value={item.protocol}><NativeSelect.Option value="tcp">TCP</NativeSelect.Option><NativeSelect.Option value="udp">UDP</NativeSelect.Option></NativeSelect.Root></FormField>
              <div class="sm:col-span-2 lg:col-span-3"><FormField label="Installation configuration JSON" error={jsonErrors[`installation:${item.id}`] ?? errors.configuration}><Textarea class="min-h-36 font-mono" bind:value={item.configurationText} /></FormField></div>
            </div>
            <Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`installation:${item.id}`)}>{#if isProcessing(`installation:${item.id}`)}<Spinner />{/if}Save installation</Button>
          </form>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Endpoints</Card.Title><Card.Description>Addresses and protocols published for this Resource.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#each endpoints as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveEndpoint(item) }}>
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <FormField label="Name"><Input bind:value={item.name} required /></FormField>
              <FormField label="Address"><Input bind:value={item.address} required /></FormField>
              <FormField label="Port"><Input type="number" min="1" max="65535" bind:value={item.port} required /></FormField>
              <FormField label="Role"><NativeSelect.Root class="w-full" bind:value={item.role}>{#each definition.endpointRoles as role}<NativeSelect.Option value={role}>{role}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <FormField label="Protocol"><NativeSelect.Root class="w-full" bind:value={item.protocol}>{#each definition.protocols as protocol}<NativeSelect.Option value={protocol}>{protocol}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <FormField label="TLS mode"><NativeSelect.Root class="w-full" bind:value={item.tlsMode}>{#each definition.tlsModes as mode}<NativeSelect.Option value={mode}>{mode}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <FormField label="Private network"><NativeSelect.Root class="w-full" bind:value={item.privateNetworkId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each options.privateNetworks as network}<NativeSelect.Option value={network.id}>{network.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
              <div class="sm:col-span-2 lg:col-span-3"><FormField label="Endpoint settings JSON" error={jsonErrors[`endpoint:${item.id}`]}><Textarea class="min-h-28 font-mono" bind:value={item.settingsText} /></FormField></div>
            </div>
            <Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`endpoint:${item.id}`)}>{#if isProcessing(`endpoint:${item.id}`)}<Spinner />{/if}Save endpoint</Button>
          </form>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Credentials</Card.Title><Card.Description>Metadata can be edited without exposing stored secrets. Enter new secret values only when rotating a credential.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#each credentials as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveCredential(item, false) }}>
            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="Display name"><Input bind:value={item.name} required /></FormField>
              <FormField label="Username"><Input bind:value={item.username} required={resource.engine === 'postgresql'} disabled={resource.engine === 'postgresql'} /></FormField>
              <div class="sm:col-span-2"><FormField label="Credential metadata JSON" error={jsonErrors[`credential:${item.id}`]}><Textarea class="min-h-28 font-mono" bind:value={item.metadataText} /></FormField></div>
              {#each definition.credentialFields as field}<FormField label={`New ${field.label}`}><Input type={field.secret ? 'password' : 'text'} value={item.secretValues[field.name] ?? ''} oninput={(event) => item.secretValues[field.name] = event.currentTarget.value} autocomplete="new-password" /></FormField>{/each}
            </div>
            <div class="flex flex-wrap gap-2"><Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`credential:${item.id}`)}>{#if isProcessing(`credential:${item.id}`)}<Spinner />{/if}Save metadata</Button>{#if definition.credentialFields.length > 0}<Button type="button" variant="outline" disabled={Boolean(processingKey) || definition.credentialFields.some((field) => field.required && !item.secretValues[field.name])} onclick={() => saveCredential(item, true)}>Rotate secret</Button>{/if}</div>
          </form>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Storage</Card.Title><Card.Description>Edit the Docker volume and choose its mount location inside the container. Remove and recreate the container after changing the mount.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#each volumes as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveVolume(item) }}>
            <div class="grid gap-4 sm:grid-cols-3"><FormField label="Volume name"><Input bind:value={item.name} required /></FormField><FormField label="Driver"><Input bind:value={item.driver} required /></FormField><FormField label="Server"><NativeSelect.Root class="w-full" bind:value={item.serverId}>{#each options.servers as server}<NativeSelect.Option value={server.id}>{server.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><div class="sm:col-span-3"><FormField label="Volume configuration JSON" error={jsonErrors[`volume:${item.id}`]}><Textarea class="min-h-28 font-mono" bind:value={item.configurationText} /></FormField></div></div>
            <Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`volume:${item.id}`)}>{#if isProcessing(`volume:${item.id}`)}<Spinner />{/if}Save volume</Button>
          </form>
        {/each}
        {#each mounts as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveMount(item) }}>
            <div class="grid gap-4 sm:grid-cols-3"><FormField label="Container mount path" error={errors.mountPath}><Input bind:value={item.mountPath} required /></FormField><FormField label="Volume"><NativeSelect.Root class="w-full" bind:value={item.resourceVolumeId}>{#each volumes as volume}<NativeSelect.Option value={volume.id}>{volume.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Installation"><NativeSelect.Root class="w-full" bind:value={item.resourceInstallationId}>{#each installations as installation}<NativeSelect.Option value={installation.id}>{installation.containerName}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={item.readOnly} /> Mount read only</label></div>
            <Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`mount:${item.id}`)}>{#if isProcessing(`mount:${item.id}`)}<Spinner />{/if}Save mount</Button>
          </form>
        {/each}
        {#if volumes.length === 0 && mounts.length === 0}<Empty.Root><Empty.Header><Empty.Title>No persistent storage</Empty.Title><Empty.Description>Add storage from the Resource page.</Empty.Description></Empty.Header></Empty.Root>{/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Health checks</Card.Title><Card.Description>Configure readiness behavior and access dependencies.</Card.Description></Card.Header>
      <Card.Content class="space-y-6">
        {#each healthChecks as item (item.id)}
          <form class="space-y-5 border border-border p-4" onsubmit={(event) => { event.preventDefault(); saveHealthCheck(item) }}>
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4"><FormField label="Name"><Input bind:value={item.name} required /></FormField><FormField label="Kind"><NativeSelect.Root class="w-full" bind:value={item.kind}>{#each definition.healthCheckKinds as kind}<NativeSelect.Option value={kind}>{kind}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Interval seconds"><Input type="number" min="1" bind:value={item.intervalSeconds} /></FormField><FormField label="Timeout seconds"><Input type="number" min="1" bind:value={item.timeoutSeconds} /></FormField><FormField label="Failure threshold"><Input type="number" min="1" bind:value={item.failureThreshold} /></FormField><FormField label="Success threshold"><Input type="number" min="1" bind:value={item.successThreshold} /></FormField><FormField label="Endpoint"><NativeSelect.Root class="w-full" bind:value={item.resourceEndpointId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each endpoints as endpoint}<NativeSelect.Option value={endpoint.id}>{endpoint.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Credential"><NativeSelect.Root class="w-full" bind:value={item.resourceCredentialId}><NativeSelect.Option value="">None</NativeSelect.Option>{#each credentials as credential}<NativeSelect.Option value={credential.id}>{credential.name}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={item.enabled} /> Enabled</label><div class="sm:col-span-2 lg:col-span-4"><FormField label="Health configuration JSON" error={jsonErrors[`health:${item.id}`]}><Textarea class="min-h-28 font-mono" bind:value={item.configurationText} /></FormField></div></div>
            <Button type="submit" disabled={Boolean(processingKey)} aria-busy={isProcessing(`health:${item.id}`)}>{#if isProcessing(`health:${item.id}`)}<Spinner />{/if}Save health check</Button>
          </form>
        {/each}
      </Card.Content>
    </Card.Root>

    <Card.Root><Card.Header><Card.Title>Selection grants</Card.Title><Card.Description>Choose which consumers may select this Resource and one of its application credentials.</Card.Description></Card.Header><Card.Content class="space-y-5">
      {#if $identity.sharingScope === 'global'}
        <p class="text-sm text-muted-foreground">Global Resources are selectable by every Environment.</p>
      {:else if $identity.sharingScope === 'environment'}
        <div class="flex flex-col gap-2 sm:flex-row"><NativeSelect.Root class="w-full" bind:value={selectedEnvironmentId}><NativeSelect.Option value="">Select an Environment</NativeSelect.Option>{#each availableEnvironments() as environment}<NativeSelect.Option value={environment.id}>{environment.applicationName} · {environment.name} ({environment.kind})</NativeSelect.Option>{/each}</NativeSelect.Root><Button class="sm:shrink-0" type="button" disabled={!selectedEnvironmentId || granting} aria-busy={granting} onclick={grantEnvironment}>{#if granting}<Spinner />{/if}Grant access</Button></div>
        {#if resource.environmentGrants.length}<div class="divide-y divide-border border border-border">{#each resource.environmentGrants as grant}<div class="flex flex-col justify-between gap-3 p-3 sm:flex-row sm:items-center"><div><p class="text-sm font-medium">{grant.applicationName} · {grant.environmentName}</p><p class="text-xs text-muted-foreground">{grant.environmentKind}</p></div><Button class="self-start sm:self-auto" type="button" size="sm" variant="destructive" onclick={() => askToRevoke('Environment', grant.environmentId, `${grant.applicationName} · ${grant.environmentName}`)}>Revoke</Button></div>{/each}</div>{:else}<Empty.Root class="border border-dashed border-border py-8"><Empty.Header><Empty.Title>No Environment grants</Empty.Title><Empty.Description>Grant access to make this Resource selectable by an Environment.</Empty.Description></Empty.Header></Empty.Root>{/if}
      {:else}
        <div class="flex flex-col gap-2 sm:flex-row"><NativeSelect.Root class="w-full" bind:value={selectedApplicationId}><NativeSelect.Option value="">Select an Application</NativeSelect.Option>{#each availableApplications() as application}<NativeSelect.Option value={application.id}>{application.name}</NativeSelect.Option>{/each}</NativeSelect.Root><Button class="sm:shrink-0" type="button" disabled={!selectedApplicationId || granting} aria-busy={granting} onclick={grantApplication}>{#if granting}<Spinner />{/if}Grant access</Button></div>
        {#if resource.applicationGrants.length}<div class="divide-y divide-border border border-border">{#each resource.applicationGrants as grant}<div class="flex flex-col justify-between gap-3 p-3 sm:flex-row sm:items-center"><p class="text-sm font-medium">{grant.applicationName}</p><Button class="self-start sm:self-auto" type="button" size="sm" variant="destructive" onclick={() => askToRevoke('Application', grant.applicationId, grant.applicationName)}>Revoke</Button></div>{/each}</div>{:else}<Empty.Root class="border border-dashed border-border py-8"><Empty.Header><Empty.Title>No Application grants</Empty.Title><Empty.Description>Grant access to make this Resource selectable by an Application.</Empty.Description></Empty.Header></Empty.Root>{/if}
      {/if}
    </Card.Content></Card.Root>
  </div>
  <ConfirmActionDialog bind:open={revokeDialogOpen} title={`Revoke ${revokeTarget?.scope ?? ''} access?`} description={`Remove ${revokeTarget?.name ?? 'this consumer'} from the selection policy for ${resource.name}. Existing connections are not changed.`} confirmLabel="Revoke access" destructive processing={revokeProcessing} error={revokeError} onconfirm={confirmRevoke} />
</DashboardLayout>
