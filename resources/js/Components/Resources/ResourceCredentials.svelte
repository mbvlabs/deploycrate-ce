<script lang="ts">
  import { toast } from "svelte-sonner";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import FormField from "@/Components/FormField.svelte";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import { routes } from "@/routes";
  import type { ResourceCredential } from "./resource.types";

  type RevealedCredential = {
    id: string;
    name: string;
    username: string;
    values: Record<string, string>;
  };

  let {
    resourceId,
    credentials,
    databaseBacked,
    managedPostgreSQL,
    containerRunning,
    canAddApplicationUser,
    onEdit,
  }: {
    resourceId: string;
    credentials: ResourceCredential[];
    databaseBacked: boolean;
    managedPostgreSQL: boolean;
    containerRunning: boolean;
    canAddApplicationUser: boolean;
    onEdit: (credential: ResourceCredential | null) => void;
  } = $props();

  let passwordDialogOpen = $state(false);
  let revealedDialogOpen = $state(false);
  let selectedCredential = $state<ResourceCredential | null>(null);
  let currentPassword = $state("");
  let revealError = $state("");
  let revealProcessing = $state(false);
  let revealedCredential = $state<RevealedCredential | null>(null);

  function askForCredential(item: ResourceCredential) {
    selectedCredential = item;
    currentPassword = "";
    revealError = "";
    passwordDialogOpen = true;
  }

  async function revealCredential(event: SubmitEvent) {
    event.preventDefault();
    if (!selectedCredential || !currentPassword || revealProcessing) return;
    revealProcessing = true;
    revealError = "";
    try {
      const response = await window.fetch(
        routes.resourceCredentialReveal(resourceId, selectedCredential.id),
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
        .catch(() => ({}))) as Partial<RevealedCredential> & { error?: string };
      if (
        !response.ok ||
        !payload.id ||
        !payload.name ||
        !payload.values ||
        Object.keys(payload.values).length === 0
      )
        throw new Error(
          payload.error || "Resource credential could not be loaded",
        );
      revealedCredential = {
        id: payload.id,
        name: payload.name,
        username: payload.username ?? "",
        values: payload.values,
      };
      currentPassword = "";
      passwordDialogOpen = false;
      revealedDialogOpen = true;
    } catch (error) {
      revealError =
        error instanceof Error
          ? error.message
          : "Resource credential could not be loaded";
    } finally {
      revealProcessing = false;
    }
  }

  function closeRevealedCredential() {
    revealedDialogOpen = false;
    revealedCredential = null;
    selectedCredential = null;
  }

  async function copyCredential(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`${label} could not be copied`);
    }
  }

  function displayLabel(value: string) {
    return value
      .replaceAll("_", " ")
      .replace(/\b\w/g, (character) => character.toUpperCase());
  }
</script>

<Card.Root>
  <Card.Header
    ><Card.Action
      ><Button
        size="sm"
        variant="outline"
        disabled={!canAddApplicationUser}
        onclick={() => onEdit(null)}>Add application credential</Button
      ></Card.Action
    ><Card.Title>Credentials</Card.Title><Card.Description
      >{databaseBacked
        ? "The administrator stays internal. Each application credential is tied to one configured Database."
        : "Encrypted application credentials for this Resource."}</Card.Description
    ></Card.Header
  >
  <Card.Content class="space-y-4">
    {#if managedPostgreSQL && !containerRunning}<p
        class="border border-border bg-muted/20 p-3 text-sm text-muted-foreground"
      >
        Start the PostgreSQL container before adding an application user.
        DeployCrate must connect to the running server to create its LOGIN role.
      </p>{/if}
    {#if credentials.length === 0}<p class="text-sm text-muted-foreground">
        No credentials configured.
      </p>{:else}<div class="overflow-hidden border border-border">
        <Table.Root
          ><Table.Header
            ><Table.Row
              ><Table.Head>Name</Table.Head><Table.Head>Username</Table.Head
              ><Table.Head>Purpose</Table.Head><Table.Head>Database</Table.Head
              ><Table.Head class="text-right">Actions</Table.Head></Table.Row
            ></Table.Header
          ><Table.Body
            >{#each credentials as item (item.id)}<Table.Row
                ><Table.Cell class="font-medium">{item.name}</Table.Cell
                ><Table.Cell class="font-mono text-xs"
                  >{item.username || "None"}</Table.Cell
                ><Table.Cell class="capitalize"
                  >{item.metadata?.purpose ?? "Application"}</Table.Cell
                ><Table.Cell
                  >{item.metadata?.database || "All / none"}</Table.Cell
                ><Table.Cell
                  ><div class="flex justify-end gap-2">
                    {#if item.hasEncryptedPayload}<Button
                        size="sm"
                        variant="ghost"
                        onclick={() => askForCredential(item)}>View</Button
                      >{/if}<Button
                      size="sm"
                      variant="outline"
                      onclick={() => onEdit(item)}>Edit</Button
                    >
                  </div></Table.Cell
                ></Table.Row
              >{/each}</Table.Body
          ></Table.Root
        >
      </div>{/if}
  </Card.Content>
</Card.Root>

<Dialog.Root
  bind:open={passwordDialogOpen}
  onOpenChange={(open) => {
    if (!open && !revealProcessing) {
      currentPassword = "";
      revealError = "";
      selectedCredential = null;
    }
  }}
>
  <Dialog.Content showCloseButton={!revealProcessing}>
    <form class="grid gap-4" onsubmit={revealCredential}>
      <Dialog.Header
        ><Dialog.Title>View Resource credential</Dialog.Title
        ><Dialog.Description
          >Enter your current password to reveal {selectedCredential?.name ??
            "this credential"}.</Dialog.Description
        ></Dialog.Header
      >
      <FormField label="Current password"
        ><Input
          type="password"
          bind:value={currentPassword}
          autocomplete="current-password"
          autofocus
          required
          disabled={revealProcessing}
        /></FormField
      >
      {#if revealError}<p
          class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
          role="alert"
        >
          {revealError}
        </p>{/if}
      <Dialog.Footer
        ><Button
          type="button"
          variant="outline"
          disabled={revealProcessing}
          onclick={() => (passwordDialogOpen = false)}>Cancel</Button
        ><Button
          type="submit"
          disabled={!currentPassword || revealProcessing}
          >{#if revealProcessing}<Spinner />{/if}Continue</Button
        ></Dialog.Footer
      >
    </form>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root
  bind:open={revealedDialogOpen}
  onOpenChange={(open) => {
    if (!open) closeRevealedCredential();
  }}
>
  <Dialog.Content class="sm:max-w-xl">
    <Dialog.Header
      ><Dialog.Title
        >{revealedCredential?.name ?? "Resource credential"}</Dialog.Title
      ><Dialog.Description
        >This decrypted credential is shown only until you close this dialog.</Dialog.Description
      ></Dialog.Header
    >
    {#if revealedCredential}
      <div class="grid gap-4">
        {#if revealedCredential.username}<div class="grid gap-2">
            <p class="text-xs font-medium">Username</p>
            <div class="flex gap-2">
              <Input value={revealedCredential.username} readonly /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyCredential(revealedCredential!.username, "Username")}
                >Copy</Button
              >
            </div>
          </div>{/if}
        {#each Object.entries(revealedCredential.values) as [name, value] (name)}
          <div class="grid gap-2">
            <p class="text-xs font-medium">{displayLabel(name)}</p>
            <div class="flex gap-2">
              <Input type="text" {value} readonly autocomplete="off" /><Button
                type="button"
                variant="outline"
                onclick={() => copyCredential(value, displayLabel(name))}
                >Copy</Button
              >
            </div>
          </div>
        {/each}
      </div>
    {/if}
    <Dialog.Footer
      ><Button type="button" onclick={closeRevealedCredential}>Done</Button
      ></Dialog.Footer
    >
  </Dialog.Content>
</Dialog.Root>
