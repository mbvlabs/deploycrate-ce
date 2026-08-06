<script lang="ts">
  import { Link } from "@inertiajs/svelte";
  import { toast } from "svelte-sonner";

  import FormField from "@/Components/FormField.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Destination = {
    id: string;
    name: string;
    provider: string;
    endpoint: string;
    region: string;
    bucket: string;
    prefix: string;
    forcePathStyle: boolean;
    verifiedAt: string | null;
    lastUsedAt: string | null;
  };
  type RecoveryMaterial = { resticPassword: string; ageIdentity: string };

  let {
    auth,
    destination,
  }: { auth: { email: string }; destination: Destination } = $props();
  let recoveryPasswordOpen = $state(false);
  let recoveryOpen = $state(false);
  let currentPassword = $state("");
  let recoveryProcessing = $state(false);
  let recoveryError = $state("");
  let recoveryMaterial = $state<RecoveryMaterial | null>(null);

  function timeLabel(value: string | null) {
    if (!value) return "Never";
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  }

  function askForRecovery() {
    currentPassword = "";
    recoveryError = "";
    recoveryMaterial = null;
    recoveryPasswordOpen = true;
  }

  async function revealRecovery(event: SubmitEvent) {
    event.preventDefault();
    if (!currentPassword || recoveryProcessing) return;
    recoveryProcessing = true;
    recoveryError = "";
    try {
      const response = await window.fetch(
        routes.objectStorageRecovery(destination.id),
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ password: currentPassword }),
        },
      );
      const payload = (await response
        .json()
        .catch(() => ({}))) as Partial<RecoveryMaterial> & { error?: string };
      if (!response.ok || !payload.resticPassword || !payload.ageIdentity)
        throw new Error(
          payload.error || "Recovery material could not be loaded",
        );
      recoveryMaterial = {
        resticPassword: payload.resticPassword,
        ageIdentity: payload.ageIdentity,
      };
      currentPassword = "";
      recoveryPasswordOpen = false;
      recoveryOpen = true;
    } catch (error) {
      recoveryError =
        error instanceof Error
          ? error.message
          : "Recovery material could not be loaded";
    } finally {
      recoveryProcessing = false;
    }
  }

  function closeRecovery() {
    recoveryOpen = false;
    recoveryMaterial = null;
  }

  async function copyRecovery(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`${label} could not be copied`);
    }
  }
</script>

<svelte:head><title>{destination.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header
      class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between"
    >
      <div>
        <Link
          class="text-xs text-muted-foreground hover:text-foreground"
          href={routes.objectStorage()}>Object Storage</Link
        >
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <h1 class="text-3xl font-semibold tracking-tight">
            {destination.name}
          </h1>
          <StatusBadge status="verified" />
        </div>
        <p class="mt-3 text-sm text-muted-foreground">
          {destination.provider.toUpperCase()} backup destination
        </p>
      </div>
      <Button type="button" onclick={askForRecovery}
        >View recovery material</Button
      >
    </header>

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Title>Destination details</Card.Title><Card.Description
          >Verified Object Storage connection used by backup policies.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        <dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <dt class="text-xs text-muted-foreground">Provider</dt>
            <dd class="mt-1 uppercase">{destination.provider}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Bucket</dt>
            <dd class="mt-1 font-mono text-xs">{destination.bucket}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Region</dt>
            <dd class="mt-1 font-mono text-xs">
              {destination.region || "Provider default"}
            </dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-xs text-muted-foreground">Endpoint</dt>
            <dd class="mt-1 break-all font-mono text-xs">
              {destination.endpoint || "Provider default"}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Prefix</dt>
            <dd class="mt-1 font-mono text-xs">
              {destination.prefix || "Bucket root"}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Addressing</dt>
            <dd class="mt-1">
              {destination.forcePathStyle ? "Path style" : "Virtual hosted"}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Verified</dt>
            <dd class="mt-1">{timeLabel(destination.verifiedAt)}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Last used</dt>
            <dd class="mt-1">{timeLabel(destination.lastUsedAt)}</dd>
          </div>
        </dl>
      </Card.Content>
    </Card.Root>
  </div>

  <Dialog.Root bind:open={recoveryPasswordOpen}>
    <Dialog.Content showCloseButton={!recoveryProcessing}>
      <form class="grid gap-5" onsubmit={revealRecovery}>
        <Dialog.Header
          ><Dialog.Title>View recovery material</Dialog.Title
          ><Dialog.Description
            >Enter your current administrator password to reveal the backup
            recovery secrets for {destination.name}.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="Current password"
          ><Input
            type="password"
            bind:value={currentPassword}
            autocomplete="current-password"
            autofocus
            required
            disabled={recoveryProcessing}
          /></FormField
        >
        {#if recoveryError}<p
            class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
            role="alert"
          >
            {recoveryError}
          </p>{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={recoveryProcessing}
            onclick={() => (recoveryPasswordOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={!currentPassword || recoveryProcessing}
            >{#if recoveryProcessing}<Spinner />{/if}Continue</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root
    bind:open={recoveryOpen}
    onOpenChange={(open) => {
      if (!open) closeRecovery();
    }}
  >
    <Dialog.Content class="sm:max-w-2xl">
      <Dialog.Header
        ><Dialog.Title>Object Storage recovery material</Dialog.Title
        ><Dialog.Description
          >Store these secrets outside DeployCrate. They are required to recover
          backups if this installation and its database are unavailable.</Dialog.Description
        ></Dialog.Header
      >
      {#if recoveryMaterial}
        <div class="grid gap-5">
          <div class="grid gap-2">
            <p class="text-xs font-medium">Restic password</p>
            <div class="flex gap-2">
              <Input value={recoveryMaterial.resticPassword} readonly /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyRecovery(
                    recoveryMaterial!.resticPassword,
                    "Restic password",
                  )}>Copy</Button
              >
            </div>
          </div>
          <div class="grid gap-2">
            <p class="text-xs font-medium">Age identity</p>
            <div class="flex items-start gap-2">
              <textarea
                class="min-h-28 w-full border border-input bg-transparent p-3 font-mono text-xs"
                readonly
                value={recoveryMaterial.ageIdentity}></textarea><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyRecovery(recoveryMaterial!.ageIdentity, "Age identity")}
                >Copy</Button
              >
            </div>
          </div>
        </div>
      {/if}
      <Dialog.Footer
        ><Button type="button" onclick={closeRecovery}>Done</Button
        ></Dialog.Footer
      >
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
