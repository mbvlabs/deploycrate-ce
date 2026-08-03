<script lang="ts">
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Resource = { id: string; name: string; resourceType: string; engine: string; databaseCount: number; sharingScope: string; connectionCount: number; grantCount: number; installationCount: number; endpointCount: number; health: string }
  let { auth, resources }: { auth: { email: string }; resources: Resource[] } = $props()
</script>

<svelte:head><title>Resources</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Resources</p><h1 class="mt-3 text-3xl font-semibold">Infrastructure connections</h1><p class="mt-2 max-w-2xl text-sm text-muted-foreground">Manage Resource identity, desired placement, encrypted credentials, storage, and health configuration.</p></div>
      <Button href={routes.resourceNew()}>New Resource</Button>
    </header>

    {#if resources.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Media variant="icon"><DatabaseIcon /></Empty.Media><Empty.Title>No Resources yet</Empty.Title><Empty.Description>Deploy a database, cache, or service and manage its access from one place.</Empty.Description></Empty.Header><Empty.Content><Button href={routes.resourceNew()}>New Resource</Button></Empty.Content></Empty.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2">
        {#each resources as resource (resource.id)}
          <Card.Root>
            <Card.Header><Card.Action><StatusBadge status={resource.health} /></Card.Action><Card.Title>{resource.name}</Card.Title><Card.Description>{resource.resourceType === 'database' ? `${resource.databaseCount} ${resource.databaseCount === 1 ? 'Database' : 'Databases'} · ` : ''}{resource.connectionCount === 0 ? 'Unattached' : `${resource.connectionCount} Connected ${resource.connectionCount === 1 ? 'Environment' : 'Environments'}`}</Card.Description></Card.Header>
            <Card.Content class="grid grid-cols-2 gap-4 text-xs sm:grid-cols-4"><div><p class="text-muted-foreground">Engine</p><p class="mt-1 font-medium">{resource.engine}</p></div><div><p class="text-muted-foreground">Runtime</p><p class="mt-1">Docker</p></div><div><p class="text-muted-foreground">Installations</p><p class="mt-1">{resource.installationCount}</p></div><div><p class="text-muted-foreground">Endpoints</p><p class="mt-1">{resource.endpointCount}</p></div></Card.Content>
            <Card.Footer class="flex-col items-start justify-between gap-3 border-t border-border sm:flex-row sm:items-center"><span class="text-xs capitalize text-muted-foreground">{resource.resourceType} · {resource.sharingScope}{resource.sharingScope === 'global' ? '' : ` · ${resource.grantCount} grants`}</span><Button class="w-full sm:w-auto" href={routes.resourceShow(resource.id)} size="sm" variant="outline">Open</Button></Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
