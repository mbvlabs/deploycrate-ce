<script lang="ts">
  import { router, useForm } from "@inertiajs/svelte";
  import { untrack } from "svelte";
  import * as Alert from "@/Components/ui/alert";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Checkbox } from "@/Components/ui/checkbox";
  import { Progress } from "@/Components/ui/progress";
  import { Spinner } from "@/Components/ui/spinner";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type BuildpackRuntime = "go" | "rails" | "laravel" | "django";
  type NodeCapabilities = {
    build: boolean;
    runtime: boolean;
    resource: boolean;
    database: boolean;
    repository: boolean;
    telemetry: boolean;
    buildpacks?: { runtimes?: BuildpackRuntime[] };
  };
  type Node = {
    id: string;
    serverId: string;
    name: string;
    address: string;
    sshPort: number;
    state: string;
    currentStep: string;
    error: string;
    fingerprint: string;
    hostKeyConfirmedAt: string | null;
    wireGuardAddress: string;
    installerVersion: string;
    jobId: number | null;
    createdAt: string;
    startedAt: string | null;
    completedAt: string | null;
    configured: boolean;
    capabilities: NodeCapabilities;
  };
  const capabilityOptions = [
    ["build", "Builds"],
    ["runtime", "Applications"],
    ["resource", "Resources"],
    ["database", "Databases"],
    ["repository", "Repositories"],
  ] as const;
  const buildpackOptions: Array<[BuildpackRuntime, string]> = [
    ["go", "Go"],
    ["rails", "Rails (amd64)"],
    ["laravel", "Laravel (amd64)"],
    ["django", "Django"],
  ];
  let { auth, node }: { auth: { email: string }; node: Node } = $props();
  let confirmed = $state(false);
  let retrying = $state(false);
  let capabilitiesEditing = $state(false);
  let buildpackSelections = $state<Record<BuildpackRuntime, boolean>>(
    untrack(() => ({
      go: node.capabilities.buildpacks?.runtimes?.includes("go") ?? false,
      rails: node.capabilities.buildpacks?.runtimes?.includes("rails") ?? false,
      laravel:
        node.capabilities.buildpacks?.runtimes?.includes("laravel") ?? false,
      django:
        node.capabilities.buildpacks?.runtimes?.includes("django") ?? false,
    })),
  );
  const capabilitiesForm = useForm(() => ({
    build: node.capabilities.build,
    runtime: node.capabilities.runtime,
    resource: node.capabilities.resource,
    database: node.capabilities.database,
    repository: node.capabilities.repository,
    buildpacks: [] as BuildpackRuntime[],
  }));
  const confirmForm = useForm(() => ({ fingerprint: node.fingerprint }));
  function confirm() {
    $confirmForm
      .transform(() => ({ fingerprint: node.fingerprint }))
      .post(routes.nodeConfirm(node.id));
  }
  function retry() {
    retrying = true;
    router.post(
      routes.nodeRetry(node.id),
      {},
      { onFinish: () => (retrying = false) },
    );
  }
  function timestamp(value: string | null) {
    return value ? new Date(value).toLocaleString() : "Not yet";
  }
  function applyCapabilities(event: SubmitEvent) {
    event.preventDefault();
    $capabilitiesForm.buildpacks = buildpackOptions
      .filter(([runtime]) => buildpackSelections[runtime])
      .map(([runtime]) => runtime);
    $capabilitiesForm.post(routes.nodeCapabilities(node.id), {
      onSuccess: () => (capabilitiesEditing = false),
    });
  }
  const progressValue = $derived(
    node.state === "ready"
      ? 100
      : node.state === "awaiting_confirmation"
        ? 15
        : node.state === "failed"
          ? 100
          : 55,
  );

  $effect(() => {
    if (["awaiting_confirmation", "ready", "failed"].includes(node.state))
      return;
    const timer = window.setInterval(
      () => router.reload({ only: ["node"], preserveScroll: true }),
      3000,
    );
    return () => window.clearInterval(timer);
  });
</script>

<svelte:head><title>{node.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-6">
    <header
      class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
    >
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Node
        </p>
        <h1 class="mt-3 text-3xl font-semibold">{node.name}</h1>
        <p class="mt-2 font-mono text-xs text-muted-foreground">
          {node.address}:{node.sshPort} · {node.wireGuardAddress}
        </p>
      </div>
      <Button href={routes.nodes()} variant="outline">Back to nodes</Button>
    </header>

    {#if node.state === "awaiting_confirmation"}
      <Card.Root class="border-amber-500/30">
        <Card.Header
          ><Card.Title>Confirm SSH host identity</Card.Title><Card.Description
            >Compare this fingerprint with the value shown in your VPS provider
            console before continuing.</Card.Description
          ></Card.Header
        >
        <Card.Content class="space-y-4"
          ><div
            class="break-all border border-border bg-muted/20 p-4 font-mono text-sm"
          >
            {node.fingerprint}
          </div>
          <label class="flex cursor-pointer items-start gap-3 text-sm"
            ><Checkbox bind:checked={confirmed} class="mt-1" /><span
              >I verified that this fingerprint belongs to the new VPS.</span
            ></label
          ></Card.Content
        >
        <Card.Footer class="justify-end"
          ><Button
            onclick={confirm}
            disabled={!confirmed || $confirmForm.processing}
            aria-busy={$confirmForm.processing}
            >{#if $confirmForm.processing}<Spinner />{/if}Confirm and install</Button
          ></Card.Footer
        >
      </Card.Root>
    {/if}

    {#if node.error}
      <Alert.Root variant="destructive"
        ><Alert.Title>Enrollment failed</Alert.Title><Alert.Description
          >{node.error}</Alert.Description
        ><Alert.Action
          ><Button
            onclick={retry}
            disabled={node.state !== "failed" || retrying}
            aria-busy={retrying}
            >{#if retrying}<Spinner />{/if}Retry enrollment</Button
          ></Alert.Action
        ></Alert.Root
      >
    {/if}

    <div class="grid gap-6 lg:grid-cols-2">
      <Card.Root
        ><Card.Header
          ><Card.Title>Status</Card.Title><Card.Description
            >Durable enrollment progress.</Card.Description
          ></Card.Header
        ><Card.Content class="grid gap-4 text-sm"
          ><div class="space-y-2">
            <div class="flex items-center justify-between gap-3">
              <StatusBadge status={node.state} /><span
                class="capitalize text-muted-foreground"
                >{node.currentStep.replaceAll("_", " ")}</span
              >
            </div>
            <Progress
              value={progressValue}
              aria-label="Node enrollment progress"
            />
          </div>
          <div>
            <p class="text-xs text-muted-foreground">Configured</p>
            <p class="mt-1">{node.configured ? "Yes" : "No"}</p>
          </div></Card.Content
        ></Card.Root
      >
      <Card.Root
        ><Card.Header class="flex-row items-start justify-between gap-4"
          ><div>
            <Card.Title>Capabilities</Card.Title><Card.Description
              >Workloads and Buildpack runtimes this node may accept.</Card.Description
            >
          </div>
          {#if node.configured && node.state === "ready" && !capabilitiesEditing}<Button
              variant="outline"
              size="sm"
              onclick={() => (capabilitiesEditing = true)}>Manage</Button
            >{/if}</Card.Header
        ><Card.Content>
          {#if capabilitiesEditing}
            <form class="space-y-4" onsubmit={applyCapabilities}>
              <div class="grid gap-3 sm:grid-cols-2">
                {#each capabilityOptions as [key, label] (key)}
                  <label
                    class="flex items-center gap-3 border border-border p-3 text-sm"
                  >
                    <Checkbox
                      bind:checked={$capabilitiesForm[key]}
                      disabled={node.capabilities[key]}
                    />
                    <span>{label}</span>
                  </label>
                {/each}
              </div>
              {#if $capabilitiesForm.build}
                <div>
                  <p class="text-sm font-medium">Buildpack runtimes</p>
                  <div class="mt-2 grid gap-3 sm:grid-cols-2">
                    {#each buildpackOptions as [runtime, label] (runtime)}
                      <label
                        class="flex items-center gap-3 border border-border p-3 text-sm"
                      >
                        <Checkbox
                          bind:checked={buildpackSelections[runtime]}
                          disabled={node.capabilities.buildpacks?.runtimes?.includes(
                            runtime,
                          )}
                        />
                        <span>{label}</span>
                      </label>
                    {/each}
                  </div>
                </div>
              {/if}
              <p class="text-xs text-muted-foreground">
                Provisioned capabilities and Buildpack runtimes cannot be
                removed.
              </p>
              <div class="flex gap-2">
                <Button
                  type="submit"
                  size="sm"
                  disabled={$capabilitiesForm.processing}
                  aria-busy={$capabilitiesForm.processing}
                  >{#if $capabilitiesForm.processing}<Spinner />{/if}Save
                  capabilities</Button
                >
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onclick={() => (capabilitiesEditing = false)}>Cancel</Button
                >
              </div>
            </form>
          {:else}
            <div class="flex flex-wrap gap-2">
              {#each capabilityOptions.filter(([key]) => node.capabilities[key]) as [key, label] (key)}
                <span class="border border-border px-2 py-1 text-xs"
                  >{label}</span
                >
              {/each}
              {#each node.capabilities.buildpacks?.runtimes ?? [] as runtime (runtime)}
                <span
                  class="border border-primary/40 bg-primary/5 px-2 py-1 text-xs capitalize"
                  >{runtime} Buildpack</span
                >
              {/each}
            </div>
          {/if}
        </Card.Content></Card.Root
      >
      <Card.Root
        ><Card.Header
          ><Card.Title>OpenTelemetry endpoint</Card.Title><Card.Description
            >Node-local OTLP/HTTP receiver available only through WireGuard and
            to workloads on this node.</Card.Description
          ></Card.Header
        ><Card.Content
          ><p class="break-all font-mono text-sm">
            http://{node.wireGuardAddress}:4318
          </p>
          <p class="mt-2 text-xs text-muted-foreground">
            Protocol: http/protobuf
          </p></Card.Content
        ></Card.Root
      >
      <Card.Root
        ><Card.Header
          ><Card.Title>Timeline</Card.Title><Card.Description
            >Enrollment lifecycle timestamps.</Card.Description
          ></Card.Header
        ><Card.Content class="grid gap-4 text-sm"
          ><div>
            <p class="text-xs text-muted-foreground">Created</p>
            <p class="mt-1">{timestamp(node.createdAt)}</p>
          </div>
          <div>
            <p class="text-xs text-muted-foreground">Started</p>
            <p class="mt-1">{timestamp(node.startedAt)}</p>
          </div>
          <div>
            <p class="text-xs text-muted-foreground">Completed</p>
            <p class="mt-1">{timestamp(node.completedAt)}</p>
          </div></Card.Content
        ></Card.Root
      >
    </div>
  </div>
</DashboardLayout>
