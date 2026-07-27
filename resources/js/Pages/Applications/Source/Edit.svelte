<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'
  type Repository = { id: string; githubInstallationId: string; fullName: string }
  type Registry = { id: string; name: string }
  let { auth, application, options }: { auth: { email: string }; application: any; options: { installations: any[]; repositories: Repository[]; registries: Registry[] } } = $props()
  const form = useForm(() => ({ applicationName: '', applicationSlug: '', environmentName: '', environmentSlug: '', environmentKind: '', githubInstallationId: application.installationId, githubRepositoryId: application.repositoryId, reference: application.reference, autoBuild: application.autoBuild, contextPath: application.contextPath, builderReference: application.builderReference?.String ?? '', buildpackSettings: application.buildpackSettings ?? { schema_version: 1 }, containerRegistryId: application.registryId, imageRepository: application.imageRepository }))
  const repositories = $derived(options.repositories.filter((repository) => repository.githubInstallationId === $form.githubInstallationId))
  function submit(event: SubmitEvent) { event.preventDefault(); $form.patch(routes.applicationSourceUpdate(application.id)) }
</script>
<svelte:head><title>Edit source · {application.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-3xl" onsubmit={submit}>
    <Card.Root><Card.Header><Card.Title>Edit GitHub and Buildpacks source</Card.Title><Card.Description>Changes are validated against the current repository grant and registry state.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="GitHub account"><select bind:value={$form.githubInstallationId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.installations as installation}<option value={installation.id}>{installation.accountLogin}</option>{/each}</select></FormField>
        <FormField label="Repository"><select bind:value={$form.githubRepositoryId} class="h-9 border border-input bg-background px-3 text-sm">{#each repositories as repository}<option value={repository.id}>{repository.fullName}</option>{/each}</select></FormField>
        <FormField label="Reference"><Input bind:value={$form.reference} required /></FormField><FormField label="Build context"><Input bind:value={$form.contextPath} required /></FormField><FormField label="Builder"><Input bind:value={$form.builderReference} /></FormField>
        <FormField label="Registry"><select bind:value={$form.containerRegistryId} class="h-9 border border-input bg-background px-3 text-sm">{#each options.registries as registry}<option value={registry.id}>{registry.name}</option>{/each}</select></FormField><FormField label="Image repository"><Input bind:value={$form.imageRepository} required /></FormField>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={$form.autoBuild} /> Build automatically</label>
      </Card.Content><Card.Footer class="border-t border-border"><Button type="submit" disabled={$form.processing}>Save source</Button></Card.Footer></Card.Root>
  </form>
</DashboardLayout>
