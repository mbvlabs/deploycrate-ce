<script lang="ts">
  import { Link, router } from "@inertiajs/svelte";
  import { toast } from "svelte-sonner";

  import ConfirmActionDialog from "@/Components/ConfirmActionDialog.svelte";
  import FormField from "@/Components/FormField.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Dialog from "@/Components/ui/dialog";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Registry = {
    id: string;
    name: string;
    slug: string;
    provider: string;
    endpoint: string;
    username: string;
    credentialName: string;
    managed: boolean;
    createdAt: string;
  };
  type RegistryCredentials = {
    endpoint: string;
    username: string;
    password: string;
  };
  type Repository = { name: string; tags: string[] };

  let {
    auth,
    registry,
    repositories,
    inventoryError,
  }: {
    auth: { email: string };
    registry: Registry;
    repositories: Repository[];
    inventoryError: string;
  } = $props();
  let passwordDialogOpen = $state(false);
  let credentialDialogOpen = $state(false);
  let currentPassword = $state("");
  let credentialProcessing = $state(false);
  let credentialError = $state("");
  let revealedCredentials = $state<RegistryCredentials | null>(null);
  let inventoryRefreshing = $state(false);
  let archiveDialogOpen = $state(false);
  let archiveProcessing = $state(false);
  let archiveError = $state("");
  let deleteImageDialogOpen = $state(false);
  let deleteImageTarget = $state<{ repository: string; tag: string } | null>(
    null,
  );
  let deleteImageProcessing = $state(false);
  let deleteImageError = $state("");

  function askToArchive() {
    archiveError = "";
    archiveDialogOpen = true;
  }

  function archive() {
    if (archiveProcessing) return;
    archiveProcessing = true;
    archiveError = "";
    router.delete(routes.registryResourceDestroy(registry.id), {
      onError: (errors) =>
        (archiveError =
          Object.values(errors).map(String).join("\n") ||
          "The registry could not be archived."),
      onFinish: () => (archiveProcessing = false),
    });
  }

  function askForCredentials() {
    currentPassword = "";
    credentialError = "";
    revealedCredentials = null;
    passwordDialogOpen = true;
  }

  async function revealCredentials(event: SubmitEvent) {
    event.preventDefault();
    if (!currentPassword || credentialProcessing) return;
    credentialProcessing = true;
    credentialError = "";
    try {
      const response = await window.fetch(
        routes.registryResourceCredentials(registry.id),
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
        .catch(() => ({}))) as Partial<RegistryCredentials> & {
        error?: string;
      };
      if (
        !response.ok ||
        !payload.endpoint ||
        !payload.username ||
        !payload.password
      ) {
        throw new Error(
          payload.error || "Registry credentials could not be loaded",
        );
      }
      revealedCredentials = {
        endpoint: payload.endpoint,
        username: payload.username,
        password: payload.password,
      };
      currentPassword = "";
      passwordDialogOpen = false;
      credentialDialogOpen = true;
    } catch (error) {
      credentialError =
        error instanceof Error
          ? error.message
          : "Registry credentials could not be loaded";
    } finally {
      credentialProcessing = false;
    }
  }

  function closeCredentialDialog() {
    credentialDialogOpen = false;
    revealedCredentials = null;
  }

  async function copyCredential(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`${label} could not be copied`);
    }
  }

  function imageReference(repository: string, tag: string) {
    return `${registry.endpoint}/${repository}:${tag}`;
  }

  function askToDeleteImage(repository: string, tag: string) {
    deleteImageError = "";
    deleteImageTarget = { repository, tag };
    deleteImageDialogOpen = true;
  }

  function deleteImage() {
    if (deleteImageProcessing || !deleteImageTarget) return;
    deleteImageProcessing = true;
    deleteImageError = "";
    router.post(
      routes.registryResourceImageDelete(registry.id),
      deleteImageTarget,
      {
        preserveScroll: true,
        onSuccess: () => {
          deleteImageDialogOpen = false;
          deleteImageTarget = null;
          deleteImageError = "";
        },
        onError: (errors) =>
          (deleteImageError =
            Object.values(errors).map(String).join("\n") ||
            "The image could not be deleted."),
        onFinish: () => (deleteImageProcessing = false),
      },
    );
  }

  function refreshInventory() {
    if (inventoryRefreshing) return;
    inventoryRefreshing = true;
    router.reload({
      only: ["repositories", "inventoryError"],
      preserveScroll: true,
      onFinish: () => (inventoryRefreshing = false),
    });
  }
</script>

<svelte:head><title>{registry.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header>
      <Link
        class="text-xs text-muted-foreground hover:text-foreground"
        href={routes.registryResources()}>Image Registry</Link
      >
      <div class="mt-3 flex flex-wrap items-center gap-3">
        <h1 class="text-3xl font-semibold tracking-tight">{registry.name}</h1>
        <StatusBadge status={registry.managed ? "managed" : "external"} />
      </div>
      <p class="mt-3 text-sm text-muted-foreground">{registry.endpoint}</p>
    </header>

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Title>Registry details</Card.Title><Card.Description
          >OCI registry connection used for publishing and deploying Application
          images.</Card.Description
        ></Card.Header
      >
      <Card.Content>
        <dl class="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <dt class="text-xs text-muted-foreground">Endpoint</dt>
            <dd class="mt-1 break-all font-mono text-xs">
              {registry.endpoint}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Username</dt>
            <dd class="mt-1 font-mono text-xs">{registry.username}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Protocol</dt>
            <dd class="mt-1">OCI Distribution</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Credential</dt>
            <dd class="mt-1">{registry.credentialName}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Provider</dt>
            <dd class="mt-1">{registry.provider}</dd>
          </div>
          <div>
            <dt class="text-xs text-muted-foreground">Created</dt>
            <dd class="mt-1">
              {new Date(registry.createdAt).toLocaleString()}
            </dd>
          </div>
        </dl>
      </Card.Content>
    </Card.Root>

    {#if registry.managed}
      <Card.Root class="max-w-4xl">
        <Card.Header
          ><Card.Action
            ><Button
              type="button"
              size="sm"
              variant="outline"
              disabled={inventoryRefreshing}
              onclick={refreshInventory}
              >{#if inventoryRefreshing}<Spinner />{/if}Refresh</Button
            ></Card.Action
          ><Card.Title>Repositories</Card.Title><Card.Description
            >Images currently published to this managed registry, grouped by
            repository and tag.</Card.Description
          ></Card.Header
        >
        <Card.Content>
          {#if inventoryError}
            <p
              class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
              role="alert"
            >
              {inventoryError}
            </p>
          {:else if repositories.length === 0}
            <p class="text-sm text-muted-foreground">
              No images have been pushed to this registry.
            </p>
          {:else}
            <div class="grid gap-4">
              {#each repositories as repository (repository.name)}
                <section class="border border-border">
                  <div
                    class="flex items-center justify-between gap-4 border-b border-border px-4 py-3"
                  >
                    <h3 class="font-mono text-sm font-medium">
                      {repository.name}
                    </h3>
                    <span class="text-xs text-muted-foreground"
                      >{repository.tags.length}
                      {repository.tags.length === 1 ? "tag" : "tags"}</span
                    >
                  </div>
                  {#if repository.tags.length === 0}
                    <p class="p-4 text-sm text-muted-foreground">
                      This repository has no tagged images.
                    </p>
                  {:else}
                    <div class="divide-y divide-border">
                      {#each repository.tags as tag (tag)}
                        <div
                          class="flex items-center justify-between gap-3 px-4 py-3"
                        >
                          <code class="min-w-0 break-all text-xs"
                            >{imageReference(repository.name, tag)}</code
                          >
                          <div class="flex shrink-0 gap-2">
                            <Button
                              type="button"
                              size="xs"
                              variant="outline"
                              onclick={() =>
                                copyCredential(
                                  imageReference(repository.name, tag),
                                  "Image reference",
                                )}>Copy</Button
                            ><Button
                              type="button"
                              size="xs"
                              variant="outline"
                              class="text-destructive hover:text-destructive"
                              onclick={() =>
                                askToDeleteImage(repository.name, tag)}
                              >Delete</Button
                            >
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </section>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}

    <Card.Root class="max-w-4xl">
      <Card.Header
        ><Card.Title>Publisher credentials</Card.Title><Card.Description
          >{registry.managed
            ? "Confirm your administrator password before revealing the push and pull credential."
            : "External registry credentials remain encrypted and cannot be revealed after they are connected."}</Card.Description
        ></Card.Header
      >
      <Card.Content
        ><p class="text-sm text-muted-foreground">
          Use this registry endpoint and username with Docker or another
          OCI-compatible client.
        </p></Card.Content
      >
      {#if registry.managed}<Card.Footer class="border-t border-border"
          ><Button type="button" onclick={askForCredentials}
            >View credentials</Button
          ></Card.Footer
        >{/if}
    </Card.Root>

    {#if !registry.managed}
      <Card.Root class="max-w-4xl">
        <Card.Header
          ><Card.Title>Archive registry</Card.Title><Card.Description
            >This registry will no longer be available for new builds or
            deployments.</Card.Description
          ></Card.Header
        >
        <Card.Footer class="border-t border-border"
          ><Button
            variant="destructive"
            disabled={archiveProcessing}
            onclick={askToArchive}>Archive registry</Button
          ></Card.Footer
        >
      </Card.Root>
    {/if}
  </div>

  <Dialog.Root
    bind:open={passwordDialogOpen}
    onOpenChange={(open) => {
      if (!open && !credentialProcessing) {
        currentPassword = "";
        credentialError = "";
      }
    }}
  >
    <Dialog.Content showCloseButton={!credentialProcessing}>
      <form class="grid gap-4" onsubmit={revealCredentials}>
        <Dialog.Header
          ><Dialog.Title>View registry credentials</Dialog.Title
          ><Dialog.Description
            >Enter your current administrator password to reveal the publisher
            credential for {registry.name}.</Dialog.Description
          ></Dialog.Header
        >
        <FormField label="Current password"
          ><Input
            type="password"
            bind:value={currentPassword}
            autocomplete="current-password"
            autofocus
            required
            disabled={credentialProcessing}
          /></FormField
        >
        {#if credentialError}<p
            class="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive"
            role="alert"
          >
            {credentialError}
          </p>{/if}
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={credentialProcessing}
            onclick={() => (passwordDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={!currentPassword || credentialProcessing}
            >{#if credentialProcessing}<Spinner />{/if}Continue</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root
    bind:open={credentialDialogOpen}
    onOpenChange={(open) => {
      if (!open) closeCredentialDialog();
    }}
  >
    <Dialog.Content class="sm:max-w-xl">
      <Dialog.Header
        ><Dialog.Title>Registry publisher credentials</Dialog.Title
        ><Dialog.Description
          >These credentials grant push and pull access to every repository in
          this managed registry.</Dialog.Description
        ></Dialog.Header
      >
      {#if revealedCredentials}
        <div class="grid gap-4">
          <div class="grid gap-2">
            <p class="text-xs font-medium">Registry endpoint</p>
            <div class="flex gap-2">
              <Input value={revealedCredentials.endpoint} readonly /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyCredential(revealedCredentials!.endpoint, "Endpoint")}
                >Copy</Button
              >
            </div>
          </div>
          <div class="grid gap-2">
            <p class="text-xs font-medium">Username</p>
            <div class="flex gap-2">
              <Input value={revealedCredentials.username} readonly /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyCredential(revealedCredentials!.username, "Username")}
                >Copy</Button
              >
            </div>
          </div>
          <div class="grid gap-2">
            <p class="text-xs font-medium">Password</p>
            <div class="flex gap-2">
              <Input
                type="text"
                value={revealedCredentials.password}
                readonly
                autocomplete="off"
              /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyCredential(revealedCredentials!.password, "Password")}
                >Copy</Button
              >
            </div>
          </div>
          <div class="grid gap-2">
            <p class="text-xs font-medium">Docker login</p>
            <div class="flex gap-2">
              <Input
                value={`docker login ${revealedCredentials.endpoint} --username ${revealedCredentials.username}`}
                readonly
              /><Button
                type="button"
                variant="outline"
                onclick={() =>
                  copyCredential(
                    `docker login ${revealedCredentials!.endpoint} --username ${revealedCredentials!.username}`,
                    "Docker login command",
                  )}>Copy</Button
              >
            </div>
            <p class="text-xs text-muted-foreground">
              Run the command and paste the password when Docker prompts for it.
            </p>
          </div>
        </div>
      {/if}
      <Dialog.Footer
        ><Button type="button" onclick={closeCredentialDialog}>Done</Button
        ></Dialog.Footer
      >
    </Dialog.Content>
  </Dialog.Root>
  <ConfirmActionDialog
    bind:open={archiveDialogOpen}
    title={`Archive ${registry.name}?`}
    description="This registry will no longer be available for new builds or deployments."
    confirmLabel="Archive registry"
    destructive
    processing={archiveProcessing}
    error={archiveError}
    onconfirm={archive}
  />
  <ConfirmActionDialog
    bind:open={deleteImageDialogOpen}
    title="Delete image?"
    description={deleteImageTarget
      ? `The stored image ${deleteImageTarget.repository}:${deleteImageTarget.tag} will be permanently removed from the registry. Deployments referencing it will continue to serve existing instances until redeployed.`
      : ""}
    confirmLabel="Delete image"
    destructive
    processing={deleteImageProcessing}
    error={deleteImageError}
    onconfirm={deleteImage}
  />
</DashboardLayout>
