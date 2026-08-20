<script lang="ts">
  import RouteIcon from "@lucide/svelte/icons/route";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import { router, useForm } from "@inertiajs/svelte";

  import FormField from "@/Components/FormField.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Backend = {
    instanceId: string;
    externalId: string;
    slot: string;
    state: string;
    address: string;
    weight: number;
  };
  type CaddyRoute = {
    id: string;
    externalId: string;
    state: string;
    hostname: string;
    applicationName: string;
    environmentName: string;
    environmentId: string;
    environmentDomainId: string;
    environmentTargetId: string;
    releaseId: string;
    releaseLabel: string;
    serverName: string;
    healthPath: string;
    appliedAt: string;
    observedAt: string;
    backends: Backend[];
  };
  type RouteInput = {
    externalId: string;
    hostname: string;
    originAddress: string;
    originPort: number;
    originProtocol: string;
    originTlsMode: string;
    healthPath: string;
  };
  type ResourceRoute = {
    id: string;
    externalId: string;
    hostname: string;
    state: string;
    lastError: string;
    resourceId: string;
    resourceName: string;
    endpointName: string;
    origin: string;
    appliedAt: string;
    observedAt: string;
  };
  type CustomRoute = {
    id: string;
    externalId: string;
    hostname: string;
    origin: string;
    state: string;
    lastError: string;
  };

  let {
    auth,
    routes: caddyRoutes,
    resourceRoutes,
    customRoutes,
  }: {
    auth: { email: string };
    routes: CaddyRoute[];
    resourceRoutes: ResourceRoute[];
    customRoutes: CustomRoute[];
  } = $props();

  const form = useForm<RouteInput>(() => {
    return {
      externalId: "",
      hostname: "",
      originAddress: "127.0.0.1",
      originPort: 8080,
      originProtocol: "http",
      originTlsMode: "disable",
      healthPath: "",
    };
  });
  let showCreate = $state(false);

  function routeIDForHostname(hostname: string) {
    return `deploycrate_route_${hostname.toLowerCase().replaceAll(".", "_").replaceAll("-", "_")}`;
  }

  function updateDefaultExternalID() {
    $form.externalId = routeIDForHostname($form.hostname);
  }

  function submitCreate(event: SubmitEvent) {
    event.preventDefault();
    $form.post(routes.caddyRouteCreate(), {
      preserveScroll: true,
      onSuccess: () => {
        showCreate = false;
        $form.reset();
      },
    });
  }
</script>

<svelte:head><title>Caddy routes</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Infrastructure
        </p>
        <h1 class="mt-3 text-3xl font-semibold">Caddy routes</h1>
        <p class="mt-2 max-w-2xl text-sm text-muted-foreground">
          DeployCrate manages these HTTPS routes and reconciles them to Caddy
          immediately.
        </p>
      </div>
      <Button onclick={() => (showCreate = true)}><PlusIcon />New route</Button>
    </header>

    {#if caddyRoutes.length === 0 && resourceRoutes.length === 0 && customRoutes.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"
        ><Empty.Header
          ><Empty.Media variant="icon"><RouteIcon /></Empty.Media><Empty.Title
            >No Caddy routes</Empty.Title
          ><Empty.Description
            >Create a direct route from any hostname to an HTTP origin.</Empty.Description
          ></Empty.Header
        ></Empty.Root
      >
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Hostname</Table.Head><Table.Head
                >Managed for</Table.Head
              ><Table.Head>Type</Table.Head><Table.Head>State</Table.Head
              ></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each caddyRoutes as route (route.externalId)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.caddyRouteShow(route.externalId))}
              >
                <Table.Cell class="font-medium">{route.hostname}</Table.Cell>
                <Table.Cell
                  >{route.applicationName} / {route.environmentName}</Table.Cell
                >
                <Table.Cell>Environment</Table.Cell>
                <Table.Cell><StatusBadge status={route.state} /></Table.Cell>
              </Table.Row>
            {/each}
            {#each resourceRoutes as route (route.externalId)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.caddyRouteShow(route.externalId))}
              >
                <Table.Cell class="font-medium">{route.hostname}</Table.Cell>
                <Table.Cell
                  >{route.resourceName} / {route.endpointName}</Table.Cell
                >
                <Table.Cell>Resource</Table.Cell>
                <Table.Cell><StatusBadge status={route.state} /></Table.Cell>
              </Table.Row>
            {/each}
            {#each customRoutes as route (route.externalId)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.caddyRouteShow(route.externalId))}
              >
                <Table.Cell class="font-medium">{route.hostname}</Table.Cell>
                <Table.Cell>{route.origin}</Table.Cell>
                <Table.Cell>Direct</Table.Cell>
                <Table.Cell><StatusBadge status={route.state} /></Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {/if}
  </div>

  <Dialog.Root bind:open={showCreate}>
    <Dialog.Content class="sm:max-w-3xl">
      <form class="space-y-5" onsubmit={submitCreate}>
        <Dialog.Header
          ><Dialog.Title>New Caddy route</Dialog.Title><Dialog.Description
            >Route any public hostname directly to an HTTP or HTTPS origin. No
            Application, domain registration, or Release association is required.</Dialog.Description
          ></Dialog.Header
        >
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Public hostname"
            ><Input
              bind:value={$form.hostname}
              onblur={updateDefaultExternalID}
              required
              placeholder="service.example.com"
            /></FormField
          ><FormField label="Caddy route ID"
            ><Input
              bind:value={$form.externalId}
              required
              placeholder="deploycrate_route_example_com"
            /></FormField
          >
          <FormField label="Origin address"
            ><Input bind:value={$form.originAddress} required placeholder="127.0.0.1" /></FormField
          ><FormField label="Origin port"
            ><Input type="number" bind:value={$form.originPort} min="1" max="65535" required /></FormField
          ><FormField label="Origin protocol"
            ><select bind:value={$form.originProtocol} class="h-9 w-full border border-input bg-transparent px-3 text-sm">
              <option value="http">HTTP</option><option value="https">HTTPS</option>
            </select></FormField
          ><FormField label="Origin TLS mode"
            ><select bind:value={$form.originTlsMode} class="h-9 w-full border border-input bg-transparent px-3 text-sm">
              <option value="disable">Disable</option><option value="require">Require</option>
              <option value="verify-ca">Verify CA</option><option value="verify-full">Verify full</option>
            </select></FormField
          ><FormField label="Health path"
            ><Input bind:value={$form.healthPath} placeholder="Optional, for example /health" /></FormField
          >
        </div>
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$form.processing}
            onclick={() => (showCreate = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$form.processing}
            aria-busy={$form.processing}
            >{#if $form.processing}<Spinner />{/if}Create and apply</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
