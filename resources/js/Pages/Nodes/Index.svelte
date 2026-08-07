<script lang="ts">
  import ServerIcon from "@lucide/svelte/icons/server";
  import { Link, router } from "@inertiajs/svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Empty from "@/Components/ui/empty";
  import * as Table from "@/Components/ui/table";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Node = {
    id: string;
    name: string;
    address: string;
    state: string;
    currentStep: string;
    wireGuardAddress: string;
    configured: boolean;
    createdAt: string;
    capabilities: Record<string, boolean>;
  };
  let { auth, nodes }: { auth: { email: string }; nodes: Node[] } = $props();
</script>

<svelte:head><title>Nodes</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Infrastructure"
      title="Nodes"
      description="Worker servers enrolled into the private WireGuard network."
    >
      {#snippet actions()}<Button
          >{#snippet child({ props })}<Link {...props} href={routes.nodeNew()}
              >Add node</Link
            >{/snippet}</Button
        >{/snippet}
    </PageHeader>

    {#if nodes.length === 0}
      <Empty.Root class="border border-dashed border-border py-14">
        <Empty.Header
          ><Empty.Media variant="icon"><ServerIcon /></Empty.Media><Empty.Title
            >No worker nodes</Empty.Title
          ><Empty.Description
            >Add a Debian 13 VPS to run builds, applications, or managed
            Resources.</Empty.Description
          ></Empty.Header
        >
      </Empty.Root>
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Name</Table.Head><Table.Head>Address</Table.Head
              ><Table.Head>State</Table.Head></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each nodes as node (node.id)}
              <Table.Row
                class="cursor-pointer"
                onclick={() => router.visit(routes.nodeShow(node.id))}
              >
                <Table.Cell class="font-medium">{node.name}</Table.Cell>
                <Table.Cell class="font-mono text-xs">{node.address}</Table.Cell
                >
                <Table.Cell><StatusBadge status={node.state} /></Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {/if}
  </div>
</DashboardLayout>
