<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Options = { servers: Array<{ id: string; name: string; address: string }>; networks: Array<{ id: string; name: string }> }
  let { auth, options }: { auth: { email: string }; options: Options } = $props()
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm'
  const form = useForm(() => ({
    name: '', slug: '', engine: 'postgresql', engineVersion: '17', sharingMode: 'dedicated', desiredInstallationMethod: 'docker',
    administratorUsername: 'postgres', administratorPassword: '',
    endpoint: { name: 'Primary', role: 'primary', address: '127.0.0.1', port: 5432, protocol: 'postgresql', tlsMode: 'disable', privateNetworkId: '' },
    placement: { serverId: '', nodeName: 'primary', storageName: 'Database data', storageDriver: 'docker', storageId: '', dataPath: '/var/lib/postgresql/data', imageReference: 'postgres:17-alpine', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', packageName: 'postgresql-17', packageVersion: '', serviceName: 'postgresql', configPath: '/etc/postgresql/17/main/conf.d/deploycrate.conf' },
    database: { name: '', encoding: 'UTF8', collation: '', resourceName: '', resourceSlug: '', sharingScope: 'environment' },
  }))

  function submit(event: SubmitEvent) {
    event.preventDefault()
    const name = $form.name.trim()
    const slug = slugify(name)
    const databaseName = slug.replaceAll('-', '_')
    $form.transform((data) => ({
      ...data,
      name,
      slug,
      engine: 'postgresql',
      engineVersion: '17',
      sharingMode: 'dedicated',
      desiredInstallationMethod: 'docker',
      administratorUsername: 'postgres',
      endpoint: { ...data.endpoint, name: 'Primary', role: 'primary', address: '127.0.0.1', port: 5432, protocol: 'postgresql', tlsMode: 'disable', privateNetworkId: null },
      placement: {
        ...data.placement,
        nodeName: 'primary',
        storageName: `${name} data`,
        storageDriver: 'docker',
        storageId: `${slug}-postgres-data`,
        dataPath: '/var/lib/postgresql/data',
        imageReference: 'postgres:17-alpine',
        imageDigest: '',
        containerName: `${slug}-postgres`,
        restartPolicy: 'unless-stopped',
      },
      database: { ...data.database, name: databaseName, encoding: 'UTF8', collation: '', resourceName: name, resourceSlug: slug, sharingScope: 'environment' },
    }))
    $form.post(routes.resourceDatabaseCreate())
  }

  function slugify(value: string) {
    return value
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
  }
</script>

<svelte:head><title>New PostgreSQL Resource</title></svelte:head>

<DashboardLayout email={auth.email}>
  <form class="mx-auto max-w-2xl space-y-6" onsubmit={submit}>
    <header>
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">New Resource</p>
      <h1 class="mt-3 text-3xl font-semibold">Create PostgreSQL</h1>
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">Deploy a dedicated PostgreSQL 17 database in Docker with persistent storage.</p>
    </header>

    <Card.Root>
      <Card.Header>
        <Card.Title>PostgreSQL</Card.Title>
        <Card.Description>Give the database a name, choose where it runs, and set its administrator password.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-5">
        <FormField label="Name" error={$form.errors.name ?? $form.errors.slug ?? $form.errors['database.name'] ?? $form.errors['database.resourceName'] ?? $form.errors['database.resourceSlug']}>
          <Input bind:value={$form.name} placeholder="Production database" required autofocus />
        </FormField>
        <FormField label="Server" error={$form.errors['placement.serverId']}>
          <select class={selectClass} bind:value={$form.placement.serverId} required>
            <option value="">Select a Server</option>
            {#each options.servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}
          </select>
        </FormField>
        <FormField label="Administrator password" error={$form.errors.administratorPassword}>
          <Input type="password" bind:value={$form.administratorPassword} autocomplete="new-password" required />
        </FormField>
        <div class="border border-border bg-muted/20 p-4 text-xs text-muted-foreground">
          PostgreSQL 17 · Docker · dedicated database · persistent storage
        </div>
      </Card.Content>
      <Card.Footer class="justify-between border-t border-border">
        <Button variant="outline" href={routes.resourceNew()}>Back</Button>
        <Button type="submit" disabled={$form.processing || !$form.name || !$form.placement.serverId || !$form.administratorPassword}>Create PostgreSQL</Button>
      </Card.Footer>
    </Card.Root>
  </form>
</DashboardLayout>
