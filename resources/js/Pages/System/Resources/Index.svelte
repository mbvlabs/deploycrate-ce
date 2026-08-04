<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import * as Table from '@/Components/ui/table'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Resource = {
    id: string
    name: string
    resourceType: string
    engine: string
    originAddress: string
    originPort: number
    health: string
  }

  let { auth, resources }: { auth: { email: string }; resources: Resource[] } = $props()
  const label = (value: string) => value ? value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()) : 'Unknown'
  const endpoint = (address: string, port: number) => address && port ? `${address}:${port}` : 'Not configured'
</script>

<svelte:head><title>System resources</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight">Resources</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Durable infrastructure services managed by DeployCrate.
      </p>
    </header>

    <Card.Root>
      <Card.Content class="p-0">
        {#if resources.length === 0}
          <Empty.Root class="py-12"><Empty.Header><Empty.Title>No active system Resources</Empty.Title><Empty.Description>Durable infrastructure services will appear here when the system creates them.</Empty.Description></Empty.Header></Empty.Root>
        {:else}
          <div class="overflow-x-auto">
            <Table.Root>
              <Table.Header class="bg-muted/30">
                <Table.Row><Table.Head>Resource</Table.Head><Table.Head>Type</Table.Head><Table.Head>Engine</Table.Head><Table.Head>Origin</Table.Head><Table.Head>Health</Table.Head></Table.Row>
              </Table.Header>
              <Table.Body>
                {#each resources as resource (resource.id)}
                  <Table.Row>
                    <Table.Cell><Link class="font-medium text-primary hover:underline" href={routes.systemResource(resource.id)}>{resource.name}</Link></Table.Cell>
                    <Table.Cell>{label(resource.resourceType)}</Table.Cell><Table.Cell>{label(resource.engine)}</Table.Cell>
                    <Table.Cell class="font-mono text-xs">{endpoint(resource.originAddress, resource.originPort)}</Table.Cell><Table.Cell><StatusBadge status={resource.health} /></Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
