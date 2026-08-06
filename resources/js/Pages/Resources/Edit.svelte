<script lang="ts">
  import { Link, router, useForm } from "@inertiajs/svelte";
  import * as Alert from "@/Components/ui/alert";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import * as Empty from "@/Components/ui/empty";
  import FormField from "@/Components/FormField.svelte";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import { Spinner } from "@/Components/ui/spinner";
  import { Textarea } from "@/Components/ui/textarea";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { slugify } from "@/lib/slug";
  import { routes } from "@/routes";

  type CredentialField = {
    name: string;
    label: string;
    required: boolean;
    secret: boolean;
  };
  type EnvironmentKey = { name: string; label: string; defaultKey: string };
  type Engine = {
    engine: string;
    label: string;
    resourceType: "database" | "cache" | "service";
    protocols: string[];
    endpointRoles: string[];
    tlsModes: string[];
    credentialFields: CredentialField[];
    environmentKeys: EnvironmentKey[];
    healthCheckKinds: string[];
    defaultPort: number;
  };
  type Options = {
    engines: Engine[];
    servers: Array<{ id: string; name: string; address: string }>;
    privateNetworks: Array<{
      id: string;
      name: string;
      serverIds: string[];
      serverAddresses: Record<string, string>;
    }>;
    registryCredentials: Array<{ id: string; name: string }>;
  };

  let {
    auth,
    resource,
    options,
    errors = {},
  }: {
    auth: { email: string };
    resource: any;
    options: Options;
    errors?: Record<string, string>;
  } = $props();

  const identity = useForm(() => ({
    name: resource.name,
    slug: resource.slug,
    environmentKeys: initialEnvironmentKeys(resource.engine),
  }));
  const definition = $derived(
    options.engines.find((engine) => engine.engine === resource.engine) ??
      options.engines[0],
  );
  let slugCustomized = $state(initialSlugCustomized());
  let processingKey = $state("");
  let jsonErrors = $state<Record<string, string>>({});

  function initializeFromResource<T>(initialize: (value: any) => T): T {
    return initialize(resource);
  }

  let installations = $state(
    initializeFromResource((initial) =>
      initial.installations.map((item: any) => ({
        ...item,
        imageDigest: item.imageDigest ?? "",
        registryCredentialId: item.registryCredentialId ?? "",
        configurationText: formattedJSON(item.configuration),
        hostPort: item.configuration?.portMappings?.[0]?.hostPort ?? 1,
        containerPort:
          item.configuration?.portMappings?.[0]?.containerPort ?? 1,
        protocol: item.configuration?.portMappings?.[0]?.protocol ?? "tcp",
      })),
    ),
  );

  function formattedJSON(value: unknown) {
    return JSON.stringify(value ?? {}, null, 2);
  }
  function initialSlugCustomized() {
    return resource.slug !== slugify(resource.name);
  }
  function editURL(url: string) {
    return `${url}?returnTo=edit`;
  }
  function isProcessing(key: string) {
    return processingKey === key;
  }

  function initialEnvironmentKeys(engine: string) {
    const engineDefinition = options.engines.find(
      (candidate) => candidate.engine === engine,
    );
    const stored = resource.configuration?.environment_keys ?? {};
    return Object.fromEntries(
      (engineDefinition?.environmentKeys ?? []).map((key) => [
        key.name,
        stored[key.name] ?? key.defaultKey,
      ]),
    );
  }

  function updateName(name: string) {
    $identity.name = name;
    if (!slugCustomized) $identity.slug = slugify(name);
  }

  function updateSlug(slug: string) {
    $identity.slug = slug;
    slugCustomized = true;
  }

  function parseJSON(key: string, value: string): unknown | undefined {
    try {
      const parsed = JSON.parse(value || "{}");
      jsonErrors[key] = "";
      return parsed;
    } catch {
      jsonErrors[key] = "Must contain valid JSON.";
      return undefined;
    }
  }

  function patch(key: string, url: string, payload: Record<string, unknown>) {
    if (processingKey) return;
    processingKey = key;
    router.patch(editURL(url), payload, {
      preserveScroll: true,
      onFinish: () => (processingKey = ""),
    });
  }

  function saveIdentity(event: SubmitEvent) {
    event.preventDefault();
    const configuration = {
      engine: resource.engine,
      databases: resource.configuration?.databases ?? [],
      environment_keys: $identity.environmentKeys,
    };
    $identity
      .transform((values) => ({
        name: values.name,
        slug: values.slug,
        resourceType: resource.resourceType,
        configuration,
      }))
      .patch(routes.resourceUpdate(resource.id));
  }

  function saveInstallation(item: any) {
    const configuration = parseJSON(
      `installation:${item.id}`,
      item.configurationText,
    );
    if (configuration === undefined) return;
    patch(
      `installation:${item.id}`,
      routes.resourceInstallationUpdate(resource.id, item.id),
      {
        imageReference: item.imageReference,
        imageDigest: item.imageDigest ?? "",
        containerName: item.containerName,
        restartPolicy: item.restartPolicy,
        configuration,
        portMappings: [
          {
            hostPort: Number(item.hostPort),
            containerPort: Number(item.containerPort),
            protocol: item.protocol,
          },
        ],
        serverId: item.serverId,
        registryCredentialId: item.registryCredentialId ?? "",
      },
    );
  }
</script>

<svelte:head><title>Settings · {resource.name}</title></svelte:head>
<DashboardLayout email={auth.email} resourceNavigation={resource}>
  <div class="mx-auto max-w-6xl space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p
          class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
        >
          Resource configuration
        </p>
        <h1 class="mt-3 text-3xl font-semibold">{resource.name}</h1>
        <p class="mt-2 max-w-2xl text-sm text-muted-foreground">Settings</p>
      </div>
      <Button variant="outline"
        >{#snippet child({ props })}<Link
            {...props}
            href={routes.resourceShow(resource.id)}>Back to overview</Link
          >{/snippet}</Button
      >
    </header>

    {#if Object.keys(errors).length > 0}<Alert.Root variant="destructive"
        ><Alert.Title>The changes could not be saved</Alert.Title
        ><Alert.Description
          ><ul class="mt-2 list-disc pl-5">
            {#each Object.entries(errors) as [field, message]}<li>
                {field}: {message}
              </li>{/each}
          </ul></Alert.Description
        ></Alert.Root
      >{/if}

    <form onsubmit={saveIdentity}>
      <div class="space-y-6">
        <Card.Root
          ><Card.Header
            ><Card.Title>Identity</Card.Title><Card.Description
              >How this Resource is named throughout DeployCrate.</Card.Description
            ></Card.Header
          ><Card.Content class="grid gap-5 sm:grid-cols-2"
            ><FormField label="Name" error={errors.name}
              ><Input
                value={$identity.name}
                oninput={(event) => updateName(event.currentTarget.value)}
                required
              /></FormField
            ><FormField label="Slug" error={errors.slug}
              ><Input
                value={$identity.slug}
                oninput={(event) => updateSlug(event.currentTarget.value)}
                required
              /></FormField
            ></Card.Content
          ></Card.Root
        >
        <Card.Root
          ><Card.Header
            ><Card.Title>Environment secret names</Card.Title><Card.Description
              >These names are owned by the Resource. Attached Environments
              receive the values as Resource-managed secrets.</Card.Description
            ></Card.Header
          ><Card.Content class="grid gap-5 sm:grid-cols-2"
            >{#each definition.environmentKeys as key}<FormField
                label={key.label}
                error={errors[`configuration.environment_keys.${key.name}`]}
                ><Input
                  bind:value={$identity.environmentKeys[key.name]}
                  placeholder={key.defaultKey}
                  autocomplete="off"
                  required
                /></FormField
              >{/each}</Card.Content
          ><Card.Footer class="border-t border-border"
            ><Button
              type="submit"
              disabled={$identity.processing}
              aria-busy={$identity.processing}
              >{#if $identity.processing}<Spinner />{/if}Save Resource settings</Button
            ></Card.Footer
          ></Card.Root
        >
      </div>
    </form>

    <Card.Root>
      <Card.Header
        ><Card.Title>Docker installation</Card.Title><Card.Description
          >Changes to the image, ports, or storage take effect after removing
          and recreating the container from the Resource page.</Card.Description
        ></Card.Header
      >
      <Card.Content class="space-y-6">
        {#if installations.length === 0}<Empty.Root
            ><Empty.Header
              ><Empty.Title>No installation</Empty.Title><Empty.Description
                >This Resource has no Docker installation to edit.</Empty.Description
              ></Empty.Header
            ></Empty.Root
          >{/if}
        {#each installations as item (item.id)}
          <form
            class="space-y-5 border border-border p-4"
            onsubmit={(event) => {
              event.preventDefault();
              saveInstallation(item);
            }}
          >
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <FormField label="Server" error={errors.serverId}
                ><NativeSelect.Root class="w-full" bind:value={item.serverId}
                  >{#each options.servers as server}<NativeSelect.Option
                      value={server.id}
                      >{server.name} · {server.address}</NativeSelect.Option
                    >{/each}</NativeSelect.Root
                ></FormField
              >
              <FormField label="Image reference" error={errors.imageReference}
                ><Input bind:value={item.imageReference} required /></FormField
              >
              <FormField label="Image digest" error={errors.imageDigest}
                ><Input
                  bind:value={item.imageDigest}
                  placeholder="Optional"
                /></FormField
              >
              <FormField label="Container name" error={errors.containerName}
                ><Input bind:value={item.containerName} required /></FormField
              >
              <FormField label="Restart policy"
                ><NativeSelect.Root
                  class="w-full"
                  bind:value={item.restartPolicy}
                  ><NativeSelect.Option value="no"
                    >No restart</NativeSelect.Option
                  ><NativeSelect.Option value="always"
                    >Always</NativeSelect.Option
                  ><NativeSelect.Option value="on-failure"
                    >On failure</NativeSelect.Option
                  ><NativeSelect.Option value="unless-stopped"
                    >Unless stopped</NativeSelect.Option
                  ></NativeSelect.Root
                ></FormField
              >
              <FormField label="Registry credential"
                ><NativeSelect.Root
                  class="w-full"
                  bind:value={item.registryCredentialId}
                  ><NativeSelect.Option value="">None</NativeSelect.Option
                  >{#each options.registryCredentials as credential}<NativeSelect.Option
                      value={credential.id}
                      >{credential.name}</NativeSelect.Option
                    >{/each}</NativeSelect.Root
                ></FormField
              >
              <FormField
                label="Published host port"
                error={errors["portMappings.0.hostPort"]}
                ><Input
                  type="number"
                  min="1"
                  max="65535"
                  bind:value={item.hostPort}
                  required
                /></FormField
              >
              <FormField
                label="Container port"
                error={errors["portMappings.0.containerPort"]}
                ><Input
                  type="number"
                  min="1"
                  max="65535"
                  bind:value={item.containerPort}
                  required
                /></FormField
              >
              <FormField label="Protocol"
                ><NativeSelect.Root class="w-full" bind:value={item.protocol}
                  ><NativeSelect.Option value="tcp">TCP</NativeSelect.Option
                  ><NativeSelect.Option value="udp">UDP</NativeSelect.Option
                  ></NativeSelect.Root
                ></FormField
              >
              <div class="sm:col-span-2 lg:col-span-3">
                <FormField
                  label="Installation configuration JSON"
                  error={jsonErrors[`installation:${item.id}`] ??
                    errors.configuration}
                  ><Textarea
                    class="min-h-36 font-mono"
                    bind:value={item.configurationText}
                  /></FormField
                >
              </div>
            </div>
            <Button
              type="submit"
              disabled={Boolean(processingKey)}
              aria-busy={isProcessing(`installation:${item.id}`)}
              >{#if isProcessing(`installation:${item.id}`)}<Spinner />{/if}Save
              installation</Button
            >
          </form>
        {/each}
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
