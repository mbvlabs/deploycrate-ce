<script lang="ts">
  import * as Accordion from '@/Components/ui/accordion'
  import * as Card from '@/Components/ui/card'
  import { Separator } from '@/Components/ui/separator'
  import JsonCode from '@/Components/JsonCode.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

  type HealthCheck = {
    id: string
    name: string
    kind: string
    configuration: unknown
    intervalSeconds: number
    timeoutSeconds: number
    failureThreshold: number
    successThreshold: number
    enabled: boolean
  }

  type Database = {
    resourceId: string
    resourceCreatedAt: string
    resourceUpdatedAt: string
    resourceName: string
    category: string
    kind: string
    sharingScope: string
    ownerEnvironmentId: string
    environmentId: string
    environmentName: string
    environmentKind: string
    bindingId: string
    bindingCreatedAt: string
    bindingUpdatedAt: string
    bindingAlias: string
    bindingConfiguration: unknown
    credentialSource: string
    credentialId: string
    credentialName: string
    credentialRole: string
    credentialUsername: string
    credentialMetadata: unknown
    credentialHasEncryptedPayload: boolean
    endpointId: string
    endpointCreatedAt: string
    endpointUpdatedAt: string
    endpointName: string
    endpointRole: string
    address: string
    port: number
    protocol: string
    tlsMode: string
    databaseName: string
    external: boolean
    endpointSettings: unknown
    privateNetworkId: string
    installationId: string
    installationCreatedAt: string | null
    installationUpdatedAt: string | null
    imageReference: string
    imageDigest: string
    containerName: string
    restartPolicy: string
    installationConfiguration: unknown
    serverId: string
    serverName: string
    serverAddress: string
    statusRecorded: boolean
    installationState: string
    installedVersion: string
    serviceState: string
    health: string
    healthReason: string
    statusSource: string
    statusDetails: unknown
    statusObservedAt: string | null
    statusExpiresAt: string | null
    healthChecks: HealthCheck[]
  }

  let { auth, database }: { auth: { email: string }; database: Database } = $props()
  let openRecords = $state<string[]>([])

  const stateLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'Unknown'
  const timestamp = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not recorded'
  const credentialSourceLabel = (source: string) => source === 'app_env'
    ? 'Application environment variables'
    : stateLabel(source)
</script>

<svelte:head>
  <title>System database</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="max-w-3xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">Database</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Resource, connection, environment binding, credential delivery, installation, status, and health-check records for the system database.
        </p>
      </div>
      <p class="text-sm text-muted-foreground">{database.external ? 'Externally managed' : 'Managed by DeployCrate'}</p>
    </section>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header>
          <Card.Title>Resource</Card.Title>
          <Card.Description>The database as a DeployCrate resource.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Name</dt>
              <dd class="mt-1 text-sm font-medium">{database.resourceName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Kind</dt>
              <dd class="mt-1 text-sm capitalize">{database.kind}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Category</dt>
              <dd class="mt-1 text-sm capitalize">{database.category}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Sharing scope</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(database.sharingScope)}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Resource ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.resourceId}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Created</dt>
              <dd class="mt-1 text-sm">{timestamp(database.resourceCreatedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Updated</dt>
              <dd class="mt-1 text-sm">{timestamp(database.resourceUpdatedAt)}</dd>
            </div>
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Connection</Card.Title>
          <Card.Description>The address used by the system environment.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Database</dt>
              <dd class="mt-1 font-mono text-xs">{database.databaseName || 'Not recorded'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Management</dt>
              <dd class="mt-1 text-sm">{database.external ? 'External' : 'Local'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Address</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.address}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Port</dt>
              <dd class="mt-1 font-mono text-xs">{database.port}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Protocol</dt>
              <dd class="mt-1 text-sm capitalize">{database.protocol}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">TLS mode</dt>
              <dd class="mt-1 text-sm">{database.tlsMode || 'Not configured'}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Connection record ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.endpointId}</dd>
            </div>
            {#if database.privateNetworkId}
              <div class="sm:col-span-2">
                <dt class="text-muted-foreground">Private network ID</dt>
                <dd class="mt-1 break-all font-mono text-xs">{database.privateNetworkId}</dd>
              </div>
            {/if}
          </dl>
        </Card.Content>
      </Card.Root>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>Environment binding and credentials</Card.Title>
        <Card.Description>How the production environment consumes this database.</Card.Description>
      </Card.Header>
      <Card.Content>
        <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
          <div>
            <dt class="text-muted-foreground">Environment</dt>
            <dd class="mt-1 text-sm font-medium">{database.environmentName}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Environment kind</dt>
            <dd class="mt-1 text-sm capitalize">{database.environmentKind}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Binding alias</dt>
            <dd class="mt-1 font-mono text-xs">{database.bindingAlias}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Credential delivery</dt>
            <dd class="mt-1 text-sm">{credentialSourceLabel(database.credentialSource)}</dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-muted-foreground">Environment ID</dt>
            <dd class="mt-1 break-all font-mono text-xs">{database.environmentId}</dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-muted-foreground">Binding ID</dt>
            <dd class="mt-1 break-all font-mono text-xs">{database.bindingId}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Binding created</dt>
            <dd class="mt-1 text-sm">{timestamp(database.bindingCreatedAt)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Binding updated</dt>
            <dd class="mt-1 text-sm">{timestamp(database.bindingUpdatedAt)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Credential record</dt>
            <dd class="mt-1 text-sm">{database.credentialId ? database.credentialName : 'Not used for this binding'}</dd>
          </div>
          {#if database.credentialId}
            <div>
              <dt class="text-muted-foreground">Credential username</dt>
              <dd class="mt-1 font-mono text-xs">{database.credentialUsername || 'Not recorded'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Encrypted payload</dt>
              <dd class="mt-1 text-sm">{database.credentialHasEncryptedPayload ? 'Stored' : 'Empty'}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Credential ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.credentialId}</dd>
            </div>
          {/if}
        </dl>
        <p class="mt-5 text-xs leading-5 text-muted-foreground">
          Secret values are never returned by this page. This binding currently receives its database credentials from the application environment.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Installation</Card.Title>
        <Card.Description>{database.installationId ? 'Local container installation record.' : 'No local installation record.'}</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if database.installationId}
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Image</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.imageReference}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Image digest</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.imageDigest || 'Not recorded'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Container</dt>
              <dd class="mt-1 font-mono text-xs">{database.containerName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Restart policy</dt>
              <dd class="mt-1 text-sm">{database.restartPolicy}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Server</dt>
              <dd class="mt-1 text-sm font-medium">{database.serverName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Server address</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.serverAddress}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Created</dt>
              <dd class="mt-1 text-sm">{timestamp(database.installationCreatedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Updated</dt>
              <dd class="mt-1 text-sm">{timestamp(database.installationUpdatedAt)}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Installation ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.installationId}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Server ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{database.serverId}</dd>
            </div>
          </dl>
        {:else}
          <p class="text-sm text-muted-foreground">This database is externally managed, so DeployCrate has no container installation to report.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header>
          <Card.Title>Observed status</Card.Title>
          <Card.Description>Latest persisted installation observation.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if database.statusRecorded}
            <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
              <div>
                <dt class="text-muted-foreground">Installation state</dt>
                <dd class="mt-1 text-sm capitalize">{stateLabel(database.installationState)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Service state</dt>
                <dd class="mt-1 text-sm capitalize">{stateLabel(database.serviceState)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Health</dt>
                <dd class="mt-1 text-sm capitalize">{stateLabel(database.health)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Installed version</dt>
                <dd class="mt-1 text-sm">{database.installedVersion || 'Not recorded'}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Source</dt>
                <dd class="mt-1 text-sm capitalize">{stateLabel(database.statusSource)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Observed</dt>
                <dd class="mt-1 text-sm">{timestamp(database.statusObservedAt)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Expires</dt>
                <dd class="mt-1 text-sm">{timestamp(database.statusExpiresAt)}</dd>
              </div>
              {#if database.healthReason}
                <div class="sm:col-span-2">
                  <dt class="text-muted-foreground">Health reason</dt>
                  <dd class="mt-1 text-sm">{database.healthReason}</dd>
                </div>
              {/if}
            </dl>
          {:else}
            <p class="text-sm text-muted-foreground">No installation status has been persisted yet.</p>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Health checks</Card.Title>
          <Card.Description>Configured database health-check definitions.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if database.healthChecks.length === 0}
            <p class="text-sm text-muted-foreground">No database health checks are configured.</p>
          {:else}
            <div class="space-y-4">
              {#each database.healthChecks as check (check.id)}
                <div class="border border-border bg-muted/10 p-4">
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <p class="text-sm font-medium">{check.name}</p>
                      <p class="mt-1 text-xs capitalize text-muted-foreground">{stateLabel(check.kind)}</p>
                    </div>
                    <span>{check.enabled ? 'Enabled' : 'Disabled'}</span>
                  </div>
                  <Separator class="my-4" />
                  <dl class="grid grid-cols-2 gap-4 text-xs">
                    <div><dt class="text-muted-foreground">Interval</dt><dd class="mt-1">{check.intervalSeconds}s</dd></div>
                    <div><dt class="text-muted-foreground">Timeout</dt><dd class="mt-1">{check.timeoutSeconds}s</dd></div>
                    <div><dt class="text-muted-foreground">Failure threshold</dt><dd class="mt-1">{check.failureThreshold}</dd></div>
                    <div><dt class="text-muted-foreground">Success threshold</dt><dd class="mt-1">{check.successThreshold}</dd></div>
                  </dl>
                </div>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>Raw configuration records</Card.Title>
        <Card.Description>The JSON configuration persisted with each part of the database topology.</Card.Description>
      </Card.Header>
      <Card.Content>
        <Accordion.Root type="multiple" bind:value={openRecords} class="grid gap-3">
          <Accordion.Item value="binding" class="border border-border px-5">
            <Accordion.Trigger class="py-4 hover:no-underline">Environment binding configuration</Accordion.Trigger>
            <Accordion.Content class="border-t border-border py-4">
              <JsonCode value={database.bindingConfiguration} />
            </Accordion.Content>
          </Accordion.Item>
          <Accordion.Item value="connection" class="border border-border px-5">
            <Accordion.Trigger class="py-4 hover:no-underline">Connection settings</Accordion.Trigger>
            <Accordion.Content class="border-t border-border py-4">
              <JsonCode value={database.endpointSettings} />
            </Accordion.Content>
          </Accordion.Item>
          {#if database.installationId}
            <Accordion.Item value="installation" class="border border-border px-5">
              <Accordion.Trigger class="py-4 hover:no-underline">Installation configuration</Accordion.Trigger>
              <Accordion.Content class="border-t border-border py-4">
                <JsonCode value={database.installationConfiguration} />
              </Accordion.Content>
            </Accordion.Item>
          {/if}
          {#if database.statusRecorded}
            <Accordion.Item value="status" class="border border-border px-5">
              <Accordion.Trigger class="py-4 hover:no-underline">Status details</Accordion.Trigger>
              <Accordion.Content class="border-t border-border py-4">
                <JsonCode value={database.statusDetails} />
              </Accordion.Content>
            </Accordion.Item>
          {/if}
        </Accordion.Root>
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
