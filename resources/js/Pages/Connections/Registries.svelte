<script lang="ts">
  import BoxesIcon from "@lucide/svelte/icons/boxes";
  import { router, useForm } from "@inertiajs/svelte";

  import FormField from "@/Components/FormField.svelte";
  import PageHeader from "@/Components/PageHeader.svelte";
  import StatusBadge from "@/Components/StatusBadge.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Dialog from "@/Components/ui/dialog";
  import * as Empty from "@/Components/ui/empty";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import * as Table from "@/Components/ui/table";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type Registry = {
    id: string;
    name: string;
    provider: string;
    endpoint: string;
    username: string;
    credentialName: string;
    managed: boolean;
    createdAt: string;
  };
  let {
    auth,
    registries,
  }: { auth: { email: string }; registries: Registry[] } = $props();
  let createDialogOpen = $state(false);
  let preset = $state("docker_hub");
  const form = useForm(() => ({
    name: "Docker Hub",
    endpoint: "docker.io",
    username: "",
    accessToken: "",
  }));

  function selectPreset() {
    if (preset === "docker_hub") {
      $form.name = "Docker Hub";
      $form.endpoint = "docker.io";
    } else {
      $form.name = "";
      $form.endpoint = "";
    }
  }

  function openCreateDialog() {
    preset = "docker_hub";
    $form.reset();
    selectPreset();
    createDialogOpen = true;
  }

  function submit(event: SubmitEvent) {
    event.preventDefault();
    $form.post(routes.registryResourceCreate(), {
      preserveScroll: true,
      onSuccess: () => {
        createDialogOpen = false;
        $form.reset();
      },
      onError: () => (createDialogOpen = true),
    });
  }
</script>

<svelte:head><title>Image Registry</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader
      eyebrow="Connections"
      title="Image Registry"
      description="Publish Application images to the DeployCrate-managed registry, Docker Hub, or another authenticated OCI registry."
    >
      {#snippet actions()}<Button type="button" onclick={openCreateDialog}
          >Add registry</Button
        >{/snippet}
    </PageHeader>

    {#if registries.length === 0}
      <Empty.Root class="border border-dashed border-border py-14">
        <Empty.Header
          ><Empty.Media variant="icon"><BoxesIcon /></Empty.Media><Empty.Title
            >No image registries</Empty.Title
          ><Empty.Description
            >Connect an external OCI registry to publish and deploy Application
            images.</Empty.Description
          ></Empty.Header
        >
      </Empty.Root>
    {:else}
      <div class="overflow-x-auto border border-border">
        <Table.Root>
          <Table.Header
            ><Table.Row
              ><Table.Head>Registry</Table.Head><Table.Head>Endpoint</Table.Head
              ><Table.Head>Status</Table.Head></Table.Row
            ></Table.Header
          >
          <Table.Body>
            {#each registries as registry (registry.id)}
              <Table.Row
                class="cursor-pointer"
                onclick={() =>
                  router.visit(routes.registryResourceShow(registry.id))}
              >
                <Table.Cell class="font-medium">{registry.name}</Table.Cell>
                <Table.Cell class="font-mono text-xs"
                  >{registry.endpoint}</Table.Cell
                >
                <Table.Cell
                  ><StatusBadge
                    status={registry.managed ? "managed" : "external"}
                  /></Table.Cell
                >
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </div>
    {/if}
  </div>

  <Dialog.Root bind:open={createDialogOpen}>
    <Dialog.Content class="sm:max-w-2xl" showCloseButton={!$form.processing}>
      <form class="grid gap-5" onsubmit={submit}>
        <Dialog.Header
          ><Dialog.Title>Add image registry</Dialog.Title><Dialog.Description
            >Credentials are verified with Docker before the access token is
            encrypted and stored. Use a scoped access token instead of an
            account password.</Dialog.Description
          ></Dialog.Header
        >
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Registry type"
            ><NativeSelect.Root
              bind:value={preset}
              onchange={selectPreset}
              class="w-full"
              ><NativeSelect.Option value="docker_hub"
                >Docker Hub</NativeSelect.Option
              ><NativeSelect.Option value="custom"
                >Custom OCI registry</NativeSelect.Option
              ></NativeSelect.Root
            ></FormField
          >
          <FormField label="Display name" error={$form.errors.name}
            ><Input
              bind:value={$form.name}
              required
              disabled={$form.processing}
            /></FormField
          >
          <FormField label="Registry endpoint" error={$form.errors.endpoint}
            ><Input
              bind:value={$form.endpoint}
              placeholder="ghcr.io"
              readonly={preset === "docker_hub"}
              required
              disabled={$form.processing}
            /></FormField
          >
          <FormField label="Username" error={$form.errors.username}
            ><Input
              bind:value={$form.username}
              autocomplete="username"
              required
              disabled={$form.processing}
            /></FormField
          >
          <FormField label="Access token" error={$form.errors.accessToken}
            ><Input
              type="password"
              bind:value={$form.accessToken}
              autocomplete="new-password"
              required
              disabled={$form.processing}
            /></FormField
          >
        </div>
        <Dialog.Footer
          ><Button
            type="button"
            variant="outline"
            disabled={$form.processing}
            onclick={() => (createDialogOpen = false)}>Cancel</Button
          ><Button
            type="submit"
            disabled={$form.processing}
            aria-busy={$form.processing}
            >{#if $form.processing}<Spinner />{/if}Connect registry</Button
          ></Dialog.Footer
        >
      </form>
    </Dialog.Content>
  </Dialog.Root>
</DashboardLayout>
