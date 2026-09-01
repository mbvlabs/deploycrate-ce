<script lang="ts" module>
  export type {
    BuildpackAdvancedSettings,
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
            scripts: ["build"],
            keep_node_runtime: false,
          })
        : null,
    };
  }

  function updateFrontendScripts(scripts: string[]) {
    if (!settings.frontend) return;
    settings = {
      ...settings,
      frontend: {
        ...settings.frontend,
        scripts,
      },
    };
  }

  function updateScript(index: number, value: string) {
    if (!settings.frontend) return;
    const scripts = [...settings.frontend.scripts];
    scripts[index] = value;
    updateFrontendScripts(scripts);
  }

  function addScript() {
    if (!settings.frontend) return;
    updateFrontendScripts([...settings.frontend.scripts, ""]);
  }

  function removeScript(index: number) {
    if (!settings.frontend || settings.frontend.scripts.length <= 1) return;
    updateFrontendScripts(
      settings.frontend.scripts.filter((_, scriptIndex) => scriptIndex !== index),
    );
  }

  function moveScript(index: number, direction: number) {
    if (!settings.frontend) return;
    const target = index + direction;
    if (target < 0 || target >= settings.frontend.scripts.length) return;
    const scripts = [...settings.frontend.scripts];
    [scripts[index], scripts[target]] = [scripts[target], scripts[index]];
    updateFrontendScripts(scripts);
  }

  function toggleKeepNodeRuntime(enabled: boolean) {
    if (!settings.frontend) return;
    settings = {
      ...settings,
      frontend: {
        ...settings.frontend,
        keep_node_runtime: enabled,
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
      const scripts: string[] = [];
      if (hints.hasBuildScript) scripts.push("build");
      if (hints.hasSSRScript) scripts.push("build:ssr");
      next.frontend = {
        runtime: "node",
        directory: hints.suggestedFrontendDirectory ?? ".",
        scripts:
          scripts.length > 0
            ? scripts
            : (next.frontend?.scripts ?? ["build"]),
        keep_node_runtime:
          hints.hasSSRScript || next.frontend?.keep_node_runtime === true,
      };
    }
    settings = next;
  }
</script>

<div class="space-y-6 sm:col-span-2">
  <section class="space-y-4">
    <div>
      <h3 class="text-sm font-medium">Application buildpack</h3>
      <p class="mt-1 text-xs text-muted-foreground">
        Framework and server used to compile the application image after source
        checkout.
      </p>
    </div>
    <div class="grid gap-5 lg:grid-cols-2">
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
    </div>
  </section>

  {#if githubRepositoryId && reference.trim()}
    <section class="space-y-3 border border-border p-4 lg:p-5">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h3 class="text-sm font-medium">Repository hints</h3>
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
        <ul class="grid gap-1 text-xs text-muted-foreground lg:grid-cols-2">
          {#if hints.hasGoMod}<li>Found go.mod in the build context.</li>{/if}
          {#if hints.hasPackageJSON}
            <li>
              Found package.json{hints.packageManager
                ? ` with ${hints.packageManager}`
                : ""}{hints.hasLockfile ? " and a lockfile" : ""}.
            </li>
          {/if}
          {#if hints.hasBuildScript}<li>Found a frontend build script.</li>{/if}
          {#if hints.hasSSRScript}<li>Found a build:ssr script.</li>{/if}
          {#if hints.suggestedGoTargets?.length}
            <li class="lg:col-span-2">
              Suggested Go targets: {hints.suggestedGoTargets.join(", ")}
            </li>
          {/if}
          {#each hints.warnings ?? [] as warning (warning)}
            <li class="text-amber-600 dark:text-amber-400 lg:col-span-2">
              {warning}
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  <div
    class="grid gap-6 xl:grid-cols-[minmax(0,1.75fr)_minmax(18rem,1fr)] xl:items-start"
  >
    <div class="space-y-6">
      <section class="space-y-5 border border-border bg-muted/10 p-4 lg:p-5">
        <div>
          <p class="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            Node.js
          </p>
          <h3 class="mt-1 text-sm font-medium">JavaScript & frontend assets</h3>
          <p class="mt-1 max-w-3xl text-xs text-muted-foreground">
            Optional pre-build for package.json projects. Installs frontend
            dependencies and runs package scripts before the application
            framework buildpack. Use this for Vite, Inertia, SSR bundles, and
            other JavaScript tooling.
          </p>
        </div>

        <div class="grid gap-4 lg:grid-cols-2">
          <label class="flex items-start gap-3 border border-border bg-background p-4">
            <Checkbox
              class="mt-1"
              checked={settings.frontend !== null}
              onCheckedChange={(selected) => toggleFrontend(selected === true)}
            />
            <span>
              <span class="block text-sm font-medium">Build frontend assets</span>
              <span class="mt-1 block text-xs text-muted-foreground">
                Run package.json scripts during the image build.
              </span>
            </span>
          </label>

          <label
            class="flex items-start gap-3 border border-border bg-background p-4 {!settings.frontend
              ? 'opacity-60'
              : ''}"
          >
            <Checkbox
              class="mt-1"
              checked={settings.frontend?.keep_node_runtime === true}
              disabled={!settings.frontend}
              onCheckedChange={(selected) =>
                toggleKeepNodeRuntime(selected === true)}
            />
            <span>
              <span class="block text-sm font-medium"
                >Keep Node.js in runtime image</span
              >
              <span class="mt-1 block text-xs text-muted-foreground">
                Leave Node available in the deployed image for SSR or other
                runtime JavaScript.
              </span>
            </span>
          </label>
        </div>

        {#if settings.frontend}
          <div class="grid gap-5 border-t border-border pt-5 lg:grid-cols-[minmax(12rem,0.45fr)_minmax(0,1fr)] lg:items-start">
            <FormField label="Node project root">
              <Input
                bind:value={settings.frontend.directory}
                placeholder="."
                required
              />
              <p class="mt-2 text-xs text-muted-foreground">
                Directory with package.json, relative to the build context.
              </p>
            </FormField>

            <div class="space-y-3">
              <div>
                <p class="text-sm font-medium">Frontend build commands</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  package.json scripts run in order after dependencies install.
                </p>
              </div>
              <div class="space-y-2">
                {#each settings.frontend.scripts as script, index (index)}
                  <div
                    class="grid gap-3 border border-border bg-background p-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end"
                  >
                    <FormField label={`Script ${index + 1}`}>
                      <Input
                        value={script}
                        oninput={(event) =>
                          updateScript(index, event.currentTarget.value)}
                        placeholder="build"
                        required
                      />
                    </FormField>
                    <div class="flex flex-wrap gap-2 lg:pb-1">
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={index === 0}
                        onclick={() => moveScript(index, -1)}>Move up</Button
                      >
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={index === settings.frontend.scripts.length - 1}
                        onclick={() => moveScript(index, 1)}>Move down</Button
                      >
                      <Button
                        type="button"
                        size="sm"
                        variant="destructive"
                        disabled={settings.frontend.scripts.length <= 1}
                        onclick={() => removeScript(index)}>Remove</Button
                      >
                    </div>
                  </div>
                {/each}
              </div>
              <Button type="button" size="sm" variant="outline" onclick={addScript}
                >Add frontend build command</Button
              >
            </div>
          </div>
        {:else}
          <p class="border border-dashed border-border px-4 py-3 text-xs text-muted-foreground">
            Enable frontend assets to configure package.json scripts, Node
            project root, and runtime Node options.
          </p>
        {/if}
      </section>

      <Collapsible.Root bind:open={advancedOpen}>
        <Collapsible.Trigger
          class="flex w-full items-center justify-between border border-border px-4 py-3 text-left text-sm font-medium"
        >
          Advanced build settings
          <span class="text-xs text-muted-foreground"
            >{advancedOpen ? "Hide" : "Show"}</span
          >
        </Collapsible.Trigger>
        <Collapsible.Content
          class="grid gap-5 border border-t-0 border-border p-4 lg:grid-cols-2 lg:p-5"
        >
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
              Optional Node version for frontend asset builds and runtime image.
            </p>
          </FormField>
          <div class="lg:col-span-2">
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

    <div class="xl:sticky xl:top-4">
      <BuildpackPipelinePreview
        {settings}
        {contextPath}
        {reference}
        {repositoryFullName}
        {goTargets}
      />
    </div>
  </div>
</div>
