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
	const form = useForm(() => ({ applicationName: '', applicationSlug: '', environmentName: '', environmentSlug: '', environmentKind: '', githubInstallationId: application.installationId, githubRepositoryId: application.repositoryId, reference: application.reference, autoBuild: application.autoBuild, contextPath: application.contextPath, builderReference: '', buildpackSettings: { schema_version: 2, frontend: (application.buildpackSettings?.frontend ?? null) as FrontendSettings | null }, registryResourceId: application.registryId, imageRepository: application.imageRepository, buildServerId: application.buildServerId }))
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
    <Card.Root><Card.Header><Card.Title>Edit GitHub and Buildpacks source</Card.Title><Card.Description>Changes are validated against the current repository grant and registry state.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="GitHub account"><select bind:value={$form.githubInstallationId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.installations as installation}<option value={installation.id}>{installation.accountLogin}</option>{/each}</select></FormField>
        <FormField label="Repository"><select bind:value={$form.githubRepositoryId} class="h-9 border border-input bg-background px-3 text-sm">{#each repositories as repository}<option value={repository.id}>{repository.fullName}</option>{/each}</select></FormField>
        <FormField label="Reference"><Input bind:value={$form.reference} required /></FormField><FormField label="Build context"><Input bind:value={$form.contextPath} required /></FormField>
        <FormField label="Build Server"><select bind:value={$form.buildServerId} class="h-9 border border-input bg-background px-3 text-sm" required>{#each options.buildServers as server}<option value={server.id}>{server.name} · {server.kind === 'worker' ? server.address : 'Control plane'}</option>{/each}</select></FormField>
        <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Builder</p><p class="mt-1 text-sm">Paketo Ubuntu Noble</p><p class="mt-1 text-xs text-muted-foreground">Managed and digest-pinned by DeployCrate for this server.</p></div>
        <label class="flex gap-3 border border-border p-4"><input class="mt-1" type="checkbox" bind:checked={buildFrontendAssets} /><span><span class="font-medium">Build Node frontend assets</span><span class="mt-1 block text-xs text-muted-foreground">Requires package.json, a supported npm, pnpm, or Bun lockfile, and a build script. DeployCrate detects and caches the selected package manager during Builds.</span></span></label>
        {#if $form.errors.settings}<p class="text-xs text-destructive sm:col-span-2">{$form.errors.settings}</p>{/if}
		<FormField label="Registry Resource"><select bind:value={$form.registryResourceId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.registries as registry}<option value={registry.id}>{registry.name} · {registry.endpoint}</option>{/each}</select></FormField><FormField label="Image repository"><Input bind:value={$form.imageRepository} required /></FormField>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={$form.autoBuild} /> Build automatically</label>
      </Card.Content><Card.Footer class="justify-between border-t border-border"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={returnUrl}>Cancel</Link>{/snippet}</Button><Button type="submit" disabled={$form.processing}>Save source</Button></Card.Footer></Card.Root>
  </form>
</DashboardLayout>
