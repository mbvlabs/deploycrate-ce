<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Installation = { id: string; accountLogin: string }
  type Repository = { id: string; githubInstallationId: string; fullName: string; defaultBranch: string }
  type Registry = { id: string; name: string; endpoint: string }
  type BuildServer = { id: string; name: string; kind: string; address: string }
  type FrontendSettings = { runtime: 'node'; script: 'build' }
  let { auth, options, environmentIntent = false }: { auth: { email: string }; options: { installations: Installation[]; repositories: Repository[]; registries: Registry[]; buildServers: BuildServer[] }; environmentIntent?: boolean } = $props()
  const installations = $derived(options.installations ?? [])
  const repositoryOptions = $derived(options.repositories ?? [])
  const registries = $derived(options.registries ?? [])
  const buildServers = $derived(options.buildServers ?? [])
  let step = $state(1)
  let buildFrontendAssets = $state(false)
	const form = useForm(() => ({ sourceType: 'buildpacks', applicationName: '', applicationSlug: '', environmentName: 'Production', environmentSlug: 'production', environmentKind: 'production', githubInstallationId: installations[0]?.id ?? '', githubRepositoryId: '', reference: '', autoBuild: true, contextPath: '.', builderReference: '', buildpackSettings: { schema_version: 2, frontend: null as FrontendSettings | null }, registryResourceId: registries[0]?.id ?? '', imageRepository: '', buildServerId: buildServers[0]?.id ?? '' }))
  const repositories = $derived(repositoryOptions.filter((repository) => repository.githubInstallationId === $form.githubInstallationId))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    $form.buildpackSettings.frontend = buildFrontendAssets ? { runtime: 'node', script: 'build' } : null
    $form.post(environmentIntent ? routes.environmentCreate() : routes.applicationCreate())
  }
</script>

<svelte:head><title>{environmentIntent ? 'Create environment' : 'New application'}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environmentIntent ? 'Environments' : 'Applications'} · Step {step} of 4</p><h1 class="mt-3 text-3xl font-semibold">{environmentIntent ? 'Create a deployable Environment' : 'Configure a build-ready application'}</h1>{#if environmentIntent}<p class="mt-2 max-w-2xl text-sm text-muted-foreground">The Environment is created with its owning Application, GitHub source, Buildpacks configuration, and image destination.</p>{/if}</header>
    {#if registries.length === 0}
      <Card.Root><Card.Header><Card.Title>Registry connection required</Card.Title><Card.Description>Connect Docker Hub or another Registry Resource before creating an Environment.</Card.Description></Card.Header></Card.Root>
    {:else}
      <form onsubmit={submit}>
        <Card.Root>
          <Card.Header><Card.Title>{['Application', 'Source', $form.sourceType === 'image' ? 'Image' : 'Buildpacks', $form.sourceType === 'image' ? 'Deployment source' : 'Image destination'][step - 1]}</Card.Title></Card.Header>
          <Card.Content class="grid gap-5 sm:grid-cols-2">
            {#if step === 1}
              <FormField label="Application name"><Input bind:value={$form.applicationName} required /></FormField>
              <FormField label="Application slug"><Input bind:value={$form.applicationSlug} required /></FormField>
              <FormField label="Environment name"><Input bind:value={$form.environmentName} required /></FormField>
              <FormField label="Environment slug"><Input bind:value={$form.environmentSlug} required /></FormField>
              <FormField label="Environment kind"><Input bind:value={$form.environmentKind} required /></FormField>
            {:else if step === 2}
              <FormField label="Deployment source"><select bind:value={$form.sourceType} class="h-9 w-full border border-input bg-background px-3 text-sm"><option value="buildpacks">GitHub with Buildpacks</option><option value="image">Registry image</option></select></FormField>
              {#if $form.sourceType === 'buildpacks'}
                <FormField label="GitHub account"><select bind:value={$form.githubInstallationId} class="h-9 w-full border border-input bg-background px-3 text-sm">{#each installations as installation}<option value={installation.id}>{installation.accountLogin}</option>{/each}</select></FormField>
                <FormField label="Repository"><select bind:value={$form.githubRepositoryId} class="h-9 w-full border border-input bg-background px-3 text-sm" required><option value="">Select a repository</option>{#each repositories as repository}<option value={repository.id}>{repository.fullName}</option>{/each}</select></FormField>
                <FormField label="Branch or full ref"><Input bind:value={$form.reference} placeholder="main" required /></FormField>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={$form.autoBuild} /> Build automatically on matching pushes</label>
                {#if installations.length === 0}<p class="text-xs text-destructive sm:col-span-2">Connect GitHub before selecting the Buildpacks source.</p>{/if}
              {:else}
                <FormField label="Registry Resource"><select bind:value={$form.registryResourceId} class="h-9 w-full border border-input bg-background px-3 text-sm">{#each registries as registry}<option value={registry.id}>{registry.name} · {registry.endpoint}</option>{/each}</select></FormField>
                <FormField label="Image repository"><Input bind:value={$form.imageRepository} placeholder="team/application" required /></FormField>
                <FormField label="Default tag or digest"><Input bind:value={$form.reference} placeholder="latest" required /></FormField>
              {/if}
            {:else if step === 3}
              {#if $form.sourceType === 'buildpacks'}
                <FormField label="Build Server"><select bind:value={$form.buildServerId} class="h-9 w-full border border-input bg-background px-3 text-sm" required>{#each buildServers as server}<option value={server.id}>{server.name} · {server.kind === 'worker' ? server.address : 'Control plane'}</option>{/each}</select></FormField>
                <FormField label="Build context"><Input bind:value={$form.contextPath} placeholder="." required /></FormField>
                <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Builder</p><p class="mt-1 text-sm">Paketo Ubuntu Noble</p><p class="mt-1 text-xs text-muted-foreground">Managed and digest-pinned by DeployCrate for this server.</p></div>
                <label class="flex gap-3 border border-border p-4 sm:col-span-2"><input class="mt-1" type="checkbox" bind:checked={buildFrontendAssets} /><span><span class="font-medium">Build Node frontend assets</span><span class="mt-1 block text-xs text-muted-foreground">Requires package.json, a supported npm, pnpm, or Bun lockfile, and a build script.</span></span></label>
                {#if buildServers.length === 0}<p class="text-xs text-destructive sm:col-span-2">Configure a Build-capable Server before using Buildpacks.</p>{/if}
              {:else}
                <div class="border border-border bg-muted/20 p-4 sm:col-span-2"><p class="font-medium">Deploy an existing OCI image</p><p class="mt-2 text-sm text-muted-foreground">DeployCrate resolves the configured tag to an immutable digest before every deployment. No Build is created.</p></div>
              {/if}
            {:else}
				{#if $form.sourceType === 'buildpacks'}<FormField label="Registry Resource"><select bind:value={$form.registryResourceId} class="h-9 w-full border border-input bg-background px-3 text-sm">{#each registries as registry}<option value={registry.id}>{registry.name} · {registry.endpoint}</option>{/each}</select></FormField><FormField label="Image repository"><Input bind:value={$form.imageRepository} placeholder="team/application" required /></FormField>{:else}<div class="border border-border p-4 sm:col-span-2"><p class="font-mono text-sm">{$form.imageRepository}:{$form.reference}</p><p class="mt-2 text-xs text-muted-foreground">The first deployment will resolve and store the registry digest.</p></div>{/if}
            {/if}
          </Card.Content>
          <Card.Footer class="justify-between border-t border-border">
            <Button type="button" variant="outline" disabled={step === 1} onclick={() => step--}>Back</Button>
            {#if step < 4}<Button type="button" onclick={() => step++}>Continue</Button>{:else}<Button type="submit" disabled={$form.processing}>{environmentIntent ? 'Create environment' : 'Create application'}</Button>{/if}
          </Card.Footer>
        </Card.Root>
      </form>
    {/if}
  </div>
</DashboardLayout>
