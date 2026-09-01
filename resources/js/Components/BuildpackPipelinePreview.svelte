<script lang="ts">
  import type { BuildpackSettings } from "@/lib/buildpack-settings";

  let {
    settings,
    contextPath = ".",
    reference = "",
    repositoryFullName = "",
    goTargets = [],
  }: {
    settings: BuildpackSettings;
    contextPath?: string;
    reference?: string;
    repositoryFullName?: string;
    goTargets?: string[];
  } = $props();

  const runtimeLabels: Record<BuildpackSettings["runtime"], string> = {
    go: "Go",
    rails: "Rails",
    laravel: "Laravel",
    django: "Django",
  };

  const steps = $derived.by(() => {
    const items: string[] = [];
    const ref = reference.trim() || "main";
    const context = contextPath.trim() || ".";
    if (repositoryFullName) {
      items.push(`Clone ${ref} from ${repositoryFullName}`);
    } else {
      items.push(`Clone ${ref} from the selected repository`);
    }
    items.push(`Use build context ${context === "." ? "./" : context}`);
    if (settings.frontend) {
      const root =
        settings.frontend.directory === "."
          ? context === "."
            ? "./"
            : context
          : `${context === "." ? "" : `${context}/`}${settings.frontend.directory}`;
      items.push(
        `Run ${settings.frontend.script} in ${root} with npm, pnpm, Yarn, or Bun`,
      );
      if (settings.frontend.ssr?.enabled) {
        items.push(
          `Run ${settings.frontend.ssr.script}, then keep Node.js in the runtime image`,
        );
      } else {
        items.push("Remove Node dependencies after the client asset build");
      }
    }
    items.push(`Build with the ${runtimeLabels[settings.runtime]} buildpack`);
    if (settings.runtime === "go" && goTargets.length > 0) {
      items.push(`Compile ${goTargets.join(", ")}`);
    }
    if (settings.advanced?.go_version) {
      items.push(`Pin buildpack Go version to ${settings.advanced.go_version}`);
    }
    if (settings.advanced?.node_version) {
      items.push(
        `Pin buildpack Node version to ${settings.advanced.node_version}`,
      );
    }
    items.push("Publish the image to the configured registry");
    return items;
  });
</script>

<div class="border border-border bg-muted/20 p-4 sm:col-span-2">
  <p class="text-sm font-medium">Build steps preview</p>
  <ol class="mt-3 space-y-2 text-xs text-muted-foreground">
    {#each steps as step, index (step)}
      <li class="flex gap-3">
        <span
          class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center border border-border text-[10px] font-medium text-foreground"
          >{index + 1}</span
        >
        <span class="leading-5">{step}</span>
      </li>
    {/each}
  </ol>
</div>
