<script lang="ts">
  import { router } from '@inertiajs/svelte'
  import * as Accordion from '@/Components/ui/accordion'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import JsonCode from '@/Components/JsonCode.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Network = {
    networkId: string
    networkCreatedAt: string
    networkUpdatedAt: string
    networkName: string
    ownerEnvironmentId: string
    environmentId: string
    environmentName: string
    environmentBindingId: number
    environmentBindingRole: string
    environmentBindingCreated: string
    targetId: string
    targetAttachedAt: string
    serverId: string
    serverName: string
    serverAddress: string
    serverNetworkId: number
    serverDriver: string
    serverExternalId: string
    serverConfiguration: unknown
    serverState: string
    serverAppliedAt: string | null
    serverObservedAt: string | null
    serverError: string
    targetNetworkId: number
    targetDriver: string
    targetExternalId: string
    targetConfiguration: unknown
    targetState: string
    targetAppliedAt: string | null
    targetObservedAt: string | null
    targetError: string
    peerId: string
    peerPublicKey: string
    peerPrivateAddress: string
    peerEndpoint: string
    peerListenPort: number
    peerActivatedAt: string | null
    peerState: string
    peerLatestHandshakeAt: string | null
    peerObservedAt: string | null
    peerError: string
    domain: string
    routeId: string
    routeExternalId: string
    routeState: string
    routeCreatedAt: string | null
    backendWeight: number
    backendService: string
    backendState: string
    backendPort: number
  }

  type Device = {
    id: string
    name: string
    ownerEmail: string
    privateAddress: string
    activatedAt: string
    grantCount: number
    state: string
    latestHandshakeAt: string | null
    observedAt: string | null
  }

  let { auth, network, devices }: { auth: { email: string }; network: Network; devices: Device[] } = $props()
  let openRecords = $state<string[]>([])

  const stateLabel = (value: string) => value ? value.replaceAll('_', ' ') : 'Unknown'
  const timestamp = (value: string | null) => value ? new Date(value).toLocaleString() : 'Not recorded'
  const configValue = (value: unknown, key: string) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return ''
    const field = (value as Record<string, unknown>)[key]
    return field === undefined || field === null ? '' : String(field)
  }
  function revokeDevice(deviceId: string) {
    router.delete(routes.systemWireGuardDeviceDestroy(deviceId))
  }
</script>

<svelte:head>
  <title>System network</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="max-w-3xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">System</p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight">Network</h1>
        <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
          Private-network ownership, environment and server attachments, WireGuard peer state, and the active public route.
        </p>
      </div>
      <p class="text-sm capitalize text-muted-foreground">{stateLabel(network.serverState)}</p>
    </section>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header>
          <Card.Title>Private network</Card.Title>
          <Card.Description>The network owned by the system environment.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Name</dt>
              <dd class="mt-1 text-sm font-medium">{network.networkName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Environment</dt>
              <dd class="mt-1 text-sm font-medium">{network.environmentName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Binding role</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(network.environmentBindingRole)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Binding record</dt>
              <dd class="mt-1 font-mono text-xs">{network.environmentBindingId}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Network ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.networkId}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Owner environment ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.ownerEnvironmentId}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Created</dt>
              <dd class="mt-1 text-sm">{timestamp(network.networkCreatedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Updated</dt>
              <dd class="mt-1 text-sm">{timestamp(network.networkUpdatedAt)}</dd>
            </div>
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>WireGuard configuration</Card.Title>
          <Card.Description>The applied values extracted from the server attachment.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Interface</dt>
              <dd class="mt-1 font-mono text-xs">{configValue(network.serverConfiguration, 'interface') || network.serverExternalId}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Driver</dt>
              <dd class="mt-1 text-sm capitalize">{network.serverDriver}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Private address</dt>
              <dd class="mt-1 font-mono text-xs">{configValue(network.serverConfiguration, 'address')}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Network CIDR</dt>
              <dd class="mt-1 font-mono text-xs">{configValue(network.serverConfiguration, 'cidr')}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Endpoint</dt>
              <dd class="mt-1 break-all font-mono text-xs">{configValue(network.serverConfiguration, 'endpoint')}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Listen port</dt>
              <dd class="mt-1 font-mono text-xs">{configValue(network.serverConfiguration, 'listen_port')}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Peer revision</dt>
              <dd class="mt-1 break-all font-mono text-xs">{configValue(network.serverConfiguration, 'peer_revision')}</dd>
            </div>
          </dl>
        </Card.Content>
      </Card.Root>
    </div>

    <div class="grid gap-4 lg:grid-cols-2">
      <Card.Root>
        <Card.Header>
          <Card.Title>Server attachment</Card.Title>
          <Card.Description>The network applied directly to the system server.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Server</dt>
              <dd class="mt-1 text-sm font-medium">{network.serverName}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Server address</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.serverAddress}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">State</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(network.serverState)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">External ID</dt>
              <dd class="mt-1 font-mono text-xs">{network.serverExternalId}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Applied</dt>
              <dd class="mt-1 text-sm">{timestamp(network.serverAppliedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Observed</dt>
              <dd class="mt-1 text-sm">{timestamp(network.serverObservedAt)}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Server ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.serverId}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Attachment record</dt>
              <dd class="mt-1 font-mono text-xs">{network.serverNetworkId}</dd>
            </div>
            {#if network.serverError}
              <div class="sm:col-span-2">
                <dt class="text-destructive">Error</dt>
                <dd class="mt-1 text-sm text-destructive">{network.serverError}</dd>
              </div>
            {/if}
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Environment target attachment</Card.Title>
          <Card.Description>The system environment target attached to the same private network.</Card.Description>
        </Card.Header>
        <Card.Content>
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            <div>
              <dt class="text-muted-foreground">Driver</dt>
              <dd class="mt-1 text-sm capitalize">{network.targetDriver}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">State</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(network.targetState)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">External ID</dt>
              <dd class="mt-1 font-mono text-xs">{network.targetExternalId}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Attached</dt>
              <dd class="mt-1 text-sm">{timestamp(network.targetAttachedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Applied</dt>
              <dd class="mt-1 text-sm">{timestamp(network.targetAppliedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Observed</dt>
              <dd class="mt-1 text-sm">{timestamp(network.targetObservedAt)}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Target ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.targetId}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-muted-foreground">Attachment record</dt>
              <dd class="mt-1 font-mono text-xs">{network.targetNetworkId}</dd>
            </div>
            {#if network.targetError}
              <div class="sm:col-span-2">
                <dt class="text-destructive">Error</dt>
                <dd class="mt-1 text-sm text-destructive">{network.targetError}</dd>
              </div>
            {/if}
          </dl>
        </Card.Content>
      </Card.Root>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>WireGuard peer</Card.Title>
        <Card.Description>The active peer and its latest persisted status. Private key material is never returned.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if network.peerId}
          <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">Private address</dt>
              <dd class="mt-1 font-mono text-xs">{network.peerPrivateAddress}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Endpoint</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.peerEndpoint}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Listen port</dt>
              <dd class="mt-1 font-mono text-xs">{network.peerListenPort}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">State</dt>
              <dd class="mt-1 text-sm capitalize">{stateLabel(network.peerState)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Activated</dt>
              <dd class="mt-1 text-sm">{timestamp(network.peerActivatedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Latest handshake</dt>
              <dd class="mt-1 text-sm">{timestamp(network.peerLatestHandshakeAt)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Status observed</dt>
              <dd class="mt-1 text-sm">{timestamp(network.peerObservedAt)}</dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Public key</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.peerPublicKey}</dd>
            </div>
            <div class="sm:col-span-2 xl:col-span-4">
              <dt class="text-muted-foreground">Peer ID</dt>
              <dd class="mt-1 break-all font-mono text-xs">{network.peerId}</dd>
            </div>
            {#if network.peerError}
              <div class="sm:col-span-2 xl:col-span-4">
                <dt class="text-destructive">Error</dt>
                <dd class="mt-1 text-sm text-destructive">{network.peerError}</dd>
              </div>
            {/if}
          </dl>
        {:else}
          <p class="text-sm text-muted-foreground">No active WireGuard peer is recorded for the system server.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Developer WireGuard devices</Card.Title>
        <Card.Description>Roaming devices enrolled for private Resource access. Revoking a device removes every remaining grant.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if devices.length === 0}
          <p class="text-sm text-muted-foreground">No developer devices are enrolled.</p>
        {:else}
          <div class="space-y-3">
            {#each devices as device (device.id)}
              <div class="flex flex-col justify-between gap-3 border border-border p-4 sm:flex-row sm:items-center">
                <div>
                  <p class="font-medium">{device.name}</p>
                  <p class="mt-1 font-mono text-xs">{device.privateAddress} · {device.grantCount} active grant{device.grantCount === 1 ? '' : 's'}</p>
                  <p class="mt-1 text-xs text-muted-foreground">Owned by {device.ownerEmail}</p>
                  <p class="mt-1 text-xs text-muted-foreground">{stateLabel(device.state)} · latest handshake {timestamp(device.latestHandshakeAt)}</p>
                </div>
                <Button variant="destructive" size="sm" onclick={() => revokeDevice(device.id)}>Revoke device</Button>
              </div>
            {/each}
          </div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Public route</Card.Title>
        <Card.Description>The primary domain and active Caddy backend.</Card.Description>
      </Card.Header>
      <Card.Content>
        <dl class="grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
          <div>
            <dt class="text-muted-foreground">Domain</dt>
            <dd class="mt-1 break-all font-mono text-xs">{network.domain || 'Not configured'}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Route state</dt>
            <dd class="mt-1 text-sm capitalize">{stateLabel(network.routeState)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Backend service</dt>
            <dd class="mt-1 break-all font-mono text-xs">{network.backendService || 'Not configured'}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Backend listener</dt>
            <dd class="mt-1 font-mono text-xs">127.0.0.1:{network.backendPort}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Backend state</dt>
            <dd class="mt-1 text-sm capitalize">{stateLabel(network.backendState)}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Backend weight</dt>
            <dd class="mt-1 text-sm">{network.backendWeight}</dd>
          </div>
          <div>
            <dt class="text-muted-foreground">Created</dt>
            <dd class="mt-1 text-sm">{timestamp(network.routeCreatedAt)}</dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-muted-foreground">Route ID</dt>
            <dd class="mt-1 break-all font-mono text-xs">{network.routeId || 'Not recorded'}</dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-muted-foreground">Caddy external ID</dt>
            <dd class="mt-1 break-all font-mono text-xs">{network.routeExternalId || 'Not recorded'}</dd>
          </div>
        </dl>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Raw configuration records</Card.Title>
        <Card.Description>The exact JSON applied to the server and environment target attachments.</Card.Description>
      </Card.Header>
      <Card.Content>
        <Accordion.Root type="multiple" bind:value={openRecords} class="grid gap-3">
          <Accordion.Item value="server" class="border border-border px-5">
            <Accordion.Trigger class="py-4 hover:no-underline">Server network configuration</Accordion.Trigger>
            <Accordion.Content class="border-t border-border py-4">
              <JsonCode value={network.serverConfiguration} />
            </Accordion.Content>
          </Accordion.Item>
          <Accordion.Item value="target" class="border border-border px-5">
            <Accordion.Trigger class="py-4 hover:no-underline">Environment target configuration</Accordion.Trigger>
            <Accordion.Content class="border-t border-border py-4">
              <JsonCode value={network.targetConfiguration} />
            </Accordion.Content>
          </Accordion.Item>
        </Accordion.Root>
      </Card.Content>
    </Card.Root>
  </div>
</DashboardLayout>
