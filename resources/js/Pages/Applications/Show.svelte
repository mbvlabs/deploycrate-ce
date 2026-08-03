<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { Button } from '@/Components/ui/button'
  import * as Card from '@/Components/ui/card'
  import DataField from '@/Components/DataField.svelte'
  import DashboardLayout from '@/Layouts/DashboardLayout.svelte'
  import { routes } from '@/routes'

  type Application = { id: string; name: string; slug: string; environmentId: string; environmentName: string; environmentKind: string; setupComplete: boolean; sourceType: 'buildpacks' | 'image'; installationAccount: string; repositoryFullName: string; repositoryRemovedAt: any; installationSuspendedAt: any; reference: string; autoBuild: boolean; contextPath: string; builderReference: any; buildServerName: string; imageRepository: string; registryName: string; latestRevision: any; latestDeliveryStatus: any; latestBuildStatus: any }
  let { auth, application }: { auth: { email: string }; application: Application } = $props()
  function nullValue(value: any) { return value?.String ?? value?.string ?? (typeof value === 'string' ? value : '') }
  const healthy = $derived(application.sourceType === 'image' || (!application.repositoryRemovedAt?.Valid && !application.installationSuspendedAt?.Valid))
</script>

<svelte:head><title>{application.name}</title></svelte:head>
<DashboardLayout email={auth.email}>
  <div class="space-y-8">
    <header class="flex items-end justify-between gap-4"><div><p class="text-[10px] font-medium uppercase tracking-[0.24em] text-primary">{application.environmentName} · {application.environmentKind}</p><h1 class="mt-3 text-3xl font-semibold">{application.name}</h1><p class="mt-2 font-mono text-xs text-muted-foreground">{application.slug}</p></div><div class="flex gap-2"><Button>{#snippet child({ props })}<Link {...props} href={application.setupComplete ? routes.environmentShow(application.id, application.environmentId) : routes.environmentSetup(application.id, application.environmentId)}>{application.setupComplete ? 'Open environment' : 'Complete setup'}</Link>{/snippet}</Button><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationEdit(application.id)}>Edit</Link>{/snippet}</Button><Button variant="outline">{#snippet child({ props })}<Link {...props} href={routes.applicationSourceEdit(application.id)}>Edit source</Link>{/snippet}</Button></div></header>
    <Card.Root>
      <Card.Header><Card.Action><span class:text-success={healthy} class:text-destructive={!healthy} class="text-xs">{healthy ? 'Connected' : 'Degraded'}</span></Card.Action><Card.Title>{application.sourceType === 'image' ? 'Registry image source' : 'GitHub source'}</Card.Title><Card.Description>{application.sourceType === 'image' ? `${application.registryName} · ${application.imageRepository}` : `${application.installationAccount} · ${application.repositoryFullName}`}</Card.Description></Card.Header>
      <Card.Content class="grid gap-5 text-sm sm:grid-cols-3">
        <DataField label={application.sourceType === 'image' ? 'Default tag or digest' : 'Reference'} value={application.reference} /><DataField label="Registry" value={application.registryName} /><DataField label="Image repository" value={application.imageRepository} />
        {#if application.sourceType === 'buildpacks'}<DataField label="Automatic builds" value={application.autoBuild ? 'Enabled' : 'Disabled'} /><DataField label="Latest revision" value={nullValue(application.latestRevision) || 'No push received'} /><DataField label="Build context" value={application.contextPath} /><DataField label="Build Server" value={application.buildServerName} /><DataField label="Builder" value={nullValue(application.builderReference) || 'Default builder'} /><DataField label="Latest delivery" value={nullValue(application.latestDeliveryStatus) || 'None'} /><DataField label="Latest build request" value={nullValue(application.latestBuildStatus) || 'None'} />{/if}
      </Card.Content>
    </Card.Root>
    <div><Button variant="destructive" onclick={() => router.delete(routes.applicationDestroy(application.id))}>Archive application</Button></div>
  </div>
</DashboardLayout>
