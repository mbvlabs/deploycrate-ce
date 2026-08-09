<script lang="ts">
  import { router } from "@inertiajs/svelte";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import FormField from "@/Components/FormField.svelte";
  import JsonCode from "@/Components/JsonCode.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as Dialog from "@/Components/ui/dialog";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
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
  type EnvironmentRoute = {
    id: string;
    externalId: string;
    environmentDomainId: string;
    environmentTargetId: string;
    releaseId: string;
    backends: Backend[];
  };
  type ResourceRoute = {
    id: string;
    hostname: string;
    originAddress: string;
    originPort: number;
    originProtocol: string;
    originTlsMode: string;
    healthPath: string;
  };
  type DomainOption = {
    id: string;
    hostname: string;
    environmentId: string;
    environmentName: string;
    applicationName: string;
  };
  type TargetOption = { id: string; environmentId: string; serverName: string };
  type ReleaseOption = { id: string; environmentId: string; label: string };
  type InstanceOption = {
    id: string;
    environmentId: string;
    environmentTargetId: string;
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
  type ResourceRouteInput = {
    hostname: string;
    originAddress: string;
    originPort: number;
    originProtocol: string;
    originTlsMode: string;
    healthPath: string;
  };
  type RouteDetail = {
    externalId: string;
    kind: string;
    hostname: string;
    state: string;
    lastError: string;
    source: string;
    target: string;
    healthPath: string;
    appliedAt: string;
    observedAt: string;
    backends: Backend[];
    configuration: unknown;
    configurationError: string;
    environmentRoute?: EnvironmentRoute;
    resourceRoute?: ResourceRoute;
    options: {
      domains: DomainOption[];
      targets: TargetOption[];
      releases: ReleaseOption[];
      instances: InstanceOption[];
    };
  };

  let { auth, route }: { auth: { email: string }; route: RouteDetail } =
    $props();
  let editOpen = $state(false);
  let editProcessing = $state(false);
  let resourceEditOpen = $state(false);
  let resourceEditProcessing = $state(false);
  let deleteOpen = $state(false);
  let deleteProcessing = $state(false);
  let editDraft = $state<RouteInput | null>(null);
  let resourceEditDraft = $state<ResourceRouteInput | null>(null);

  const editEnvironmentID = $derived(
    route.options.domains.find(
      (domain) => domain.id === editDraft?.environmentDomainId,
    )?.environmentId ?? "",
  );
  const editTargets = $derived(
    route.options.targets.filter(
      (target) => target.environmentId === editEnvironmentID,
    ),
  );
  const editReleases = $derived(
    route.options.releases.filter(
      (release) => release.environmentId === editEnvironmentID,
    ),
  );
  const editInstances = $derived(
    route.options.instances.filter(
      (instance) =>
        instance.environmentTargetId === editDraft?.environmentTargetId,
    ),
  );

  function formatTime(value: string) {
    return value
      ? new Intl.DateTimeFormat(undefined, {
          dateStyle: "medium",
          timeStyle: "short",
        }).format(new Date(value))
      : "Never";
  }

  function beginEdit() {
    if (route.environmentRoute) {
      editDraft = {
        externalId: route.environmentRoute.externalId,
        environmentDomainId: route.environmentRoute.environmentDomainId,
        environmentTargetId: route.environmentRoute.environmentTargetId,
        releaseId: route.environmentRoute.releaseId,
        backends: route.environmentRoute.backends.map((backend) => ({
          instanceId: backend.instanceId,
          weight: backend.weight,
        })),
      };
      editOpen = true;
      return;
    }
    if (route.resourceRoute) {
      resourceEditDraft = {
        hostname: route.resourceRoute.hostname,
        originAddress: route.resourceRoute.originAddress,
        originPort: route.resourceRoute.originPort,
        originProtocol: route.resourceRoute.originProtocol,
        originTlsMode: route.resourceRoute.originTlsMode,
        healthPath: route.resourceRoute.healthPath,
      };
      resourceEditOpen = true;
    }
  }

  function selectEditTarget(targetID: string) {
    if (!editDraft) return;
    editDraft.environmentTargetId = targetID;
    editDraft.backends = [];
  }

  function selectEditDomain(domainID: string) {
    if (!editDraft) return;
    const environmentID =
      route.options.domains.find((domain) => domain.id === domainID)
        ?.environmentId ?? "";
    const targets = route.options.targets.filter(
      (target) => target.environmentId === environmentID,
    );
    const releases = route.options.releases.filter(
      (release) => release.environmentId === environmentID,
    );
    editDraft.environmentDomainId = domainID;
    editDraft.environmentTargetId = targets[0]?.id ?? "";
    editDraft.releaseId = releases[0]?.id ?? "";
    editDraft.backends = [];
  }

  function toggleBackend(instanceID: string, selected: boolean) {
    if (!editDraft) return;
    editDraft.backends = selected
      ? [
          ...editDraft.backends,
          {
            instanceId: instanceID,
            weight: editDraft.backends.length === 0 ? 100 : 0,
          },
        ]
      : editDraft.backends.filter(
          (backend) => backend.instanceId !== instanceID,
        );
  }

  function backendSelected(instanceID: string) {
    return (
      editDraft?.backends.some(
        (backend) => backend.instanceId === instanceID,
      ) ?? false
    );
  }
  function backendWeight(instanceID: string) {
    return (
      editDraft?.backends.find((backend) => backend.instanceId === instanceID)
        ?.weight ?? 0
    );
  }
  function setBackendWeight(instanceID: string, weight: number) {
    if (editDraft)
      editDraft.backends = editDraft.backends.map((backend) =>
        backend.instanceId === instanceID ? { ...backend, weight } : backend,
      );
  }

  function saveEdit() {
    if (!route.environmentRoute || !editDraft || editProcessing) return;
    editProcessing = true;
    router.patch(
      `${routes.caddyRouteUpdate(route.environmentRoute.id)}?returnTo=details`,
      editDraft,
      {
        preserveScroll: true,
        onSuccess: () => (editOpen = false),
        onFinish: () => (editProcessing = false),
      },
    );
  }

  function saveResourceEdit() {
    if (!route.resourceRoute || !resourceEditDraft || resourceEditProcessing)
      return;
    resourceEditProcessing = true;
    router.patch(
      routes.caddyResourceRouteUpdate(route.resourceRoute.id),
      resourceEditDraft,
      {
        preserveScroll: true,
        onSuccess: () => (resourceEditOpen = false),
        onFinish: () => (resourceEditProcessing = false),
      },
    );
  }

  function confirmDelete() {
    if (!route.environmentRoute || deleteProcessing) return;
    deleteProcessing = true;
    router.delete(routes.caddyRouteDestroy(route.environmentRoute.id), {
      onSuccess: () => (deleteOpen = false),
      onFinish: () => (deleteProcessing = false),
    });
  }
</script>

<svelte:head><title>{route.hostname} · Caddy route</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Infrastructure · Caddy
        </p>
        <h1 class="mt-3 text-3xl font-semibold">{route.hostname}</h1>
        <p class="mt-2 break-all font-mono text-xs text-muted-foreground">
          {route.externalId}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        {#if route.environmentRoute || route.resourceRoute}<Button
            variant="outline"
            onclick={beginEdit}>Edit</Button
          >{/if}{#if route.environmentRoute}<Button
            variant="destructive"
            onclick={() => (deleteOpen = true)}>Delete</Button
          >{/if}<Button href={routes.caddyRoutes()} variant="outline"
          >Back to Caddy</Button
        >
      </div>
    </header>

    <Card.Root>
      <Card.Header
        ><Card.Action><StatusBadge status={route.state} /></Card.Action
        ><Card.Title>Route details</Card.Title><Card.Description
          >DeployCrate desired state and its current Caddy observation.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        <dl class="grid gap-5 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <dt class="text-muted-foreground">Kind</dt>
            <dd class="mt-1 capitalize">{route.kind}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Managed for</dt>
            <dd class="mt-1">{route.source}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Target</dt>
            <dd class="mt-1 font-mono text-xs">{route.target}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Health path</dt>
            <dd class="mt-1 font-mono text-xs">{route.healthPath || "None"}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Applied</dt>
            <dd class="mt-1">{formatTime(route.appliedAt)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Observed</dt>
            <dd class="mt-1">{formatTime(route.observedAt)}</dd>
          </div>
        </dl>
        {#if route.lastError}<p
            class="mt-5 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
          >
            {route.lastError}
          </p>{/if}
      </Card.Content>
    </Card.Root>

    {#if route.backends.length > 0}
      <Card.Root
        ><Card.Header><Card.Title>Backends</Card.Title></Card.Header
        ><Card.Content
          ><div class="overflow-x-auto border border-border">
            <Table.Root
              ><Table.Header
                ><Table.Row
                  ><Table.Head>Backend</Table.Head><Table.Head
                    >Address</Table.Head
                  ><Table.Head>State</Table.Head><Table.Head class="text-right"
                    >Weight</Table.Head
                  ></Table.Row
                ></Table.Header
              ><Table.Body
                >{#each route.backends as backend (backend.instanceId)}<Table.Row
                    ><Table.Cell class="font-mono text-xs"
                      >{backend.externalId || "Origin"}</Table.Cell
                    ><Table.Cell class="font-mono text-xs"
                      >{backend.address}</Table.Cell
                    ><Table.Cell
                      >{#if backend.state}<StatusBadge
                          status={backend.state}
                        />{:else}<span class="text-xs text-muted-foreground"
                          >Not observed</span
                        >{/if}</Table.Cell
                    ><Table.Cell class="text-right tabular-nums"
                      >{backend.weight}</Table.Cell
                    ></Table.Row
                  >{/each}</Table.Body
              ></Table.Root
            >
          </div></Card.Content
        ></Card.Root
      >
    {/if}

    <Card.Root>
      <Card.Header
        ><Card.Title>Live Caddy route JSON</Card.Title><Card.Description
          >Read directly from the local Caddy Admin API using this route ID.</Card.Description
        ></Card.Header
      >
      <Card.Content
        >{#if route.configurationError}<p
            class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
          >
            {route.configurationError}
          </p>{:else}<JsonCode
            value={route.configuration}
            expanded
          />{/if}</Card.Content
      >
    </Card.Root>
  </div>

  <Dialog.Root
    bind:open={editOpen}
    onOpenChange={(open) => {
      if (!open && !editProcessing) editDraft = null;
    }}
  >
    <Dialog.Content class="sm:max-w-3xl">
      <Dialog.Header
        ><Dialog.Title>Edit Caddy route</Dialog.Title><Dialog.Description
          >Update the Environment target, release, and backend weights.</Dialog.Description
        ></Dialog.Header
      >
      {#if editDraft}
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Domain"
            ><NativeSelect.Root
              class="w-full"
              value={editDraft.environmentDomainId}
              onchange={(event) => selectEditDomain(event.currentTarget.value)}
              >{#each route.options.domains as domain (domain.id)}<NativeSelect.Option
                  value={domain.id}
                  >{domain.hostname} · {domain.applicationName} / {domain.environmentName}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Caddy route ID"
            ><Input bind:value={editDraft.externalId} readonly /></FormField
          >
          <FormField label="Target"
            ><NativeSelect.Root
              class="w-full"
              value={editDraft.environmentTargetId}
              onchange={(event) => selectEditTarget(event.currentTarget.value)}
              >{#each editTargets as target (target.id)}<NativeSelect.Option
                  value={target.id}>{target.serverName}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Active release"
            ><NativeSelect.Root class="w-full" bind:value={editDraft.releaseId}
              >{#each editReleases as release (release.id)}<NativeSelect.Option
                  value={release.id}>{release.label}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <div class="sm:col-span-2">
            <p class="mb-2 text-xs font-medium">Backends and weights</p>
            <div class="divide-y divide-border border border-border">
              {#each editInstances as instance (instance.id)}<div
                  class="grid grid-cols-[auto_1fr_7rem] items-center gap-3 p-3"
                >
                  <Checkbox
                    checked={backendSelected(instance.id)}
                    onCheckedChange={(checked) =>
                      toggleBackend(instance.id, checked === true)}
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
                    disabled={!backendSelected(instance.id)}
                    value={backendWeight(instance.id)}
                    oninput={(event) =>
                      setBackendWeight(
                        instance.id,
                        Number(event.currentTarget.value),
                      )}
                  />
                </div>{/each}
            </div>
          </div>
        </div>
        <Dialog.Footer
          ><Button
            variant="outline"
            disabled={editProcessing}
            onclick={() => (editOpen = false)}>Cancel</Button
          ><Button
            disabled={editProcessing || editDraft.backends.length === 0}
            aria-busy={editProcessing}
            onclick={saveEdit}
            >{#if editProcessing}<Spinner />{/if}Save and apply</Button
          ></Dialog.Footer
        >
      {/if}
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root
    bind:open={resourceEditOpen}
    onOpenChange={(open) => {
      if (!open && !resourceEditProcessing) resourceEditDraft = null;
    }}
  >
    <Dialog.Content class="sm:max-w-3xl">
      <Dialog.Header
        ><Dialog.Title>Edit Resource Caddy route</Dialog.Title
        ><Dialog.Description
          >Update the public hostname and HTTP origin used by Caddy.</Dialog.Description
        ></Dialog.Header
      >
      {#if resourceEditDraft}
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Public hostname"
            ><Input
              bind:value={resourceEditDraft.hostname}
              required
              placeholder="telemetry.example.com"
            /></FormField
          >
          <FormField label="Health path"
            ><Input
              bind:value={resourceEditDraft.healthPath}
              placeholder="Optional, for example /health"
            /></FormField
          >
          <FormField label="Origin address"
            ><Input
              bind:value={resourceEditDraft.originAddress}
              required
              placeholder="127.0.0.1"
            /></FormField
          >
          <FormField label="Origin port"
            ><Input
              type="number"
              min="1"
              max="65535"
              bind:value={resourceEditDraft.originPort}
              required
            /></FormField
          >
          <FormField label="Origin protocol"
            ><NativeSelect.Root
              class="w-full"
              bind:value={resourceEditDraft.originProtocol}
              ><NativeSelect.Option value="http">HTTP</NativeSelect.Option
              ><NativeSelect.Option value="https">HTTPS</NativeSelect.Option
              ></NativeSelect.Root
            ></FormField
          >
          <FormField label="Origin TLS mode"
            ><NativeSelect.Root
              class="w-full"
              bind:value={resourceEditDraft.originTlsMode}
              ><NativeSelect.Option value="disable">Disable</NativeSelect.Option
              ><NativeSelect.Option value="prefer">Prefer</NativeSelect.Option
              ><NativeSelect.Option value="require">Require</NativeSelect.Option
              ><NativeSelect.Option value="verify-ca"
                >Verify CA</NativeSelect.Option
              ><NativeSelect.Option value="verify-full"
                >Verify full</NativeSelect.Option
              ></NativeSelect.Root
            ></FormField
          >
        </div>
        <Dialog.Footer
          ><Button
            variant="outline"
            disabled={resourceEditProcessing}
            onclick={() => (resourceEditOpen = false)}>Cancel</Button
          ><Button
            disabled={resourceEditProcessing}
            aria-busy={resourceEditProcessing}
            onclick={saveResourceEdit}
            >{#if resourceEditProcessing}<Spinner />{/if}Save and apply</Button
          ></Dialog.Footer
        >
      {/if}
    </Dialog.Content>
  </Dialog.Root>

  <ConfirmActionDialog
    bind:open={deleteOpen}
    title="Delete Caddy route?"
    description={`Remove ${route.hostname} from Caddy and mark its desired state as removed.`}
    confirmLabel="Delete route"
    requiredPhrase="DELETE"
    destructive
    processing={deleteProcessing}
    onconfirm={confirmDelete}
  />
</DashboardLayout>
