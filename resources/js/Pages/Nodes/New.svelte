<script lang="ts">
  import { useForm } from "@inertiajs/svelte";
  import { Button } from "@/Components/ui/button";
  import * as Card from "@/Components/ui/card";
  import { Checkbox } from "@/Components/ui/checkbox";
  import * as Field from "@/Components/ui/field";
  import FormField from "@/Components/FormField.svelte";
  import { Input } from "@/Components/ui/input";
  import { Spinner } from "@/Components/ui/spinner";
  import { Textarea } from "@/Components/ui/textarea";
  import DashboardLayout from "@/Layouts/DashboardLayout.svelte";
  import { routes } from "@/routes";

  type BuildpackRuntime = "go" | "rails" | "laravel" | "django";
  type CapabilityKey =
    | "build"
    | "runtime"
    | "resource"
    | "database"
    | "repository";
  const capabilityOptions: Array<[CapabilityKey, string, string]> = [
    ["build", "Builds", "Buildpacks and image creation"],
    ["runtime", "Applications", "Environment workload deployments"],
    ["resource", "Resources", "Managed Resource installations"],
    ["database", "Databases", "Database cluster nodes"],
    ["repository", "Repositories", "OCI registry repositories"],
  ];
  const buildpackOptions: Array<[BuildpackRuntime, string, string]> = [
    ["go", "Go", "Go modules and Paketo process targets"],
    ["rails", "Rails", "Ruby and Rails applications (amd64)"],
    [
      "laravel",
      "Laravel",
      "PHP, Composer, NGINX, and public/ web root (amd64)",
    ],
    [
      "django",
      "Django",
      "Python applications using requirements.txt or pyproject.toml",
    ],
  ];
  const capabilityCheckboxClass =
    "mt-1 data-checked:border-foreground data-checked:bg-foreground data-checked:text-background dark:data-checked:border-foreground dark:data-checked:bg-foreground";

  let { auth }: { auth: { email: string } } = $props();
  const form = useForm(() => ({
    name: "",
    address: "",
    port: 22,
    username: "root",
    privateKey: "",
    passphrase: "",
    capabilities: {
      build: false,
      runtime: true,
      resource: true,
      database: false,
      repository: false,
      telemetry: true,
      buildpacks: { runtimes: ["go"] as BuildpackRuntime[] },
    },
  }));
  let buildpackSelections = $state<Record<BuildpackRuntime, boolean>>({
    go: true,
    rails: false,
    laravel: false,
    django: false,
  });
  function capabilityCardClass(selected: boolean) {
    return [
      "flex cursor-pointer items-start gap-3 border p-3 transition-colors hover:border-foreground/40",
      selected ? "border-foreground/40 bg-muted/40" : "border-border",
    ];
  }
  function submit(event: SubmitEvent) {
    event.preventDefault();
    $form.capabilities.buildpacks.runtimes = $form.capabilities.build
      ? buildpackOptions
          .filter(([runtime]) => buildpackSelections[runtime])
          .map(([runtime]) => runtime)
      : [];
    $form.post(routes.nodeCreate());
  }
</script>

<svelte:head><title>Add Node</title></svelte:head>

<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-3xl space-y-6" onsubmit={submit}>
    <header>
      <p
        class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary"
      >
        Infrastructure
      </p>
      <h1 class="mt-3 text-3xl font-semibold">Add node</h1>
      <p class="mt-2 text-sm text-muted-foreground">
        Connect an existing Debian 13 VPS. DeployCrate does not create the VPS.
      </p>
    </header>
    <Card.Root>
      <Card.Header
        ><Card.Title>Server access</Card.Title><Card.Description
          >The key is encrypted until permanent CA access succeeds, then
          removed.</Card.Description
        ></Card.Header
      >
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <div class="sm:col-span-2">
          <FormField label="Name" error={$form.errors.name}
            ><Input
              bind:value={$form.name}
              placeholder="Edge worker 1"
              required
              autofocus
            /></FormField
          >
        </div>
        <FormField label="Address" error={$form.errors.address}
          ><Input
            bind:value={$form.address}
            placeholder="203.0.113.10"
            required
          /></FormField
        >
        <FormField label="SSH port" error={$form.errors.port}
          ><Input
            type="number"
            min="1"
            max="65535"
            bind:value={$form.port}
            required
          /></FormField
        >
        <div class="sm:col-span-2">
          <FormField label="SSH username" error={$form.errors.username}
            ><Input
              bind:value={$form.username}
              autocomplete="username"
              required
            /></FormField
          >
        </div>
        <div class="sm:col-span-2">
          <FormField label="Private key" error={$form.errors.privateKey}
            ><Textarea
              class="min-h-44 font-mono text-xs"
              bind:value={$form.privateKey}
              autocomplete="off"
              spellcheck={false}
              required
            /></FormField
          >
        </div>
        <div class="sm:col-span-2">
          <FormField
            label="Private key passphrase"
            description="Optional. Leave empty if the key is not encrypted."
            error={$form.errors.passphrase}
            ><Input
              type="password"
              bind:value={$form.passphrase}
              autocomplete="off"
              placeholder="Leave empty if unencrypted"
            /></FormField
          >
        </div>
        <div class="sm:col-span-2 border-t border-border pt-5">
          <Field.Field data-invalid={Boolean($form.errors.capabilities)}>
            <Field.Set>
              <Field.Legend variant="label">Node capabilities</Field.Legend>
              <Field.Description>
                Choose the workloads this node may run.
              </Field.Description>
              <div class="mt-2 grid gap-3 sm:grid-cols-2">
                {#each capabilityOptions as [key, label, description] (key)}
                  <label class={capabilityCardClass($form.capabilities[key])}>
                    <Checkbox
                      class={capabilityCheckboxClass}
                      bind:checked={$form.capabilities[key]}
                    />
                    <span>
                      <span class="block text-sm font-medium">{label}</span>
                      <span class="mt-1 block text-xs text-muted-foreground"
                        >{description}</span
                      >
                    </span>
                  </label>
                  {#if key === "build" && $form.capabilities.build}
                    <div
                      class="grid gap-3 border border-border bg-muted/20 p-3 sm:col-span-2 sm:grid-cols-2"
                    >
                      <div class="sm:col-span-2">
                        <p class="text-sm font-medium">Buildpack runtimes</p>
                        <p class="mt-1 text-xs text-muted-foreground">
                          Select every application framework this Node may
                          build.
                        </p>
                      </div>
                      {#each buildpackOptions as [runtime, runtimeLabel, runtimeDescription] (runtime)}
                        <label
                          class={capabilityCardClass(
                            buildpackSelections[runtime],
                          )}
                        >
                          <Checkbox
                            class={capabilityCheckboxClass}
                            bind:checked={buildpackSelections[runtime]}
                          />
                          <span>
                            <span class="block text-sm font-medium"
                              >{runtimeLabel}</span
                            >
                            <span
                              class="mt-1 block text-xs text-muted-foreground"
                              >{runtimeDescription}</span
                            >
                          </span>
                        </label>
                      {/each}
                    </div>
                  {/if}
                {/each}
              </div>
              {#if $form.errors.capabilities}
                <Field.Error>{$form.errors.capabilities}</Field.Error>
              {/if}
            </Field.Set>
          </Field.Field>
        </div>
      </Card.Content>
      <Card.Footer class="justify-end gap-3"
        ><Button href={routes.nodes()} variant="outline">Cancel</Button><Button
          type="submit"
          disabled={$form.processing}
          aria-busy={$form.processing}
          >{#if $form.processing}<Spinner />{/if}Inspect host key</Button
        ></Card.Footer
      >
    </Card.Root>
  </form>
</DashboardLayout>
