<script lang="ts">
  import DatabaseIcon from "@lucide/svelte/icons/database";
  import { Link, router } from "@inertiajs/svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import { Button } from "@/Components/ui/button";
  import { Badge } from "@/Components/ui/badge";
  import * as Empty from "@/Components/ui/empty";
  import * as Table from "@/Components/ui/table";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Resource = {
    id: string;
    name: string;
    resourceType: string;
    engine: string;
    systemManaged: boolean;
    environmentAttachable: boolean;
    databaseCount: number;
    connectionCount: number;
    installationCount: number;
    endpointCount: number;
    health: string;
  };
  let { auth, resources }: { auth: { email: string }; resources: Resource[] } =
    $props();
</script>

<svelte:head><title>Resources</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Resources"
      title="Infrastructure connections"
      description="Manage resource identity, desired placement, encrypted credentials, storage, and health configuration."
    >
      {#snippet actions()}<Button
          >{#snippet child({ props })}<Link
              {...props}
              href={routes.resourceNew()}>New resource</Link
            >{/snippet}</Button
        >{/snippet}
    </PageHeader>

    {#if resources.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"
        ><Empty.Header
          ><Empty.Media variant="icon"><DatabaseIcon /></Empty.Media
          ><Empty.Title>No resources yet</Empty.Title><Empty.Description
            >Deploy a database, cache, or service and manage its access from one
            place.</Empty.Description
          ></Empty.Header
        ></Empty.Root
      >
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Name</Table.Head><Table.Head>Engine</Table.Head
              ><Table.Head>Status</Table.Head></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each resources as resource (resource.id)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(
                    resource.systemManaged
                      ? routes.systemResource(resource.id)
                      : routes.resourceShow(resource.id),
                  )}
              >
                <Table.Cell
                  ><span class="font-medium">{resource.name}</span
                  >{#if resource.systemManaged}
                    <Badge variant="secondary" class="ml-2"
                      >System Resource</Badge
                    >{/if}</Table.Cell
                >
                <Table.Cell class="capitalize">{resource.engine}</Table.Cell>
                <Table.Cell
                  ><StatusBadge
                    status={resource.health || "unknown"}
                  /></Table.Cell
                >
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {/if}
  </div>
</DashboardLayout>
