<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  let { auth }: { auth: { email: string } } = $props()
  const form = useForm(() => ({ name: '', address: '', port: 22, username: 'root', privateKey: '', passphrase: '', capabilities: { build: false, runtime: true, resource: true, database: false, repository: false, telemetry: true } }))
  const textareaClass = 'min-h-44 w-full border border-input bg-background px-3 py-2 font-mono text-xs'
  function submit(event: SubmitEvent) { event.preventDefault(); $form.post(routes.nodeCreate()) }
</script>

<svelte:head><title>Add Node</title></svelte:head>

<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-3xl space-y-6" onsubmit={submit}>
    <header><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Infrastructure</p><h1 class="mt-3 text-3xl font-semibold">Add node</h1><p class="mt-2 text-sm text-muted-foreground">Connect an existing Debian 13 VPS. DeployCrate does not create the VPS.</p></header>
    <Card.Root>
      <Card.Header><Card.Title>Server access</Card.Title><Card.Description>The key is encrypted until permanent CA access succeeds, then removed.</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <div class="sm:col-span-2"><FormField label="Name" error={$form.errors.name}><Input bind:value={$form.name} placeholder="Edge worker 1" required autofocus /></FormField></div>
        <FormField label="Address" error={$form.errors.address}><Input bind:value={$form.address} placeholder="203.0.113.10" required /></FormField>
        <FormField label="SSH port" error={$form.errors.port}><Input type="number" min="1" max="65535" bind:value={$form.port} required /></FormField>
        <div class="sm:col-span-2"><FormField label="SSH username" error={$form.errors.username}><Input bind:value={$form.username} autocomplete="username" required /></FormField></div>
        <div class="sm:col-span-2"><FormField label="Private key" error={$form.errors.privateKey}><textarea class={textareaClass} bind:value={$form.privateKey} autocomplete="off" spellcheck="false" required></textarea></FormField></div>
        <div class="sm:col-span-2"><FormField label="Private key passphrase" error={$form.errors.passphrase}><Input type="password" bind:value={$form.passphrase} autocomplete="off" /></FormField></div>
        <div class="sm:col-span-2 border-t border-border pt-5">
          <FormField label="Node capabilities" error={$form.errors.capabilities}>
            <div class="mt-2 grid gap-3 sm:grid-cols-2">
              <label class="flex items-start gap-3 border border-border p-3"><input type="checkbox" class="mt-1" bind:checked={$form.capabilities.build} /><span><span class="block text-sm font-medium">Builds</span><span class="mt-1 block text-xs text-muted-foreground">Buildpacks and image creation</span></span></label>
              <label class="flex items-start gap-3 border border-border p-3"><input type="checkbox" class="mt-1" bind:checked={$form.capabilities.runtime} /><span><span class="block text-sm font-medium">Applications</span><span class="mt-1 block text-xs text-muted-foreground">Environment workload deployments</span></span></label>
              <label class="flex items-start gap-3 border border-border p-3"><input type="checkbox" class="mt-1" bind:checked={$form.capabilities.resource} /><span><span class="block text-sm font-medium">Resources</span><span class="mt-1 block text-xs text-muted-foreground">Managed Resource installations</span></span></label>
              <label class="flex items-start gap-3 border border-border p-3"><input type="checkbox" class="mt-1" bind:checked={$form.capabilities.database} /><span><span class="block text-sm font-medium">Databases</span><span class="mt-1 block text-xs text-muted-foreground">Database cluster nodes</span></span></label>
              <label class="flex items-start gap-3 border border-border p-3"><input type="checkbox" class="mt-1" bind:checked={$form.capabilities.repository} /><span><span class="block text-sm font-medium">Repositories</span><span class="mt-1 block text-xs text-muted-foreground">OCI registry repositories</span></span></label>
            </div>
          </FormField>
        </div>
      </Card.Content>
      <Card.Footer class="justify-end gap-3"><Button href={routes.nodes()} variant="outline">Cancel</Button><Button type="submit" disabled={$form.processing}>Inspect host key</Button></Card.Footer>
    </Card.Root>
  </form>
</DashboardLayout>
