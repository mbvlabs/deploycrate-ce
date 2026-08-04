<script lang="ts">
  import { Link, useForm } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import EnvironmentDeleteDialog from '@/Components/EnvironmentDeleteDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type ResourceInput = { resourceId: string; endpointId: string; credentialId?: string; alias: string; database: string; credentialProjection: 'connection_url' | 'individual_parts' }
  type ResourceOption = { id: string; name: string; engine: string; database: string; endpointId: string; endpoint: string; credentialId?: string; credential: string; serverId?: string; credentialFields: string[]; supportsConnectionUrl: boolean; environmentKeys: Record<string, string> }
  type DNSZone = { zoneId: string; zoneName: string; connectionId: string; connectionName: string }
  type Configuration = { name: string; slug: string; kind: string; hostname: string; containerPort: number; healthPath: string; bpGoTargets: string; resources: ResourceInput[]; serverIds: string[]; serverNames: string[]; dnsMode: 'manual' | 'cloudflare'; dnsZoneId?: string | null }
  type Environment = { applicationId: string; applicationName: string; sourceType: 'buildpacks' | 'image'; environment: { id: string; name: string; kind: string }; repository: string; reference: string; contextPath: string }

  let { auth, environment, configuration, options }: { auth: { email: string }; environment: Environment; configuration: Configuration; options: { resources: ResourceOption[]; dnsZones: DNSZone[] } } = $props()
  let selectedResource = $state('')
  const form = useForm(() => ({ ...configuration, dnsZoneId: configuration.dnsZoneId ?? '', resources: configuration.resources.map((resource) => ({ ...resource })) }))
  const availableResources = $derived(options.resources.filter((resource) => !resource.serverId || configuration.serverIds.includes(resource.serverId)))

  function attachedResourceOption(resource: ResourceInput) {
    return options.resources.find((option) => option.id === resource.resourceId && option.endpointId === resource.endpointId && option.credentialId === resource.credentialId)
  }

  function resourceAlias(engine: string) {
    return engine.toUpperCase().replace(/[^A-Z0-9]+/g, '_')
  }

  function addResource() {
    const option = availableResources.find((candidate) => `${candidate.id}:${candidate.endpointId}:${candidate.credentialId ?? ''}` === selectedResource)
    if (!option || $form.resources.some((resource) => resource.resourceId === option.id)) return
    $form.resources = [...$form.resources, {
      resourceId: option.id, endpointId: option.endpointId, credentialId: option.credentialId,
      alias: resourceAlias(option.engine), database: option.database, credentialProjection: option.supportsConnectionUrl ? 'connection_url' : 'individual_parts',
    }]
  }

  function resourceManagedKeys(resource: ResourceInput) {
    const option = attachedResourceOption(resource)
    if (!option) return []
    const logicalKeys = resource.credentialProjection === 'connection_url'
      ? ['url']
      : ['host', 'port', 'protocol', 'tls_mode', ...(resource.database ? ['database'] : []), ...(option.credentialId ? ['username', ...option.credentialFields] : [])]
    return logicalKeys.map((logicalKey) => option.environmentKeys[logicalKey]).filter(Boolean)
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    $form.patch(routes.environmentUpdate(environment.applicationId, environment.environment.id))
  }
</script>

<svelte:head><title>Edit {environment.environment.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-4xl space-y-8" onsubmit={submit}>
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environment.applicationName} · Environment</p><h1 class="mt-3 text-3xl font-semibold">Edit {environment.environment.name}</h1><p class="mt-2 text-sm text-muted-foreground">Saving updates desired state. Deploy explicitly when you are ready.</p></div>
      <div class="flex flex-wrap gap-2"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(environment.applicationId, environment.environment.id)}>Edit source and registry</Link>{/snippet}</Button><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentShow(environment.applicationId, environment.environment.id)}>Cancel</Link>{/snippet}</Button><EnvironmentDeleteDialog applicationId={environment.applicationId} environmentId={environment.environment.id} environmentName={environment.environment.name} /></div>
    </header>

    <Card.Root>
      <Card.Header><Card.Title>Identity</Card.Title><Card.Description>Environment-owned naming and classification.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-3">
        <FormField label="Name" error={$form.errors.name}><Input bind:value={$form.name} required /></FormField>
        <FormField label="Slug" error={$form.errors.slug}><Input bind:value={$form.slug} required /></FormField>
        <FormField label="Kind" error={$form.errors.kind}><Input bind:value={$form.kind} placeholder="production" required /></FormField>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Domain</Card.Title><Card.Description>Edit the Environment's primary public domain. HTTPS and Caddy routing are managed from this value.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="Domain hostname" error={$form.errors.hostname}><Input bind:value={$form.hostname} placeholder="app.example.com" required /></FormField>
        <FormField label="DNS management"><NativeSelect.Root bind:value={$form.dnsMode} class="w-full"><NativeSelect.Option value="manual">Manual DNS</NativeSelect.Option><NativeSelect.Option value="cloudflare" disabled={(options.dnsZones ?? []).length === 0}>Cloudflare managed</NativeSelect.Option></NativeSelect.Root></FormField>
        {#if $form.dnsMode === 'cloudflare'}<FormField label="Cloudflare zone" error={$form.errors.dnsZoneId}><NativeSelect.Root bind:value={$form.dnsZoneId} class="w-full" required><NativeSelect.Option value="">Select a zone</NativeSelect.Option>{#each options.dnsZones ?? [] as zone}<NativeSelect.Option value={zone.zoneId}>{zone.zoneName} · {zone.connectionName}</NativeSelect.Option>{/each}</NativeSelect.Root><p class="mt-2 text-xs text-muted-foreground">Hostname and zone changes stay staged until the next explicit deployment.</p></FormField>{/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Runtime</Card.Title><Card.Description>Edit the Environment runtime values.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <div class="sm:col-span-2"><FormField label="Runtime Server targets"><Input value={configuration.serverNames.join(', ')} readonly /></FormField><p class="mt-2 text-xs text-muted-foreground">Runtime placement is fixed after setup. Create a new Environment to change its Server targets.</p></div>
        <FormField label="Container port" error={$form.errors.containerPort}><Input type="number" min="1" max="65535" bind:value={$form.containerPort} required /></FormField>
        <FormField label="HTTP health path" error={$form.errors.healthPath}><Input bind:value={$form.healthPath} placeholder="/health" /></FormField>
        {#if environment.sourceType === 'buildpacks'}<FormField label="Target" error={$form.errors.bpGoTargets}><Input bind:value={$form.bpGoTargets} placeholder="./cmd/app" /></FormField>{/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Resources</Card.Title><Card.Description>Replace active Resource connections. Injected key names are managed from each Resource.</Card.Description></Card.Header>
      <Card.Content class="space-y-4">
        <div class="flex flex-col gap-2 sm:flex-row"><NativeSelect.Root bind:value={selectedResource} class="w-full flex-1"><NativeSelect.Option value="">Select a Resource</NativeSelect.Option>{#each availableResources as option}<NativeSelect.Option value={`${option.id}:${option.endpointId}:${option.credentialId ?? ''}`}>{option.name} · {option.engine}{option.database ? ` · ${option.database}` : ''} · {option.endpoint} · {option.credential || 'without credentials'}</NativeSelect.Option>{/each}</NativeSelect.Root><Button type="button" variant="outline" disabled={!selectedResource} onclick={addResource}>Attach</Button></div>
        {#each $form.resources as resource, index}
          <div class="grid gap-3 border border-border p-4 sm:grid-cols-2">
            <FormField label="Connection alias" error={$form.errors[`resources.${index}.alias`]}><Input bind:value={resource.alias} /></FormField>
            {#if resource.database}<FormField label="Database"><Input bind:value={resource.database} readonly /></FormField>{/if}
            {#if attachedResourceOption(resource)?.supportsConnectionUrl}<FormField label="Connection format" error={$form.errors[`resources.${index}.credentialProjection`]}><NativeSelect.Root bind:value={resource.credentialProjection} class="w-full"><NativeSelect.Option value="connection_url">Connection URL</NativeSelect.Option><NativeSelect.Option value="individual_parts">Individual parts</NativeSelect.Option></NativeSelect.Root></FormField>{/if}
            <div class="border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground"><span class="font-medium text-foreground">Resource-managed keys</span><span class="mt-1 block font-mono">{resourceManagedKeys(resource).join(', ') || 'No values projected'}</span></div>
            <Button type="button" variant="ghost" onclick={() => $form.resources = $form.resources.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button>
          </div>
        {:else}<p class="text-sm text-muted-foreground">No Resources attached.</p>{/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Source and secrets</Card.Title><Card.Description>Both remain fully editable from this Environment workflow.</Card.Description></Card.Header>
      <Card.Content class="grid gap-4 sm:grid-cols-2"><div class="border border-border p-4"><p class="font-medium">GitHub and Buildpacks</p><p class="mt-1 text-sm text-muted-foreground">{environment.repository} at {environment.reference}, context {environment.contextPath}</p><Button class="mt-4" type="button" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(environment.applicationId, environment.environment.id)}>Edit source</Link>{/snippet}</Button></div><div class="border border-border p-4"><p class="font-medium">Environment secrets</p><p class="mt-1 text-sm text-muted-foreground">Add, rotate, and delete write-only values from the Environment page.</p><Button class="mt-4" type="button" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentShow(environment.applicationId, environment.environment.id)}>Manage secrets</Link>{/snippet}</Button></div></Card.Content>
    </Card.Root>

    <div class="flex justify-end"><Button type="submit" disabled={$form.processing} aria-busy={$form.processing}>{#if $form.processing}<Spinner />{/if}Save changes</Button></div>
  </form>
</DashboardLayout>
