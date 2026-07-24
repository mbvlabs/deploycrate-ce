<script lang="ts">
  import { Link } from '@inertiajs/svelte'

  import * as Accordion from '@/Components/ui/accordion'
  import { Button } from '@/Components/ui/button'
  import { Separator } from '@/Components/ui/separator'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type SystemOverview = {
    applicationName: string
    applicationSlug: string
    environmentName: string
    environmentKind: string
    serverName: string
    serverAddress: string
    serverStatus: string
    operatingSystem: string
    distribution: string
    distributionVersion: string
    architecture: string
    networkName: string
    networkDriver: string
    networkState: string
    databaseId: string
    databaseName: string
    databaseCategory: string
    databaseKind: string
    databaseSharingScope: string
    databaseBindingAlias: string
    databaseCredentialSource: string
    databaseHasCredential: boolean
    databaseEndpointName: string
    databaseEndpointRole: string
    databaseAddress: string
    databasePort: number
    databaseProtocol: string
    databaseTlsMode: string
    databaseExternal: boolean
    databaseHasInstallation: boolean
    databaseImageReference: string
    databaseContainerName: string
    databaseRestartPolicy: string
    databaseVolume: string
    databaseBind: string
    releaseVersion: string
    artifactReference: string
    deploymentStatus: string
    deploymentStep: string
    activeSlot: string
    activeService: string
    activeState: string
    activePort: number
    domain: string
    routeExternalId: string
    routeState: string
    observedAt: string
  }

  type SystemHealth = {
    ok: boolean
    checkedAt: string
    checks: Array<{
      name: string
      ok: boolean
      detail: string
    }>
  }

  type BackupHealthPolicy = {
    policyId: string
    targetType: string
    schedule: string
    provider: string
    bucket: string
    prefix: string
    lastStatus: string
    lastError: string
    lastSuccessfulAt: string | null
    lastVerifiedAt: string | null
    lastSizeBytes: number
    activeOrRetrying: boolean
  }

  let { auth, system, health, backups }: {
    auth: { email: string }
    system: SystemOverview
    health: SystemHealth
    backups: BackupHealthPolicy[]
  } = $props()
  let openSections = $state(['network', 'runtime', 'resource', 'backups', 'deployments'])

  const stateLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'Unknown'
  const checkLabel = (value: string) => stateLabel(value).replace(/\b\w/g, (letter) => letter.toUpperCase())
  const versionLabel = (version: string) => version ? `v${version.replace(/^v/, '')}` : 'Development build'
  const credentialSourceLabel = (source: string) => source === 'app_env' ? 'Application environment' : stateLabel(source)
  const artifactAge = (value: string | null) => {
    if (!value) return 'No verified backup'
    const hours = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 3_600_000))
    if (hours < 24) return `${hours}h ago`
    return `${Math.floor(hours / 24)}d ago`
  }
  const formatBytes = (value: number) => {
    if (!value) return 'Unknown'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
    return `${(value / (1024 ** index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
  }
  const platformLabel = $derived(
    [system.distribution, system.distributionVersion, system.architecture].filter(Boolean).join(' ') || system.operatingSystem || 'Unknown',
  )
</script>

<svelte:head>
  <title>System overview</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div class="max-w-3xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">{system.applicationName}</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          The control plane is managed as a protected system application. Its runtime, network, resources, and deployment state are available here as read-only operational data.
        </p>
      </div>
      <Button variant="outline" size="sm">
        {#snippet child({ props })}
          <Link {...props} href={routes.systemUpdate()}>Manage updates</Link>
        {/snippet}
      </Button>
    </section>

    <Accordion.Root type="multiple" bind:value={openSections} class="grid gap-3">
      <Accordion.Item value="network" class="border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Network</p>
              <p class="mt-1 font-normal text-muted-foreground">WireGuard network and public route</p>
            </div>
            <span class="capitalize text-muted-foreground">{stateLabel(system.networkState)}</span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">Network</dt>
              <dd class="mt-1 text-sm font-medium">{system.networkName || 'Not configured'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Driver</dt>
              <dd class="mt-1 text-sm capitalize">{system.networkDriver || 'Unknown'}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Domain</dt>
              <dd class="mt-1 break-all font-mono text-xs">{system.domain}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Route state</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(system.routeState)}</dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Caddy route</dt>
              <dd class="mt-1 break-all font-mono text-xs">{system.routeExternalId}</dd>
            </div>
          </dl>
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="runtime" class="border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Runtime</p>
              <p class="mt-1 font-normal text-muted-foreground">System identity, host, service, and health</p>
            </div>
            <span class={health.ok ? 'text-success' : 'text-destructive'}>
              {health.ok ? 'Healthy' : 'Attention required'}
            </span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <div class="space-y-6">
            <div>
              <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">System identity</h3>
              <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                <div>
                  <dt class="text-muted-foreground">Application</dt>
                  <dd class="mt-1 text-sm font-medium">{system.applicationName}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Application slug</dt>
                  <dd class="mt-1 font-mono text-xs">{system.applicationSlug}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Environment</dt>
                  <dd class="mt-1 text-sm font-medium">{system.environmentName}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Environment kind</dt>
                  <dd class="mt-1 text-sm capitalize">{system.environmentKind}</dd>
                </div>
              </dl>
            </div>

            <Separator />

            <div>
              <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Host</h3>
              <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                <div>
                  <dt class="text-muted-foreground">Server</dt>
                  <dd class="mt-1 text-sm font-medium">{system.serverName}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Server state</dt>
                  <dd class="mt-1 text-sm capitalize">{stateLabel(system.serverStatus)}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Address</dt>
                  <dd class="mt-1 font-mono text-xs">{system.serverAddress}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Platform</dt>
                  <dd class="mt-1 text-sm">{platformLabel}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Service</dt>
                  <dd class="mt-1 font-mono text-xs">{system.activeService}</dd>
                </div>
                <div>
                  <dt class="text-muted-foreground">Listener</dt>
                  <dd class="mt-1 font-mono text-xs">127.0.0.1:{system.activePort}</dd>
                </div>
              </dl>
            </div>

            <Separator />

            <div>
              <div class="flex flex-wrap items-end justify-between gap-3">
                <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Health checks</h3>
                <p class="text-xs text-muted-foreground">Checked {new Date(health.checkedAt).toLocaleString()}</p>
              </div>
              <div class="mt-4 grid gap-3 md:grid-cols-2">
                {#each health.checks as check (check.name)}
                  <div class="border border-border/70 bg-muted/20 p-3">
                    <div class="flex items-start justify-between gap-4">
                      <p class="text-sm font-medium">{checkLabel(check.name)}</p>
                      <span class={check.ok ? 'text-success' : 'text-destructive'}>
                        {check.ok ? 'Passed' : 'Failed'}
                      </span>
                    </div>
                    <p class="mt-1 break-words text-xs leading-5 text-muted-foreground">{check.detail}</p>
                  </div>
                {/each}
              </div>
            </div>
          </div>
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="resource" class="border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Resource</p>
              <p class="mt-1 font-normal text-muted-foreground">Database resource bound to the system environment</p>
            </div>
            <span class="capitalize text-muted-foreground">
              {system.databaseId ? (system.databaseExternal ? 'External' : 'Local') : 'Not configured'}
            </span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          {#if system.databaseId}
            <div class="space-y-6">
              <div>
                <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Resource identity</h3>
                <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                  <div>
                    <dt class="text-muted-foreground">Name</dt>
                    <dd class="mt-1 text-sm font-medium">{system.databaseName}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Category</dt>
                    <dd class="mt-1 text-sm capitalize">{system.databaseCategory}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Kind</dt>
                    <dd class="mt-1 text-sm capitalize">{system.databaseKind}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Sharing scope</dt>
                    <dd class="mt-1 text-sm capitalize">{stateLabel(system.databaseSharingScope)}</dd>
                  </div>
                  <div class="sm:col-span-2 xl:col-span-4">
                    <dt class="text-muted-foreground">Resource ID</dt>
                    <dd class="mt-1 break-all font-mono text-xs">{system.databaseId}</dd>
                  </div>
                </dl>
              </div>

              <Separator />

              <div>
                <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Environment binding</h3>
                <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                  <div>
                    <dt class="text-muted-foreground">Environment</dt>
                    <dd class="mt-1 text-sm font-medium">{system.environmentName}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Alias</dt>
                    <dd class="mt-1 font-mono text-xs">{system.databaseBindingAlias}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Credential source</dt>
                    <dd class="mt-1 text-sm">{credentialSourceLabel(system.databaseCredentialSource)}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Managed credential</dt>
                    <dd class="mt-1 text-sm">{system.databaseHasCredential ? 'Configured' : 'Not configured'}</dd>
                  </div>
                </dl>
              </div>

              <Separator />

              <div>
                <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Endpoint</h3>
                <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                  <div>
                    <dt class="text-muted-foreground">Name</dt>
                    <dd class="mt-1 text-sm font-medium">{system.databaseEndpointName}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Role</dt>
                    <dd class="mt-1 text-sm capitalize">{system.databaseEndpointRole}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Protocol</dt>
                    <dd class="mt-1 text-sm capitalize">{system.databaseProtocol}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">TLS mode</dt>
                    <dd class="mt-1 text-sm">{system.databaseTlsMode || 'Not configured'}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Address</dt>
                    <dd class="mt-1 break-all font-mono text-xs">{system.databaseAddress}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Port</dt>
                    <dd class="mt-1 font-mono text-xs">{system.databasePort}</dd>
                  </div>
                  <div>
                    <dt class="text-muted-foreground">Management</dt>
                    <dd class="mt-1 text-sm">{system.databaseExternal ? 'Externally managed' : 'Managed by DeployCrate'}</dd>
                  </div>
                </dl>
              </div>

              <Separator />

              <div>
                <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Installation</h3>
                {#if system.databaseHasInstallation}
                  <dl class="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
                    <div>
                      <dt class="text-muted-foreground">Image</dt>
                      <dd class="mt-1 break-all font-mono text-xs">{system.databaseImageReference}</dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Container</dt>
                      <dd class="mt-1 font-mono text-xs">{system.databaseContainerName}</dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Restart policy</dt>
                      <dd class="mt-1 text-sm">{system.databaseRestartPolicy}</dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Server</dt>
                      <dd class="mt-1 text-sm font-medium">{system.serverName}</dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Volume</dt>
                      <dd class="mt-1 font-mono text-xs">{system.databaseVolume || 'Not configured'}</dd>
                    </div>
                    <div>
                      <dt class="text-muted-foreground">Bind</dt>
                      <dd class="mt-1 font-mono text-xs">{system.databaseBind || 'Not configured'}</dd>
                    </div>
                  </dl>
                {:else}
                  <p class="mt-4 text-sm text-muted-foreground">
                    This resource is externally managed and has no local DeployCrate installation.
                  </p>
                {/if}
              </div>
            </div>
          {:else}
            <p class="text-sm text-muted-foreground">No active database resource is bound to the system environment.</p>
          {/if}
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="backups" class="border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Backups</p>
              <p class="mt-1 font-normal text-muted-foreground">Independent server and database recovery artifacts</p>
            </div>
            <span class="capitalize text-muted-foreground">
              {backups.length ? (backups.some((backup) => backup.activeOrRetrying) ? 'Active or retrying' : 'Configured') : 'Not configured'}
            </span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          {#if backups.length}
            <div class="grid gap-4 lg:grid-cols-2">
              {#each backups as backup (backup.policyId)}
                <div class="border border-border/70 bg-muted/20 p-4">
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <p class="text-sm font-medium capitalize">{backup.targetType}</p>
                      <p class="mt-1 font-mono text-xs text-muted-foreground">{backup.schedule}</p>
                    </div>
                    <span class={backup.lastStatus === 'failed' || backup.lastStatus === 'verification_failed' ? 'capitalize text-destructive' : 'capitalize text-muted-foreground'}>
                      {backup.activeOrRetrying ? 'Active or retrying' : stateLabel(backup.lastStatus)}
                    </span>
                  </div>
                  <dl class="mt-4 grid gap-4 sm:grid-cols-2">
                    <div>
                      <dt class="text-xs text-muted-foreground">Destination</dt>
                      <dd class="mt-1 break-all font-mono text-xs">{backup.provider.toUpperCase()} / {backup.bucket}{backup.prefix ? `/${backup.prefix}` : ''}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">Artifact age</dt>
                      <dd class="mt-1 text-sm">{artifactAge(backup.lastVerifiedAt)}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">Last successful</dt>
                      <dd class="mt-1 text-sm">{backup.lastSuccessfulAt ? new Date(backup.lastSuccessfulAt).toLocaleString() : 'Never'}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">Last verified</dt>
                      <dd class="mt-1 text-sm">{backup.lastVerifiedAt ? new Date(backup.lastVerifiedAt).toLocaleString() : 'Never'}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-muted-foreground">Size</dt>
                      <dd class="mt-1 text-sm">{formatBytes(backup.lastSizeBytes)}</dd>
                    </div>
                  </dl>
                  {#if backup.lastError}
                    <p class="mt-4 break-words text-xs leading-5 text-destructive">{backup.lastError}</p>
                  {/if}
                </div>
              {/each}
            </div>
          {:else}
            <p class="text-sm text-muted-foreground">Backups were not configured during bootstrap.</p>
          {/if}
        </Accordion.Content>
      </Accordion.Item>

      <Accordion.Item value="deployments" class="border border-border px-5">
        <Accordion.Trigger class="py-5 hover:no-underline">
          <div class="flex w-full items-center justify-between gap-6">
            <div>
              <p class="text-sm font-semibold">Deployments</p>
              <p class="mt-1 font-normal text-muted-foreground">Active release, deployment, and systemd slot</p>
            </div>
            <span class="capitalize text-muted-foreground">{stateLabel(system.deploymentStatus)}</span>
          </div>
        </Accordion.Trigger>
        <Accordion.Content class="border-t border-border py-5">
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">Release</dt>
              <dd class="mt-1 text-sm font-medium">{versionLabel(system.releaseVersion)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Deployment status</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(system.deploymentStatus)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Current step</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(system.deploymentStep)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Observed</dt>
              <dd class="mt-1 text-sm">{new Date(system.observedAt).toLocaleString()}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Active slot</dt>
              <dd class="mt-1 text-sm capitalize">{system.activeSlot}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Instance state</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(system.activeState)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Service</dt>
              <dd class="mt-1 font-mono text-xs">{system.activeService}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Listener</dt>
              <dd class="mt-1 font-mono text-xs">127.0.0.1:{system.activePort}</dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Artifact</dt>
              <dd class="mt-1 break-all font-mono text-xs">{system.artifactReference}</dd>
            </div>
          </dl>
        </Accordion.Content>
      </Accordion.Item>
    </Accordion.Root>
  </div>
</DashboardLayout>
