<script lang="ts">
  import AppWindowIcon from "@lucide/svelte/icons/app-window";
  import { Link, router } from "@inertiajs/svelte";
  import { Button } from "@/Components/ui/button";
  import * as Empty from "@/Components/ui/empty";
  import * as Table from "@/Components/ui/table";
  import PageHeader from "@/Components/PageHeader.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Environment = {
    id: string;
    environmentName: string;
    environmentKind: string;
    repositoryFullName: string;
    reference: string;
    sourceHealthy: boolean;
    sourceType: string;
  };
  type Application = {
    id: string;
    name: string;
    slug: string;
    environments: Environment[];
  };
  let {
    auth,
    applications,
  }: { auth: { email: string }; applications: Application[] } = $props();

  function applicationStatus(application: Application) {
    return application.environments.some(
      (environment) => !environment.sourceHealthy,
    )
      ? "degraded"
      : "ready";
  }
</script>

<svelte:head><title>Applications</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Applications"
      title="Build-ready services"
      description="Create and manage deployable services, their source configuration, and runtime environments."
    >
      {#snippet actions()}<Button
          >{#snippet child({ props })}<Link
              {...props}
              href={routes.applicationNew()}>New application</Link
            >{/snippet}</Button
        >{/snippet}
    </PageHeader>
    {#if applications.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"
        ><Empty.Header
          ><Empty.Media variant="icon"><AppWindowIcon /></Empty.Media
          ><Empty.Title>No applications yet</Empty.Title><Empty.Description
            >Create an application to configure its source, environments, and
            deployment targets.</Empty.Description
          ></Empty.Header
        ></Empty.Root
      >
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Name</Table.Head><Table.Head>Environments</Table.Head
              ><Table.Head>Status</Table.Head></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each applications as application (application.id)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.applicationShow(application.id))}
              >
                <Table.Cell class="font-medium">{application.name}</Table.Cell>
                <Table.Cell
                  >{application.environments.length}
                  {application.environments.length === 1
                    ? "environment"
                    : "environments"}</Table.Cell
                >
                <Table.Cell
                  ><StatusBadge
                    status={applicationStatus(application)}
                    label={applicationStatus(application) === "ready"
                      ? "Sources ready"
                      : "Source degraded"}
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
