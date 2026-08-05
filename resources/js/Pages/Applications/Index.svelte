<script lang="ts">
  import AppWindowIcon from '@lucide/svelte/icons/app-window'
  import { Link } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import * as Empty from '@/Components/ui/empty'
  import PageHeader from '@/Components/PageHeader.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = { id: string; environmentName: string; environmentKind: string; repositoryFullName: string; reference: string; sourceHealthy: boolean; sourceType: string }
  type Application = { id: string; name: string; slug: string; environments: Environment[] }
  let { auth, applications }: { auth: { email: string }; applications: Application[] } = $props()

  function applicationStatus(application: Application) {
    return application.environments.some((environment) => !environment.sourceHealthy) ? 'degraded' : 'ready'
  }
</script>

<svelte:head><title>Applications</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <PageHeader eyebrow="Applications" title="Build-ready services" description="Create and manage deployable services, their source configuration, and runtime environments.">
      {#snippet actions()}<Button>{#snippet child({ props })}<Link {...props} href={routes.applicationNew()}>New application</Link>{/snippet}</Button>{/snippet}
    </PageHeader>
    {#if applications.length === 0}
      <Empty.Root class="border border-dashed border-border py-14"><Empty.Header><Empty.Media variant="icon"><AppWindowIcon /></Empty.Media><Empty.Title>No applications yet</Empty.Title><Empty.Description>Create an application to configure its source, environments, and deployment targets.</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {#each applications as application (application.id)}
          <Card.Root class="h-full">
            <Card.Header><Card.Action><StatusBadge status={applicationStatus(application)} label={applicationStatus(application) === 'ready' ? 'Sources ready' : 'Source degraded'} /></Card.Action><Card.Title>{application.name}</Card.Title><Card.Description>{application.environments.length} {application.environments.length === 1 ? 'environment' : 'environments'} · {application.slug}</Card.Description></Card.Header>
            <Card.Content>
              {#if application.environments.length > 0}
                <div class="divide-y divide-border border border-border">{#each application.environments as environment (environment.id)}<div class="flex items-start justify-between gap-4 p-3"><div class="min-w-0"><p class="text-sm font-medium">{environment.environmentName}</p><p class="mt-1 capitalize text-xs text-muted-foreground">{environment.environmentKind} · {environment.sourceType}</p><p class="mt-1 truncate font-mono text-xs text-muted-foreground">{environment.repositoryFullName || 'Source pending'} · {environment.reference || 'Reference pending'}</p></div><StatusBadge status={environment.sourceHealthy ? 'ready' : 'degraded'} label={environment.sourceHealthy ? 'Source ready' : 'Source degraded'} /></div>{/each}</div>
              {:else}
                <Empty.Root class="border border-dashed border-border py-8"><Empty.Header><Empty.Title>No environments</Empty.Title><Empty.Description>Add an environment to make this application deployable.</Empty.Description></Empty.Header></Empty.Root>
              {/if}
            </Card.Content>
            <Card.Footer class="mt-auto justify-end"><Button class="w-full sm:w-auto" size="sm" variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationShow(application.id)}>Open application</Link>{/snippet}</Button></Card.Footer>
          </Card.Root>
        {/each}
      </div>
    {/if}
  </div>
</DashboardLayout>
