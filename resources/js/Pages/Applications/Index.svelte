<script lang="ts">
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import { Link } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Application = { id: string; name: string; slug: string; environmentName: string; environmentKind: string; repositoryFullName: string; reference: string; sourceHealthy: boolean; sourceType: string }
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
            <Card.Header><Card.Action><span class:text-success={application.sourceHealthy} class:text-destructive={!application.sourceHealthy} class="text-xs">{application.sourceHealthy ? 'Source ready' : 'Source degraded'}</span></Card.Action><Card.Title>{application.name}</Card.Title><Card.Description>{application.environmentName} · {application.environmentKind}</Card.Description></Card.Header>
            <Card.Content><p class="font-mono text-xs">{application.repositoryFullName}</p><p class="mt-2 font-mono text-xs text-muted-foreground">{application.reference}</p></Card.Content>
            <Card.Footer class="border-t border-border"><Button size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationShow(application.id)}>Open</Link>{/snippet}</Button></Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
