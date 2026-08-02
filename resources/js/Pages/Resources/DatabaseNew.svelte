<script lang="ts">
  import { useForm } from '@inertiajs/svelte'
  import * as Accordion from '@/Components/ui/accordion'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import FormField from '@/Components/FormField.svelte'
  import { Input } from '@/Components/ui/input'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Options = { servers: Array<{ id: string; name: string; kind: string; address: string }>; networks: Array<{ id: string; name: string }> }
  let { auth, options }: { auth: { email: string }; options: Options } = $props()
  const selectClass = 'h-9 w-full border border-input bg-background px-3 text-sm'
  const form = useForm(() => ({
    name: '', slug: '', engine: 'postgresql', engineVersion: '17', desiredInstallationMethod: 'docker',
    administratorUsername: 'postgres', administratorPassword: '',
    endpoint: { name: 'Primary', role: 'primary', address: '127.0.0.1', port: 5433, protocol: 'postgresql', tlsMode: 'disable', privateNetworkId: '' },
    placement: { serverId: '', nodeName: 'primary', storageName: 'Database data', storageDriver: 'docker', storageId: '', dataPath: '/var/lib/postgresql/data', imageReference: 'postgres:17-alpine', imageDigest: '', containerName: '', restartPolicy: 'unless-stopped', packageName: 'postgresql-17', packageVersion: '', serviceName: 'postgresql', configPath: '/etc/postgresql/17/main/conf.d/deploycrate.conf' },
    database: { name: '', encoding: 'UTF8', collation: '', applicationUsername: '', applicationPassword: '', resourceName: '', resourceSlug: '', sharingScope: 'environment' },
  }))
  const selectedServer = $derived(options.servers.find((server) => server.id === $form.placement.serverId))

  function selectServer(serverId: string) {
    $form.placement.serverId = serverId
    if (options.servers.find((server) => server.id === serverId)?.kind === 'worker') $form.desiredInstallationMethod = 'docker'
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    const name = $form.name.trim()
    const slug = slugify(name)
    const databaseName = slug.replaceAll('-', '_')
    const engineVersion = dataEngineVersion($form.desiredInstallationMethod, $form.engineVersion, $form.placement.imageReference)
    $form.transform((data) => ({
      ...data,
      name,
      slug,
      engine: 'postgresql',
      engineVersion,
      endpoint: { ...data.endpoint, name: 'Primary', role: 'primary', address: '127.0.0.1', protocol: 'postgresql', privateNetworkId: data.endpoint.privateNetworkId || null },
      placement: {
        ...data.placement,
        nodeName: 'primary',
        storageName: data.desiredInstallationMethod === 'docker' ? (data.placement.storageId.trim() || `${slug}-postgres-data`) : `${name} data`,
        storageDriver: data.desiredInstallationMethod === 'docker' ? 'docker' : 'filesystem',
        storageId: data.desiredInstallationMethod === 'docker' ? (data.placement.storageId.trim() || `${slug}-postgres-data`) : '',
        dataPath: '/var/lib/postgresql/data',
        imageDigest: '',
        containerName: data.placement.containerName.trim() || `${slug}-postgres`,
        restartPolicy: 'unless-stopped',
        packageName: `postgresql-${engineVersion}`,
        serviceName: 'postgresql',
        configPath: `/etc/postgresql/${engineVersion}/main/conf.d/deploycrate.conf`,
      },
      database: { ...data.database, name: databaseName, encoding: 'UTF8', collation: '', resourceName: name, resourceSlug: slug },
    }))
    $form.post(routes.resourceDatabaseCreate())
  }

  function dataEngineVersion(method: string, nativeVersion: string, imageReference: string) {
    if (method !== 'docker') return nativeVersion
    const reference = imageReference.split('@', 1)[0]
    const lastSlash = reference.lastIndexOf('/')
    const tagSeparator = reference.lastIndexOf(':')
    if (tagSeparator <= lastSlash) return 'latest'
    const tag = reference.slice(tagSeparator + 1)
    const majorVersion = tag.match(/^\d+/)?.[0]
    return majorVersion || tag || 'latest'
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
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">Configure a PostgreSQL Resource that can publish multiple Databases through its endpoints.</p>
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
        <FormField label="Installation method" error={$form.errors.desiredInstallationMethod}>
          <select class={selectClass} bind:value={$form.desiredInstallationMethod} required>
            <option value="docker">Docker</option>
            <option value="native" disabled={selectedServer?.kind === 'worker'}>Native package</option>
          </select>
        </FormField>
        <div class="sm:col-span-2">
          <FormField label="Server" error={$form.errors['placement.serverId']}>
            <select class={selectClass} value={$form.placement.serverId} onchange={(event) => selectServer(event.currentTarget.value)} required>
              <option value="">Select a Server</option>
              {#each options.servers as server}<option value={server.id}>{server.name} · {server.address}</option>{/each}
            </select>
          </FormField>
        </div>
        <div class="grid gap-5 sm:col-span-2 sm:grid-cols-2">
          {#if $form.desiredInstallationMethod === 'native'}
            <FormField label="PostgreSQL version" error={$form.errors.engineVersion}>
              <select class={selectClass} bind:value={$form.engineVersion} required>
                <option value="17">17</option>
                <option value="16">16</option>
                <option value="15">15</option>
              </select>
            </FormField>
          {/if}
          <FormField label={$form.desiredInstallationMethod === 'docker' ? 'Host port' : 'Service port'} error={$form.errors['endpoint.port']}>
            <Input type="number" min="1" max="65535" bind:value={$form.endpoint.port} required />
          </FormField>
          {#if $form.desiredInstallationMethod === 'docker'}
            <div class="border border-border bg-muted/20 px-3 py-2"><p class="text-[10px] uppercase tracking-wider text-muted-foreground">Container port</p><p class="mt-1 font-mono text-sm">5432/tcp</p></div>
          {/if}
        </div>
        <FormField label="Administrator username" error={$form.errors.administratorUsername}>
          <Input bind:value={$form.administratorUsername} autocomplete="username" required />
        </FormField>
        <FormField label="Administrator password" error={$form.errors.administratorPassword}>
          <Input type="password" bind:value={$form.administratorPassword} autocomplete="new-password" required />
        </FormField>
        <div class="grid gap-5 border-t border-border pt-5 sm:col-span-2 sm:grid-cols-2">
          <div class="sm:col-span-2">
            <p class="text-sm font-medium">Application access</p>
            <p class="mt-1 text-xs text-muted-foreground">This credential is exposed through the Resource. The cluster administrator remains internal.</p>
          </div>
          <FormField label="Application username" error={$form.errors['database.applicationUsername'] ?? $form.errors['database.username']}>
            <Input bind:value={$form.database.applicationUsername} autocomplete="username" required />
          </FormField>
          <FormField label="Application password" error={$form.errors['database.applicationPassword'] ?? $form.errors['database.secretValues.password']}>
            <Input type="password" bind:value={$form.database.applicationPassword} autocomplete="new-password" required />
          </FormField>
        </div>
        {#if $form.desiredInstallationMethod === 'docker'}
          <div class="grid gap-5 border-t border-border pt-5 sm:col-span-2 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <FormField label="Docker image" error={$form.errors['placement.imageReference']}>
                <Input bind:value={$form.placement.imageReference} required />
              </FormField>
            </div>
            <FormField label="Container name" error={$form.errors['placement.containerName']}>
              <Input bind:value={$form.placement.containerName} placeholder={`${slugify($form.name) || 'database'}-postgres`} />
            </FormField>
            <FormField label="Volume name" error={$form.errors['placement.storageId']}>
              <Input bind:value={$form.placement.storageId} placeholder={`${slugify($form.name) || 'database'}-postgres-data`} />
            </FormField>
          </div>
        {/if}
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
            </Accordion.Content>
          </Accordion.Item>
        </Accordion.Root>
      </Card.Content>
      <Card.Footer class="justify-between border-t border-border">
        <Button variant="outline" href={routes.resourceNew()}>Back</Button>
        <Button type="submit" disabled={$form.processing || !$form.name || !$form.placement.serverId || !$form.administratorPassword || !$form.database.applicationUsername || !$form.database.applicationPassword}>Create PostgreSQL</Button>
      </Card.Footer>
    </Card.Root>
  </form>
</DashboardLayout>
