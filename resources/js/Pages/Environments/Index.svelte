<script lang="ts">
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import { Link } from '@inertiajs/svelte'

  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = {
    id: string
    name: string
    kind: string
    applicationId: string
    applicationName: string
    domain: string
    repository: string
    reference: string
    latestBuildStatus: string
  }

  let { auth, environments }: { auth: { email: string }; environments: Environment[] } = $props()
  const destination = (environment: Environment) => routes.environmentShow(environment.applicationId, environment.id)
</script>

<svelte:head><title>Environments</title></svelte:head>

<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Environments</p>
        <h1 class="mt-3 text-3xl font-semibold">Build and deploy</h1>
        <p class="mt-2 max-w-2xl text-sm text-muted-foreground">Configure Staging and Production while creating their owning Application, then follow Builds and Deployments here.</p>
      </div>
      <Button>{#snippet child({ props })}<Link {...props} href={routes.applicationNew()}>New application</Link>{/snippet}</Button>
    </header>

    {#if environments.length === 0}
      <Card.Root><Card.Content><Empty.Root class="py-14"><Empty.Header><Empty.Media variant="icon"><BoxesIcon /></Empty.Media><Empty.Title>No Environments yet</Empty.Title><Empty.Description>Create an Application with its Production and optional Staging Environment.</Empty.Description></Empty.Header><Empty.Content><Button>{#snippet child({ props })}<Link {...props} href={routes.applicationNew()}>New application</Link>{/snippet}</Button></Empty.Content></Empty.Root></Card.Content></Card.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {#each environments as environment (environment.id)}
          <Card.Root>
            <Card.Header>
              <Card.Action><StatusBadge status={environment.latestBuildStatus || 'pending'} label={environment.latestBuildStatus ? `Build ${environment.latestBuildStatus}` : 'Build pending'} /></Card.Action>
              <Card.Title>{environment.name}</Card.Title>
              <Card.Description>{environment.applicationName} · {environment.kind}</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-2 text-xs">
              <p class="font-mono">{environment.repository || 'Source pending'}</p>
              <p class="font-mono text-muted-foreground">{environment.reference || 'Reference pending'}</p>
              <p class="text-muted-foreground">{environment.domain || 'Domain not configured'}</p>
            </Card.Content>
            <Card.Footer class="border-t border-border">
              <Button size="sm" variant="outline">
                {#snippet child({ props })}<Link {...props} href={destination(environment)}>Open environment</Link>{/snippet}
              </Button>
            </Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
