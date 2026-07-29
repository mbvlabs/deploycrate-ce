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
  let { auth, options, environmentIntent = false }: { auth: { email: string }; options: { installations: Installation[]; repositories: Repository[]; registries: Registry[] }; environmentIntent?: boolean } = $props()
  const installations = $derived(options.installations ?? [])
  const repositoryOptions = $derived(options.repositories ?? [])
  const registries = $derived(options.registries ?? [])
  let step = $state(1)
  const form = useForm(() => ({ applicationName: '', applicationSlug: '', environmentName: 'Production', environmentSlug: 'production', environmentKind: 'production', githubInstallationId: installations[0]?.id ?? '', githubRepositoryId: '', reference: '', autoBuild: true, contextPath: '.', builderReference: '', buildpackSettings: { schema_version: 1 }, containerRegistryId: registries[0]?.id ?? '', imageRepository: '' }))
  const repositories = $derived(repositoryOptions.filter((repository) => repository.githubInstallationId === $form.githubInstallationId))
  function submit(event: SubmitEvent) { event.preventDefault(); $form.post(environmentIntent ? routes.environmentCreate() : routes.applicationCreate()) }
</script>

<svelte:head><title>{environmentIntent ? 'Create environment' : 'New application'}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{environmentIntent ? 'Environments' : 'Applications'} · Step {step} of 4</p><h1 class="mt-3 text-3xl font-semibold">{environmentIntent ? 'Create a deployable Environment' : 'Configure a build-ready application'}</h1>{#if environmentIntent}<p class="mt-2 max-w-2xl text-sm text-muted-foreground">The Environment is created with its owning Application, GitHub source, Buildpacks configuration, and image destination.</p>{/if}</header>
    {#if installations.length === 0 || registries.length === 0}
      <Card.Root><Card.Header><Card.Title>Setup prerequisites required</Card.Title><Card.Description>Connect GitHub and add a registry under Connections → Container Registries before creating an Environment.</Card.Description></Card.Header></Card.Root>
    {:else}
      <form onsubmit={submit}>
        <Card.Root>
          <Card.Header><Card.Title>{['Application', 'Source', 'Buildpacks', 'Image destination'][step - 1]}</Card.Title></Card.Header>
          <Card.Content class="grid gap-5 sm:grid-cols-2">
            {#if step === 1}
              <FormField label="Application name"><Input bind:value={$form.applicationName} required /></FormField>
              <FormField label="Application slug"><Input bind:value={$form.applicationSlug} required /></FormField>
              <FormField label="Environment name"><Input bind:value={$form.environmentName} required /></FormField>
              <FormField label="Environment slug"><Input bind:value={$form.environmentSlug} required /></FormField>
              <FormField label="Environment kind"><Input bind:value={$form.environmentKind} required /></FormField>
            {:else if step === 2}
              <FormField label="GitHub account"><select bind:value={$form.githubInstallationId} class="h-9 w-full border border-input bg-background px-3 text-sm">{#each installations as installation}<option value={installation.id}>{installation.accountLogin}</option>{/each}</select></FormField>
              <FormField label="Repository"><select bind:value={$form.githubRepositoryId} class="h-9 w-full border border-input bg-background px-3 text-sm" required><option value="">Select a repository</option>{#each repositories as repository}<option value={repository.id}>{repository.fullName}</option>{/each}</select></FormField>
              <FormField label="Branch or full ref"><Input bind:value={$form.reference} placeholder="main" required /></FormField>
              <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={$form.autoBuild} /> Build automatically on matching pushes</label>
            {:else if step === 3}
              <FormField label="Build context"><Input bind:value={$form.contextPath} placeholder="." required /></FormField>
              <div class="border border-border bg-muted/20 px-3 py-2">
                <p class="text-[10px] uppercase tracking-wider text-muted-foreground">Builder</p>
                <p class="mt-1 text-sm">Paketo Ubuntu Noble</p>
                <p class="mt-1 text-xs text-muted-foreground">Managed and digest-pinned by DeployCrate for this server.</p>
              </div>
            {:else}
              <FormField label="Container registry"><select bind:value={$form.containerRegistryId} class="h-9 w-full border border-input bg-background px-3 text-sm">{#each registries as registry}<option value={registry.id}>{registry.name} · {registry.endpoint}</option>{/each}</select></FormField>
              <FormField label="Image repository"><Input bind:value={$form.imageRepository} placeholder="team/application" required /></FormField>
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
