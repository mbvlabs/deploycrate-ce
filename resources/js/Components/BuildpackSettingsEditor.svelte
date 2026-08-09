<script lang="ts" module>
  export type BuildpackRuntime = "go" | "rails" | "laravel" | "django";
  export type FrontendBuildSettings = {
    runtime: "node";
    directory: string;
    script: string;
  };
  export type BuildpackSettings = {
    schema_version: 3;
    runtime: BuildpackRuntime;
    frontend: FrontendBuildSettings | null;
  };
  export type BuildServer = {
    id: string;
    name: string;
    kind: string;
    address: string;
    architecture: string;
    buildpacks: BuildpackRuntime[];
  };
</script>

<script lang="ts">
  import FormField from "@/Components/FormField.svelte";
  import { Checkbox } from "@/Components/ui/checkbox";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";

  let {
    settings = $bindable(),
    buildServerId = $bindable(),
    buildServers,
    runtimeLocked = false,
  }: {
    settings: BuildpackSettings;
    buildServerId: string;
    buildServers: BuildServer[];
    runtimeLocked?: boolean;
  } = $props();

  const runtimeOptions: Array<[BuildpackRuntime, string]> = [
    ["go", "Go"],
    ["rails", "Rails"],
    ["laravel", "Laravel"],
    ["django", "Django"],
  ];
  const compatibleServers = $derived(
    buildServers.filter(
      (server) =>
        server.buildpacks.includes(settings.runtime) &&
        (!(["rails", "laravel"] as BuildpackRuntime[]).includes(
          settings.runtime,
        ) ||
          server.architecture === "amd64"),
    ),
  );

  function selectRuntime(runtime: BuildpackRuntime) {
    settings = { ...settings, runtime };
    const selected = buildServers.find((server) => server.id === buildServerId);
    const compatible =
      selected?.buildpacks.includes(runtime) &&
      (!(["rails", "laravel"] as BuildpackRuntime[]).includes(runtime) ||
        selected.architecture === "amd64");
    if (!compatible) {
      buildServerId =
        buildServers.find(
          (server) =>
            server.buildpacks.includes(runtime) &&
            (!(["rails", "laravel"] as BuildpackRuntime[]).includes(runtime) ||
              server.architecture === "amd64"),
        )?.id ?? "";
    }
  }

  function toggleFrontend(enabled: boolean) {
    settings = {
      ...settings,
      frontend: enabled
        ? (settings.frontend ?? {
            runtime: "node",
            directory: ".",
            script: "build",
          })
        : null,
    };
  }
</script>

<div class="grid gap-5 sm:col-span-2 sm:grid-cols-2">
  <FormField label="Application framework">
    <NativeSelect.Root
      value={settings.runtime}
      onchange={(event) =>
        selectRuntime(event.currentTarget.value as BuildpackRuntime)}
      class="w-full"
      required
      disabled={runtimeLocked}
    >
      {#each runtimeOptions as [runtime, label] (runtime)}
        <NativeSelect.Option value={runtime}>{label}</NativeSelect.Option>
      {/each}
    </NativeSelect.Root>
    {#if runtimeLocked}
      <p class="mt-2 text-xs text-muted-foreground">
        The framework is fixed after Environment setup.
      </p>
    {/if}
  </FormField>
  <FormField label="Build Server">
    <NativeSelect.Root bind:value={buildServerId} class="w-full" required>
      <NativeSelect.Option value=""
        >Select a compatible Server</NativeSelect.Option
      >
      {#each compatibleServers as server (server.id)}
        <NativeSelect.Option value={server.id}>
          {server.name} · {server.kind === "worker"
            ? server.address
            : "Control plane"} · {server.architecture}
        </NativeSelect.Option>
      {/each}
    </NativeSelect.Root>
    {#if compatibleServers.length === 0}
      <p class="mt-2 text-xs text-destructive">
        No Build Server advertises support for this framework.
      </p>
    {/if}
  </FormField>
  <label class="flex items-start gap-3 border border-border p-4 sm:col-span-2">
    <Checkbox
      class="mt-1"
      checked={settings.frontend !== null}
      onCheckedChange={(selected) => toggleFrontend(selected === true)}
    />
    <span>
      <span class="block text-sm font-medium">Build JavaScript assets</span>
      <span class="mt-1 block text-xs text-muted-foreground">
        Runs a package script with npm, pnpm, Yarn, or Bun before the framework
        buildpack.
      </span>
    </span>
  </label>
  {#if settings.frontend}
    <FormField label="Frontend directory">
      <Input
        bind:value={settings.frontend.directory}
        placeholder="."
        required
      />
      <p class="mt-2 text-xs text-muted-foreground">
        Relative to the Build context; useful for monorepos and nested clients.
      </p>
    </FormField>
    <FormField label="Package script">
      <Input
        bind:value={settings.frontend.script}
        placeholder="build"
        required
      />
      <p class="mt-2 text-xs text-muted-foreground">
        The matching non-empty script from package.json.
      </p>
    </FormField>
  {/if}
</div>
