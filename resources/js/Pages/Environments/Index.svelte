<script lang="ts">
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import { Link } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = {
    id: string
    name: string
    kind: string
    applicationId: string
    applicationName: string
    setupComplete: boolean
    domain: string
    repository: string
    reference: string
    latestBuildStatus: string
  }

  let { auth, environments }: { auth: { email: string }; environments: Environment[] } = $props()
  const destination = (environment: Environment) => environment.setupComplete
    ? routes.environmentShow(environment.applicationId, environment.id)
    : routes.environmentSetup(environment.applicationId, environment.id)
</script>

<svelte:head><title>Environments</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Environments</p>
        <h1 class="mt-3 text-3xl font-semibold">Build and deploy</h1>
        <p class="mt-2 max-w-2xl text-sm text-muted-foreground">Create an Environment, complete its runtime setup, and follow Builds and Deployments from one workspace.</p>
      </div>
      <Button>{#snippet child({ props })}<Link {...props} href={routes.environmentNew()}>Create environment</Link>{/snippet}</Button>
    </header>

    {#if environments.length === 0}
      <Card.Root>
        <Card.Content class="grid place-items-center gap-4 py-14 text-center">
          <BoxesIcon class="size-8 text-muted-foreground" />
          <div><p class="font-medium">No Environments yet</p><p class="mt-1 text-sm text-muted-foreground">Create the owning Application, source binding, and initial Environment together.</p></div>
          <Button>{#snippet child({ props })}<Link {...props} href={routes.environmentNew()}>Create environment</Link>{/snippet}</Button>
        </Card.Content>
      </Card.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {#each environments as environment (environment.id)}
          <Card.Root>
            <Card.Header>
              <Card.Action><span class:text-success={environment.setupComplete} class:text-primary={!environment.setupComplete} class="text-xs">{environment.setupComplete ? 'Ready' : 'Setup required'}</span></Card.Action>
              <Card.Title>{environment.name}</Card.Title>
              <Card.Description>{environment.applicationName} · {environment.kind}</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-2 text-xs">
              <p class="font-mono">{environment.repository || 'Source pending'}</p>
              <p class="font-mono text-muted-foreground">{environment.reference || 'Reference pending'}</p>
              <p class="text-muted-foreground">{environment.domain || 'Domain not configured'}{environment.latestBuildStatus ? ` · Build ${environment.latestBuildStatus}` : ''}</p>
            </Card.Content>
            <Card.Footer class="border-t border-border">
              <Button size="sm" variant={environment.setupComplete ? 'outline' : 'default'}>
                {#snippet child({ props })}<Link {...props} href={destination(environment)}>{environment.setupComplete ? 'Open environment' : 'Complete setup'}</Link>{/snippet}
              </Button>
            </Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
