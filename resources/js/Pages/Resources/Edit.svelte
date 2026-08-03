<script lang="ts">
  import { Link, router, useForm } from '@inertiajs/svelte'
  import * as Alert from '@/Components/ui/alert'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
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
  let selectedEnvironmentId = $state('')
  let selectedApplicationId = $state('')
  let slugCustomized = $state(initialSlugCustomized())
  let granting = $state(false)
  let revokeTarget = $state<{ scope: 'Environment' | 'Application'; id: string; name: string } | null>(null)
  let revokeDialogOpen = $state(false)
  let revokeProcessing = $state(false)
  let revokeError = $state('')

  function initialSlugCustomized() { return resource.slug !== slugify(resource.name) }

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
  function grantEnvironment() { if (!selectedEnvironmentId) return; granting = true; router.post(`${routes.resourceEnvironmentGrantCreate(resource.id, selectedEnvironmentId)}?returnTo=edit`, {}, { preserveScroll: true, onSuccess: () => selectedEnvironmentId = '', onFinish: () => (granting = false) }) }
  function grantApplication() { if (!selectedApplicationId) return; granting = true; router.post(`${routes.resourceApplicationGrantCreate(resource.id, selectedApplicationId)}?returnTo=edit`, {}, { preserveScroll: true, onSuccess: () => selectedApplicationId = '', onFinish: () => (granting = false) }) }
  function askToRevoke(scope: 'Environment' | 'Application', id: string, name: string) { revokeTarget = { scope, id, name }; revokeError = ''; revokeDialogOpen = true }
  function confirmRevoke() {
    if (!revokeTarget) return
    revokeProcessing = true
    revokeError = ''
    const url = revokeTarget.scope === 'Environment'
      ? routes.resourceEnvironmentGrantDestroy(resource.id, revokeTarget.id)
      : routes.resourceApplicationGrantDestroy(resource.id, revokeTarget.id)
    router.delete(`${url}?returnTo=edit`, {
      preserveScroll: true,
      onSuccess: () => (revokeDialogOpen = false),
      onError: () => (revokeError = 'Access could not be revoked. Please try again.'),
      onFinish: () => (revokeProcessing = false),
    })
  }
</script>

<svelte:head><title>Edit {resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="mx-auto max-w-5xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Resource configuration</p><h1 class="mt-3 text-3xl font-semibold">Edit {resource.name}</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Edit Resource identity and selection policy. Docker runtime and endpoint operations are available on the Resource page.</p></div><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.resourceShow(resource.id)}>Done editing</Link>{/snippet}</Button></header>

    {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"><Alert.Title>The changes could not be saved</Alert.Title><Alert.Description><ul class="mt-2 list-disc pl-5">{#each Object.entries(errors) as [field, message]}<li>{field}: {message}</li>{/each}</ul></Alert.Description></Alert.Root>{/if}

    <form onsubmit={saveIdentity}>
      <Card.Root><Card.Header><Card.Title>Identity and engine</Card.Title><Card.Description>The slug follows the name until you customize it. The Resource type is derived from the selected engine.</Card.Description></Card.Header><Card.Content class="grid gap-5 sm:grid-cols-2"><FormField label="Name" error={errors.name}><Input value={$identity.name} oninput={(event) => updateName(event.currentTarget.value)} required /></FormField><FormField label="Slug" error={errors.slug}><Input value={$identity.slug} oninput={(event) => updateSlug(event.currentTarget.value)} required /></FormField><FormField label="Engine" error={errors['configuration.engine']}><NativeSelect.Root class="w-full" bind:value={$identity.engine} required>{#each options.engines as engine}<NativeSelect.Option value={engine.engine}>{engine.label} · {engine.resourceType}</NativeSelect.Option>{/each}</NativeSelect.Root></FormField><FormField label="Engine version" error={errors['configuration.engine_version']}><Input bind:value={$identity.engineVersion} placeholder="Optional" /></FormField><FormField label="Sharing scope" error={errors.sharingScope}><NativeSelect.Root class="w-full" bind:value={$identity.sharingScope}><NativeSelect.Option value="environment">Environment policy</NativeSelect.Option><NativeSelect.Option value="application">Application policy</NativeSelect.Option><NativeSelect.Option value="global">Global policy</NativeSelect.Option></NativeSelect.Root></FormField></Card.Content><Card.Footer class="border-t border-border"><Button type="submit" disabled={$identity.processing} aria-busy={$identity.processing}>{#if $identity.processing}<Spinner />{/if}Save Resource</Button></Card.Footer></Card.Root>
    </form>

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
