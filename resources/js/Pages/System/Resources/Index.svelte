<script lang="ts">
  import DatabaseIcon from '@lucide/svelte/icons/database'
  import { Link } from '@inertiajs/svelte'
  import PageHeader from '@/Components/PageHeader.svelte'
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
    <PageHeader eyebrow="System" title="Resources" description="Durable infrastructure services managed by DeployCrate." />

    {#if resources.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Media variant="icon"><DatabaseIcon /></Empty.Media><Empty.Title>No active system Resources</Empty.Title><Empty.Description>Durable infrastructure services will appear here when the system creates them.</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <Card.Root>
        <Card.Content class="p-0">
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
        </Card.Content>
      </Card.Root>
    {/if}
  </div>
</DashboardLayout>
