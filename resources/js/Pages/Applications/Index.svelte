<script lang="ts">
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import { Link } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = { id: string; environmentName: string; environmentKind: string; repositoryFullName: string; reference: string; sourceHealthy: boolean; sourceType: string }
  type Application = { id: string; name: string; slug: string; environments: Environment[] }
  let { auth, applications }: { auth: { email: string }; applications: Application[] } = $props()
</script>

<svelte:head><title>Applications</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex items-end justify-between gap-4">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Applications</p><h1 class="mt-3 text-3xl font-semibold">Build-ready services</h1></div>
      <Button>{#snippet child({ props })}<Link {...props} href={routes.applicationNew()}>New application</Link>{/snippet}</Button>
    </header>
    {#if applications.length === 0}
      <Card.Root><Card.Content class="grid place-items-center gap-3 py-14 text-center"><AppWindowIcon class="size-8 text-muted-foreground" /><p class="text-sm text-muted-foreground">No applications have been configured.</p></Card.Content></Card.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2">
        {#each applications as application (application.id)}
          <Card.Root>
            <Card.Header><Card.Title>{application.name}</Card.Title><Card.Description>{application.slug}</Card.Description></Card.Header>
            <Card.Content class="space-y-3">{#each application.environments as environment (environment.id)}<div class="flex items-start justify-between gap-4 border border-border p-3"><div><p class="text-sm font-medium">{environment.environmentName}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{environment.repositoryFullName} · {environment.reference}</p></div><span class:text-success={environment.sourceHealthy} class:text-destructive={!environment.sourceHealthy} class="text-xs">{environment.sourceHealthy ? 'Ready' : 'Degraded'}</span></div>{/each}</Card.Content>
            <Card.Footer class="border-t border-border"><Button size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationShow(application.id)}>Open</Link>{/snippet}</Button></Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
