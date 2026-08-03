<script lang="ts">
  import MailIcon from '@lucide/svelte/icons/mail'
  import NetworkIcon from '@lucide/svelte/icons/network'
  import ServerIcon from '@lucide/svelte/icons/server'
  import ShieldCheckIcon from '@lucide/svelte/icons/shield-check'

  import StatusBadge from '@/Components/StatusBadge.svelte'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'

  let { auth }: { auth: { email: string } } = $props()

  const systems = [
    { label: 'Authentication', detail: 'Session active', icon: ShieldCheckIcon },
    { label: 'Servers', detail: 'Infrastructure model ready', icon: ServerIcon },
    { label: 'Networks', detail: 'Private network model ready', icon: NetworkIcon },
    { label: 'Email', detail: 'Transactional delivery configured', icon: MailIcon },
  ]
</script>

<svelte:head>
  <title>Dashboard</title>
</svelte:head>

<DashboardLayout email={auth.email}>
  <div>
    <section class="max-w-3xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Deployment control plane</p>
      <h1 class="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">DeployCrate CE is ready.</h1>
      <p class="mt-4 max-w-2xl text-sm leading-6 text-muted-foreground">
        Your self-hosted workspace is online. Server and network infrastructure is ready for exploration.
      </p>
    </section>

    <section class="mt-10 grid gap-4 sm:grid-cols-2" aria-label="System status">
      {#each systems as system}
        {@const Icon = system.icon}
        <Card.Root>
          <Card.Header>
            <Card.Action>
              <StatusBadge status="ready" />
            </Card.Action>
            <span class="mb-2 grid size-8 place-items-center border border-primary/40 bg-primary/10 text-primary">
              <Icon class="size-4" />
            </span>
            <Card.Title>{system.label}</Card.Title>
            <Card.Description>{system.detail}</Card.Description>
          </Card.Header>
        </Card.Root>
      {/each}
    </section>
  </div>
</DashboardLayout>
