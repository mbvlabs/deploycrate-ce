<script lang="ts">
  import { Link, useForm } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import EnvironmentDeleteDialog from '@/Components/EnvironmentDeleteDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type ResourceInput = { resourceId: string; endpointId: string; credentialId?: string; alias: string; database: string; credentialProjection: 'connection_url' | 'individual_parts' }
  type ResourceOption = { id: string; name: string; engine: string; database: string; endpointId: string; endpoint: string; credentialId?: string; credential: string; serverId?: string }
  type Configuration = { name: string; slug: string; kind: string; hostname: string; containerPort: number; healthPath: string; bpGoTargets: string; resources: ResourceInput[]; serverId: string; serverName: string }
  type Environment = { applicationId: string; applicationName: string; environment: { id: string; name: string; kind: string }; repository: string; reference: string; contextPath: string }

  let { auth, environment, configuration, options }: { auth: { email: string }; environment: Environment; configuration: Configuration; options: { resources: ResourceOption[] } } = $props()
  let selectedResource = $state('')
  const form = useForm(() => ({ ...configuration, resources: configuration.resources.map((resource) => ({ ...resource })) }))
  const availableResources = $derived(options.resources.filter((resource) => !resource.serverId || resource.serverId === configuration.serverId))

  function addResource() {
    const option = availableResources.find((candidate) => `${candidate.id}:${candidate.endpointId}:${candidate.credentialId ?? ''}` === selectedResource)
    if (!option || $form.resources.some((resource) => resource.resourceId === option.id)) return
    $form.resources = [...$form.resources, {
      resourceId: option.id, endpointId: option.endpointId, credentialId: option.credentialId,
      alias: 'DATABASE', database: option.database, credentialProjection: 'connection_url',
    }]
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
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environment.applicationName} · Environment</p><h1 class="mt-3 text-3xl font-semibold">Edit {environment.environment.name}</h1><p class="mt-2 text-sm text-muted-foreground">Saving creates a new desired-state revision and queues a replacement Build.</p></div>
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
      <Card.Content>
        <FormField label="Domain hostname" error={$form.errors.hostname}><Input bind:value={$form.hostname} placeholder="app.example.com" required /></FormField>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Runtime</Card.Title><Card.Description>Edit every user-controlled Go Buildpacks runtime value.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <div class="sm:col-span-2"><FormField label="Runtime Server"><Input value={configuration.serverName} readonly /></FormField><p class="mt-2 text-xs text-muted-foreground">Runtime placement is fixed after setup. Create a new Environment to move workloads safely between Servers.</p></div>
        <FormField label="Container port" error={$form.errors.containerPort}><Input type="number" min="1" max="65535" bind:value={$form.containerPort} required /></FormField>
        <FormField label="HTTP health path" error={$form.errors.healthPath}><Input bind:value={$form.healthPath} placeholder="/health" /></FormField>
        <FormField label="BP_GO_TARGETS" error={$form.errors.bpGoTargets}><Input bind:value={$form.bpGoTargets} placeholder="./cmd/app" /></FormField>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Resources</Card.Title><Card.Description>Replace the active Resource connections and their managed Environment variables.</Card.Description></Card.Header>
      <Card.Content class="space-y-4">
        <div class="flex gap-2"><select bind:value={selectedResource} class="h-9 flex-1 border border-input bg-background px-3 text-sm"><option value="">Select a PostgreSQL Resource</option>{#each availableResources as option}<option value={`${option.id}:${option.endpointId}:${option.credentialId ?? ''}`}>{option.name} · {option.database} · {option.endpoint} · {option.credential || 'No credential'}</option>{/each}</select><Button type="button" variant="outline" onclick={addResource}>Attach</Button></div>
        {#each $form.resources as resource, index}
          <div class="grid gap-3 border border-border p-4 sm:grid-cols-2">
            <FormField label="Alias" error={$form.errors[`resources.${index}.alias`]}><Input bind:value={resource.alias} /></FormField>
            <FormField label="Database"><Input bind:value={resource.database} readonly /></FormField>
            <FormField label="Connection format" error={$form.errors[`resources.${index}.credentialProjection`]}><select bind:value={resource.credentialProjection} class="h-9 w-full border border-input bg-background px-3 text-sm"><option value="connection_url">Connection URL</option><option value="individual_parts">Individual parts</option></select></FormField>
            <div class="border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">{resource.credentialProjection === 'connection_url' ? `${resource.alias.trim().toUpperCase() || 'DATABASE'}_URL` : ['HOST', 'PORT', 'USER', 'PASSWORD', 'TLS_MODE'].map((suffix) => `${resource.alias.trim().toUpperCase() || 'DATABASE'}_${suffix}`).join(', ')}</div>
            <Button type="button" variant="ghost" onclick={() => $form.resources = $form.resources.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button>
          </div>
        {:else}<p class="text-sm text-muted-foreground">No Resources attached.</p>{/each}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Source and secrets</Card.Title><Card.Description>Both remain fully editable from this Environment workflow.</Card.Description></Card.Header>
      <Card.Content class="grid gap-4 sm:grid-cols-2"><div class="border border-border p-4"><p class="font-medium">GitHub and Buildpacks</p><p class="mt-1 text-sm text-muted-foreground">{environment.repository} at {environment.reference}, context {environment.contextPath}</p><Button class="mt-4" type="button" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(environment.applicationId, environment.environment.id)}>Edit source</Link>{/snippet}</Button></div><div class="border border-border p-4"><p class="font-medium">Environment secrets</p><p class="mt-1 text-sm text-muted-foreground">Add, rotate, and delete write-only values from the Environment page.</p><Button class="mt-4" type="button" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentShow(environment.applicationId, environment.environment.id)}>Manage secrets</Link>{/snippet}</Button></div></Card.Content>
    </Card.Root>

    <div class="flex justify-end"><Button type="submit" disabled={$form.processing}>{$form.processing ? 'Saving...' : 'Save and deploy'}</Button></div>
  </form>
</DashboardLayout>
