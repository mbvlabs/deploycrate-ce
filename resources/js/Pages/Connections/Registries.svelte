<script lang="ts">
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import { router, useForm } from '@inertiajs/svelte'

  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import FormField from '@/Components/FormField.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import { Input } from '@/Components/ui/input'
  import * as NativeSelect from '@/Components/ui/native-select'
  import { Spinner } from '@/Components/ui/spinner'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Registry = { id: string; name: string; provider: string; endpoint: string; username: string; credentialName: string; managed: boolean; createdAt: string }
  let { auth, registries }: { auth: { email: string }; registries: Registry[] } = $props()
  let preset = $state('docker_hub')
  let archiveTarget = $state<Registry | null>(null)
  let archiveDialogOpen = $state(false)
  let archiveProcessing = $state(false)
  let archiveError = $state('')
  const form = useForm(() => ({ name: 'Docker Hub', endpoint: 'docker.io', username: '', accessToken: '' }))

  function selectPreset() {
    if (preset === 'docker_hub') {
      $form.name = 'Docker Hub'
      $form.endpoint = 'docker.io'
    } else {
      $form.name = ''
      $form.endpoint = ''
    }
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
		$form.post(routes.registryResourceCreate(), { onSuccess: () => $form.reset() })
  }

  function askToArchive(registry: Registry) {
    archiveTarget = registry
    archiveError = ''
    archiveDialogOpen = true
  }

  function archive() {
    if (!archiveTarget || archiveProcessing) return
    archiveProcessing = true
    archiveError = ''
    router.delete(routes.registryResourceDestroy(archiveTarget.id), {
      onSuccess: () => { archiveDialogOpen = false; archiveTarget = null },
      onError: (errors) => (archiveError = Object.values(errors).map(String).join('\n') || 'The registry could not be archived.'),
      onFinish: () => (archiveProcessing = false),
    })
  }
</script>

<svelte:head><title>Registry Resources</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Connections</p>
		<h1 class="mt-3 text-3xl font-semibold">Registry Resources</h1>
      <p class="mt-4 text-sm leading-6 text-muted-foreground">Publish Application images to the DeployCrate-managed registry, Docker Hub, or another authenticated OCI registry.</p>
    </header>

    <Card.Root class="max-w-3xl">
      <Card.Header><Card.Title>Connect external registry</Card.Title><Card.Description>Credentials are verified with Docker before the access token is encrypted and stored. Use a scoped access token instead of an account password.</Card.Description></Card.Header>
      <Card.Content>
        <form class="grid gap-5 sm:grid-cols-2" onsubmit={submit}>
          <FormField label="Registry type">
            <NativeSelect.Root bind:value={preset} onchange={selectPreset} class="w-full"><NativeSelect.Option value="docker_hub">Docker Hub</NativeSelect.Option><NativeSelect.Option value="custom">Custom OCI registry</NativeSelect.Option></NativeSelect.Root>
          </FormField>
          <FormField label="Display name" error={$form.errors.name}><Input bind:value={$form.name} required /></FormField>
          <FormField label="Registry endpoint" error={$form.errors.endpoint}><Input bind:value={$form.endpoint} placeholder="ghcr.io" readonly={preset === 'docker_hub'} required /></FormField>
          <FormField label="Username" error={$form.errors.username}><Input bind:value={$form.username} autocomplete="username" required /></FormField>
          <FormField label="Access token" error={$form.errors.accessToken}><Input type="password" bind:value={$form.accessToken} autocomplete="new-password" required /></FormField>
          <div class="flex items-end"><Button type="submit" disabled={$form.processing} aria-busy={$form.processing}>{#if $form.processing}<Spinner />{/if}Connect registry</Button></div>
        </form>
      </Card.Content>
    </Card.Root>

    <section class="space-y-4">
      <div><h2 class="text-xl font-semibold">Available registries</h2><p class="mt-1 text-sm text-muted-foreground">These destinations are selectable when creating or editing an Application source.</p></div>
      {#if registries.length === 0}
        <Empty.Root class="border border-border"><Empty.Header><Empty.Media variant="icon"><BoxesIcon /></Empty.Media><Empty.Title>No Registry Resources</Empty.Title><Empty.Description>Connect an external OCI registry to publish and deploy Application images.</Empty.Description></Empty.Header></Empty.Root>
      {:else}
        <div class="grid gap-4 md:grid-cols-2">
          {#each registries as registry (registry.id)}
            <Card.Root>
              <Card.Header><Card.Action><StatusBadge status={registry.managed ? 'managed' : 'external'} /></Card.Action><Card.Title>{registry.name}</Card.Title><Card.Description>{registry.endpoint}</Card.Description></Card.Header>
              <Card.Content class="grid gap-3 text-sm sm:grid-cols-2"><div><p class="text-xs text-muted-foreground">Protocol</p><p class="mt-1">OCI Distribution</p></div><div><p class="text-xs text-muted-foreground">Username</p><p class="mt-1 font-mono">{registry.username}</p></div></Card.Content>
					{#if !registry.managed}<Card.Footer class="border-t border-border"><Button size="sm" variant="destructive" onclick={() => askToArchive(registry)}>Archive</Button></Card.Footer>{/if}
            </Card.Root>
          {/each}
        </div>
      {/if}
    </section>
  </div>
  <ConfirmActionDialog bind:open={archiveDialogOpen} title={`Archive ${archiveTarget?.name ?? 'registry'}?`} description="This registry will no longer be available for new builds or deployments." confirmLabel="Archive registry" destructive processing={archiveProcessing} error={archiveError} onconfirm={archive} />
</DashboardLayout>
