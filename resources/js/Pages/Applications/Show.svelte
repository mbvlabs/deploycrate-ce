<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import ConfirmActionDialog from '@/Components/ConfirmActionDialog.svelte'
  import DataField from '@/Components/DataField.svelte'
  import StatusBadge from '@/Components/StatusBadge.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Environment = { environmentId: string; environmentName: string; environmentKind: string; setupComplete: boolean; sourceType: 'buildpacks' | 'image'; installationAccount: string; repositoryFullName: string; repositoryRemovedAt: any; installationSuspendedAt: any; reference: string; autoBuild: boolean; contextPath: string; builderReference: any; buildServerName: string; imageRepository: string; registryName: string; latestRevision: any; latestDeliveryStatus: any; latestBuildStatus: any }
  type Application = { id: string; name: string; slug: string; environments: Environment[] }
  let { auth, application }: { auth: { email: string }; application: Application } = $props()
  let archiveDialogOpen = $state(false)
  let archiveProcessing = $state(false)
  function nullValue(value: any) { return value?.String ?? value?.string ?? (typeof value === 'string' ? value : '') }
  function hasTimestamp(value: any) {
    if (typeof value === 'string') return value.trim() !== ''
    return Boolean(value?.Valid ?? value?.valid)
  }
  function healthy(environment: Environment) { return environment.sourceType === 'image' || (!hasTimestamp(environment.repositoryRemovedAt) && !hasTimestamp(environment.installationSuspendedAt)) }
  function archiveApplication() {
    if (archiveProcessing) return
    archiveProcessing = true
    router.delete(routes.applicationDestroy(application.id), {
      onSuccess: () => (archiveDialogOpen = false),
      onFinish: () => (archiveProcessing = false),
    })
  }
</script>

<svelte:head><title>{application.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">Application</p><h1 class="mt-3 text-3xl font-semibold">{application.name}</h1><p class="mt-2 font-mono text-xs text-muted-foreground">{application.slug}</p></div><div class="flex gap-2"><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationEdit(application.id)}>Edit application</Link>{/snippet}</Button><Button>{#snippet child({ props })}<Link {...props} href={routes.environmentNew(application.id)}>Add Environment</Link>{/snippet}</Button></div></header>

    <div class="space-y-5">
      {#each application.environments as environment (environment.environmentId)}
        <Card.Root>
          <Card.Header>
            <Card.Action><StatusBadge status={healthy(environment) ? 'ready' : 'degraded'} label={healthy(environment) ? 'Source ready' : 'Source degraded'} /></Card.Action>
            <Card.Title>{environment.environmentName}</Card.Title>
            <Card.Description>{environment.sourceType === 'image' ? `Repository · ${environment.registryName} · ${environment.imageRepository}` : `Buildpack · ${environment.installationAccount} · ${environment.repositoryFullName}`}</Card.Description>
          </Card.Header>
          <Card.Content class="grid gap-5 text-sm sm:grid-cols-3">
            <DataField label={environment.sourceType === 'image' ? 'Tag or digest' : 'Git reference'} value={environment.reference} />
            <DataField label="Registry" value={environment.registryName} />
            <DataField label="Image repository" value={environment.imageRepository} />
            {#if environment.sourceType === 'buildpacks'}
              <DataField label="Automatic builds" value={environment.autoBuild ? 'Enabled' : 'Disabled'} />
              <DataField label="Latest revision" value={nullValue(environment.latestRevision) || 'No push received'} />
              <DataField label="Build context" value={environment.contextPath} />
              <DataField label="Build Server" value={environment.buildServerName} />
              <DataField label="Builder" value={nullValue(environment.builderReference) || 'Default builder'} />
              <DataField label="Latest delivery" value={nullValue(environment.latestDeliveryStatus) || 'None'} />
              <DataField label="Latest build request" value={nullValue(environment.latestBuildStatus) || 'None'} />
            {/if}
          </Card.Content>
          <Card.Footer class="flex-wrap justify-end gap-2 border-t border-border">
            <Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentSourceEdit(application.id, environment.environmentId)}>Edit source</Link>{/snippet}</Button>
            <Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.environmentEdit(application.id, environment.environmentId)}>Edit configuration</Link>{/snippet}</Button>
            <Button>{#snippet child({ props })}<Link {...props} href={routes.environmentShow(application.id, environment.environmentId)}>Open Environment</Link>{/snippet}</Button>
          </Card.Footer>
        </Card.Root>
      {/each}
    </div>

    <div><Button variant="destructive" onclick={() => (archiveDialogOpen = true)}>Archive application</Button></div>
  </div>
  <ConfirmActionDialog bind:open={archiveDialogOpen} title="Archive application?" description={`Archive ${application.name} and remove it from active application workflows.`} confirmLabel="Archive application" destructive processing={archiveProcessing} onconfirm={archiveApplication} />
</DashboardLayout>
