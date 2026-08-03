<script lang="ts">
  import { Link, useForm } from '@inertiajs/svelte'
  import { untrack } from 'svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  type Repository = { id: string; githubInstallationId: string; fullName: string }
  type Registry = { id: string; name: string; endpoint: string }
  type BuildServer = { id: string; name: string; kind: string; address: string }
  type FrontendSettings = { runtime: 'node'; script: 'build' }
  let { auth, application, options, updateUrl, returnUrl }: { auth: { email: string }; application: any; options: { installations: any[]; repositories: Repository[]; registries: Registry[]; buildServers: BuildServer[] }; updateUrl: string; returnUrl: string } = $props()
  let buildFrontendAssets = $state(untrack(() => Boolean(application.buildpackSettings?.frontend)))
	const form = useForm(() => ({ sourceType: application.sourceType ?? 'buildpacks', applicationName: '', applicationSlug: '', environmentName: '', environmentSlug: '', environmentKind: '', githubInstallationId: application.sourceType === 'image' ? (options.installations[0]?.id ?? '') : application.installationId, githubRepositoryId: application.sourceType === 'image' ? '' : application.repositoryId, reference: application.reference, autoBuild: application.autoBuild, contextPath: application.contextPath, builderReference: '', buildpackSettings: { schema_version: 2, frontend: (application.buildpackSettings?.frontend ?? null) as FrontendSettings | null }, registryResourceId: application.registryId, imageRepository: application.imageRepository, buildServerId: application.sourceType === 'image' ? (options.buildServers[0]?.id ?? '') : application.buildServerId }))
  const repositories = $derived(options.repositories.filter((repository) => repository.githubInstallationId === $form.githubInstallationId))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    $form.buildpackSettings.frontend = buildFrontendAssets ? { runtime: 'node', script: 'build' } : null
    $form.patch(updateUrl)
  }
</script>
<svelte:head><title>Edit source · {application.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-3xl" onsubmit={submit}>
    <Card.Root><Card.Header><Card.Title>Edit deployment source</Card.Title><Card.Description>Choose a GitHub Buildpacks build or an existing image from a connected registry.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="Source type"><select bind:value={$form.sourceType} class="h-9 border border-input bg-background px-3 text-sm"><option value="buildpacks">GitHub with Buildpacks</option><option value="image">Registry image</option></select></FormField>
        <FormField label="Registry Resource"><select bind:value={$form.registryResourceId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.registries as registry}<option value={registry.id}>{registry.name} · {registry.endpoint}</option>{/each}</select></FormField>
        {#if $form.sourceType === 'buildpacks'}
          <FormField label="GitHub account"><select bind:value={$form.githubInstallationId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.installations as installation}<option value={installation.id}>{installation.accountLogin}</option>{/each}</select></FormField>
          <FormField label="Repository"><select bind:value={$form.githubRepositoryId} class="h-9 border border-input bg-background px-3 text-sm">{#each repositories as repository}<option value={repository.id}>{repository.fullName}</option>{/each}</select></FormField>
          <FormField label="Branch or full ref"><Input bind:value={$form.reference} required /></FormField><FormField label="Build context"><Input bind:value={$form.contextPath} required /></FormField>
          <FormField label="Build Server"><select bind:value={$form.buildServerId} class="h-9 border border-input bg-background px-3 text-sm" required>{#each options.buildServers as server}<option value={server.id}>{server.name} · {server.kind === 'worker' ? server.address : 'Control plane'}</option>{/each}</select></FormField>
          <FormField label="Output image repository"><Input bind:value={$form.imageRepository} required /></FormField>
          <label class="flex gap-3 border border-border p-4"><input class="mt-1" type="checkbox" bind:checked={buildFrontendAssets} /><span><span class="font-medium">Build Node frontend assets</span><span class="mt-1 block text-xs text-muted-foreground">Build frontend assets before the Go Buildpacks build.</span></span></label>
          <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={$form.autoBuild} /> Build automatically</label>
        {:else}
          <FormField label="Image repository"><Input bind:value={$form.imageRepository} placeholder="team/application" required /></FormField>
          <FormField label="Default tag or digest"><Input bind:value={$form.reference} placeholder="latest" required /></FormField>
          <div class="border border-border bg-muted/20 p-4 sm:col-span-2"><p class="text-sm">DeployCrate resolves this reference to an immutable digest for each Release.</p></div>
        {/if}
      </Card.Content><Card.Footer class="justify-between border-t border-border"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={returnUrl}>Cancel</Link>{/snippet}</Button><Button type="submit" disabled={$form.processing}>Save source</Button></Card.Footer></Card.Root>
  </form>
</DashboardLayout>
