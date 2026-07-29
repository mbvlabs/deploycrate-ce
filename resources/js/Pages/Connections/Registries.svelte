<script lang="ts">
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import { router, useForm } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Registry = { id: string; name: string; provider: string; endpoint: string; username: string; credentialName: string; managed: boolean; createdAt: string }
  let { auth, registries }: { auth: { email: string }; registries: Registry[] } = $props()
  let preset = $state('docker_hub')
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
    $form.post(routes.containerRegistryCreate(), { onSuccess: () => $form.reset() })
  }
</script>

<svelte:head><title>Container registries</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Connections</p>
      <h1 class="mt-3 text-3xl font-semibold">Container registries</h1>
      <p class="mt-4 text-sm leading-6 text-muted-foreground">Publish Application images to the DeployCrate-managed registry, Docker Hub, or another authenticated OCI registry.</p>
    </header>

    <Card.Root class="max-w-3xl">
      <Card.Header><Card.Title>Connect external registry</Card.Title><Card.Description>Credentials are verified with Docker before the access token is encrypted and stored. Use a scoped access token instead of an account password.</Card.Description></Card.Header>
      <Card.Content>
        <form class="grid gap-5 sm:grid-cols-2" onsubmit={submit}>
          <FormField label="Registry type">
            <select bind:value={preset} onchange={selectPreset} class="h-9 w-full border border-input bg-background px-3 text-sm"><option value="docker_hub">Docker Hub</option><option value="custom">Custom OCI registry</option></select>
          </FormField>
          <FormField label="Display name" error={$form.errors.name}><Input bind:value={$form.name} required /></FormField>
          <FormField label="Registry endpoint" error={$form.errors.endpoint}><Input bind:value={$form.endpoint} placeholder="ghcr.io" readonly={preset === 'docker_hub'} required /></FormField>
          <FormField label="Username" error={$form.errors.username}><Input bind:value={$form.username} autocomplete="username" required /></FormField>
          <FormField label="Access token" error={$form.errors.accessToken}><Input type="password" bind:value={$form.accessToken} autocomplete="new-password" required /></FormField>
          <div class="flex items-end"><Button type="submit" disabled={$form.processing}>Connect registry</Button></div>
        </form>
      </Card.Content>
    </Card.Root>

    <section class="space-y-4">
      <div><h2 class="text-xl font-semibold">Available registries</h2><p class="mt-1 text-sm text-muted-foreground">These destinations are selectable when creating or editing an Application source.</p></div>
      {#if registries.length === 0}
        <Card.Root><Card.Content class="grid place-items-center gap-3 py-12 text-center"><BoxesIcon class="size-7 text-muted-foreground" /><p class="text-sm text-muted-foreground">No container registries are available.</p></Card.Content></Card.Root>
      {:else}
        <div class="grid gap-4 md:grid-cols-2">
          {#each registries as registry (registry.id)}
            <Card.Root>
              <Card.Header><Card.Action><span class="text-xs" class:text-success={registry.managed}>{registry.managed ? 'Managed' : 'External'}</span></Card.Action><Card.Title>{registry.name}</Card.Title><Card.Description>{registry.endpoint}</Card.Description></Card.Header>
              <Card.Content class="grid gap-3 text-sm sm:grid-cols-2"><div><p class="text-xs text-muted-foreground">Protocol</p><p class="mt-1">OCI Distribution</p></div><div><p class="text-xs text-muted-foreground">Username</p><p class="mt-1 font-mono">{registry.username}</p></div></Card.Content>
              {#if !registry.managed}<Card.Footer class="border-t border-border"><Button size="sm" variant="destructive" onclick={() => router.delete(routes.containerRegistryDestroy(registry.id))}>Archive</Button></Card.Footer>{/if}
            </Card.Root>
          {/each}
        </div>
      {/if}
    </section>
  </div>
</DashboardLayout>
