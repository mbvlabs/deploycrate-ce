<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import * as Accordion from '@/Components/ui/accordion'
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
      engineVersion: data.engineVersion,
      sharingMode: 'dedicated',
      endpoint: { ...data.endpoint, name: 'Primary', role: 'primary', address: '127.0.0.1', port: 5432, protocol: 'postgresql', privateNetworkId: data.endpoint.privateNetworkId || null },
      placement: {
        ...data.placement,
        nodeName: 'primary',
        storageName: `${name} data`,
        storageDriver: 'docker',
        storageId: `${slug}-postgres-data`,
        dataPath: '/var/lib/postgresql/data',
        imageDigest: '',
        containerName: `${slug}-postgres`,
        restartPolicy: 'unless-stopped',
        packageName: `postgresql-${data.engineVersion}`,
        serviceName: 'postgresql',
        configPath: `/etc/postgresql/${data.engineVersion}/main/conf.d/deploycrate.conf`,
      },
      database: { ...data.database, name: databaseName, encoding: 'UTF8', collation: '', resourceName: name, resourceSlug: slug },
    }))
    $form.post(routes.resourceDatabaseCreate())
  }

  function chooseVersion() {
    $form.placement.imageReference = `postgres:${$form.engineVersion}-alpine`
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
  <form class="mx-auto max-w-3xl space-y-6" onsubmit={submit}>
    <header>
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">New Resource</p>
      <h1 class="mt-3 text-3xl font-semibold">Create PostgreSQL</h1>
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">Configure a dedicated PostgreSQL database with persistent storage.</p>
    </header>

    <Card.Root>
      <Card.Header>
        <Card.Title>PostgreSQL</Card.Title>
        <Card.Description>Choose the database identity, runtime, and placement.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-5 sm:grid-cols-2">
        <div class="sm:col-span-2">
          <FormField label="Name" error={$form.errors.name ?? $form.errors.slug ?? $form.errors['database.name'] ?? $form.errors['database.resourceName'] ?? $form.errors['database.resourceSlug']}>
            <Input bind:value={$form.name} placeholder="Production database" required autofocus />
          </FormField>
        </div>
        <FormField label="PostgreSQL version" error={$form.errors.engineVersion}>
          <select class={selectClass} bind:value={$form.engineVersion} onchange={chooseVersion} required>
            <option value="17">17</option>
            <option value="16">16</option>
            <option value="15">15</option>
          </select>
        </FormField>
        <FormField label="Installation method" error={$form.errors.desiredInstallationMethod}>
          <select class={selectClass} bind:value={$form.desiredInstallationMethod} required>
            <option value="docker">Docker</option>
            <option value="native">Native package</option>
          </select>
        </FormField>
        <div class="sm:col-span-2">
          <FormField label="Server" error={$form.errors['placement.serverId']}>
            <select class={selectClass} bind:value={$form.placement.serverId} required>
              <option value="">Select a Server</option>
              {#each options.servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}
            </select>
          </FormField>
        </div>
        <FormField label="Administrator username" error={$form.errors.administratorUsername}>
          <Input bind:value={$form.administratorUsername} autocomplete="username" required />
        </FormField>
        <FormField label="Administrator password" error={$form.errors.administratorPassword}>
          <Input type="password" bind:value={$form.administratorPassword} autocomplete="new-password" required />
        </FormField>
        <Accordion.Root type="multiple" class="sm:col-span-2">
          <Accordion.Item value="advanced" class="border border-border px-4">
            <Accordion.Trigger class="py-4 hover:no-underline">
              <span><span class="block">Advanced settings</span><span class="mt-1 block font-normal text-muted-foreground">Networking, access policy, and runtime image</span></span>
            </Accordion.Trigger>
            <Accordion.Content class="grid gap-5 border-t border-border py-4 sm:grid-cols-2">
              <FormField label="Sharing scope" error={$form.errors['database.sharingScope']}>
                <select class={selectClass} bind:value={$form.database.sharingScope}>
                  <option value="environment">Environment</option>
                  <option value="application">Application</option>
                  <option value="global">Global</option>
                </select>
              </FormField>
              <FormField label="Private network" error={$form.errors['endpoint.privateNetworkId']}>
                <select class={selectClass} bind:value={$form.endpoint.privateNetworkId}>
                  <option value="">None</option>
                  {#each options.networks as network}<option value={network.id}>{network.name}</option>{/each}
                </select>
              </FormField>
              <FormField label="TLS mode" error={$form.errors['endpoint.tlsMode']}>
                <select class={selectClass} bind:value={$form.endpoint.tlsMode}>
                  <option value="disable">Disable</option>
                  <option value="prefer">Prefer</option>
                  <option value="require">Require</option>
                </select>
              </FormField>
              {#if $form.desiredInstallationMethod === 'docker'}
                <FormField label="Docker image" error={$form.errors['placement.imageReference']}>
                  <Input bind:value={$form.placement.imageReference} required />
                </FormField>
              {/if}
            </Accordion.Content>
          </Accordion.Item>
        </Accordion.Root>
      </Card.Content>
      <Card.Footer class="justify-between border-t border-border">
        <Button variant="outline" href={routes.resourceNew()}>Back</Button>
        <Button type="submit" disabled={$form.processing || !$form.name || !$form.placement.serverId || !$form.administratorPassword}>Create PostgreSQL</Button>
      </Card.Footer>
    </Card.Root>
  </form>
</DashboardLayout>
