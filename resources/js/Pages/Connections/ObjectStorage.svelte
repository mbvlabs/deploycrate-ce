<script lang="ts">
  import CloudIcon from '@lucide/svelte/icons/cloud'

  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

  type Destination = {
    id: string
    name: string
    provider: string
    endpoint: string
    region: string
    bucket: string
    prefix: string
    verifiedAt: string | null
    lastUsedAt: string | null
  }

  let { auth, destinations }: { auth: { email: string }; destinations: Destination[] } = $props()

  function timeLabel(value: string | null) {
    if (!value) return 'Never'
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
  }
</script>

<svelte:head><title>Object Storage</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Connections</p>
      <h1 class="mt-3 text-3xl font-semibold">Object Storage</h1>
      <p class="mt-4 text-sm leading-6 text-muted-foreground">Verified backup destinations created during bootstrap. Resource backup policies can reuse these connections without exposing or re-entering access credentials.</p>
    </header>

    {#if destinations.length === 0}
      <Card.Root><Card.Content class="grid place-items-center gap-3 py-12 text-center"><CloudIcon class="size-7 text-muted-foreground" /><p class="text-sm text-muted-foreground">No active, verified Object Storage destination is available.</p></Card.Content></Card.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2">
        {#each destinations as destination (destination.id)}
          <Card.Root>
            <Card.Header>
              <Card.Action><span class="text-xs text-success">Verified</span></Card.Action>
              <Card.Title>{destination.name}</Card.Title>
              <Card.Description>{destination.provider.toUpperCase()} backup destination</Card.Description>
            </Card.Header>
            <Card.Content class="grid gap-4 text-sm sm:grid-cols-2">
              <div><p class="text-xs text-muted-foreground">Bucket</p><p class="mt-1 font-mono">{destination.bucket}</p></div>
              <div><p class="text-xs text-muted-foreground">Region</p><p class="mt-1 font-mono">{destination.region || 'Provider default'}</p></div>
              <div class="sm:col-span-2"><p class="text-xs text-muted-foreground">Endpoint</p><p class="mt-1 break-all font-mono">{destination.endpoint || 'Provider default'}</p></div>
              <div><p class="text-xs text-muted-foreground">Prefix</p><p class="mt-1 font-mono">{destination.prefix || 'None'}</p></div>
              <div><p class="text-xs text-muted-foreground">Verified</p><p class="mt-1">{timeLabel(destination.verifiedAt)}</p></div>
              <div><p class="text-xs text-muted-foreground">Last used</p><p class="mt-1">{timeLabel(destination.lastUsedAt)}</p></div>
            </Card.Content>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
