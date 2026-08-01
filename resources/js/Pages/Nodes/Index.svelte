<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Node = { id: string; name: string; address: string; state: string; currentStep: string; wireGuardAddress: string; configured: boolean; createdAt: string; capabilities: Record<string, boolean> }
  let { auth, nodes }: { auth: { email: string }; nodes: Node[] } = $props()

  function stateClass(state: string) {
    if (state === 'ready') return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'
    if (state === 'failed') return 'border-red-500/30 bg-red-500/10 text-red-400'
    return 'border-amber-500/30 bg-amber-500/10 text-amber-400'
  }
</script>

<svelte:head><title>Nodes</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Infrastructure</p>
        <h1 class="mt-3 text-3xl font-semibold">Nodes</h1>
        <p class="mt-3 max-w-2xl text-sm text-muted-foreground">Worker servers enrolled into the private WireGuard network.</p>
      </div>
      <Button href={routes.nodeNew()}>Add node</Button>
    </header>

    <Card.Root>
      <Card.Header><Card.Title>Managed nodes</Card.Title><Card.Description>{nodes.length} worker node{nodes.length === 1 ? '' : 's'} registered.</Card.Description></Card.Header>
      <Card.Content>
        <div class="divide-y divide-border border border-border">
          {#each nodes as node (node.id)}
            <Link href={routes.nodeShow(node.id)} class="flex flex-col gap-3 p-4 hover:bg-muted/20 sm:flex-row sm:items-center sm:justify-between">
              <div><p class="font-medium">{node.name}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{node.address} · {node.wireGuardAddress}</p><p class="mt-2 text-xs capitalize text-muted-foreground">{Object.entries(node.capabilities).filter(([key, enabled]) => key !== 'telemetry' && enabled).map(([key]) => key).join(' · ')}</p></div>
              <div class="text-left sm:text-right"><span class={`inline-flex border px-2 py-0.5 text-xs capitalize ${stateClass(node.state)}`}>{node.state.replaceAll('_', ' ')}</span><p class="mt-1 text-xs text-muted-foreground">{node.currentStep.replaceAll('_', ' ')}</p></div>
            </Link>
          {:else}
            <p class="p-10 text-center text-sm text-muted-foreground">No worker nodes have been added.</p>
          {/each}
        </div>
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
