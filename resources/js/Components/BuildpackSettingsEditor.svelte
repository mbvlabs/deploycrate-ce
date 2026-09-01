<script lang="ts" module>
  export type {
    BuildpackAdvancedSettings,
    BuildpackFrontendSSRSettings,
    BuildpackRepositoryHints,
    BuildpackRuntime,
    BuildpackSettings,
    BuildServer,
    FrontendBuildSettings,
  } from "@/lib/buildpack-settings";
  export {
    defaultBuildpackSettings,
    normalizeBuildpackSettings,
  } from "@/lib/buildpack-settings";
</script>

<script lang="ts">
  import BuildpackPipelinePreview from "@/Components/BuildpackPipelinePreview.svelte";
  import FormField from "@/Components/FormField.svelte";
  import { Button } from "@/Components/ui/button";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as Collapsible from "@/Components/ui/collapsible";
  import { Input } from "@/Components/ui/input";
  import * as NativeSelect from "@/Components/ui/native-select";
  import {
    buildHintsURL,
    defaultBuildpackSettings,
    type BuildpackRepositoryHints,
    type BuildpackRuntime,
    type BuildpackSettings,
    type BuildServer,
  } from "@/lib/buildpack-settings";

  let {
    settings = $bindable(),
    buildServerId = $bindable(),
    buildServers,
    runtimeLocked = false,
    contextPath = ".",
    reference = "",
    githubRepositoryId = "",
    repositoryFullName = "",
    goTargets = [],
  }: {
    settings: BuildpackSettings;
    buildServerId: string;
    buildServers: BuildServer[];
    runtimeLocked?: boolean;
    contextPath?: string;
    reference?: string;
    githubRepositoryId?: string;
    repositoryFullName?: string;
    goTargets?: string[];
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

  let hints = $state<BuildpackRepositoryHints | null>(null);
  let hintsLoading = $state(false);
  let hintsError = $state("");
  let advancedOpen = $state(false);

  $effect(() => {
    const repositoryId = githubRepositoryId;
    const gitReference = reference.trim();
    const buildContext = contextPath.trim() || ".";
    if (!repositoryId || !gitReference) {
      hints = null;
      hintsError = "";
      hintsLoading = false;
      return;
    }
    const controller = new AbortController();
    const timeout = window.setTimeout(async () => {
      hintsLoading = true;
      hintsError = "";
      try {
        const response = await window.fetch(
          buildHintsURL(repositoryId, gitReference, buildContext),
          { signal: controller.signal },
        );
        const payload = await response.json();
        if (!response.ok) {
          throw new Error(payload.error ?? "Repository hints could not be loaded");
        }
        hints = payload as BuildpackRepositoryHints;
      } catch (error) {
        if (controller.signal.aborted) return;
        hints = null;
        hintsError =
          error instanceof Error
            ? error.message
            : "Repository hints could not be loaded";
      } finally {
        if (!controller.signal.aborted) hintsLoading = false;
      }
    }, 350);
    return () => {
      controller.abort();
      window.clearTimeout(timeout);
    };
  });

  $effect(() => {
    if (
      settings.advanced &&
      (settings.advanced.go_version ||
        settings.advanced.go_build_flags ||
        settings.advanced.node_version)
    ) {
      advancedOpen = true;
    }
  });

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
            ssr: null,
          })
        : null,
    };
  }

  function toggleSSR(enabled: boolean) {
    if (!settings.frontend) return;
    settings = {
      ...settings,
      frontend: {
        ...settings.frontend,
        ssr: enabled
          ? (settings.frontend.ssr ?? { enabled: true, script: "build:ssr" })
          : null,
      },
    };
  }

  function updateAdvanced(field: "go_version" | "go_build_flags" | "node_version", value: string) {
    const advanced = {
      ...(settings.advanced ?? {}),
      [field]: value,
    };
    const hasValues =
      advanced.go_version?.trim() ||
      advanced.go_build_flags?.trim() ||
      advanced.node_version?.trim();
    settings = {
      ...settings,
      advanced: hasValues ? advanced : null,
    };
  }

  function applyRepositoryHints() {
    if (!hints) return;
    const next = { ...settings };
    if (hints.hasPackageJson) {
      next.frontend = {
        runtime: "node",
        directory: hints.suggestedFrontendDirectory ?? ".",
        script: hints.hasBuildScript ? "build" : next.frontend?.script ?? "build",
        ssr:
          hints.hasSSRScript || next.frontend?.ssr?.enabled
            ? {
                enabled: true,
                script: hints.hasSSRScript
                  ? "build:ssr"
                  : (next.frontend?.ssr?.script ?? "build:ssr"),
              }
            : null,
      };
    }
    settings = next;
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

  {#if githubRepositoryId && reference.trim()}
    <div class="space-y-3 border border-border p-4 sm:col-span-2">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p class="text-sm font-medium">Repository hints</p>
          <p class="mt-1 text-xs text-muted-foreground">
            Detected from the selected repository, ref, and build context.
          </p>
        </div>
        {#if hints}
          <Button type="button" variant="outline" onclick={applyRepositoryHints}
            >Apply suggestions</Button
          >
        {/if}
      </div>
      {#if hintsLoading}
        <p class="text-xs text-muted-foreground">Inspecting repository…</p>
      {:else if hintsError}
        <p class="text-xs text-destructive">{hintsError}</p>
      {:else if hints}
        <ul class="space-y-1 text-xs text-muted-foreground">
          {#if hints.hasGoMod}<li>Found go.mod in the build context.</li>{/if}
          {#if hints.hasPackageJSON}
            <li>
              Found package.json{hints.packageManager
                ? ` with ${hints.packageManager}`
                : ""}{hints.hasLockfile ? " and a lockfile" : ""}.
            </li>
          {/if}
          {#if hints.hasBuildScript}<li>Found a build script.</li>{/if}
          {#if hints.hasSSRScript}<li>Found a build:ssr script.</li>{/if}
          {#if hints.suggestedGoTargets?.length}
            <li>
              Suggested Go targets: {hints.suggestedGoTargets.join(", ")}
            </li>
          {/if}
          {#each hints.warnings ?? [] as warning (warning)}
            <li class="text-amber-600 dark:text-amber-400">{warning}</li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <label class="flex items-start gap-3 border border-border p-4 sm:col-span-2">
    <Checkbox
      class="mt-1"
      checked={settings.frontend !== null}
      onCheckedChange={(selected) => toggleFrontend(selected === true)}
    />
    <span>
      <span class="block text-sm font-medium">Build JavaScript assets</span>
      <span class="mt-1 block text-xs text-muted-foreground">
        Installs dependencies and runs a package script before the framework
        buildpack. Vite, Inertia, and other tooling live in package.json.
      </span>
    </span>
  </label>

  {#if settings.frontend}
    <FormField label="Node project root">
      <Input
        bind:value={settings.frontend.directory}
        placeholder="."
        required
      />
      <p class="mt-2 text-xs text-muted-foreground">
        Directory containing package.json, relative to the build context.
      </p>
    </FormField>
    <FormField label="Client build script">
      <Input bind:value={settings.frontend.script} placeholder="build" required />
      <p class="mt-2 text-xs text-muted-foreground">
        Non-empty script from package.json, usually vite build.
      </p>
    </FormField>

    <label class="flex items-start gap-3 border border-border p-4 sm:col-span-2">
      <Checkbox
        class="mt-1"
        checked={settings.frontend.ssr?.enabled === true}
        onCheckedChange={(selected) => toggleSSR(selected === true)}
      />
      <span>
        <span class="block text-sm font-medium">Server-side rendering</span>
        <span class="mt-1 block text-xs text-muted-foreground">
          Runs a second package script and keeps Node.js in the runtime image for
          SSR. Configure the SSR start command in Processes.
        </span>
      </span>
    </label>

    {#if settings.frontend.ssr?.enabled}
      <FormField label="SSR build script">
        <Input
          bind:value={settings.frontend.ssr.script}
          placeholder="build:ssr"
          required
        />
        <p class="mt-2 text-xs text-muted-foreground">
          Builds the server bundle after the client build script finishes.
        </p>
      </FormField>
    {/if}
  {/if}

  <BuildpackPipelinePreview
    {settings}
    {contextPath}
    {reference}
    {repositoryFullName}
    {goTargets}
  />

  <Collapsible.Root bind:open={advancedOpen} class="sm:col-span-2">
    <Collapsible.Trigger class="flex w-full items-center justify-between border border-border px-4 py-3 text-left text-sm font-medium">
      Advanced build settings
      <span class="text-xs text-muted-foreground">{advancedOpen ? "Hide" : "Show"}</span>
    </Collapsible.Trigger>
    <Collapsible.Content class="grid gap-5 border border-t-0 border-border p-4 sm:grid-cols-2">
      <FormField label="Buildpack Go version">
        <Input
          value={settings.advanced?.go_version ?? ""}
          oninput={(event) =>
            updateAdvanced("go_version", event.currentTarget.value)}
          placeholder="1.23.4"
        />
        <p class="mt-2 text-xs text-muted-foreground">
          Optional Go toolchain version for the Paketo Go buildpack.
        </p>
      </FormField>
      <FormField label="Buildpack Node version">
        <Input
          value={settings.advanced?.node_version ?? ""}
          oninput={(event) =>
            updateAdvanced("node_version", event.currentTarget.value)}
          placeholder="22.11.0"
        />
        <p class="mt-2 text-xs text-muted-foreground">
          Optional Node version for the asset build and SSR runtime.
        </p>
      </FormField>
      <div class="sm:col-span-2">
        <FormField label="Buildpack Go build flags">
          <Input
            value={settings.advanced?.go_build_flags ?? ""}
            oninput={(event) =>
              updateAdvanced("go_build_flags", event.currentTarget.value)}
            placeholder="-tags=production"
          />
          <p class="mt-2 text-xs text-muted-foreground">
            Passed to go build through the Paketo Go buildpack.
          </p>
        </FormField>
      </div>
    </Collapsible.Content>
  </Collapsible.Root>
</div>
