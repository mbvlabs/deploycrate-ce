<script lang="ts">
  import { router } from "@inertiajs/svelte";
  import { untrack } from "svelte";

  import BulkEnvironmentSecretsDialog from "@/Components/BulkEnvironmentSecretsDialog.svelte";
  import BuildpackSettingsEditor, {
    defaultBuildpackSettings,
    type BuildpackSettings,
    type BuildServer,
  } from "@/Components/BuildpackSettingsEditor.svelte";
  import EnvironmentProcessEditor, {
    type ProcessInput,
  } from "@/Components/EnvironmentProcessEditor.svelte";
  import FormField from "@/Components/FormField.svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Input } from "@/Components/ui/input";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as NativeSelect from "@/Components/ui/native-select";
  import * as RadioGroup from "@/Components/ui/radio-group";
  import { Spinner } from "@/Components/ui/spinner";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { slugify } from "@/lib/slug";
  import { routes } from "@/routes";

  type Installation = { id: string; accountLogin: string };
  type Repository = {
    id: string;
    githubInstallationId: string;
    fullName: string;
    defaultBranch: string;
  };
  type Registry = { id: string; name: string; endpoint: string };
  type Server = { id: string; name: string; kind: string; address: string };
  type DNSZone = {
    zoneId: string;
    zoneName: string;
    connectionId: string;
    connectionName: string;
  };
  type ResourceOption = {
    id: string;
    name: string;
    engine: string;
    database: string;
    endpointId: string;
    endpoint: string;
    credentialId?: string;
    credential: string;
    serverId?: string;
    credentialFields: string[];
    supportsConnectionUrl: boolean;
    environmentKeys: Record<string, string>;
  };
  type EnvironmentResource = {
    resourceId: string;
    endpointId: string;
    credentialId?: string;
    alias: string;
    database: string;
    credentialProjection: "connection_url" | "individual_parts";
  };
  type EnvironmentSecret = { key: string; value: string };
  type Options = {
    installations: Installation[];
    repositories: Repository[];
    registries: Registry[];
    buildServers: BuildServer[];
    servers: Server[];
    resources: ResourceOption[];
    dnsZones: DNSZone[];
  };
  type Application = { id: string; name: string; slug: string };

  let {
    auth,
    application,
    options,
    errors = {},
    setupError = "",
  }: {
    auth: { email: string };
    application: Application;
    options: Options;
    errors?: Record<string, string>;
    setupError?: string;
  } = $props();
  const installations = $derived(options.installations ?? []);
  const repositories = $derived(options.repositories ?? []);
  const registries = $derived(options.registries ?? []);
  const buildServers = $derived(options.buildServers ?? []);
  const servers = $derived(options.servers ?? []);
  const resourceOptions = $derived(options.resources ?? []);
  const dnsZones = $derived(options.dnsZones ?? []);

  let environmentName = $state("");
  let environmentSlug = $state("");
  let environmentKind = $state("development");
  let slugCustomized = $state(false);
  let sourceType = $state<"buildpacks" | "image">("buildpacks");
  let githubInstallationId = $state(untrack(() => installations[0]?.id ?? ""));
  let githubRepositoryId = $state("");
  let reference = $state("");
  let autoBuild = $state(false);
  let buildpackSettings = $state<BuildpackSettings>(defaultBuildpackSettings());
  let contextPath = $state(".");
  let registryResourceId = $state(untrack(() => registries[0]?.id ?? ""));
  let imageRepository = $state("");
  let buildServerId = $state(
    untrack(
      () =>
        buildServers.find((server) => server.buildpacks.includes("go"))?.id ??
        "",
    ),
  );
  let serverIds = $state<string[]>([]);
  let hostname = $state("");
  let dnsMode = $state<"manual" | "cloudflare">("manual");
  let dnsZoneId = $state("");
  let containerPort = $state(8080);
  let healthPath = $state("/health");
  let processes = $state<ProcessInput[]>([
    {
      name: "web",
      kind: "web",
      command: null,
      arguments: [],
      replicas: 1,
      containerPort: 8080,
      healthPath: "/health",
      target: "",
    },
  ]);
  let resources = $state<EnvironmentResource[]>([]);
  let secrets = $state<EnvironmentSecret[]>([]);
  let deploy = $state(false);
  let selectedResource = $state("");
  let processing = $state(false);
  let responseErrors = $state<Record<string, string>>({});
  let bulkSecretDialogOpen = $state(false);
  const displayedErrors = $derived(
    Object.keys(responseErrors).length > 0 ? responseErrors : errors,
  );
  const repositoriesForInstallation = $derived(
    repositories.filter(
      (repository) => repository.githubInstallationId === githubInstallationId,
    ),
  );
  const availableResources = $derived(
    resourceOptions.filter(
      (resource) => !resource.serverId || serverIds.includes(resource.serverId),
    ),
  );

  function attachedResourceOption(resource: EnvironmentResource) {
    return (
      resourceOptions.find(
        (option) =>
          option.id === resource.resourceId &&
          option.endpointId === resource.endpointId &&
          option.credentialId === resource.credentialId,
      ) ??
      resourceOptions.find(
        (option) =>
          option.id === resource.resourceId &&
          option.endpointId === resource.endpointId &&
          option.engine === "opentelemetry",
      )
    );
  }

  function resourceAlias(engine: string) {
    return engine.toUpperCase().replace(/[^A-Z0-9]+/g, "_");
  }

  function updateName(value: string) {
    environmentName = value;
    if (!slugCustomized) environmentSlug = slugify(value);
  }

  function repositoryFullName() {
    return (
      repositoriesForInstallation.find(
        (repository) => repository.id === githubRepositoryId,
      )?.fullName ?? ""
    );
  }

  function webGoTargets() {
    return processes
      .map((process) => process.target?.trim())
      .filter((target): target is string => Boolean(target));
  }

  function chooseInstallation(value: string) {
    githubInstallationId = value;
    githubRepositoryId = "";
  }

  function chooseRepository(value: string) {
    githubRepositoryId = value;
    const repository = repositories.find((candidate) => candidate.id === value);
    if (repository && reference.trim() === "")
      reference = repository.defaultBranch;
  }

  function toggleServer(serverId: string, selected: boolean) {
    serverIds = selected
      ? [...new Set([...serverIds, serverId])]
      : serverIds.filter((candidate) => candidate !== serverId);
    const available = new Set(
      availableResources.map((resource) => resource.id),
    );
    resources = resources.filter((resource) =>
      available.has(resource.resourceId),
    );
    selectedResource = "";
  }

  function addResource() {
    const option = availableResources.find(
      (candidate) =>
        `${candidate.id}:${candidate.endpointId}:${candidate.credentialId ?? ""}` ===
        selectedResource,
    );
    if (
      !option ||
      resources.some((resource) => resource.resourceId === option.id)
    )
      return;
    resources = [
      ...resources,
      {
        resourceId: option.id,
        endpointId: option.endpointId,
        credentialId: option.credentialId,
        alias: resourceAlias(option.engine),
        database: option.database,
        credentialProjection: option.supportsConnectionUrl
          ? "connection_url"
          : "individual_parts",
      },
    ];
    selectedResource = "";
  }

  function resourceManagedKeys(resource: EnvironmentResource) {
    const option = attachedResourceOption(resource);
    if (!option) return [];
    const logicalKeys =
      resource.credentialProjection === "connection_url"
        ? ["url"]
        : [
            "host",
            "port",
            "protocol",
            "tls_mode",
            ...(resource.database ? ["database"] : []),
            ...(option.credentialId
              ? ["username", ...option.credentialFields]
              : []),
          ];
    return logicalKeys
      .map((logicalKey) => option.environmentKeys[logicalKey])
      .filter(Boolean);
  }

  function reservedSecretKeys() {
    const keys = ["PORT"];
    for (const resource of resources) {
      keys.push(...resourceManagedKeys(resource));
    }
    return keys;
  }

  function submit(event: SubmitEvent) {
    event.preventDefault();
    processing = true;
    responseErrors = {};
    const web = processes.find((process) => process.kind === "web");
    router.post(
      routes.environmentCreate(application.id),
      {
        environmentName,
        environmentSlug,
        environmentKind,
        sourceType,
        githubInstallationId,
        githubRepositoryId,
        reference,
        autoBuild,
        contextPath,
        builderReference: "",
        buildpackSettings,
        registryResourceId,
        imageRepository,
        buildServerId,
        serverIds,
        hostname,
        dnsMode,
        dnsZoneId,
        containerPort: web?.containerPort ?? containerPort,
        healthPath: web?.healthPath ?? healthPath,
        processes,
        resources,
        secrets,
        deploy,
      },
      {
        onError: (validationErrors) => (responseErrors = validationErrors),
        onFinish: () => (processing = false),
      },
    );
  }
</script>

<svelte:head><title>Add Environment to {application.name}</title></svelte:head>

<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-5xl space-y-6" onsubmit={submit}>
    <header>
      <p
        class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
      >
        {application.name} · Environments
      </p>
      <h1 class="mt-3 text-3xl font-semibold">Add Environment</h1>
      <p class="mt-2 max-w-2xl text-sm text-muted-foreground">
        Configure the source, runtime, resources, secrets, and optional first
        deployment on one page.
      </p>
    </header>

    {#if setupError || Object.keys(displayedErrors).length > 0}<div
        class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
      >
        <p class="font-medium">The Environment could not be created.</p>
        {#if setupError}<p class="mt-1">
            {setupError}
          </p>{/if}{#each [...new Set(Object.values(displayedErrors))] as error (error)}<p
            class="mt-1"
          >
            {error}
          </p>{/each}
      </div>{/if}
    {#if registries.length === 0}<div
        class="border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
      >
        Connect a Registry Resource before adding an Environment.
      </div>{/if}

    <Card.Root>
      <Card.Header
        ><Card.Title>Environment</Card.Title><Card.Description
          >Name this Environment and describe its role.</Card.Description
        ></Card.Header
      >
      <Card.Content class="grid gap-5 sm:grid-cols-3">
        <FormField label="Name" error={displayedErrors.environmentName}
          ><Input
            value={environmentName}
            oninput={(event) => updateName(event.currentTarget.value)}
            placeholder="QA"
            required
            autofocus
          /></FormField
        >
        <FormField label="Slug" error={displayedErrors.environmentSlug}
          ><Input
            value={environmentSlug}
            oninput={(event) => {
              environmentSlug = event.currentTarget.value;
              slugCustomized = true;
            }}
            placeholder="qa"
            required
          /></FormField
        >
        <FormField label="Kind" error={displayedErrors.environmentKind}
          ><Input
            bind:value={environmentKind}
            placeholder="development"
            required
          /></FormField
        >
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header
        ><Card.Title>Build</Card.Title><Card.Description
          >Choose the source repository, buildpack settings, and image
          destination.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-5">
        <RadioGroup.Root
          bind:value={sourceType}
          class="grid gap-3 sm:grid-cols-2"
        >
          <label
            class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${sourceType === "buildpacks" ? "border-primary bg-primary/5" : "border-border"}`}
            ><RadioGroup.Item class="mt-1" value="buildpacks" /><span
              ><span class="block text-sm font-medium">Buildpack</span><span
                class="mt-1 block text-xs leading-5 text-muted-foreground"
                >Build source from GitHub and publish the resulting image.</span
              ></span
            ></label
          >
          <label
            class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${sourceType === "image" ? "border-primary bg-primary/5" : "border-border"}`}
            ><RadioGroup.Item class="mt-1" value="image" /><span
              ><span class="block text-sm font-medium">Repository</span><span
                class="mt-1 block text-xs leading-5 text-muted-foreground"
                >Deploy directly from an existing container repository.</span
              ></span
            ></label
          >
        </RadioGroup.Root>
        {#if sourceType === "buildpacks"}
          <div class="grid gap-5 border-t border-border pt-5 sm:grid-cols-2">
            <FormField label="GitHub account"
              ><NativeSelect.Root
                value={githubInstallationId}
                onchange={(event) =>
                  chooseInstallation(event.currentTarget.value)}
                class="w-full"
                required
                ><NativeSelect.Option value=""
                  >Select an account</NativeSelect.Option
                >{#each installations as installation (installation.id)}<NativeSelect.Option
                    value={installation.id}
                    >{installation.accountLogin}</NativeSelect.Option
                  >{/each}</NativeSelect.Root
              ></FormField
            >
            <FormField label="Source repository"
              ><NativeSelect.Root
                value={githubRepositoryId}
                onchange={(event) =>
                  chooseRepository(event.currentTarget.value)}
                class="w-full"
                required
                ><NativeSelect.Option value=""
                  >Select a repository</NativeSelect.Option
                >{#each repositoriesForInstallation as repository (repository.id)}<NativeSelect.Option
                    value={repository.id}
                    >{repository.fullName}</NativeSelect.Option
                  >{/each}</NativeSelect.Root
              ></FormField
            >
            <FormField label="Git reference"
              ><Input
                bind:value={reference}
                placeholder="main"
                required
              /></FormField
            >
            <BuildpackSettingsEditor
              bind:settings={buildpackSettings}
              bind:buildServerId
              {buildServers}
              {contextPath}
              {reference}
              {githubRepositoryId}
              repositoryFullName={repositoryFullName()}
              goTargets={webGoTargets()}
            />
            <FormField label="Build context"
              ><Input
                bind:value={contextPath}
                placeholder="."
                required
              /></FormField
            >
            <FormField label="Image destination"
              ><Input
                bind:value={imageRepository}
                placeholder={`team/${application.slug}-${environmentSlug || "environment"}`}
                required
              /></FormField
            >
            <FormField label="Registry"
              ><NativeSelect.Root
                bind:value={registryResourceId}
                class="w-full"
                required
                ><NativeSelect.Option value=""
                  >Select a Registry</NativeSelect.Option
                >{#each registries as registry (registry.id)}<NativeSelect.Option
                    value={registry.id}
                    >{registry.name} · {registry.endpoint}</NativeSelect.Option
                  >{/each}</NativeSelect.Root
              ></FormField
            >
            <label class="flex items-center gap-2 self-end pb-2 text-sm"
              ><Checkbox bind:checked={autoBuild} /> Build automatically on matching
              pushes</label
            >
          </div>
        {:else}
          <div class="grid gap-5 border-t border-border pt-5 sm:grid-cols-2">
            <FormField label="Registry"
              ><NativeSelect.Root
                bind:value={registryResourceId}
                class="w-full"
                required
                ><NativeSelect.Option value=""
                  >Select a Registry</NativeSelect.Option
                >{#each registries as registry (registry.id)}<NativeSelect.Option
                    value={registry.id}
                    >{registry.name} · {registry.endpoint}</NativeSelect.Option
                  >{/each}</NativeSelect.Root
              ></FormField
            ><FormField label="Repository"
              ><Input
                bind:value={imageRepository}
                placeholder="team/application"
                required
              /></FormField
            ><FormField label="Default tag or digest"
              ><Input
                bind:value={reference}
                placeholder="stable"
                required
              /></FormField
            >
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header
        ><Card.Title>Runtime</Card.Title><Card.Description
          >Choose target servers, DNS, and the processes that run in the built
          image.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-5">
        {#if servers.length === 0}<div
            class="flex flex-col items-start justify-between gap-4 border border-dashed border-border p-4 sm:flex-row sm:items-center"
          >
            <p class="text-sm text-muted-foreground">
              No runtime target servers are available.
            </p>
            <Button type="button" href={routes.nodeNew()} variant="outline"
              >Add node</Button
            >
          </div>{:else}<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {#each servers as server (server.id)}<label
                class={`flex cursor-pointer gap-3 border p-4 transition-colors hover:border-primary/60 ${serverIds.includes(server.id) ? "border-primary bg-primary/5" : "border-border"}`}
                ><Checkbox
                  class="mt-1"
                  checked={serverIds.includes(server.id)}
                  onCheckedChange={(selected) =>
                    toggleServer(server.id, selected === true)}
                /><span
                  ><span class="block text-sm font-medium">{server.name}</span
                  ><span class="mt-1 block text-xs text-muted-foreground"
                    >{server.kind === "worker"
                      ? server.address
                      : "Control plane"}</span
                  ></span
                ></label
              >{/each}
          </div>{/if}
        <div class="grid gap-5 sm:grid-cols-2">
          <FormField label="Domain" error={displayedErrors.hostname}
            ><Input
              bind:value={hostname}
              placeholder="qa.example.com"
              required
            /></FormField
          >
          <FormField label="DNS management"
            ><NativeSelect.Root bind:value={dnsMode} class="w-full"
              ><NativeSelect.Option value="manual"
                >Manual DNS</NativeSelect.Option
              ><NativeSelect.Option
                value="cloudflare"
                disabled={dnsZones.length === 0}
                >Cloudflare managed</NativeSelect.Option
              ></NativeSelect.Root
            ></FormField
          >
          {#if dnsMode === "cloudflare"}<FormField
              label="Cloudflare zone"
              error={displayedErrors.dnsZoneId}
              ><NativeSelect.Root bind:value={dnsZoneId} class="w-full" required
                ><NativeSelect.Option value=""
                  >Select a zone</NativeSelect.Option
                >{#each dnsZones as zone (`${zone.connectionId}:${zone.zoneId}`)}<NativeSelect.Option
                    value={zone.zoneId}
                    >{zone.zoneName} · {zone.connectionName}</NativeSelect.Option
                  >{/each}</NativeSelect.Root
              >
              <p class="mt-2 text-xs text-muted-foreground">
                The hostname must be this zone or one of its subdomains.
              </p></FormField
            >{/if}
        </div>
        <EnvironmentProcessEditor
          bind:processes
          showGoTargets={sourceType === "buildpacks" &&
            buildpackSettings.runtime === "go"}
        />
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header
        ><Card.Title>Resources</Card.Title><Card.Description
          >Attach Resources here. Injected secret names are managed from each
          Resource.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-4"
        ><div class="flex flex-col gap-2 sm:flex-row">
          <NativeSelect.Root bind:value={selectedResource} class="w-full flex-1"
            ><NativeSelect.Option value=""
              >Select a Resource</NativeSelect.Option
            >{#each availableResources as option (`${option.id}:${option.endpointId}:${option.credentialId ?? ""}`)}<NativeSelect.Option
                value={`${option.id}:${option.endpointId}:${option.credentialId ?? ""}`}
                >{option.name} · {option.engine}{option.database
                  ? ` · ${option.database}`
                  : ""} · {option.endpoint} · {option.credential ||
                  (option.engine === "opentelemetry"
                    ? "Environment credential created on attach"
                    : "without credentials")}</NativeSelect.Option
              >{/each}</NativeSelect.Root
          ><Button
            type="button"
            variant="outline"
            disabled={!selectedResource}
            onclick={addResource}>Attach</Button
          >
        </div>
        {#each resources as resource, index (resource)}<div
            class="grid gap-3 border border-border p-4 sm:grid-cols-2"
          >
            <FormField label="Connection alias"
              ><Input bind:value={resource.alias} /></FormField
            >{#if resource.database}<FormField label="Database"
                ><Input bind:value={resource.database} readonly /></FormField
              >{/if}{#if attachedResourceOption(resource)?.supportsConnectionUrl}<FormField
                label="Connection format"
                ><NativeSelect.Root
                  bind:value={resource.credentialProjection}
                  class="w-full"
                  ><NativeSelect.Option value="connection_url"
                    >Connection URL</NativeSelect.Option
                  ><NativeSelect.Option value="individual_parts"
                    >Individual parts</NativeSelect.Option
                  ></NativeSelect.Root
                ></FormField
              >{/if}
            <div
              class="border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground"
            >
              <span class="font-medium text-foreground"
                >Resource-managed keys</span
              ><span class="mt-1 block font-mono"
                >{resourceManagedKeys(resource).join(", ") ||
                  "No values projected"}</span
              >
            </div>
            <Button
              type="button"
              variant="ghost"
              onclick={() =>
                (resources = resources.filter(
                  (_, itemIndex) => itemIndex !== index,
                ))}>Remove</Button
            >
          </div>{:else}<p
            class="border border-dashed border-border p-4 text-xs text-muted-foreground"
          >
            No Resources attached.
          </p>{/each}</Card.Content
      >
    </Card.Root>

    <Card.Root>
      <Card.Header
        ><Card.Action
          ><div class="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onclick={() => (bulkSecretDialogOpen = true)}
              >Import secrets</Button
            ><Button
              type="button"
              variant="outline"
              onclick={() => (secrets = [...secrets, { key: "", value: "" }])}
              >Add secret</Button
            >
          </div></Card.Action
        ><Card.Title>Secrets</Card.Title><Card.Description
          >Add the write-only values required by this Environment.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-4"
        >{#each secrets as secret, index (secret)}<div
            class="grid gap-3 border border-border p-4 sm:grid-cols-2"
          >
            <FormField label="Key"
              ><Input bind:value={secret.key} autocomplete="off" /></FormField
            ><FormField label="Value"
              ><Input
                type="password"
                bind:value={secret.value}
                autocomplete="new-password"
              /></FormField
            ><Button
              type="button"
              variant="ghost"
              onclick={() =>
                (secrets = secrets.filter(
                  (_, itemIndex) => itemIndex !== index,
                ))}>Remove</Button
            >
          </div>{:else}<p class="text-xs text-muted-foreground">
            No secrets added.
          </p>{/each}</Card.Content
      >
    </Card.Root>

    <label class="flex items-center gap-3 border border-border bg-muted/20 p-4"
      ><Checkbox bind:checked={deploy} /><span class="text-sm font-medium"
        >Deploy after create</span
      ></label
    >

    <div
      class="flex flex-col-reverse justify-end gap-3 border-t border-border pt-5 sm:flex-row"
    >
      <Button href={routes.applicationShow(application.id)} variant="outline"
        >Cancel</Button
      ><Button
        type="submit"
        disabled={processing ||
          registries.length === 0 ||
          serverIds.length === 0}
        aria-busy={processing}
        >{#if processing}<Spinner />{/if}Add Environment</Button
      >
    </div>
  </form>

  <BulkEnvironmentSecretsDialog
    bind:open={bulkSecretDialogOpen}
    existingSecrets={secrets}
    reservedKeys={reservedSecretKeys()}
    onImport={(imported) => (secrets = [...secrets, ...imported])}
  />
</DashboardLayout>
