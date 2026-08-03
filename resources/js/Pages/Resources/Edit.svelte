<script lang="ts">
  import { Link, router, useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { slugify } from '@/lib/slug'
  import { routes } from '@/routes'

  type Engine = { engine: string; label: string; resourceType: 'database' | 'cache' | 'service' }
  type EnvironmentOption = { id: string; name: string; kind: string; applicationId: string; applicationName: string }
  type ApplicationOption = { id: string; name: string }
  type Options = { engines: Engine[]; environments: EnvironmentOption[] }
  let { auth, resource, options, errors = {} }: { auth: { email: string }; resource: any; options: Options; errors?: Record<string, string> } = $props()

  const identity = useForm(() => ({ name: resource.name, slug: resource.slug, engine: resource.engine, engineVersion: resource.configuration?.engine_version ?? '', sharingScope: resource.sharingScope }))
  const definition = $derived(options.engines.find((engine) => engine.engine === $identity.engine) ?? options.engines[0])
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm aria-invalid:border-destructive'
  let selectedEnvironmentId = $state('')
  let selectedApplicationId = $state('')
  let slugCustomized = $state(resource.slug !== slugify(resource.name))

  function updateName(name: string) {
    $identity.name = name
    if (!slugCustomized) $identity.slug = slugify(name)
  }

  function updateSlug(slug: string) {
    $identity.slug = slug
    slugCustomized = true
  }

  function saveIdentity(event: SubmitEvent) {
    event.preventDefault()
    const configuration = { ...resource.configuration, engine: $identity.engine }
    if ($identity.engineVersion) configuration.engine_version = $identity.engineVersion
    else delete configuration.engine_version
    $identity.transform((values) => ({ name: values.name, slug: values.slug, resourceType: definition.resourceType, configuration, sharingScope: values.sharingScope })).patch(routes.resourceUpdate(resource.id))
  }

  function availableEnvironments() { const granted = new Set(resource.environmentGrants.map((grant: any) => grant.environmentId)); return options.environments.filter((environment) => !granted.has(environment.id)) }
  function applications() { const values = new Map<string, ApplicationOption>(); for (const environment of options.environments) values.set(environment.applicationId, { id: environment.applicationId, name: environment.applicationName }); return [...values.values()].sort((left, right) => left.name.localeCompare(right.name)) }
  function availableApplications() { const granted = new Set(resource.applicationGrants.map((grant: any) => grant.applicationId)); return applications().filter((application) => !granted.has(application.id)) }
  function grantEnvironment() { if (!selectedEnvironmentId) return; router.post(`${routes.resourceEnvironmentGrantCreate(resource.id, selectedEnvironmentId)}?returnTo=edit`, {}, { preserveScroll: true, onSuccess: () => selectedEnvironmentId = '' }) }
  function revokeEnvironment(environmentId: string) { router.delete(`${routes.resourceEnvironmentGrantDestroy(resource.id, environmentId)}?returnTo=edit`, { preserveScroll: true }) }
  function grantApplication() { if (!selectedApplicationId) return; router.post(`${routes.resourceApplicationGrantCreate(resource.id, selectedApplicationId)}?returnTo=edit`, {}, { preserveScroll: true, onSuccess: () => selectedApplicationId = '' }) }
  function revokeApplication(applicationId: string) { router.delete(`${routes.resourceApplicationGrantDestroy(resource.id, applicationId)}?returnTo=edit`, { preserveScroll: true }) }
</script>

<svelte:head><title>Edit {resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="mx-auto max-w-5xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Resource configuration</p><h1 class="mt-3 text-3xl font-semibold">Edit {resource.name}</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Edit Resource identity and selection policy. Docker runtime and endpoint operations are available on the Resource page.</p></div><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.resourceShow(resource.id)}>Done editing</Link>{/snippet}</Button></header>

    {#if Object.keys(errors).length > 0}<div class="border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"><p class="font-medium">The changes could not be saved.</p><ul class="mt-2 list-disc pl-5">{#each Object.entries(errors) as [field, message]}<li>{field}: {message}</li>{/each}</ul></div>{/if}

    <form onsubmit={saveIdentity}>
      <Card.Root><Card.Header><Card.Title>Identity and engine</Card.Title><Card.Description>The slug follows the name until you customize it. The Resource type is derived from the selected engine.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Name" error={errors.name}><Input value={$identity.name} oninput={(event) => updateName(event.currentTarget.value)} required /></FormField><FormField label="Slug" error={errors.slug}><Input value={$identity.slug} oninput={(event) => updateSlug(event.currentTarget.value)} required /></FormField><FormField label="Engine" error={errors['configuration.engine']}><select bind:value={$identity.engine} class={selectClass} required>{#each options.engines as engine}<option value={engine.engine}>{engine.label} · {engine.resourceType}</option>{/each}</select></FormField><FormField label="Engine version" error={errors['configuration.engine_version']}><Input bind:value={$identity.engineVersion} placeholder="Optional" /></FormField><FormField label="Sharing scope" error={errors.sharingScope}><select bind:value={$identity.sharingScope} class={selectClass}><option value="environment">Environment policy</option><option value="application">Application policy</option><option value="global">Global policy</option></select></FormField></Card.Content><Card.Footer class="border-t border-border"><Button type="submit" disabled={$identity.processing}>Save Resource</Button></Card.Footer></Card.Root>
    </form>

    <Card.Root><Card.Header><Card.Title>Selection grants</Card.Title><Card.Description>Choose which consumers may select this Resource and one of its application credentials.</Card.Description></Card.Header><Card.Content class="space-y-5">
      {#if $identity.sharingScope === 'global'}
        <p class="text-sm text-muted-foreground">Global Resources are selectable by every Environment.</p>
      {:else if $identity.sharingScope === 'environment'}
        <div class="flex flex-col gap-2 sm:flex-row"><select bind:value={selectedEnvironmentId} class={selectClass}><option value="">Select an Environment</option>{#each availableEnvironments() as environment}<option value={environment.id}>{environment.applicationName} · {environment.name} ({environment.kind})</option>{/each}</select><Button type="button" disabled={!selectedEnvironmentId} onclick={grantEnvironment}>Grant access</Button></div>
        <div class="divide-y divide-border border border-border">{#each resource.environmentGrants as grant}<div class="flex items-center justify-between gap-3 p-3"><div><p class="text-sm font-medium">{grant.applicationName} · {grant.environmentName}</p><p class="text-xs text-muted-foreground">{grant.environmentKind}</p></div><Button type="button" size="sm" variant="destructive" onclick={() => revokeEnvironment(grant.environmentId)}>Revoke</Button></div>{:else}<p class="p-3 text-sm text-muted-foreground">No Environment grants.</p>{/each}</div>
      {:else}
        <div class="flex flex-col gap-2 sm:flex-row"><select bind:value={selectedApplicationId} class={selectClass}><option value="">Select an Application</option>{#each availableApplications() as application}<option value={application.id}>{application.name}</option>{/each}</select><Button type="button" disabled={!selectedApplicationId} onclick={grantApplication}>Grant access</Button></div>
        <div class="divide-y divide-border border border-border">{#each resource.applicationGrants as grant}<div class="flex items-center justify-between gap-3 p-3"><p class="text-sm font-medium">{grant.applicationName}</p><Button type="button" size="sm" variant="destructive" onclick={() => revokeApplication(grant.applicationId)}>Revoke</Button></div>{:else}<p class="p-3 text-sm text-muted-foreground">No Application grants.</p>{/each}</div>
      {/if}
    </Card.Content></Card.Root>
  </div>
</DashboardLayout>
