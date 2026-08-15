<script lang="ts">
  import * as Alert from "@/Components/ui/alert";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import { Spinner } from "@/Components/ui/spinner";
  import DataField from "@/Components/DataField.svelte";
  import { routes } from "@/routes";
  import type { ResourceInstallation } from "./resource.types";

  let {
    resourceId,
    installations,
    pendingAction,
    onLifecycle,
    onRemove,
  }: {
    resourceId: string;
    installations: ResourceInstallation[];
    pendingAction: string;
    onLifecycle: (
      installationId: string,
      action: "start" | "stop" | "restart",
    ) => void;
    onRemove: (installationId: string, containerName: string) => void;
  } = $props();

  let logsOpen = $state(false);
  let logsInstallation = $state<ResourceInstallation | null>(null);
  let logs = $state("");
  let logsError = $state("");
  let logsLoading = $state(false);

  async function openLogs(installation: ResourceInstallation) {
    logsInstallation = installation;
    logs = "";
    logsError = "";
    logsLoading = true;
    logsOpen = true;
    try {
      const response = await window.fetch(
        `${routes.resourceInstallationLogs(resourceId, installation.id)}?tail=200`,
        {
          credentials: "same-origin",
          headers: { Accept: "application/json" },
        },
      );
      const payload = (await response.json().catch(() => ({}))) as {
        logs?: string;
        error?: string;
      };
      if (!response.ok)
        throw new Error(payload.error || "Container logs could not be loaded");
      logs = payload.logs || "The container has not written any logs.";
    } catch (error) {
      logsError =
        error instanceof Error
          ? error.message
          : "Container logs could not be loaded";
    } finally {
      logsLoading = false;
    }
  }

  function observedLabel(value: string | null) {
    if (!value) return "Never observed";
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Docker installations</Card.Title>
    <Card.Description
      >Observed state and controls for each Resource installation.</Card.Description
    >
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if installations.length === 0}<p class="text-sm text-muted-foreground">
        No installation is configured.
      </p>{/if}
    {#each installations as item (item.id)}
      <article class="border border-border p-4">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="font-medium">{item.containerName}</h3>
              <span
                class="border border-border px-1.5 py-0.5 text-[10px] uppercase tracking-wider"
                >Docker</span
              >
            </div>
            <p class="mt-1 font-mono text-xs text-muted-foreground">
              {item.imageReference}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            {#if item.state !== "missing"}<Button
                size="sm"
                variant="outline"
                disabled={logsLoading && logsInstallation?.id === item.id}
                onclick={() => openLogs(item)}
                >{#if logsLoading && logsInstallation?.id === item.id}<Spinner
                  />{/if}View logs</Button
              >{/if}
            {#if item.canControl}
              {#if item.serviceState === "running"}
                <Button
                  size="sm"
                  variant="outline"
                  disabled={Boolean(pendingAction)}
                  onclick={() => onLifecycle(item.id, "stop")}>Stop</Button
                >
                <Button
                  size="sm"
                  variant="outline"
                  disabled={Boolean(pendingAction)}
                  onclick={() => onLifecycle(item.id, "restart")}>Restart</Button
                >
              {:else}
                <Button
                  size="sm"
                  disabled={Boolean(pendingAction)}
                  onclick={() => onLifecycle(item.id, "start")}>Start</Button
                >
              {/if}
              {#if item.state !== "missing"}<Button
                  size="sm"
                  variant="destructive"
                  disabled={Boolean(pendingAction)}
                  onclick={() => onRemove(item.id, item.containerName)}
                  >Remove container</Button
                >{/if}
            {/if}
          </div>
        </div>
        {#if item.state !== "missing" && item.serviceState !== "running"}
          <Alert.Root variant="destructive" class="mt-4">
            <Alert.Title
              >Container is {item.serviceState || "not running"}</Alert.Title
            >
            <Alert.Description
              >Docker reports exit code {item.containerDetails?.exitCode ??
                "unknown"} and {item.containerDetails?.restartCount ?? 0}
              restarts. Open the container logs to see the process error.</Alert.Description
            >
          </Alert.Root>
        {/if}
        <div class="mt-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-5">
          <DataField label="Service" value={item.serviceState || "Unknown"} />
          <DataField label="Health" value={item.health || "Unknown"} />
          <DataField label="Server" value={item.serverName} />
          <DataField
            label="Container ID"
            value={item.containerDetails?.id?.slice(0, 12) || "Not created"}
          />
          <DataField label="Observed" value={observedLabel(item.observedAt)} />
        </div>
        {#if item.healthReason}<p
            class="mt-4 border-l-2 border-border pl-3 text-xs text-muted-foreground"
          >
            {item.healthReason}
          </p>{/if}
        {#if !item.canControl}<p class="mt-4 text-xs text-muted-foreground">
            Container controls are unavailable because the selected Server
            cannot currently be reached.
          </p>{/if}
      </article>
    {/each}
  </Card.Content>
</Card.Root>

<Dialog.Root bind:open={logsOpen}>
  <Dialog.Content class="sm:max-w-4xl">
    <Dialog.Header>
      <Dialog.Title>Container logs</Dialog.Title>
      <Dialog.Description
        >{logsInstallation?.containerName ?? "Resource container"} · latest 200
        lines, limited to 64 KiB.</Dialog.Description
      >
    </Dialog.Header>
    {#if logsLoading}
      <div class="flex min-h-40 items-center justify-center"><Spinner /></div>
    {:else if logsError}
      <Alert.Root variant="destructive"
        ><Alert.Title>Logs unavailable</Alert.Title><Alert.Description
          >{logsError}</Alert.Description
        ></Alert.Root
      >
    {:else}
      <pre
        class="max-h-[60vh] overflow-auto border border-border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap">{logs}</pre>
    {/if}
    <Dialog.Footer
      ><Button type="button" variant="outline" onclick={() => (logsOpen = false)}
        >Close</Button
      ></Dialog.Footer
    >
  </Dialog.Content>
</Dialog.Root>
