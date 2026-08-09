<script lang="ts">
  import { Link, useForm } from "@inertiajs/svelte";
  import BuildpackSettingsEditor, {
    type BuildpackSettings,
    type BuildServer,
  } from "@/Components/BuildpackSettingsEditor.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import FormField from "@/Components/FormField.svelte";
  import { Input } from "@/Components/ui/input";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  type Repository = {
    id: string;
    githubInstallationId: string;
    fullName: string;
  };
  type Registry = { id: string; name: string; endpoint: string };
  let {
    auth,
    application,
    options,
    updateUrl,
    returnUrl,
    navigation = "application",
  }: {
    auth: { email: string };
    application: any;
    options: {
      installations: any[];
      repositories: Repository[];
      registries: Registry[];
      buildServers: BuildServer[];
    };
    updateUrl: string;
    returnUrl: string;
    navigation?: string;
  } = $props();
  const applicationNavigation = $derived(
    navigation === "application"
      ? { id: application.id, name: application.name }
      : null,
  );
  const environmentNavigation = $derived(
    navigation === "environment"
      ? {
          applicationId: application.id,
          applicationName: application.name,
          id: application.environmentId,
          name: application.environmentName,
        }
      : null,
  );
  const form = useForm(() => ({
    sourceType: application.sourceType ?? "buildpacks",
    applicationName: "",
    applicationSlug: "",
    environmentName: "",
    environmentSlug: "",
    environmentKind: "",
    githubInstallationId:
      application.sourceType === "image"
        ? (options.installations[0]?.id ?? "")
        : application.installationId,
    githubRepositoryId:
      application.sourceType === "image" ? "" : application.repositoryId,
    reference: application.reference,
    autoBuild: application.autoBuild,
    contextPath: application.contextPath,
    builderReference: "",
    buildpackSettings: {
      schema_version: 3,
      runtime:
        application.runtime || application.buildpackSettings?.runtime || "go",
      frontend: application.buildpackSettings?.frontend
        ? {
            runtime: "node",
            directory: application.buildpackSettings.frontend.directory ?? ".",
            script: application.buildpackSettings.frontend.script ?? "build",
          }
        : null,
    } as BuildpackSettings,
    registryResourceId: application.registryId,
    imageRepository: application.imageRepository,
    buildServerId:
      application.sourceType === "image"
        ? (options.buildServers[0]?.id ?? "")
        : application.buildServerId,
  }));
  const repositories = $derived(
    options.repositories.filter(
      (repository) =>
        repository.githubInstallationId === $form.githubInstallationId,
    ),
  );
  function submit(event: SubmitEvent) {
    event.preventDefault();
    $form.patch(updateUrl);
  }
</script>

<svelte:head><title>Edit source · {application.name}</title></svelte:head>
<DashboardLayout
  email={auth.email}
  {applicationNavigation}
  {environmentNavigation}
>
  <form class="mx-auto max-w-3xl" onsubmit={submit}>
    <Card.Root
      ><Card.Header
        ><Card.Title>Edit deployment source</Card.Title><Card.Description
          >Choose a GitHub Buildpacks build or an existing image from a
          connected registry.</Card.Description
        ></Card.Header
      >
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <FormField label="Source type"
          ><NativeSelect.Root bind:value={$form.sourceType} class="w-full"
            ><NativeSelect.Option value="buildpacks"
              >GitHub with Buildpacks</NativeSelect.Option
            ><NativeSelect.Option value="image"
              >Registry image</NativeSelect.Option
            ></NativeSelect.Root
          ></FormField
        >
        <FormField label="Registry Resource"
          ><NativeSelect.Root
            bind:value={$form.registryResourceId}
            class="w-full"
            >{#each options.registries as registry (registry.id)}<NativeSelect.Option
                value={registry.id}
                >{registry.name} · {registry.endpoint}</NativeSelect.Option
              >{/each}</NativeSelect.Root
          ></FormField
        >
        {#if $form.sourceType === "buildpacks"}
          <FormField label="GitHub account"
            ><NativeSelect.Root
              bind:value={$form.githubInstallationId}
              class="w-full"
              >{#each options.installations as installation (installation.id)}<NativeSelect.Option
                  value={installation.id}
                  >{installation.accountLogin}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Repository"
            ><NativeSelect.Root
              bind:value={$form.githubRepositoryId}
              class="w-full"
              >{#each repositories as repository (repository.id)}<NativeSelect.Option
                  value={repository.id}
                  >{repository.fullName}</NativeSelect.Option
                >{/each}</NativeSelect.Root
            ></FormField
          >
          <FormField label="Branch or full ref"
            ><Input bind:value={$form.reference} required /></FormField
          ><FormField label="Build context"
            ><Input bind:value={$form.contextPath} required /></FormField
          >
          <BuildpackSettingsEditor
            bind:settings={$form.buildpackSettings}
            bind:buildServerId={$form.buildServerId}
            buildServers={options.buildServers}
            runtimeLocked={Boolean(application.setupComplete)}
          />
          <FormField label="Output image repository"
            ><Input bind:value={$form.imageRepository} required /></FormField
          >
          <label class="flex items-center gap-2 text-sm"
            ><Checkbox bind:checked={$form.autoBuild} /> Build automatically</label
          >
        {:else}
          <FormField label="Image repository"
            ><Input
              bind:value={$form.imageRepository}
              placeholder="team/application"
              required
            /></FormField
          >
          <FormField label="Default tag or digest"
            ><Input
              bind:value={$form.reference}
              placeholder="latest"
              required
            /></FormField
          >
          <div class="border border-border bg-muted/20 p-4 sm:col-span-2">
            <p class="text-sm">
              DeployCrate resolves this reference to an immutable digest for
              each Release.
            </p>
          </div>
        {/if}
      </Card.Content><Card.Footer class="justify-between border-t border-border"
        ><Button variant="outline"
          >{#snippet child({ props })}<Link {...props} href={returnUrl}
              >Cancel</Link
            >{/snippet}</Button
        ><Button
          type="submit"
          disabled={$form.processing}
          aria-busy={$form.processing}
          >{#if $form.processing}<Spinner />{/if}Save source</Button
        ></Card.Footer
      ></Card.Root
    >
  </form>
</DashboardLayout>
