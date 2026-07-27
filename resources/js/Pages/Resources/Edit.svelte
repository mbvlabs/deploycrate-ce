<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Kind = { kind: string; label: string; category: string }
  type Environment = { id: string; name: string; applicationName: string }
  type Resource = { id: string; name: string; category: string; kind: string; managementMode: string; sharingScope: string; ownerEnvironmentId: string }
  let { auth, resource, options }: { auth: { email: string }; resource: Resource; options: { kinds: Kind[]; environments: Environment[] } } = $props()
  const form = useForm(() => ({ name: resource.name, category: resource.category, kind: resource.kind, managementMode: resource.managementMode, sharingScope: resource.sharingScope, ownerEnvironmentId: resource.ownerEnvironmentId }))
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm'
  function selectKind() { const definition = options.kinds.find((kind) => kind.kind === $form.kind); if (definition) $form.category = definition.category }
  function submit(event: SubmitEvent) { event.preventDefault(); $form.patch(routes.resourceUpdate(resource.id)) }
</script>

<svelte:head><title>Edit {resource.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-3xl" onsubmit={submit}>
    <Card.Root>
      <Card.Header><Card.Title>Edit Resource</Card.Title><Card.Description>Changing owner, kind, or management mode is blocked while active child topology is incompatible.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="Name"><Input bind:value={$form.name} required /></FormField>
        <FormField label="Owner Environment"><select bind:value={$form.ownerEnvironmentId} class={selectClass} required>{#each options.environments as environment}<option value={environment.id}>{environment.applicationName} / {environment.name}</option>{/each}</select></FormField>
        <FormField label="Kind"><select bind:value={$form.kind} onchange={selectKind} class={selectClass} required>{#each options.kinds as kind}<option value={kind.kind}>{kind.label}</option>{/each}</select></FormField>
        <FormField label="Category"><Input bind:value={$form.category} readonly={resource.kind !== 'custom'} required /></FormField>
        <FormField label="Management mode"><select bind:value={$form.managementMode} class={selectClass}><option value="managed">Managed</option><option value="external">External</option></select></FormField>
        <FormField label="Sharing scope"><select bind:value={$form.sharingScope} class={selectClass}><option value="environment">Owner Environment</option><option value="application">Application Environments</option><option value="global">Global</option></select></FormField>
      </Card.Content>
      <Card.Footer class="gap-2 border-t border-border"><Button type="submit" disabled={$form.processing}>Save changes</Button><Button variant="outline">{#snippet child({ props })}<a {...props} href={routes.resourceShow(resource.id)}>Cancel</a>{/snippet}</Button></Card.Footer>
    </Card.Root>
  </form>
</DashboardLayout>
