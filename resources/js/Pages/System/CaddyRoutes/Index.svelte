<script lang="ts">
  import RouteIcon from "@lucide/svelte/icons/route";
  import PlusIcon from "@lucide/svelte/icons/plus";
  import { router, useForm } from "@inertiajs/svelte";

  import FormField from "@/Components/FormField.svelte";
  import { Button } from "@/Components/ui/button";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
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
  type DomainOption = {
    id: string;
    hostname: string;
    environmentId: string;
    environmentName: string;
    applicationName: string;
  };
  type TargetOption = { id: string; environmentId: string; serverName: string };
  type ReleaseOption = {
    id: string;
    environmentId: string;
    label: string;
    artifactReference: string;
  };
  type InstanceOption = {
    id: string;
    environmentId: string;
    environmentTargetId: string;
    releaseId: string;
    externalId: string;
    slot: string;
    state: string;
    address: string;
  };
  type RouteInput = {
    externalId: string;
    environmentDomainId: string;
    environmentTargetId: string;
    releaseId: string;
    backends: { instanceId: string; weight: number }[];
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

  let {
    auth,
    routes: caddyRoutes,
    resourceRoutes,
    options,
  }: {
    auth: { email: string };
    routes: CaddyRoute[];
    resourceRoutes: ResourceRoute[];
    options: {
      domains: DomainOption[];
      targets: TargetOption[];
      releases: ReleaseOption[];
      instances: InstanceOption[];
    };
  } = $props();

  const form = useForm<RouteInput>(() => {
    const domain = options.domains[0];
    return {
      externalId: domain ? routeIDForHostname(domain.hostname) : "",
      environmentDomainId: domain?.id ?? "",
      environmentTargetId: "",
      releaseId: "",
      backends: [],
    };
  });
  let showCreate = $state(false);

  const createEnvironmentID = $derived(
    options.domains.find((domain) => domain.id === $form.environmentDomainId)
      ?.environmentId ?? "",
  );
  const createTargets = $derived(
    options.targets.filter(
      (target) => target.environmentId === createEnvironmentID,
    ),
  );
  const createReleases = $derived(
    options.releases.filter(
      (release) => release.environmentId === createEnvironmentID,
    ),
  );
  const createInstances = $derived(
    options.instances.filter(
      (instance) => instance.environmentTargetId === $form.environmentTargetId,
    ),
  );

  $effect(() => {
    if (
      createTargets.length > 0 &&
      !createTargets.some((target) => target.id === $form.environmentTargetId)
    ) {
      $form.environmentTargetId = createTargets[0].id;
      $form.backends = [];
    }
    if (
      createReleases.length > 0 &&
      !createReleases.some((release) => release.id === $form.releaseId)
    ) {
      $form.releaseId = createReleases[0].id;
    }
  });

  function defaultExternalID(domainID: string) {
    const domain = options.domains.find((item) => item.id === domainID);
    if (!domain || $form.externalId) return;
    $form.externalId = routeIDForHostname(domain.hostname);
  }

  function routeIDForHostname(hostname: string) {
    return `deploycrate_route_${hostname.toLowerCase().replaceAll(".", "_").replaceAll("-", "_")}`;
  }

  function selectCreateDomain(domainID: string) {
    $form.environmentDomainId = domainID;
    $form.environmentTargetId = "";
    $form.releaseId = "";
    $form.backends = [];
    defaultExternalID(domainID);
  }

  function selectCreateTarget(targetID: string) {
    $form.environmentTargetId = targetID;
    $form.backends = [];
  }

  function toggleBackend(
    input: RouteInput,
    instanceID: string,
    selected: boolean,
  ) {
    if (selected) {
      input.backends = [
        ...input.backends,
        {
          instanceId: instanceID,
          weight: input.backends.length === 0 ? 100 : 0,
        },
      ];
    } else {
      input.backends = input.backends.filter(
        (backend) => backend.instanceId !== instanceID,
      );
    }
  }

  function backendSelected(input: RouteInput, instanceID: string) {
    return input.backends.some((backend) => backend.instanceId === instanceID);
  }

  function backendWeight(input: RouteInput, instanceID: string) {
    return (
      input.backends.find((backend) => backend.instanceId === instanceID)
        ?.weight ?? 0
    );
  }

  function setBackendWeight(
    input: RouteInput,
    instanceID: string,
    weight: number,
  ) {
    input.backends = input.backends.map((backend) =>
      backend.instanceId === instanceID ? { ...backend, weight } : backend,
    );
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

    {#if caddyRoutes.length === 0 && resourceRoutes.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"
        ><Empty.Header
          ><Empty.Media variant="icon"><RouteIcon /></Empty.Media><Empty.Title
            >No Caddy routes</Empty.Title
          ><Empty.Description
            >Create a route to send an Environment domain to its active release.</Empty.Description
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
            >Select one Environment domain and the running instances that should
            receive its traffic.</Dialog.Description
          ></Dialog.Header
        >
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Domain"
            ><NativeSelect.Root
              class="w-full"
              value={$form.environmentDomainId}
              onchange={(event) =>
                selectCreateDomain(event.currentTarget.value)}
              required
              >{#each options.domains as domain}<NativeSelect.Option
                  value={domain.id}
                  >{domain.hostname} · {domain.applicationName} / {domain.environmentName}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Caddy route ID"
            ><Input
              bind:value={$form.externalId}
              required
              placeholder="deploycrate_route_example_com"
            /></FormField
          >
          <FormField label="Target"
            ><NativeSelect.Root
              class="w-full"
              value={$form.environmentTargetId}
              onchange={(event) =>
                selectCreateTarget(event.currentTarget.value)}
              required
              >{#each createTargets as target}<NativeSelect.Option
                  value={target.id}>{target.serverName}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Active release"
            ><NativeSelect.Root
              class="w-full"
              bind:value={$form.releaseId}
              required
              >{#each createReleases as release}<NativeSelect.Option
                  value={release.id}>{release.label}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <div class="sm:col-span-2">
            <p class="mb-2 text-xs font-medium">Backends and weights</p>
            <div class="divide-y divide-border border border-border">
              {#each createInstances as instance}<div
                  class="grid grid-cols-[auto_1fr_7rem] items-center gap-3 p-3"
                >
                  <Checkbox
                    checked={backendSelected($form, instance.id)}
                    onCheckedChange={(checked) =>
                      toggleBackend($form, instance.id, checked)}
                    aria-label={`Use ${instance.externalId} as a backend`}
                  />
                  <div>
                    <p class="font-mono text-xs">{instance.externalId}</p>
                    <div class="mt-1 flex flex-wrap items-center gap-2">
                      <span class="text-xs text-muted-foreground"
                        >{instance.address} · {instance.slot}</span
                      ><StatusBadge status={instance.state} />
                    </div>
                  </div>
                  <Input
                    type="number"
                    min="0"
                    max="100"
                    disabled={!backendSelected($form, instance.id)}
                    value={backendWeight($form, instance.id)}
                    oninput={(event) =>
                      setBackendWeight(
                        $form,
                        instance.id,
                        Number(event.currentTarget.value),
                      )}
                  />
                </div>{:else}<p class="p-4 text-sm text-muted-foreground">
                  No active instances are available on this target.
                </p>{/each}
            </div>
            <p class="mt-2 text-xs text-muted-foreground">
              Selected weights must total 100.
            </p>
          </div>
        </div>
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$form.processing}
            onclick={() => (showCreate = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$form.processing || $form.backends.length === 0}
            aria-busy={$form.processing}
            >{#if $form.processing}<Spinner />{/if}Create and apply</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
