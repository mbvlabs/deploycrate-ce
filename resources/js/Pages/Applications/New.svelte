<script lang="ts">
  import { router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import BulkEnvironmentSecretsDialog from '@/Components/BulkEnvironmentSecretsDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import { Checkbox } from '@/Components/ui/checkbox'
  import * as NativeSelect from '@/Components/ui/native-select'
  import * as RadioGroup from '@/Components/ui/radio-group'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { slugify } from '@/lib/slug'
  import { routes } from '@/routes'

  type Installation = { id: string; accountLogin: string }
  type Repository = { id: string; githubInstallationId: string; fullName: string; defaultBranch: string }
  type Registry = { id: string; name: string; endpoint: string }
  type Server = { id: string; name: string; kind: string; address: string }
  type ResourceOption = { id: string; name: string; engine: string; database: string; endpointId: string; endpoint: string; credentialId?: string; credential: string; serverId?: string; credentialFields: string[]; supportsConnectionUrl: boolean }
  type EnvironmentResource = { resourceId: string; endpointId: string; credentialId?: string; alias: string; database: string; credentialProjection: 'connection_url' | 'individual_parts' }
  type EnvironmentSecret = { key: string; value: string }
  type EnvironmentForm = {
    kind: 'staging' | 'production'
    sourceType: 'buildpacks' | 'image'
    githubInstallationId: string
    githubRepositoryId: string
    reference: string
    autoBuild: boolean
    buildFrontendAssets: boolean
    contextPath: string
    registryResourceId: string
    imageRepository: string
    buildServerId: string
    serverIds: string[]
    hostname: string
    containerPort: number
    healthPath: string
    bpGoTargets: string
    resources: EnvironmentResource[]
    secrets: EnvironmentSecret[]
    deploy: boolean
    selectedResource: string
  }
  type Options = { installations: Installation[]; repositories: Repository[]; registries: Registry[]; buildServers: Server[]; servers: Server[]; resources: ResourceOption[] }

  let { auth, options, errors = {} }: { auth: { email: string }; options: Options; errors?: Record<string, string> } = $props()
  const installations = $derived(options.installations ?? [])
  const repositories = $derived(options.repositories ?? [])
  const registries = $derived(options.registries ?? [])
  const buildServers = $derived(options.buildServers ?? [])
  const servers = $derived(options.servers ?? [])
  const resources = $derived(options.resources ?? [])
  let applicationName = $state('')
  let applicationSlug = $state('')
  let slugCustomized = $state(false)
  let includeStaging = $state(false)
  let processing = $state(false)
  let responseErrors = $state<Record<string, string>>({})
  const displayedErrors = $derived(Object.keys(responseErrors).length > 0 ? responseErrors : errors)
  let staging = $state(initialEnvironment('staging'))
  let production = $state(initialEnvironment('production'))
  let bulkSecretDialogOpen = $state(false)
  let bulkSecretEnvironment = $state<EnvironmentForm | null>(null)

  function initialEnvironment(kind: 'staging' | 'production'): EnvironmentForm {
    return {
      kind,
      sourceType: 'buildpacks',
      githubInstallationId: installations[0]?.id ?? '',
      githubRepositoryId: '',
      reference: '',
      autoBuild: false,
      buildFrontendAssets: false,
      contextPath: '.',
      registryResourceId: registries[0]?.id ?? '',
      imageRepository: '',
      buildServerId: buildServers[0]?.id ?? '',
      serverIds: [],
      hostname: '',
      containerPort: 8080,
      healthPath: '/health',
      bpGoTargets: '',
      resources: [],
      secrets: [],
      deploy: false,
      selectedResource: '',
    }
  }

  function updateApplicationName(value: string) {
    applicationName = value
    if (!slugCustomized) applicationSlug = slugify(value)
  }

  function updateApplicationSlug(value: string) {
    applicationSlug = value
    slugCustomized = true
  }

  function repositoryOptions(environment: EnvironmentForm) {
    return repositories.filter((repository) => repository.githubInstallationId === environment.githubInstallationId)
  }

  function chooseInstallation(environment: EnvironmentForm, installationId: string) {
    environment.githubInstallationId = installationId
    environment.githubRepositoryId = ''
  }

  function chooseRepository(environment: EnvironmentForm, repositoryId: string) {
    environment.githubRepositoryId = repositoryId
    const repository = repositories.find((candidate) => candidate.id === repositoryId)
    if (repository && environment.reference.trim() === '') environment.reference = repository.defaultBranch
  }

  function availableResources(environment: EnvironmentForm) {
    return resources.filter((resource) => !resource.serverId || environment.serverIds.includes(resource.serverId))
  }

  function attachedResourceOption(resource: EnvironmentResource) {
    return resources.find((option) => option.id === resource.resourceId && option.endpointId === resource.endpointId && option.credentialId === resource.credentialId)
  }

  function resourceAlias(engine: string) {
    return engine.toUpperCase().replace(/[^A-Z0-9]+/g, '_')
  }

  function toggleRuntimeServer(environment: EnvironmentForm, serverId: string, selected: boolean) {
    environment.serverIds = selected
      ? [...new Set([...environment.serverIds, serverId])]
      : environment.serverIds.filter((candidate) => candidate !== serverId)
    const available = new Set(availableResources(environment).map((resource) => resource.id))
    environment.resources = environment.resources.filter((resource) => available.has(resource.resourceId))
    environment.selectedResource = ''
  }

  function addResource(environment: EnvironmentForm) {
    const option = availableResources(environment).find((candidate) => `${candidate.id}:${candidate.endpointId}:${candidate.credentialId ?? ''}` === environment.selectedResource)
    if (!option || environment.resources.some((resource) => resource.resourceId === option.id)) return
    environment.resources = [...environment.resources, { resourceId: option.id, endpointId: option.endpointId, credentialId: option.credentialId, alias: resourceAlias(option.engine), database: option.database, credentialProjection: option.supportsConnectionUrl ? 'connection_url' : 'individual_parts' }]
    environment.selectedResource = ''
  }

  function addSecret(environment: EnvironmentForm) {
    environment.secrets = [...environment.secrets, { key: '', value: '' }]
  }

  function openBulkSecrets(environment: EnvironmentForm) {
    bulkSecretEnvironment = environment
    bulkSecretDialogOpen = true
  }

  function reservedSecretKeys(environment: EnvironmentForm) {
    const keys = ['PORT']
    for (const resource of environment.resources) {
      const option = attachedResourceOption(resource)
      const alias = resource.alias.trim().toUpperCase() || resourceAlias(option?.engine ?? 'RESOURCE')
      const suffixes = resource.credentialProjection === 'connection_url'
        ? ['URL']
        : ['HOST', 'PORT', 'PROTOCOL', 'TLS_MODE', ...(resource.database ? ['DATABASE'] : []), ...(option?.credentialId ? ['USER', ...(option.credentialFields ?? []).map((field) => field.toUpperCase())] : [])]
      keys.push(...suffixes.map((suffix) => `${alias}_${suffix}`))
    }
    return keys
  }

  function importBulkSecrets(secrets: EnvironmentSecret[]) {
    if (!bulkSecretEnvironment) return
    bulkSecretEnvironment.secrets = [...bulkSecretEnvironment.secrets, ...secrets]
  }

  function environmentPayload(environment: EnvironmentForm) {
    return {
      sourceType: environment.sourceType,
      githubInstallationId: environment.githubInstallationId,
      githubRepositoryId: environment.githubRepositoryId,
      reference: environment.reference,
      autoBuild: environment.autoBuild,
      contextPath: environment.contextPath,
      builderReference: '',
      buildpackSettings: { schema_version: 2, frontend: environment.buildFrontendAssets ? { runtime: 'node', script: 'build' } : null },
      registryResourceId: environment.registryResourceId,
      imageRepository: environment.imageRepository,
      buildServerId: environment.buildServerId,
      serverIds: environment.serverIds,
      hostname: environment.hostname,
      containerPort: environment.containerPort,
      healthPath: environment.healthPath,
      bpGoTargets: environment.bpGoTargets,
      resources: environment.resources,
      secrets: environment.secrets,
      deploy: environment.deploy,
    }
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    processing = true
    responseErrors = {}
    router.post(routes.applicationCreate(), {
      applicationName,
      applicationSlug,
      staging: includeStaging ? environmentPayload(staging) : null,
      production: environmentPayload(production),
    }, {
      onError: (validationErrors) => { responseErrors = validationErrors },
      onFinish: () => { processing = false },
    })
  }
</script>

{#snippet environmentCard(title: string, environment: EnvironmentForm, enabled: boolean)}
  <Card.Root>
    <Card.Header>
      <Card.Action>{#if environment.kind === 'staging'}<label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={includeStaging} /> Include Staging</label>{:else}<span class="text-[10px] font-medium uppercase tracking-[0.2em] text-muted-foreground">Required</span>{/if}</Card.Action>
      <Card.Title>{title}</Card.Title>
      <Card.Description>{enabled ? `Choose how this Environment receives its application and where it runs.` : 'Optional. Enable Staging to configure it independently from Production.'}</Card.Description>
    </Card.Header>
    {#if enabled}
    <Card.Content class="space-y-6">
      <section class="space-y-3">
        <div><p class="text-sm font-medium">Delivery method</p><p class="mt-1 text-xs text-muted-foreground">This choice belongs only to the {title} Environment.</p></div>
        <RadioGroup.Root value={environment.sourceType} onValueChange={(source) => environment.sourceType = source as 'buildpacks' | 'image'} class="grid gap-3 sm:grid-cols-2">
          <label class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${environment.sourceType === 'buildpacks' ? 'border-primary bg-primary/5' : 'border-border'}`}>
            <RadioGroup.Item class="mt-1" value="buildpacks" />
            <span><span class="block text-sm font-medium">Buildpack</span><span class="mt-1 block text-xs leading-5 text-muted-foreground">Build source from GitHub and publish the resulting image.</span></span>
          </label>
          <label class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${environment.sourceType === 'image' ? 'border-primary bg-primary/5' : 'border-border'}`}>
            <RadioGroup.Item class="mt-1" value="image" />
            <span><span class="block text-sm font-medium">Repository</span><span class="mt-1 block text-xs leading-5 text-muted-foreground">Deploy directly from an existing container repository.</span></span>
          </label>
        </RadioGroup.Root>
      </section>

      {#if environment.sourceType === 'buildpacks'}
        <section class="grid gap-5 border-t border-border pt-5 sm:grid-cols-2">
          <FormField label="GitHub account"><NativeSelect.Root value={environment.githubInstallationId} onchange={(event) => chooseInstallation(environment, event.currentTarget.value)} class="w-full" required><NativeSelect.Option value="">Select an account</NativeSelect.Option>{#each installations as installation}<NativeSelect.Option value={installation.id}>{installation.accountLogin}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Source repository"><NativeSelect.Root value={environment.githubRepositoryId} onchange={(event) => chooseRepository(environment, event.currentTarget.value)} class="w-full" required><NativeSelect.Option value="">Select a repository</NativeSelect.Option>{#each repositoryOptions(environment) as repository}<NativeSelect.Option value={repository.id}>{repository.fullName}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Git reference"><Input value={environment.reference} oninput={(event) => environment.reference = event.currentTarget.value} placeholder={environment.kind === 'production' ? 'main' : 'staging'} required /></FormField>
          <FormField label="Build server"><NativeSelect.Root value={environment.buildServerId} onchange={(event) => environment.buildServerId = event.currentTarget.value} class="w-full" required><NativeSelect.Option value="">Select a Build Server</NativeSelect.Option>{#each buildServers as server}<NativeSelect.Option value={server.id}>{server.name} · {server.kind === 'worker' ? server.address : 'Control plane'}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Build context"><Input value={environment.contextPath} oninput={(event) => environment.contextPath = event.currentTarget.value} placeholder="." required /></FormField>
          <FormField label="Image destination"><Input value={environment.imageRepository} oninput={(event) => environment.imageRepository = event.currentTarget.value} placeholder={`team/application-${environment.kind}`} required /></FormField>
          <FormField label="Registry"><NativeSelect.Root value={environment.registryResourceId} onchange={(event) => environment.registryResourceId = event.currentTarget.value} class="w-full" required><NativeSelect.Option value="">Select a Registry</NativeSelect.Option>{#each registries as registry}<NativeSelect.Option value={registry.id}>{registry.name} · {registry.endpoint}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <label class="flex items-center gap-2 self-end pb-2 text-sm"><Checkbox checked={environment.autoBuild} onCheckedChange={(selected) => environment.autoBuild = selected} /> Build automatically on matching pushes</label>
          <label class="flex items-start gap-3 border border-border p-4 sm:col-span-2"><Checkbox class="mt-1" checked={environment.buildFrontendAssets} onCheckedChange={(selected) => environment.buildFrontendAssets = selected} /><span><span class="block text-sm font-medium">Build Node frontend assets</span><span class="mt-1 block text-xs text-muted-foreground">Requires a supported lockfile and build script.</span></span></label>
        </section>
      {:else}
        <section class="grid gap-5 border-t border-border pt-5 sm:grid-cols-2">
          <FormField label="Registry"><NativeSelect.Root value={environment.registryResourceId} onchange={(event) => environment.registryResourceId = event.currentTarget.value} class="w-full" required><NativeSelect.Option value="">Select a Registry</NativeSelect.Option>{#each registries as registry}<NativeSelect.Option value={registry.id}>{registry.name} · {registry.endpoint}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField>
          <FormField label="Repository"><Input value={environment.imageRepository} oninput={(event) => environment.imageRepository = event.currentTarget.value} placeholder="team/application" required /></FormField>
          <FormField label="Tag or digest"><Input value={environment.reference} oninput={(event) => environment.reference = event.currentTarget.value} placeholder="stable" required /></FormField>
        </section>
      {/if}

      <section class="space-y-5 border-t border-border pt-5">
        <p class="text-sm font-medium">Target servers</p>
        {#if servers.length === 0}
          <div class="flex items-center justify-between gap-4 border border-border p-4"><p class="text-sm text-muted-foreground">No runtime target servers are available.</p><Button type="button" href={routes.nodeNew()} variant="outline">Add node</Button></div>
        {:else}
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {#each servers as server}
              <label class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${environment.serverIds.includes(server.id) ? 'border-primary bg-primary/5' : 'border-border'}`}>
                <Checkbox class="mt-1" checked={environment.serverIds.includes(server.id)} onCheckedChange={(selected) => toggleRuntimeServer(environment, server.id, selected)} />
                <span><span class="block text-sm font-medium">{server.name}</span><span class="mt-1 block text-xs text-muted-foreground">{server.kind === 'worker' ? server.address : 'Control plane'}</span></span>
              </label>
            {/each}
          </div>
        {/if}
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Domain"><Input value={environment.hostname} oninput={(event) => environment.hostname = event.currentTarget.value} placeholder={`${environment.kind}.example.com`} required /></FormField>
          <FormField label="Container port"><Input type="number" min="1" max="65535" value={environment.containerPort} oninput={(event) => environment.containerPort = Number(event.currentTarget.value)} required /></FormField>
          <FormField label="Health path"><Input value={environment.healthPath} oninput={(event) => environment.healthPath = event.currentTarget.value} placeholder="/health" /></FormField>
          {#if environment.sourceType === 'buildpacks'}<FormField label="Target"><Input value={environment.bpGoTargets} oninput={(event) => environment.bpGoTargets = event.currentTarget.value} placeholder="./cmd/server" /></FormField>{/if}
        </div>
      </section>

      <section class="space-y-3 border-t border-border pt-5">
        <p class="text-sm font-medium">Resources</p>
        <div class="flex flex-col gap-2 sm:flex-row"><NativeSelect.Root value={environment.selectedResource} onchange={(event) => environment.selectedResource = event.currentTarget.value} class="w-full flex-1"><NativeSelect.Option value="">Select a Resource</NativeSelect.Option>{#each availableResources(environment) as option}<NativeSelect.Option value={`${option.id}:${option.endpointId}:${option.credentialId ?? ''}`}>{option.name} · {option.engine}{option.database ? ` · ${option.database}` : ''} · {option.endpoint} · {option.credential || 'without credentials'}</NativeSelect.Option>{/each}</NativeSelect.Root><Button type="button" variant="outline" disabled={!environment.selectedResource} onclick={() => addResource(environment)}>Attach</Button></div>
        {#each environment.resources as resource, index}
          <div class="grid gap-3 border border-border p-4 sm:grid-cols-2"><FormField label="Alias"><Input bind:value={resource.alias} /></FormField>{#if resource.database}<FormField label="Database"><Input bind:value={resource.database} readonly /></FormField>{/if}{#if attachedResourceOption(resource)?.supportsConnectionUrl}<FormField label="Connection format"><NativeSelect.Root bind:value={resource.credentialProjection} class="w-full"><NativeSelect.Option value="connection_url">Connection URL</NativeSelect.Option><NativeSelect.Option value="individual_parts">Individual parts</NativeSelect.Option></NativeSelect.Root></FormField>{/if}<Button type="button" variant="ghost" onclick={() => environment.resources = environment.resources.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button></div>
        {:else}<p class="text-xs text-muted-foreground">No Resources attached.</p>{/each}
      </section>

      <section class="space-y-3 border-t border-border pt-5">
        <div class="flex items-start justify-between gap-3"><div><p class="text-sm font-medium">Secrets</p><p class="mt-1 text-xs text-muted-foreground">Add the write-only values required by this Environment.</p></div><div class="flex gap-2"><Button type="button" variant="outline" onclick={() => openBulkSecrets(environment)}>Import secrets</Button><Button type="button" variant="outline" onclick={() => addSecret(environment)}>Add secret</Button></div></div>
        {#each environment.secrets as secret, index}
          <div class="grid gap-3 border border-border p-4 sm:grid-cols-2"><FormField label="Key"><Input bind:value={secret.key} autocomplete="off" /></FormField><FormField label="Value"><Input type="password" bind:value={secret.value} autocomplete="new-password" /></FormField><Button type="button" variant="ghost" onclick={() => environment.secrets = environment.secrets.filter((_, itemIndex) => itemIndex !== index)}>Remove</Button></div>
        {:else}<p class="text-xs text-muted-foreground">No secrets added.</p>{/each}
      </section>

      <label class="flex items-center gap-3 border border-border bg-muted/20 p-4"><Checkbox checked={environment.deploy} onCheckedChange={(selected) => environment.deploy = selected} /><span class="text-sm font-medium">Deploy after create</span></label>
    </Card.Content>
    {/if}
  </Card.Root>
{/snippet}

<svelte:head><title>New application</title></svelte:head>
<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-5xl space-y-6" onsubmit={submit}>
    <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Applications</p><h1 class="mt-3 text-3xl font-semibold">Set up a new application</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Configure the application, optional Staging Environment, and required Production Environment on one page.</p></header>

    {#if Object.keys(displayedErrors).length > 0}<div class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"><p class="font-medium">The application could not be created.</p>{#each [...new Set(Object.values(displayedErrors))] as error}<p class="mt-1">{error}</p>{/each}</div>{/if}
    {#if registries.length === 0}<div class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">Connect a Registry Resource before creating an application.</div>{/if}

    <Card.Root>
      <Card.Header><Card.Title>Application</Card.Title><Card.Description>The slug follows the name until you customize it.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Name" error={displayedErrors.applicationName}><Input value={applicationName} oninput={(event) => updateApplicationName(event.currentTarget.value)} placeholder="Acme API" required autofocus /></FormField><FormField label="Slug" error={displayedErrors.applicationSlug}><Input value={applicationSlug} oninput={(event) => updateApplicationSlug(event.currentTarget.value)} placeholder="acme-api" required /></FormField></Card.Content>
    </Card.Root>

    {@render environmentCard('Staging', staging, includeStaging)}
    {@render environmentCard('Production', production, true)}

    <div class="flex flex-col gap-3 border-t border-border pt-5 sm:flex-row sm:items-center sm:justify-between"><p class="text-xs text-muted-foreground">Nothing deploys unless selected in its Environment.</p><div class="flex flex-col-reverse gap-3 sm:flex-row"><Button href={routes.applications()} variant="outline">Cancel</Button><Button type="submit" disabled={processing || registries.length === 0 || production.serverIds.length === 0 || (includeStaging && staging.serverIds.length === 0)} aria-busy={processing}>{#if processing}<Spinner />{/if}Create application</Button></div></div>
  </form>

  <BulkEnvironmentSecretsDialog
    bind:open={bulkSecretDialogOpen}
    existingSecrets={bulkSecretEnvironment?.secrets ?? []}
    reservedKeys={bulkSecretEnvironment ? reservedSecretKeys(bulkSecretEnvironment) : []}
    onImport={importBulkSecrets}
  />
</DashboardLayout>
