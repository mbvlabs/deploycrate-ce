<script lang="ts">
  import BoxesIcon from '@lucide/svelte/icons/boxes'
  import { Link } from '@inertiajs/svelte'

  import PageHeader from '@/Components/PageHeader.svelte'
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
    <PageHeader eyebrow="Environments" title="Build and deploy" description="Configure staging and production while creating their owning application, then follow builds and deployments here.">
      {#snippet actions()}<Button>{#snippet child({ props })}<Link {...props} href={routes.applicationNew()}>New application</Link>{/snippet}</Button>{/snippet}
    </PageHeader>

    {#if environments.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Media variant="icon"><BoxesIcon /></Empty.Media><Empty.Title>No environments yet</Empty.Title><Empty.Description>Create an application with its production and optional staging environment.</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {#each environments as environment (environment.id)}
          <Card.Root class="h-full">
            <Card.Header>
              <Card.Action><StatusBadge status={environment.latestBuildStatus || 'pending'} label={environment.latestBuildStatus ? `Build ${environment.latestBuildStatus}` : 'Build pending'} /></Card.Action>
              <Card.Title>{environment.name}</Card.Title>
              <Card.Description>{environment.applicationName} · {environment.kind}</Card.Description>
            </Card.Header>
            <Card.Content>
              <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-xs">
                <div class="col-span-2"><dt class="text-muted-foreground">Repository</dt><dd class="mt-1 truncate font-mono">{environment.repository || 'Source pending'}</dd></div>
                <div><dt class="text-muted-foreground">Reference</dt><dd class="mt-1 truncate font-mono">{environment.reference || 'Reference pending'}</dd></div>
                <div><dt class="text-muted-foreground">Domain</dt><dd class="mt-1 truncate">{environment.domain || 'Not configured'}</dd></div>
              </dl>
            </Card.Content>
            <Card.Footer class="mt-auto justify-end">
              <Button class="w-full sm:w-auto" size="sm" variant="outline">
                {#snippet child({ props })}<Link {...props} href={destination(environment)}>Open environment</Link>{/snippet}
              </Button>
            </Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
