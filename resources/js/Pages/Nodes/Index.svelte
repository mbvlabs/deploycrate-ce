<script lang="ts">
  import ServerIcon from '@lucide/svelte/icons/server'
  import { Link } from '@inertiajs/svelte'
  import PageHeader from '@/Components/PageHeader.svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Node = { id: string; name: string; address: string; state: string; currentStep: string; wireGuardAddress: string; configured: boolean; createdAt: string; capabilities: Record<string, boolean> }
  let { auth, nodes }: { auth: { email: string }; nodes: Node[] } = $props()

</script>

<svelte:head><title>Nodes</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader eyebrow="Infrastructure" title="Nodes" description="Worker servers enrolled into the private WireGuard network.">
      {#snippet actions()}<Button>{#snippet child({ props })}<Link {...props} href={routes.nodeNew()}>Add node</Link>{/snippet}</Button>{/snippet}
    </PageHeader>

    {#if nodes.length}
      <Card.Root>
        <Card.Header><Card.Title>Managed nodes</Card.Title><Card.Description>{nodes.length} worker node{nodes.length === 1 ? '' : 's'} registered.</Card.Description></Card.Header>
        <Card.Content>
          <div class="divide-y divide-border border border-border">
            {#each nodes as node (node.id)}
              <Link href={routes.nodeShow(node.id)} class="flex flex-col gap-3 p-4 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between">
                <div><p class="font-medium">{node.name}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{node.address} · {node.wireGuardAddress}</p><p class="mt-2 text-xs capitalize text-muted-foreground">{Object.entries(node.capabilities).filter(([key, enabled]) => key !== 'telemetry' && enabled).map(([key]) => key).join(' · ')}</p></div>
                <div class="text-left sm:text-right"><StatusBadge status={node.state} /><p class="mt-1 text-xs text-muted-foreground">{node.currentStep.replaceAll('_', ' ')}</p></div>
              </Link>
            {/each}
          </div>
        </Card.Content>
      </Card.Root>
    {:else}
      <Empty.Root class="border border-dashed border-border py-14">
        <Empty.Header><Empty.Media variant="icon"><ServerIcon /></Empty.Media><Empty.Title>No worker nodes</Empty.Title><Empty.Description>Add a Debian 13 VPS to run builds, applications, or managed Resources.</Empty.Description></Empty.Header>
      </Empty.Root>
    {/if}
  </div>
</DashboardLayout>
