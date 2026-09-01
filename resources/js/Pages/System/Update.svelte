<script lang="ts">
  import DownloadIcon from "@lucide/svelte/icons/download";
  import EyeIcon from "@lucide/svelte/icons/eye";
  import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";
  import { router } from "@inertiajs/svelte";

  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import * as Table from "@/Components/ui/table";
  import DeploymentDialog, {
    type Deployment,
  } from "@/Components/System/DeploymentDialog.svelte";
  import UpdateDialog, {
    type UpdateStatus,
  } from "@/Components/System/UpdateDialog.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Spinner } from "@/Components/ui/spinner";
  import { formatVersion, releaseUpdateSummary } from "@/lib/version";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type UpdateStatusResponse = {
    currentVersion: string;
    update: UpdateStatus;
  };

  type AvailableUpdate = {
    channel: "stable" | "edge" | "";
    version: string;
    available: boolean;
    error?: string;
  };

  let {
    auth,
    currentVersion: initialCurrentVersion,
    update: initialUpdate,
    availableUpdate,
    deployments,
  }: {
    auth: { email: string };
    currentVersion: string;
    update: UpdateStatus;
    availableUpdate: AvailableUpdate;
    deployments: Deployment[];
  } = $props();

  let liveStatus = $state<UpdateStatusResponse | null>(null);
  let starting = $state(false);
  let checking = $state(false);
  let reconnecting = $state(false);
  let updateDialogOpen = $state(false);
  let updateDialogTracking = $state(false);
  let deploymentDialogOpen = $state(false);
  let selectedDeployment = $state<Deployment | null>(null);
  const currentVersion = $derived(
    liveStatus?.currentVersion ?? initialCurrentVersion,
  );
  const update = $derived(liveStatus?.update ?? initialUpdate);
  const running = $derived(
    update.state === "queued" || update.state === "in_progress",
  );
  const canUpdate = $derived(!running && availableUpdate.available);

  $effect(() => {
    if (!running) return;
    updateDialogTracking = true;
    updateDialogOpen = true;
  });

  $effect(() => {
    if (!running) return;

    const abortController = new AbortController();
    let timer: number | undefined;
    let retryDelay = 1000;

    async function pollStatus() {
      try {
        const response = await window.fetch(routes.systemUpdateStatus(), {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" },
          signal: abortController.signal,
        });
        if (!response.ok)
          throw new Error(`Update status returned ${response.status}`);

        const status = (await response.json()) as UpdateStatusResponse;
        if (abortController.signal.aborted) return;

        liveStatus = status;
        reconnecting = false;
        retryDelay = 1000;
        if (
          status.update.state !== "queued" &&
          status.update.state !== "in_progress"
        ) {
          router.reload({ only: ["deployments"] });
          return;
        }
      } catch {
        if (abortController.signal.aborted) return;
        reconnecting = true;
        retryDelay = Math.min(retryDelay * 2, 5000);
      }

      timer = window.setTimeout(pollStatus, retryDelay);
    }

    timer = window.setTimeout(pollStatus, retryDelay);

    return () => {
      abortController.abort();
      if (timer !== undefined) window.clearTimeout(timer);
    };
  });

  function checkForUpdates() {
    router.reload({
      only: ["availableUpdate"],
      onStart: () => (checking = true),
      onFinish: () => (checking = false),
    });
  }

  function startUpdate() {
    updateDialogTracking = true;
    starting = true;
    liveStatus = null;
    router.post(
      routes.systemUpdateCreate(),
      {},
      {
        preserveScroll: true,
        onFinish: () => (starting = false),
      },
    );
  }

  function openUpdateDialog() {
    updateDialogTracking = running;
    updateDialogOpen = true;
  }

  function openDeployment(deployment: Deployment) {
    selectedDeployment = deployment;
    deploymentDialogOpen = true;
  }

  function versionLabel(version: string) {
    return formatVersion(version);
  }

  function changeSummaryLabel(deployment: Deployment) {
    if (deployment.changeKind === "system_update" && deployment.releaseVersion) {
      return releaseUpdateSummary(deployment.releaseVersion);
    }
    return deployment.changeSummary || "No change summary recorded.";
  }

  function timestamp(value: string) {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  }
</script>

<svelte:head>
  <title>Updates</title>
</svelte:head>

<DashboardLayout email={auth.email} version={currentVersion}>
  <div class="space-y-8">
    <section
      class="flex flex-col justify-between gap-5 lg:flex-row lg:items-end"
    >
      <div class="max-w-3xl">
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          System
        </p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">Updates</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Keep DeployCrate CE current and review every system deployment from
          one place.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          onclick={checkForUpdates}
          disabled={checking || running}
          aria-busy={checking}
        >
          {#if checking}<Spinner />{:else}<RefreshCwIcon />{/if}
          Check for updates
        </Button>
        <Button
          onclick={openUpdateDialog}
          disabled={starting || !availableUpdate.available}
          aria-busy={running || starting}
        >
          {#if running || starting}
            <Spinner />
            Updating
          {:else}
            <DownloadIcon />
            Update
          {/if}
        </Button>
      </div>
    </section>

    <Card.Root>
      <Card.Header>
        <Card.Action>
          {#if running}
            <StatusBadge
              status={reconnecting ? "reconnecting" : "running"}
              label={reconnecting ? "Reconnecting" : "Live"}
            />
          {:else}
            <StatusBadge status={update.state} />
          {/if}
        </Card.Action>
        <Card.Title>Current installation</Card.Title>
        <Card.Description>
          {#if availableUpdate.error}
            Update check failed: {availableUpdate.error}
          {:else if availableUpdate.available}
            Version {versionLabel(availableUpdate.version)} is available on the
            {availableUpdate.channel} channel.
          {:else}
            This installation is current on the {availableUpdate.channel} channel.
          {/if}
        </Card.Description>
      </Card.Header>
      <Card.Content>
        <div
          class="grid gap-5 border border-border bg-muted/30 p-4 sm:grid-cols-3"
        >
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Installed version
            </p>
            <p class="mt-2 font-mono text-lg font-semibold">
              {versionLabel(currentVersion)}
            </p>
          </div>
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Previous instance
            </p>
            <p class="mt-2 font-mono text-sm">
              {update.activeInstanceBefore || "Unknown"}
            </p>
          </div>
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-[0.16em] text-muted-foreground"
            >
              Serving instance
            </p>
            <p class="mt-2 font-mono text-sm">
              {update.activeInstance || "Unknown"}
            </p>
          </div>
          <p
            class="text-xs leading-5 text-muted-foreground sm:col-span-3 sm:border-t sm:border-border sm:pt-4"
          >
            Updates use a blue-green cutover and restore the previous binary if
            health validation fails. Progress is shown in the update dialog.
          </p>
          {#if running}
            <div class="sm:col-span-3">
              <Button variant="outline" size="sm" onclick={openUpdateDialog}>
                <Spinner />
                View update progress
              </Button>
            </div>
          {/if}
        </div>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Deployment history</Card.Title>
        <Card.Description
          >{deployments.length} deployment{deployments.length === 1 ? "" : "s"},
          newest first.</Card.Description
        >
      </Card.Header>
      <Card.Content>
        {#if deployments.length === 0}
          <Empty.Root class="border border-dashed border-border py-12">
            <Empty.Header>
              <Empty.Title>No system deployments</Empty.Title>
              <Empty.Description
                >Deployment history will appear after the first system release
                is installed.</Empty.Description
              >
            </Empty.Header>
          </Empty.Root>
        {:else}
          <div class="overflow-x-auto border border-border">
            <Table.Root class="min-w-[720px]">
              <Table.Header>
                <Table.Row>
                  <Table.Head>Version</Table.Head>
                  <Table.Head>Status</Table.Head>
                  <Table.Head>Change</Table.Head>
                  <Table.Head>Created</Table.Head>
                  <Table.Head class="w-24"
                    ><span class="sr-only">Actions</span></Table.Head
                  >
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#each deployments as deployment (deployment.id)}
                  <Table.Row>
                    <Table.Cell>
                      <div class="flex items-center gap-2">
                        <span class="font-mono font-medium"
                          >{versionLabel(deployment.releaseVersion)}</span
                        >
                        {#if deployment.active}<StatusBadge
                            status="serving"
                          />{/if}
                      </div>
                    </Table.Cell>
                    <Table.Cell
                      ><StatusBadge status={deployment.status} /></Table.Cell
                    >
                    <Table.Cell class="max-w-md whitespace-normal">
                      <p class="line-clamp-2">
                        {changeSummaryLabel(deployment)}
                      </p>
                    </Table.Cell>
                    <Table.Cell>{timestamp(deployment.createdAt)}</Table.Cell>
                    <Table.Cell class="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        onclick={() => openDeployment(deployment)}
                      >
                        <EyeIcon />
                        View
                      </Button>
                    </Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <UpdateDialog
      bind:open={updateDialogOpen}
      tracking={updateDialogTracking}
      {currentVersion}
      {update}
      {running}
      {starting}
      {reconnecting}
      {canUpdate}
      onStart={startUpdate}
    />
    <DeploymentDialog
      bind:open={deploymentDialogOpen}
      deployment={selectedDeployment}
    />
  </div>
</DashboardLayout>
