<script lang="ts">
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import { Link } from '@inertiajs/svelte'
  import PageHeader from '@/Components/PageHeader.svelte'
  import { Button } from '@/Components/ui/button'
  import { Badge } from '@/Components/ui/badge'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Resource = { id: string; name: string; resourceType: string; engine: string; systemManaged: boolean; environmentAttachable: boolean; databaseCount: number; connectionCount: number; installationCount: number; endpointCount: number; health: string }
  let { auth, resources }: { auth: { email: string }; resources: Resource[] } = $props()
</script>

<svelte:head><title>Resources</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader eyebrow="Resources" title="Infrastructure connections" description="Manage resource identity, desired placement, encrypted credentials, storage, and health configuration.">
      {#snippet actions()}<Button>{#snippet child({ props })}<Link {...props} href={routes.resourceNew()}>New resource</Link>{/snippet}</Button>{/snippet}
    </PageHeader>

    {#if resources.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Media variant="icon"><DatabaseIcon /></Empty.Media><Empty.Title>No resources yet</Empty.Title><Empty.Description>Deploy a database, cache, or service and manage its access from one place.</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {#each resources as resource (resource.id)}
          <Card.Root class="h-full">
            <Card.Header><Card.Action><StatusBadge status={resource.health || 'unknown'} /></Card.Action><div class="flex flex-wrap items-center gap-2"><Card.Title>{resource.name}</Card.Title>{#if resource.systemManaged}<Badge variant="secondary">System Resource</Badge>{/if}{#if !resource.environmentAttachable}<Badge variant="outline">Not attachable</Badge>{/if}</div><Card.Description>{resource.resourceType === 'database' ? `${resource.databaseCount} ${resource.databaseCount === 1 ? 'database' : 'databases'} · ` : ''}{resource.connectionCount === 0 ? 'Unattached' : `${resource.connectionCount} attached ${resource.connectionCount === 1 ? 'environment' : 'environments'}`}</Card.Description></Card.Header>
            <Card.Content>
              <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-xs">
                <div><dt class="text-muted-foreground">Engine</dt><dd class="mt-1 font-medium capitalize">{resource.engine}</dd></div>
                <div><dt class="text-muted-foreground">Type</dt><dd class="mt-1 font-medium capitalize">{resource.resourceType}</dd></div>
                <div><dt class="text-muted-foreground">Installations</dt><dd class="mt-1 font-medium">{resource.installationCount}</dd></div>
                <div><dt class="text-muted-foreground">Endpoints</dt><dd class="mt-1 font-medium">{resource.endpointCount}</dd></div>
              </dl>
            </Card.Content>
            <Card.Footer class="mt-auto justify-end"><Button class="w-full sm:w-auto" size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={resource.systemManaged ? routes.systemResource(resource.id) : routes.resourceShow(resource.id)}>Open resource</Link>{/snippet}</Button></Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
